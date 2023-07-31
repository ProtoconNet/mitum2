package launch

import (
	"container/list"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/network/quicstream"
	"github.com/spikeekips/mitum/util"
	"golang.org/x/time/rate"
)

var (
	ErrRateLimited                   = util.NewIDError("over rate limit")
	defaultRateLimiter               = RateLimiterRule{Limit: rate.Every(time.Second), Burst: 3} //nolint:gomnd //...
	defaultRateLimitHandlerPoolSizes = []uint64{1 << 7, 1 << 7, 1 << 7}
)

type RateLimiter struct {
	*rate.Limiter
	updatedAt time.Time
	checksum  string
	sync.RWMutex
}

func NewRateLimiter(limit rate.Limit, burst int, checksum string) *RateLimiter {
	return &RateLimiter{
		Limiter:   rate.NewLimiter(limit, burst),
		updatedAt: time.Now(),
		checksum:  checksum,
	}
}

func (r *RateLimiter) Checksum() string {
	r.RLock()
	defer r.RUnlock()

	return r.checksum
}

func (r *RateLimiter) UpdatedAt() time.Time {
	r.RLock()
	defer r.RUnlock()

	return r.updatedAt
}

func (r *RateLimiter) Update(limit rate.Limit, burst int, checksum string) *RateLimiter {
	r.Lock()
	defer r.Unlock()

	if r.Limit() != limit || r.Burst() != burst {
		r.Limiter = rate.NewLimiter(limit, burst)
	}

	if r.checksum != checksum {
		r.checksum = checksum
	}

	r.updatedAt = time.Now()

	return r
}

type RateLimitHandlerArgs struct {
	GetLastSuffrageFunc func(context.Context) (statehash util.Hash, _ base.Suffrage, found bool, _ error)
	Rules               *RateLimiterRules
	PoolSizes           []uint64
	// ExpireAddr sets the expire duration for idle addr. if addr is over
	// ExpireAddr, it will be removed.
	ExpireAddr           time.Duration
	ShrinkInterval       time.Duration
	LastSuffrageInterval time.Duration
	// MaxAddrs limits the number of network addresses; if new address over
	// MaxAddrs, the oldes addr will be removed.
	MaxAddrs uint64
}

func NewRateLimitHandlerArgs() *RateLimitHandlerArgs {
	return &RateLimitHandlerArgs{
		ExpireAddr:           time.Second * 33, //nolint:gomnd //...
		ShrinkInterval:       time.Second * 33, //nolint:gomnd // long enough
		LastSuffrageInterval: time.Second * 2,  //nolint:gomnd //...
		PoolSizes:            defaultRateLimitHandlerPoolSizes,
		GetLastSuffrageFunc: func(context.Context) (util.Hash, base.Suffrage, bool, error) {
			return nil, nil, false, nil
		},
		Rules:    NewRateLimiterRules(defaultRateLimiter),
		MaxAddrs: math.MaxUint32,
	}
}

type RateLimitHandler struct {
	*util.ContextDaemon
	args         *RateLimitHandlerArgs
	rules        *RateLimiterRules
	pool         *addrPool
	lastSuffrage *util.Locked[[2]interface{}]
}

func NewRateLimitHandler(args *RateLimitHandlerArgs) (*RateLimitHandler, error) {
	pool, err := newAddrPool(args.PoolSizes)
	if err != nil {
		return nil, err
	}

	r := &RateLimitHandler{
		args:         args,
		pool:         pool,
		rules:        args.Rules,
		lastSuffrage: util.EmptyLocked[[2]interface{}](),
	}

	r.ContextDaemon = util.NewContextDaemon(r.start)

	return r, nil
}

func (r *RateLimitHandler) HandlerFunc(handlerPrefix string, handler quicstream.Handler) quicstream.Handler {
	return func(ctx context.Context, addr net.Addr, ir io.Reader, iw io.WriteCloser) (context.Context, error) {
		if l, allowed := r.allow(addr, handlerPrefix); !allowed {
			return ctx, ErrRateLimited.Errorf("prefix=%q limit=%v burst=%d", handlerPrefix, l.Limit(), l.Burst())
		}

		return handler(ctx, addr, ir, iw)
	}
}

func (r *RateLimitHandler) AddNode(addr net.Addr, node base.Address) bool {
	return r.pool.addNode(addr.String(), node)
}

func (r *RateLimitHandler) RemoveNode(node base.Address) bool {
	return r.pool.removeNode(node)
}

