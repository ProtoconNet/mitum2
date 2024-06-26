package launch

import (
	"context"
	"math"
	mathrand "math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
	"golang.org/x/time/rate"
)

type testRateLimiter struct {
	suite.Suite
}

func (t *testRateLimiter) TestAllow() {
	l := NewRateLimiter(rate.Every(time.Millisecond*333), 3, "", "", "a")

	for range make([]int, 3) {
		t.True(l.Allow())
	}
	t.False(l.Allow())

	<-time.After(time.Millisecond * 333)
	t.True(l.Allow())
}

func (t *testRateLimiter) TestNoLimit() {
	t.Run("limit all", func() {
		l := NewRateLimiter(0, 3, "", "", "a")
		for range make([]int, 3) {
			t.False(l.Allow())
		}
	})

	t.Run("zero burst, limit all", func() {
		l := NewRateLimiter(rate.Every(time.Second), 0, "", "", "a")
		for range make([]int, 3) {
			t.False(l.Allow())
		}
	})

	t.Run("no limit", func() {
		l := NewRateLimiter(rate.Inf, 3, "", "", "a")
		for range make([]int, 3) {
			t.True(l.Allow())
		}
	})
}

func TestRateLimiter(t *testing.T) {
	suite.Run(t, new(testRateLimiter))
}

type testRateLimitHandler struct {
	suite.Suite
}

func (t *testRateLimitHandler) printL(
	h *RateLimitHandler,
	f func(addr, handler string, r *RateLimiter) bool,
) {
	h.pool.l.Traverse(func(addr string, m util.LockedMap[string, *RateLimiter]) bool {
		m.Traverse(func(handler string, r *RateLimiter) bool {
			b, _ := util.MarshalJSON(r)

			t.T().Logf("addr=%q handler=%q limiter=%s", addr, handler, string(b))

			return f(addr, handler, r)
		})

		return true
	})
}

func (t *testRateLimitHandler) newargs() *RateLimitHandlerArgs {
	args := NewRateLimitHandlerArgs()
	args.ExpireAddr = time.Second
	args.PoolSizes = []uint64{2, 2}

	return args
}

func (t *testRateLimitHandler) TestNew() {
	h, err := NewRateLimitHandler(t.newargs())
	t.NoError(err)

	addr := quicstream.RandomUDPAddr()
	t.T().Log("addr:", addr)

	l, allowed := h.allow(addr, util.UUID().String(), RateLimitRuleHint{})
	t.NotNil(l)
	t.True(allowed)

	t.printL(h, func(addr, handler string, r *RateLimiter) bool {
		t.T().Logf("addr=%q handler=%q limit=%v burst=%v createdAt=%v", addr, handler, r.Limit(), r.Burst(), r.UpdatedAt())

		return true
	})
}

func (t *testRateLimitHandler) TestAllowByDefault() {
	h, err := NewRateLimitHandler(t.newargs())
	t.NoError(err)

	addr := quicstream.RandomUDPAddr()

	handler0 := util.UUID().String()
	handler1 := util.UUID().String()

	defaultrule := NewRateLimiterRule(time.Minute, 1)

	ruleset := NewNetRateLimiterRuleSet()
	ruleset.Add(
		&net.IPNet{IP: addr.IP, Mask: net.CIDRMask(24, 32)},
		NewRateLimiterRuleMap(
			&defaultrule,
			map[string]RateLimiterRule{
				handler1: NewRateLimiterRule(time.Minute, 0),
			},
		),
	)

	t.NoError(h.rules.SetNetRuleSet(ruleset))

	t.Run("unknown handler; default used", func() {
		l, allowed := h.allow(addr, handler0, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)
	})

	t.Run("rule used", func() {
		l, allowed := h.allow(addr, handler1, RateLimitRuleHint{})
		t.NotNil(l)
		t.False(allowed)
	})
}

