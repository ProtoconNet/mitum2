package isaac

import "github.com/spikeekips/mitum/base"

type BootingHandler struct {
	*baseStateHandler
}

func NewBootingHandler() *BootingHandler {
	return &BootingHandler{
		baseStateHandler: newBaseStateHandler(StateBooting),
	}
}

func (*BootingHandler) enter(stateSwitchContext) error {
	// NOTE find last manifest
	// NOTE find last init and accept voteproof
	// NOTE if ok, moves to joining

	return nil
}

func (*BootingHandler) newVoteproof(base.Voteproof) error {
	// NOTE in booting, do nothing
	return nil
}

func (*BootingHandler) newProposal(base.ProposalFact) error {
	// NOTE in booting, do nothing
	return nil
}

type bootingSwitchContext struct {
	baseStateSwitchContext
}

func newBootingSwitchContext() bootingSwitchContext {
	return bootingSwitchContext{
		baseStateSwitchContext: newBaseStateSwitchContext(StateStopped, StateBooting),
	}
}