func (r *RateLimitHandler) Rules() *RateLimiterRules {
	return r.rules
}

func (r *RateLimitHandler) allow(addr net.Addr, handler string) (*RateLimiter, bool) {
	l := r.pool.rateLimiter(addr.String(), handler,
		r.rateLimiterFunc(addr, handler),
	)

	return l, l.Allow()
}

func (r *RateLimitHandler) shrink(ctx context.Context) (removed uint64) {
	point := time.Now().Add(r.args.ExpireAddr * -1)

	return r.pool.shrink(ctx, point, r.args.MaxAddrs)
}

func (r *RateLimitHandler) start(ctx context.Context) error {
	sticker := time.NewTicker(r.args.ShrinkInterval)
	defer sticker.Stop()

	lticker := time.NewTicker(r.args.LastSuffrageInterval)
	defer lticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sticker.C:
			r.shrink(ctx)
		case <-lticker.C:
			_ = r.checkLastSuffrage(ctx)
		}
	}
}

func (r *RateLimitHandler) checkLastSuffrage(ctx context.Context) error {
	switch st, suf, found, err := r.args.GetLastSuffrageFunc(ctx); {
	case err != nil:
		return err
	case !found:
		return nil
	default:
		var updated bool

		_, _ = r.lastSuffrage.Set(func(i [2]interface{}, isempty bool) ([2]interface{}, error) {
			if isempty {
				updated = true

				return [2]interface{}{st, suf}, nil
			}

			if prev, ok := i[0].(util.Hash); ok && prev.Equal(st) {
				return [2]interface{}{}, util.ErrLockedSetIgnore.WithStack()
			}

			updated = true

			return [2]interface{}{st, suf}, nil
		})

		if updated {
			_ = r.Rules().SetSuffrage(suf, st)
		}

		return nil
	}
}

func (r *RateLimitHandler) isOldNodeInSuffrage(node base.Address, checksum string) bool {
	i, isempty := r.lastSuffrage.Value()
	if isempty {
		return false
	}

	switch suf, ok := i[1].(base.Suffrage); {
	case !ok:
		return false
	case !suf.Exists(node):
		return len(checksum) > 0
	}

	switch i, ok := i[0].(util.Hash); {
	case !ok:
		return false
	default:
		return i.String() != checksum
	}
}

func (r *RateLimitHandler) rateLimiterFunc(
	addr net.Addr,
	handler string,
) func(_ *RateLimiter, found, created bool, node base.Address) (*RateLimiter, error) {
	return func(l *RateLimiter, found, created bool, node base.Address) (*RateLimiter, error) {
		if !found || l.UpdatedAt().Before(r.rules.UpdatedAt()) {
			// NOTE if RateLimiter is older than rule updated

			checksum, rule := r.rules.Rule(addr, node, handler)

			if !found {
				return NewRateLimiter(rule.Limit, rule.Burst, checksum), nil
			}

			_ = l.Update(rule.Limit, rule.Burst, checksum)

			return nil, util.ErrLockedSetIgnore.WithStack()
		}

		if found && node != nil {
			if r.isOldNodeInSuffrage(node, l.Checksum()) { // NOTE check node is in suffrage
				checksum, rule := r.rules.Rule(addr, node, handler)

				_ = l.Update(rule.Limit, rule.Burst, checksum)

				return nil, util.ErrLockedSetIgnore.WithStack()
			}
		}

		return nil, util.ErrLockedSetIgnore.WithStack()
	}
}

type RateLimiterRule struct {
	Limit rate.Limit
	Burst int
}

type RateLimiterRules struct {
	updatedAt      time.Time
	suffrage       RateLimiterRuleSet
	nodes          RateLimiterRuleSet
	nets           RateLimiterRuleSet
	defaultSet     map[ /* handler */ string]RateLimiterRule
	defaultLimiter RateLimiterRule
	sync.RWMutex
}

func NewRateLimiterRules(defaultLimiter RateLimiterRule) *RateLimiterRules {
	return &RateLimiterRules{
		defaultLimiter: defaultLimiter,
		updatedAt:      time.Now(),
	}
}

func (r *RateLimiterRules) DefaultLimiter() RateLimiterRule {
	r.RLock()
	defer r.RUnlock()

	return r.defaultLimiter
}

func (r *RateLimiterRules) Rule(addr net.Addr, node base.Address, handler string) (string, RateLimiterRule) {
	checksum, l := r.rule(addr, node, handler)

	return checksum, l
}