func (t *testRateLimitHandler) TestNotAllow() {
	h, err := NewRateLimitHandler(t.newargs())
	t.NoError(err)

	addr0 := quicstream.RandomUDPAddr()
	t.T().Log("addr0:", addr0)
	addr1 := quicstream.RandomUDPAddr()
	t.T().Log("addr1:", addr1)

	handler := util.UUID().String()

	ruleset := NewNetRateLimiterRuleSet()
	ruleset.Add(
		&net.IPNet{IP: addr0.IP, Mask: net.CIDRMask(24, 32)},
		NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
			handler: NewRateLimiterRule(time.Minute, 1),
		}),
	)

	t.NoError(h.rules.SetNetRuleSet(ruleset))

	t.Run("check allow", func() {
		l, allowed := h.allow(addr0, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		l, allowed = h.allow(addr1, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(addr, handler string, r *RateLimiter) bool {
			if addr == addr0.String() {
				t.Equal(rate.Every(time.Minute), r.Limit())
				t.Equal(1, r.Burst())
				t.True(r.Tokens() < 1)
			}

			if addr == addr1.String() {
				t.Equal(defaultRateLimiter.Limit, r.Limit())
				t.Equal(defaultRateLimiter.Burst, r.Burst())
				t.True(r.Tokens() > 0)
			}

			return true
		})
	})

	t.Run("check again", func() {
		l, allowed := h.allow(addr0, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.False(allowed)

		l, allowed = h.allow(addr1, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(addr, handler string, r *RateLimiter) bool {
			if addr == addr0.String() {
				t.Equal(rate.Every(time.Minute), r.Limit())
				t.Equal(1, r.Burst())
				t.True(r.Tokens() < 1)
			}

			if addr == addr1.String() {
				t.Equal(defaultRateLimiter.Limit, r.Limit())
				t.Equal(defaultRateLimiter.Burst, r.Burst())
				t.True(r.Tokens() > 0)
			}

			return true
		})
	})
}

func (t *testRateLimitHandler) TestRuleSetUpdated() {
	h, err := NewRateLimitHandler(t.newargs())
	t.NoError(err)

	addr := quicstream.RandomUDPAddr()
	t.T().Log("addr:", addr)

	handler := util.UUID().String()

	for range make([]int, defaultRateLimiter.Burst) {
		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)
	}

	t.printL(h, func(addr, handler string, r *RateLimiter) bool {
		t.Equal(defaultRateLimiter.Limit, r.Limit())
		t.Equal(defaultRateLimiter.Burst, r.Burst())
		t.True(r.Tokens() < 1)

		return true
	})

	newburst := defaultRateLimiter.Burst + 1

	t.T().Log("ruleset updated; rate limiters will be resetted")
	ruleset := NewNetRateLimiterRuleSet()
	ruleset.Add(
		&net.IPNet{IP: addr.IP, Mask: net.CIDRMask(24, 32)},
		NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
			handler: NewRateLimiterRule(time.Minute, newburst),
		}),
	)

	t.NoError(h.rules.SetNetRuleSet(ruleset))

	t.T().Log("check allow")
	l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
	t.NotNil(l)
	t.True(allowed)

	t.printL(h, func(addr, handler string, r *RateLimiter) bool {
		t.Equal(makeLimit(time.Minute, newburst), r.Limit())
		t.Equal(newburst, r.Burst())
		t.True(r.Tokens() >= 0, "tokens=%0.9f")

		return true
	})
}

