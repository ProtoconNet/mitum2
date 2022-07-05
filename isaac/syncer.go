package isaac

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/network/quicstream"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/valuehash"
	"golang.org/x/crypto/sha3"
)

var (
	ErrEmptySyncSources = util.NewError("empty sync sources; will retry")
	ErrRetrySyncSources = util.NewError("sync sources problem; will retry")
)

type Syncer interface {
	Add(base.Height) bool
	Finished() <-chan base.Height
	Done() <-chan struct{} // revive:disable-line:nested-structs
	Err() error
	IsFinished() (base.Height, bool)
	Cancel() error
}

type SyncSourcePool struct {
	current   NodeConnInfo
	sourcesid string
	currentid string
	sources   []NodeConnInfo
	sourceids []string
	index     int
	sync.RWMutex
}

func NewSyncSourcePool(sources []NodeConnInfo) *SyncSourcePool {
	p := &SyncSourcePool{
		sources: sources,
	}

	p.sourcesid, p.sourceids = p.makeid(sources)

	return p
}

func (p *SyncSourcePool) Retry(
	ctx context.Context,
	f func(NodeConnInfo) (bool, error),
	limit int,
	interval time.Duration,
) error {
	var lastid string

	return util.Retry(
		ctx,
		func() (bool, error) {
			nci, id, err := p.Next(lastid)
			if errors.Is(err, ErrEmptySyncSources) {
				return true, nil
			}
			lastid = id

			if _, err = nci.UDPConnInfo(); err != nil {
				return true, err
			}

			keep, err := f(nci)

			switch {
			case err == nil:
			case errors.Is(err, ErrRetrySyncSources),
				quicstream.IsNetworkError(err):
				return true, nil
			}

			return keep, err
		},
		limit,
		interval,
	)
}

func (p *SyncSourcePool) Update(sources []NodeConnInfo) bool {
	p.Lock()
	defer p.Unlock()

	id, ids := p.makeid(sources)
	if id == p.sourcesid {
		return false
	}

	p.sources = sources
	p.sourcesid, p.sourceids = id, ids

	p.index = 0
	p.current = nil
	p.currentid = ""

	return true
}

func (p *SyncSourcePool) Next(previd string) (NodeConnInfo, string, error) {
	p.Lock()
	defer p.Unlock()

	if len(p.sources) < 1 {
		return nil, "", ErrEmptySyncSources.Call()
	}

	// revive:disable-next-line:optimize-operands-order
	if p.current != nil && (len(previd) < 1 || previd != p.currentid) {
		return p.current, p.currentid, nil
	}

	index := p.nextIndex()

	p.current = p.sources[index]
	p.currentid = p.sourceids[index]

	return p.current, p.currentid, nil
}

func (p *SyncSourcePool) makeid(sources []NodeConnInfo) (string, []string) {
	if len(sources) < 1 {
		return "", nil
	}

	gh := sha3.New256()

	ids := make([]string, len(sources))

	for i := range sources {
		id := p.makesourceid(sources[i])
		ids[i] = id

		_, _ = gh.Write([]byte(id))
	}

	var v valuehash.L32

	gh.Sum(v[:0])

	return v.String(), ids
}

func (*SyncSourcePool) makesourceid(source NodeConnInfo) string {
	h := sha3.New256()
	_, _ = h.Write(source.Address().Bytes())
	_, _ = h.Write([]byte(source.String()))

	var v valuehash.L32

	h.Sum(v[:0])

	return v.String()
}

func (p *SyncSourcePool) nextIndex() int {
	switch {
	case p.index == len(p.sources):
		p.index = 1
	default:
		p.index++
	}

	return p.index - 1
}