func (r *RateLimiterRules) SetDefaultLimiter(l RateLimiterRule) *RateLimiterRules {
	r.Lock()
	defer r.Unlock()

	r.defaultLimiter = l
	r.updatedAt = time.Now()

	return r
}

func (r *RateLimiterRules) SetDefaultRuleSet(l map[string]RateLimiterRule) *RateLimiterRules {
	r.Lock()
	defer r.Unlock()

	r.defaultSet = l
	r.updatedAt = time.Now()

	return r
}

func (r *RateLimiterRules) SetSuffrage(suf base.Suffrage, st util.Hash) error {
	if i, ok := r.suffrage.(rulesetSuffrageSetter); ok {
		i.SetSuffrage(suf, st)
	}

	return nil
}

func (r *RateLimiterRules) SetSuffrageRuleSet(l RateLimiterRuleSet) error {
	r.Lock()
	defer r.Unlock()

	if _, ok := l.(rulesetSuffrageSetter); !ok {
		return errors.Errorf("expected rulesetSuffrageSetter, but %T", l)
	}

	r.suffrage = l
	r.updatedAt = time.Now()

	return nil
}

func (r *RateLimiterRules) SetNodesRuleSet(l RateLimiterRuleSet) error {
	r.Lock()
	defer r.Unlock()

	r.nodes = l
	r.updatedAt = time.Now()

	return nil
}

func (r *RateLimiterRules) SetNetRuleSet(l RateLimiterRuleSet) error {
	r.Lock()
	defer r.Unlock()

	r.nets = l
	r.updatedAt = time.Now()

	return nil
}

func (r *RateLimiterRules) UpdatedAt() time.Time {
	r.RLock()
	defer r.RUnlock()

	return r.updatedAt
}

func (r *RateLimiterRules) rule(addr net.Addr, node base.Address, handler string) (string, RateLimiterRule) {
	r.RLock()
	defer r.RUnlock()

	if node != nil && r.nodes != nil {
		if checksum, l, found := r.nodes.Rule(node, handler); found {
			return checksum, l
		}
	}

	if r.nets != nil {
		if checksum, l, found := r.nets.Rule(addr, handler); found {
			return checksum, l
		}
	}

	if node != nil && r.suffrage != nil {
		if checksum, l, found := r.suffrage.Rule(node, handler); found {
			return checksum, l
		}
	}

	if r.defaultSet != nil {
		if l, found := r.defaultSet[handler]; found {
			return "", l
		}
	}

	return "", r.defaultLimiter
}

type RateLimiterRuleSet interface {
	Rule(_ interface{}, handler string) (checksum string, _ RateLimiterRule, found bool)
}

type NetRateLimiterRuleSet struct {
	rules  map[ /* ipnet */ string]map[ /* handler */ string]RateLimiterRule
	ipnets []*net.IPNet
}

func NewNetRateLimiterRuleSet() *NetRateLimiterRuleSet {
	return &NetRateLimiterRuleSet{
		rules: map[string]map[string]RateLimiterRule{},
	}
}

func (rs *NetRateLimiterRuleSet) Add(ipnet *net.IPNet, rule map[string]RateLimiterRule) *NetRateLimiterRuleSet {
	rs.ipnets = append(rs.ipnets, ipnet)
	rs.rules[ipnet.String()] = rule

	return rs
}

func (rs *NetRateLimiterRuleSet) Rule(i interface{}, handler string) (string, RateLimiterRule, bool) {
	l, found := rs.rule(i, handler)

	return "", l, found
}

func (rs *NetRateLimiterRuleSet) rule(i interface{}, handler string) (rule RateLimiterRule, _ bool) {
	var ip net.IP

	switch t := i.(type) {
	case *net.UDPAddr:
		ip = t.IP
	case net.IP:
		ip = t
	default:
		return rule, false
	}

	for i := range rs.ipnets {
		in := rs.ipnets[i]
		if !in.Contains(ip) {
			continue
		}

		m, found := rs.rules[in.String()]
		if !found {
			return rule, false
		}

		l, found := m[handler]
		if !found {
			return rule, false
		}

		return l, true
	}

	return rule, false
}

type NodesRateLimiterRuleSet struct {
	rules map[ /* node address */ string]map[ /* handler */ string]RateLimiterRule
}

func (rs NodesRateLimiterRuleSet) Rule(i interface{}, handler string) (string, RateLimiterRule, bool) {
	l, found := rs.rule(i, handler)

	return "", l, found
}