func (t *testRateLimitHandler) TestSuffrageNode() {
	args := t.newargs()

	d := NewRateLimiterRule(time.Second, 1)
	args.Rules = NewRateLimiterRules()
	args.Rules.SetDefaultRuleMap(NewRateLimiterRuleMap(&d, nil))

	h, err := NewRateLimitHandler(args)
	t.NoError(err)

	handler := util.UUID().String()

	sufst := valuehash.RandomSHA256()
	_, members := isaac.NewTestSuffrage(2)

	args.Rules.SetIsInConsensusNodesFunc(func() (util.Hash, func(base.Address) bool, error) {
		return sufst, func(base.Address) bool { return true }, nil
	})

	node := members[0].Address()
	nodeLimiter := NewRateLimiterRule(time.Second*3, 3)

	ruleset := NewSuffrageRateLimiterRuleSet(NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
		handler: nodeLimiter,
	}))
	t.NoError(h.rules.SetSuffrageRuleSet(ruleset))

	addr := quicstream.RandomUDPAddr()
	t.T().Log("addr:", addr)

	t.Run("check allow; node not in suffrage", func() {
		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(addr, handler string, r *RateLimiter) bool {
			t.Equal(d.Limit, r.Limit())
			t.Equal(d.Burst, r.Burst())
			t.True(r.Tokens() < 1)

			return true
		})

		l, allowed = h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.False(allowed)
	})

	t.Run("add node", func() {
		t.True(h.AddNode(addr, node))
	})

	t.Run("check allow; node in suffrage", func() {
		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(addr, handler string, r *RateLimiter) bool {
			t.Equal(nodeLimiter.Limit, r.Limit())
			t.Equal(nodeLimiter.Burst, r.Burst())
			t.True(r.Tokens() > 0)

			return true
		})
	})

	t.Run("check allow; node out of suffrage", func() {
		nsufst := valuehash.RandomSHA256()
		args.Rules.SetIsInConsensusNodesFunc(func() (util.Hash, func(base.Address) bool, error) {
			return nsufst, func(base.Address) bool { return false }, nil
		})

		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(addr, handler string, r *RateLimiter) bool {
			t.Equal(d.Limit, r.Limit())
			t.Equal(d.Burst, r.Burst())
			t.True(r.Tokens() < 1)

			return true
		})

		l, allowed = h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.False(allowed)
	})

	t.Run("check allow; node in suffrage back", func() {
		args.Rules.SetIsInConsensusNodesFunc(func() (util.Hash, func(base.Address) bool, error) {
			return sufst, func(base.Address) bool { return true }, nil
		})

		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(addr, handler string, r *RateLimiter) bool {
			t.Equal(nodeLimiter.Limit, r.Limit())
			t.Equal(nodeLimiter.Burst, r.Burst())
			t.True(r.Tokens() > 0)

			return true
		})
	})
}

func (t *testRateLimitHandler) TestConcurrent() {
	sufst := valuehash.RandomSHA256()
	_, members := isaac.NewTestSuffrage(3)

	nodes := make([]string, len(members)+3)
	for i := range members {
		nodes[i] = members[i].Address().String()
	}

	for i := len(members); i < len(nodes); i++ {
		nodes[i] = base.RandomAddress("").String()
	}

	nodeAddrs := map[string]net.Addr{}
	for i := range nodes {
		node := nodes[i]
		addr := quicstream.RandomUDPAddr()

		nodeAddrs[node] = addr
		t.T().Logf("node=%q addr=%q", node, addr)
	}

	handlers := make([]string, 3)
	for i := range handlers {
		handlers[i] = util.UUID().String()
	}

	args := t.newargs()
	args.ExpireAddr = time.Millisecond * 11
	// args.ShrinkInterval = time.Millisecond * 11
	args.Rules = NewRateLimiterRules()
	rule := NewRateLimiterRule(time.Second, math.MaxInt)
	args.Rules.SetDefaultRuleMap(NewRateLimiterRuleMap(&rule, nil))
	args.MaxAddrs = 3

	args.Rules.SetIsInConsensusNodesFunc(func() (util.Hash, func(base.Address) bool, error) {
		return sufst, func(base.Address) bool { return true }, nil
	})

	h, err := NewRateLimitHandler(args)
	t.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.NoError(h.Start(ctx))
	defer h.Stop()

	for i := range members {
		member := members[i].Address()
		addr := nodeAddrs[member.String()]
		handler := handlers[mathrand.Intn(len(handlers))]

		_, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.True(allowed)

		t.True(h.AddNode(addr, member))
	}

	rules := map[string]RateLimiterRule{}
	for i := range handlers {
		rules[handlers[i]] = NewRateLimiterRule(time.Second, math.MaxInt)
	}

	ruleset := NewSuffrageRateLimiterRuleSet(NewRateLimiterRuleMap(nil, rules))
	t.NoError(h.rules.SetSuffrageRuleSet(ruleset))

	var tloglock sync.Mutex
	tlog := func(a ...interface{}) {
		tloglock.Lock()
		defer tloglock.Unlock()

		t.T().Log(a...)
	}

	t.Run("check allow and shrink", func() {
		worker, err := util.NewBaseJobWorker(ctx, int64(1<<10))
		t.NoError(err)
		defer worker.Close()

		go func() {
			for i := range make([]int, 1<<13) {
				handler := handlers[mathrand.Intn(len(handlers))]
				node := nodes[mathrand.Intn(len(nodes))]
				addr := nodeAddrs[node]

				_ = worker.NewJob(func(context.Context, uint64) error {
					_, allowed := h.allow(addr, handler, RateLimitRuleHint{})
					if !allowed {
						return errors.Errorf("not allowed")
					}

					<-time.After(time.Millisecond * 33)

					return nil
				})

				if i%1000 == 0 {
					_ = worker.NewJob(func(_ context.Context, i uint64) error {
						removed := h.shrink(ctx)
						tlog("\t> shrink:", i, removed)

						return nil
					})
				}
			}

			worker.Done()
		}()

		t.NoError(worker.Wait())
	})

	t.printL(h, func(addr, handler string, r *RateLimiter) bool { return true })
}

