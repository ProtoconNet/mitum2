package isaacstates

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
)

type StateType string

const (
	StateEmpty = StateType("")
	// StateStopped indicates all processes is finished.
	StateStopped = StateType("STOPPED")
	// StateBooting indicates node checks it's state.
	StateBooting = StateType("BOOTING")
	// StateJoining indicates node is trying to join consensus.
	StateJoining = StateType("JOINING")
	// StateConsensus indicates node participates consensus with the other
	// nodes.
	StateConsensus = StateType("CONSENSUS")
	// StateSyncing indicates node is syncing block.
	StateSyncing = StateType("SYNCING")
	// StateHandover indicates node tries to replace the existing same node.
	StateHandover = StateType("HANDOVER")
	// StateBroken is used when something wrong in states.
	StateBroken = StateType("BROKEN")
)

func (s StateType) String() string {
	return string(s)
}

type handler interface {
	state() StateType
	enter(switchContext) (func(), error)
	exit(switchContext) (func(), error)
	newVoteproof(base.Voteproof) error
}

func handlerLog(st handler) fmt.Stringer {
	return util.Stringer(func() string {
		if st == nil {
			return ""
		}

		return st.state().String()
	})
}

type switchContext interface {
	from() StateType
	next() StateType
	Error() string
}

type baseSwitchContext struct {
	f StateType
	n StateType
}

func newBaseSwitchContext(from, next StateType) baseSwitchContext {
	return baseSwitchContext{
		f: from,
		n: next,
	}
}

func (s baseSwitchContext) from() StateType {
	return s.f
}

func (s baseSwitchContext) next() StateType {
	return s.n
}

func (baseSwitchContext) Error() string {
	return ""
}

func (s baseSwitchContext) MarshalZerologObject(e *zerolog.Event) {
	e.Stringer("from", s.f).Stringer("next", s.n)
}

func switchContextLog(sctx switchContext) *zerolog.Event {
	e := zerolog.Dict()

	o, ok := sctx.(zerolog.LogObjectMarshaler)
	switch {
	case ok:
		e = e.Object("next_state", o)
	default:
		e = e.Stringer("from", sctx.from()).Stringer("next", sctx.next())
	}

	return e
}