func (rs NodesRateLimiterRuleSet) rule(i interface{}, handler string) (rule RateLimiterRule, _ bool) {
	var node string

	switch t := i.(type) {
	case base.Address:
		node = t.String()
	case fmt.Stringer:
		node = t.String()
	case string:
		node = t
	default:
		return rule, false
	}

	m, found := rs.rules[node]
	if !found {
		return rule, false
	}

	l, found := m[handler]

	return l, found
}

type SuffrageRateLimiterRuleSet struct {
	suf base.Suffrage
	st  util.Hash
	r   map[string] /* handler */ RateLimiterRule
	d   RateLimiterRule
	sync.RWMutex
}

func NewSuffrageRateLimiterRuleSet(
	suf base.Suffrage,
	rule map[string]RateLimiterRule,
	defaultLimiter RateLimiterRule,
) *SuffrageRateLimiterRuleSet {
	return &SuffrageRateLimiterRuleSet{suf: suf, r: rule, d: defaultLimiter}
}

func (rs *SuffrageRateLimiterRuleSet) SetSuffrage(suf base.Suffrage, st util.Hash) {
	rs.Lock()
	defer rs.Unlock()

	rs.suf = suf
	rs.st = st
}

func (rs *SuffrageRateLimiterRuleSet) inSuffrage(node base.Address) bool {
	rs.RLock()
	defer rs.RUnlock()

	return rs.suf.Exists(node)
}

func (rs *SuffrageRateLimiterRuleSet) Rule(i interface{}, handler string) (string, RateLimiterRule, bool) {
	l, found := rs.rule(i, handler)

	return rs.st.String(), l, found
}

func (rs *SuffrageRateLimiterRuleSet) rule(i interface{}, handler string) (rule RateLimiterRule, _ bool) {
	node, ok := i.(base.Address)
	if !ok {
		return rule, false
	}

	if !rs.inSuffrage(node) {
		return rule, false
	}

	l, found := rs.r[handler]
	if !found && rs.d.Limit > 0 {
		return rs.d, true
	}

	return l, found
}

type addrPool struct {
	l              *util.ShardedMap[string, util.LockedMap[string, *RateLimiter]]
	addrNodes      *util.ShardedMap[string, base.Address]
	nodesAddr      *util.ShardedMap[string, string]
	lastAccessedAt *util.ShardedMap[string, time.Time]
	addrsHistory   *rateLimitAddrsHistory
}

func newAddrPool(poolsize []uint64) (*addrPool, error) {
	l, err := util.NewDeepShardedMap[string, util.LockedMap[string, *RateLimiter]](
		poolsize,
		func() util.LockedMap[string, util.LockedMap[string, *RateLimiter]] {
			return util.NewSingleLockedMap[string, util.LockedMap[string, *RateLimiter]]()
		},
	)
	if err != nil {
		return nil, err
	}

	addrNodes, err := util.NewDeepShardedMap[string, base.Address](poolsize, nil)
	if err != nil {
		return nil, err
	}

	nodesAddr, err := util.NewDeepShardedMap[string, string](poolsize, nil)
	if err != nil {
		return nil, err
	}

	lastAccessedAt, err := util.NewDeepShardedMap[string, time.Time](poolsize, nil)
	if err != nil {
		return nil, err
	}

	return &addrPool{
		l:              l,
		addrNodes:      addrNodes,
		nodesAddr:      nodesAddr,
		lastAccessedAt: lastAccessedAt,
		addrsHistory:   newRateLimitAddrsHistory(),
	}, nil
}

func (p *addrPool) addNode(addr string, node base.Address) bool {
	var created bool

	_ = p.l.Get(addr, func(_ util.LockedMap[string, *RateLimiter], found bool) error {
		if !found {
			return nil
		}

		_, created, _ = p.addrNodes.Set(addr, func(_ base.Address, found bool) (base.Address, error) {
			if found {
				return nil, util.ErrLockedSetIgnore.WithStack()
			}

			_ = p.nodesAddr.SetValue(node.String(), addr)
			_ = p.lastAccessedAt.SetValue(addr, time.Now())

			return node, nil
		})

		return nil
	})

	return created
}