func (t *testRateLimitHandler) TestMaxAddr() {
	args := t.newargs()
	args.MaxAddrs = 3

	h, err := NewRateLimitHandler(args)
	t.NoError(err)

	prevs := make([]string, args.MaxAddrs)
	for i := range make([]int, args.MaxAddrs) {
		addr := quicstream.RandomUDPAddr()
		_, allowed := h.allow(addr, util.UUID().String(), RateLimitRuleHint{})
		t.True(allowed)

		removed := h.pool.shrinkAddrsQueue(args.MaxAddrs)

		t.T().Logf("queue=%d removed=%d", h.pool.addrsQueue.Len(), removed)

		prevs[i] = addr.String()
	}

	t.Run("over max", func() {
		for range make([]int, 3) {
			_, allowed := h.allow(quicstream.RandomUDPAddr(), util.UUID().String(), RateLimitRuleHint{})
			t.True(allowed)

			removed := h.pool.shrinkAddrsQueue(args.MaxAddrs)

			t.T().Logf("queue=%d removed=%d", h.pool.addrsQueue.Len(), removed)

			t.T().Log("queue:", h.pool.addrsQueue.Len())
			t.Equal(int(args.MaxAddrs), h.pool.addrsQueue.Len())
		}
	})

	t.Run("previous addrs removed", func() {
		for i := range prevs {
			addr := prevs[i]

			t.False(h.pool.l.Exists(addr), "l")
			t.False(h.pool.addrs.Exists(addr), "addrNodes")
			t.False(h.pool.lastAccessedAt.Exists(addr), "lastAccessedAt")
		}
	})
}

func (t *testRateLimitHandler) TestRuleSetOrder() {
	h, err := NewRateLimitHandler(t.newargs())
	t.NoError(err)

	addr := quicstream.RandomUDPAddr()
	handler := util.UUID().String()
	clientid := util.UUID().String()
	node := base.RandomAddress("")

	clientidlimiter := NewRateLimiterRule(time.Second*3, 3)
	clientidrs := NewClientIDRateLimiterRuleSet(map[string]RateLimiterRuleMap{
		clientid: NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
			handler: clientidlimiter,
		}),
	})
	t.NoError(h.rules.SetClientIDRuleSet(clientidrs))

	netlimiter := NewRateLimiterRule(time.Second*5, 5)
	netrs := NewNetRateLimiterRuleSet()
	netrs.Add(
		&net.IPNet{IP: addr.IP, Mask: net.CIDRMask(24, 32)},
		NewRateLimiterRuleMap(&netlimiter, nil),
	)
	t.NoError(h.rules.SetNetRuleSet(netrs))

	nodelimiter := NewRateLimiterRule(time.Second*4, 4)
	noders := NewNodeRateLimiterRuleSet(map[string]RateLimiterRuleMap{
		node.String(): NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
			handler: nodelimiter,
		}),
	})
	t.NoError(h.rules.SetNodeRuleSet(noders))

	t.Run("clientid", func() {
		l, allowed := h.allow(addr, handler, RateLimitRuleHint{ClientID: clientid})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(_, _ string, r *RateLimiter) bool {
			t.Equal("clientid", r.Type())
			t.Equal(clientidlimiter.Limit, r.Limit())
			t.Equal(clientidlimiter.Burst, r.Burst())

			return true
		})
	})

	t.Run("net", func() {
		t.NoError(h.rules.SetClientIDRuleSet(nil))

		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(_, _ string, r *RateLimiter) bool {
			t.Equal("net", r.Type())
			t.Equal(netlimiter.Limit, r.Limit())
			t.Equal(netlimiter.Burst, r.Burst())

			return true
		})
	})

	t.Run("node", func() {
		t.NoError(h.rules.SetNetRuleSet(nil))
		t.True(h.AddNode(addr, node))

		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(_, _ string, r *RateLimiter) bool {
			t.Equal("node", r.Type())
			t.Equal(nodelimiter.Limit, r.Limit())
			t.Equal(nodelimiter.Burst, r.Burst())

			return true
		})
	})

	t.Run("default", func() {
		t.NoError(h.rules.SetNodeRuleSet(nil))

		l, allowed := h.allow(addr, handler, RateLimitRuleHint{})
		t.NotNil(l)
		t.True(allowed)

		t.printL(h, func(_, _ string, r *RateLimiter) bool {
			t.Equal("defaultmap", r.Type())
			t.Equal(defaultRateLimiter.Limit, r.Limit())
			t.Equal(defaultRateLimiter.Burst, r.Burst())

			return true
		})
	})
}

func TestRateLimitHandler(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(testRateLimitHandler))
}

type testNetRateLimiterRuleSet struct {
	suite.Suite
}

func (t *testNetRateLimiterRuleSet) newipnet() *net.IPNet {
	addr := quicstream.RandomUDPAddr()

	ipnet := &net.IPNet{IP: addr.IP, Mask: net.CIDRMask(24, 32)}
	_, i, _ := net.ParseCIDR(ipnet.String())

	return i
}

func (t *testNetRateLimiterRuleSet) TestValid() {
	rs := NewNetRateLimiterRuleSet()
	rs.
		Add(
			t.newipnet(),
			NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
				"a": NewRateLimiterRule(time.Second*33, 44),
				"b": NoLimitRateLimiterRule(),
				"c": LimitRateLimiterRule(),
			}),
		).
		Add(
			t.newipnet(),
			NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
				"d": NewRateLimiterRule(time.Second*55, 66),
				"e": NoLimitRateLimiterRule(),
				"f": LimitRateLimiterRule(),
			}),
		)

	t.NoError(rs.IsValid(nil))
}

func (t *testNetRateLimiterRuleSet) TestWrongLength() {
	rs := NewNetRateLimiterRuleSet()
	rs.ipnets = []*net.IPNet{
		t.newipnet(),
		t.newipnet(),
		t.newipnet(),
	}

	rs.rules[rs.ipnets[0].String()] = NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
		"a": NewRateLimiterRule(time.Second*33, 44),
		"b": NoLimitRateLimiterRule(),
		"c": LimitRateLimiterRule(),
	})
	rs.rules[rs.ipnets[1].String()] = NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
		"d": NewRateLimiterRule(time.Second*55, 66),
		"e": NoLimitRateLimiterRule(),
		"f": LimitRateLimiterRule(),
	})

	err := rs.IsValid(nil)
	t.Error(err)
	t.ErrorContains(err, "rules length != ipnet length")
}

func (t *testNetRateLimiterRuleSet) TestUnknownIPNet() {
	rs := NewNetRateLimiterRuleSet()
	rs.
		Add(
			t.newipnet(),
			NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
				"a": NewRateLimiterRule(time.Second*33, 44),
				"b": NoLimitRateLimiterRule(),
				"c": LimitRateLimiterRule(),
			}),
		).
		Add(
			t.newipnet(),
			NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
				"d": NewRateLimiterRule(time.Second*55, 66),
				"e": NoLimitRateLimiterRule(),
				"f": LimitRateLimiterRule(),
			}),
		)

	rs.ipnets[1] = t.newipnet()

	err := rs.IsValid(nil)
	t.ErrorContains(err, "no rule")
}

func TestNetRateLimiterRuleSet(t *testing.T) {
	suite.Run(t, new(testNetRateLimiterRuleSet))
}