func (p *addrPool) rateLimiter(
	addr,
	handler string,
	f func(l *RateLimiter, found, created bool, node base.Address) (*RateLimiter, error),
) *RateLimiter {
	var l *RateLimiter

	_ = p.l.GetOrCreate(
		addr,
		func(i util.LockedMap[string, *RateLimiter], _ bool) error {
			_ = p.lastAccessedAt.SetValue(addr, time.Now())

			var created bool

			if i.Len() < 1 {
				created = true

				p.addrsHistory.Add(addr)
			}

			node, _ := p.addrNodes.Value(addr)

			l, _, _ = i.Set(
				handler,
				func(l *RateLimiter, found bool) (*RateLimiter, error) {
					return f(l, found, created, node)
				},
			)

			return nil
		},
		func() (util.LockedMap[string, *RateLimiter], error) {
			return util.NewSingleLockedMap[string, *RateLimiter](), nil
		},
	)

	return l
}

func (p *addrPool) remove(addr string) bool {
	removed, _ := p.l.Remove(addr, func(util.LockedMap[string, *RateLimiter], bool) error {
		_ = p.lastAccessedAt.RemoveValue(addr)

		if node, found := p.addrNodes.Value(addr); found {
			_ = p.nodesAddr.RemoveValue(node.String())
		}

		_ = p.addrNodes.RemoveValue(addr)

		_ = p.addrsHistory.Remove(addr)

		return nil
	})

	return removed
}

func (p *addrPool) removeNode(node base.Address) bool {
	switch addr, found := p.nodesAddr.Value(node.String()); {
	case !found:
		return false
	default:
		return p.remove(addr)
	}
}

func (p *addrPool) shrink(ctx context.Context, expire time.Time, maxAddrs uint64) (removed uint64) {
	p.lastAccessedAt.TraverseMap(
		func(m util.LockedMap[string, time.Time]) bool {
			return util.AwareContext(ctx, func(context.Context) error {
				var gathered []string

				m.Traverse(func(addr string, accessed time.Time) bool {
					if accessed.Before(expire) {
						gathered = append(gathered, addr)
					}

					return true
				})

				for i := range gathered {
					_ = p.remove(gathered[i])
				}

				removed += uint64(len(gathered))

				return nil
			}) == nil
		},
	)

	// NOTE remove orphan node
	_ = p.nodesAddr.TraverseMap(func(m util.LockedMap[string, string]) bool {
		return util.AwareContext(ctx, func(context.Context) error {
			var addrs []string
			m.Traverse(func(_, addr string) bool {
				addrs = append(addrs, addr)
				return true
			})

			for i := range addrs {
				addr := addrs[i]
				if !p.l.Exists(addr) {
					_ = p.remove(addr)
					removed++
				}
			}

			return nil
		}) == nil
	})

	removed += p.shrinkAddrsHistory(maxAddrs)

	return removed
}

func (p *addrPool) shrinkAddrsHistory(maxAddrs uint64) uint64 {
	var removed uint64

	for uint64(p.addrsHistory.Len()) > maxAddrs {
		switch addr := p.addrsHistory.Pop(); {
		case len(addr) < 1:
			continue
		default:
			if p.remove(addr) {
				removed++
			}
		}
	}

	return removed
}

type rateLimitAddrsHistory struct {
	l *list.List
	m map[string]*list.Element
	sync.RWMutex
}

func newRateLimitAddrsHistory() *rateLimitAddrsHistory {
	return &rateLimitAddrsHistory{
		l: list.New(),
		m: map[string]*list.Element{},
	}
}

func (h *rateLimitAddrsHistory) Len() int {
	h.RLock()
	defer h.RUnlock()

	return h.l.Len()
}

func (h *rateLimitAddrsHistory) Add(addr string) {
	h.Lock()
	defer h.Unlock()

	if i, found := h.m[addr]; found {
		_ = h.l.Remove(i)
	}

	h.m[addr] = h.l.PushBack(addr)
}

func (h *rateLimitAddrsHistory) Remove(addr string) bool {
	h.Lock()
	defer h.Unlock()

	switch i, found := h.m[addr]; {
	case found:
		_ = h.l.Remove(i)
		delete(h.m, addr)

		return true
	default:
		return false
	}
}

func (h *rateLimitAddrsHistory) Pop() string {
	h.Lock()
	defer h.Unlock()

	e := h.l.Front()
	if e != nil {
		_ = h.l.Remove(e)
	}

	var addr string

	if e != nil {
		addr = e.Value.(string) //nolint:forcetypeassert //...

		delete(h.m, addr)
	}

	return addr
}

type rulesetSuffrageSetter interface {
	SetSuffrage(base.Suffrage, util.Hash)
}
