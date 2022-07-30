package isaacstates

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/util"
)

type ConsensusHandler struct {
	*baseHandler
	getManifest          func(base.Height) (base.Manifest, error)
	nodeInConsensusNodes isaac.NodeInConsensusNodesFunc
	whenNewBlockSaved    func(base.Height)
	pps                  *isaac.ProposalProcessors
}

type NewConsensusHandlerType struct {
	*ConsensusHandler
}

func NewNewConsensusHandlerType(
	local base.LocalNode,
	policy isaac.NodePolicy,
	proposalSelector isaac.ProposalSelector,
	getManifest func(base.Height) (base.Manifest, error),
	nodeInConsensusNodes isaac.NodeInConsensusNodesFunc,
	voteFunc func(base.Ballot) (bool, error),
	whenNewBlockSaved func(base.Height),
	pps *isaac.ProposalProcessors,
) *NewConsensusHandlerType {
	baseHandler := newBaseHandler(StateConsensus, local, policy, proposalSelector)

	if voteFunc != nil {
		baseHandler.voteFunc = preventVotingWithEmptySuffrage(voteFunc, local, nodeInConsensusNodes)
	}

	return &NewConsensusHandlerType{
		ConsensusHandler: &ConsensusHandler{
			baseHandler:          baseHandler,
			getManifest:          getManifest,
			nodeInConsensusNodes: nodeInConsensusNodes,
			whenNewBlockSaved:    whenNewBlockSaved,
			pps:                  pps,
		},
	}
}

func (h *NewConsensusHandlerType) new() (handler, error) {
	return &ConsensusHandler{
		baseHandler:          h.baseHandler.new(),
		getManifest:          h.getManifest,
		nodeInConsensusNodes: h.nodeInConsensusNodes,
		whenNewBlockSaved:    h.whenNewBlockSaved,
		pps:                  h.pps,
	}, nil
}

func (st *ConsensusHandler) enter(i switchContext) (func(), error) {
	e := util.StringErrorFunc("failed to enter consensus state")

	deferred, err := st.baseHandler.enter(i)
	if err != nil {
		return nil, e(err, "")
	}

	var sctx consensusSwitchContext

	switch j, ok := i.(consensusSwitchContext); {
	case !ok:
		return nil, e(nil, "invalid stateSwitchContext, not for consensus state; %T", i)
	case j.ivp == nil:
		return nil, e(nil, "invalid stateSwitchContext, empty init voteproof")
	case j.ivp.Result() != base.VoteResultMajority:
		return nil, e(nil, "invalid stateSwitchContext, wrong vote result of init voteproof, %q", j.ivp.Result())
	default:
		sctx = j
	}

	switch suf, found, err := st.nodeInConsensusNodes(st.local, sctx.ivp.Point().Height()); { //nolint:govet //...
	case err != nil:
		return nil, e(err, "")
	case suf == nil || suf.Len() < 1:
		return nil, e(nil, "empty suffrage of init voteproof")
	case !found:
		st.Log().Debug().
			Dict("state_context", switchContextLog(sctx)).
			Interface("height", sctx.ivp.Point().Height()).
			Msg("local is not in consensus nodes at entering consensus state; moves to syncing state")

		return nil, newSyncingSwitchContext(StateEmpty, sctx.ivp.Point().Height())
	}

	process, err := st.processProposal(sctx.ivp)
	if err != nil {
		return nil, e(err, "")
	}

	return func() {
		deferred()

		if process != nil {
			var sctx switchContext

			switch err := process(st.ctx); {
			case err == nil:
			case errors.As(err, &sctx):
				go st.switchState(sctx)
			default:
				go st.switchState(newBrokenSwitchContext(StateConsensus, err))
			}
		}
	}, nil
}

func (st *ConsensusHandler) exit(sctx switchContext) (func(), error) {
	e := util.StringErrorFunc("failed to exit from consensus state")

	deferred, err := st.baseHandler.exit(sctx)
	if err != nil {
		return nil, e(err, "")
	}

	if err := st.pps.Cancel(); err != nil {
		return nil, e(err, "failed to cancel proposal processors")
	}

	return func() {
		deferred()

		var timers []util.TimerID

		if sctx != nil {
			switch sctx.next() { //nolint:exhaustive //...
			case StateJoining, StateHandover:
				timers = []util.TimerID{timerIDBroadcastINITBallot, timerIDBroadcastACCEPTBallot}
			}
		}

		if len(timers) < 1 {
			if err := st.timers.StopTimersAll(); err != nil {
				st.Log().Error().Err(err).Msg("failed to stop timers; ignore")
			}
		} else if err := st.timers.StartTimers([]util.TimerID{
			timerIDBroadcastINITBallot,
			timerIDBroadcastACCEPTBallot,
		}, true); err != nil {
			st.Log().Error().Err(err).Msg("failed to start timers; ignore")
		}
	}, nil
}

func (st *ConsensusHandler) processProposal(ivp base.INITVoteproof) (func(context.Context) error, error) {
	facthash := ivp.BallotMajority().Proposal()
	l := st.Log().With().Stringer("fact", facthash).Logger()
	l.Debug().Msg("trying to process proposal")

	e := util.StringErrorFunc("failed to process proposal")

	var process isaac.ProcessorProcessFunc

	switch i, err := st.processProposalInternal(ivp); {
	case err == nil:
		process = i
	case errors.Is(err, isaac.NotProposalProcessorProcessedError):
		go st.nextRound(ivp, ivp.BallotMajority().PreviousBlock())

		return nil, nil
	case i == nil:
		l.Debug().Msg("failed to process proposal; empty manifest; ignore")

		return nil, nil
	default:
		err = e(err, "")

		l.Error().Err(err).Msg("failed to process proposal; moves to broken state")

		return nil, newBrokenSwitchContext(StateConsensus, err)
	}

	return func(ctx context.Context) error {
		manifest, err := process(ctx)
		if err != nil {
			return e(err, "")
		}

		eavp := st.lastVoteproofs().ACCEPT()

		if err := st.prepareACCEPTBallot(ivp, manifest, time.Nanosecond); err != nil {
			l.Error().Err(err).Msg("failed to prepare accept ballot")

			return e(err, "")
		}

		if eavp == nil || !eavp.Point().Point.Equal(ivp.Point().Point) {
			return nil
		}

		ll := l.With().Dict("accept_voteproof", base.VoteproofLog(eavp)).Logger()

		var sctx switchContext

		switch saved, err := st.handleACCEPTVoteproofAfterProcessingProposal(manifest, eavp); {
		case saved:
			ll.Debug().Msg("new block saved by accept voteproof after processing proposal")
		case err == nil:
			return nil
		case errors.As(err, &sctx):
		default:
			ll.Error().Err(err).Msg("failed to save new block by accept voteproof after processing proposal")

			sctx = newBrokenSwitchContext(StateConsensus, errors.Wrap(err, "failed to save proposal"))
		}

		return sctx
	}, nil
}

func (st *ConsensusHandler) processProposalInternal(ivp base.INITVoteproof) (isaac.ProcessorProcessFunc, error) {
	e := util.StringErrorFunc("failed to process proposal")

	facthash := ivp.BallotMajority().Proposal()

	var previous base.Manifest

	switch m, err := st.getManifest(ivp.Point().Height() - 1); {
	case err != nil:
		return nil, e(err, "")
	default:
		previous = m
	}

	switch process, err := st.pps.Process(st.ctx, facthash, previous, ivp); {
	case err != nil:
		return nil, e(err, "")
	case process == nil:
		return nil, nil
	default:
		return func(ctx context.Context) (base.Manifest, error) {
			switch manifest, err := process(ctx); {
			case err != nil:
				st.Log().Error().Err(err).Msg("failed to process proposal")

				if errors.Is(err, context.Canceled) {
					return nil, nil
				}

				if err0 := st.pps.Cancel(); err0 != nil {
					return nil, e(err0, "failed to cancel proposal processors")
				}

				return nil, err
			case manifest == nil:
				return nil, nil
			default:
				st.Log().Debug().Msg("proposal processed")

				return manifest, nil
			}
		}, nil
	}
}

func (st *ConsensusHandler) handleACCEPTVoteproofAfterProcessingProposal(
	manifest base.Manifest, avp base.ACCEPTVoteproof,
) (saved bool, _ error) {
	l := st.Log().With().Dict("accept_voteproof", base.VoteproofLog(avp)).Logger()

	switch { // NOTE check last accept voteproof is the execpted
	case avp.Result() != base.VoteResultMajority:
		if err := st.pps.Cancel(); err != nil {
			l.Error().Err(err).
				Msg("expected accept voteproof is not majority result; cancel processor, but failed")

			return false, err
		}

		l.Debug().Msg("expected accept voteproof is not majority result; ignore")

		return false, nil
	case !manifest.Hash().Equal(avp.BallotMajority().NewBlock()):
		if err := st.pps.Cancel(); err != nil {
			l.Error().Err(err).
				Msg("expected accept voteproof has different new block; cancel processor, but failed")

			return false, err
		}

		l.Debug().Msg("expected accept voteproof has different new block; moves to syncing")

		return false, newSyncingSwitchContext(StateConsensus, avp.Point().Height())
	default:
		l.Debug().Msg("proposal processed and expected voteproof found")
	}

	var sctx switchContext

	switch i, err := st.saveBlock(avp); {
	case err == nil:
		saved = i
	case errors.As(err, &sctx):
	default:
		sctx = newBrokenSwitchContext(StateConsensus, errors.Wrap(err, "failed to save proposal"))
	}

	return saved, sctx
}

func (st *ConsensusHandler) prepareINITBallot(bl base.INITBallot, initialWait time.Duration, ids []util.TimerID) error {
	go func() {
		<-time.After(initialWait)

		switch _, err := st.vote(bl); {
		case err == nil:
		case errors.Is(err, errNotInConsensusNodes):
			st.Log().Debug().Err(err).Msg("failed to vote init ballot; moves to syncing state")

			go st.switchState(newSyncingSwitchContext(StateConsensus, bl.Point().Height()-1))
		default:
			st.Log().Debug().Err(err).Msg("failed to vote init ballot; moves to broken state")

			go st.switchState(newBrokenSwitchContext(StateConsensus, err))
		}
	}()

	if err := st.broadcastINITBallot(bl, initialWait); err != nil {
		return err
	}

	return st.timers.StartTimers(ids, true)
}

func (st *ConsensusHandler) prepareACCEPTBallot(
	ivp base.INITVoteproof,
	manifest base.Manifest,
	initialWait time.Duration,
) error {
	e := util.StringErrorFunc("failed to prepare accept ballot")

	afact := isaac.NewACCEPTBallotFact(ivp.Point().Point, ivp.BallotMajority().Proposal(), manifest.Hash())
	signedFact := isaac.NewACCEPTBallotSignedFact(st.local.Address(), afact)

	if err := signedFact.Sign(st.local.Privatekey(), st.policy.NetworkID()); err != nil {
		return e(err, "")
	}

	bl := isaac.NewACCEPTBallot(ivp, signedFact)

	go func() {
		<-time.After(initialWait)

		switch _, err := st.vote(bl); {
		case err == nil:
		case errors.Is(err, errNotInConsensusNodes):
			st.Log().Debug().Err(err).Msg("failed to vote init ballot; moves to syncing state")

			go st.switchState(newSyncingSwitchContext(StateConsensus, ivp.Point().Height()-1))
		default:
			st.Log().Error().Err(err).Msg("failed to vote accept ballot; moves to broken state")

			go st.switchState(newBrokenSwitchContext(StateConsensus, err))
		}
	}()

	if err := st.broadcastACCEPTBallot(bl, initialWait); err != nil {
		return e(err, "failed to broadcast accept ballot")
	}

	if err := st.timers.StartTimers([]util.TimerID{
		timerIDBroadcastINITBallot,
		timerIDBroadcastACCEPTBallot,
	}, true); err != nil {
		return e(err, "failed to start timers for broadcasting accept ballot")
	}

	return nil
}

func (st *ConsensusHandler) newVoteproof(vp base.Voteproof) error {
	var lvps LastVoteproofs

	switch i, v := st.baseHandler.setNewVoteproof(vp); {
	case v == nil:
		return nil
	default:
		lvps = i
	}

	e := util.StringErrorFunc("failed to handle new voteproof")

	switch vp.Point().Stage() {
	case base.StageINIT:
		return st.newINITVoteproof(vp.(base.INITVoteproof), lvps) //nolint:forcetypeassert //...
	case base.StageACCEPT:
		return st.newACCEPTVoteproof(vp.(base.ACCEPTVoteproof), lvps) //nolint:forcetypeassert //...
	default:
		return e(nil, "invalid voteproof received, %T", vp)
	}
}

func (st *ConsensusHandler) newINITVoteproof(ivp base.INITVoteproof, lvps LastVoteproofs) error {
	c := lvps.Cap()
	l := st.Log().With().
		Dict("init_voteproof", base.VoteproofLog(ivp)).
		Dict("last_voteproof", base.VoteproofLog(c)).
		Logger()

	l.Debug().Msg("new init voteproof received")

	switch c.Point().Stage() { //nolint:exhaustive //...
	case base.StageINIT:
		return st.newINITVoteproofWithLastINITVoteproof(ivp, lvps)
	case base.StageACCEPT:
		return st.newINITVoteproofWithLastACCEPTVoteproof(ivp, lvps)
	}

	return nil
}

func (st *ConsensusHandler) newACCEPTVoteproof(avp base.ACCEPTVoteproof, lvps LastVoteproofs) error {
	lvp := lvps.Cap()
	l := st.Log().With().
		Dict("accept_voteproof", base.VoteproofLog(avp)).
		Dict("last_voteproof", base.VoteproofLog(lvp)).
		Logger()

	l.Debug().Msg("new accept voteproof received")

	switch lvp.Point().Stage() { //nolint:exhaustive //...
	case base.StageINIT:
		return st.newACCEPTVoteproofWithLastINITVoteproof(avp, lvps)
	case base.StageACCEPT:
		return st.newACCEPTVoteproofWithLastACCEPTVoteproof(avp, lvps)
	}

	return nil
}

func (st *ConsensusHandler) newINITVoteproofWithLastINITVoteproof(
	ivp base.INITVoteproof, lvps LastVoteproofs,
) error {
	livp := lvps.Cap().(base.INITVoteproof) //nolint:forcetypeassert //...

	l := st.Log().With().
		Dict("last_init_voteproof", base.VoteproofLog(livp)).
		Logger()

	switch {
	case ivp.Point().Height() > livp.Point().Height(): // NOTE higher height; moves to syncing state
		l.Debug().Msg("higher init voteproof; moves to syncing state")

		return newSyncingSwitchContext(StateConsensus, ivp.Point().Height()-1)
	case livp.Result() == base.VoteResultMajority:
		return nil
	case ivp.Result() == base.VoteResultMajority: // NOTE new init voteproof has same height, but higher round
		lavp := lvps.ACCEPT()

		ll := st.Log().With().
			Dict("init_voteproof", base.VoteproofLog(ivp)).
			Dict("last_accept_voteproof", base.VoteproofLog(lavp)).
			Logger()

		if lavp == nil {
			ll.Debug().Msg("empty last accept voteproof; moves to broken state")

			return newBrokenSwitchContext(StateConsensus, errors.Errorf("empty last accept voteproof"))
		}

		if m := lavp.BallotMajority(); m == nil || !ivp.BallotMajority().PreviousBlock().Equal(m.NewBlock()) {
			// NOTE local stored block is different with other nodes
			ll.Debug().
				Stringer("previous_block", ivp.BallotMajority().PreviousBlock()).
				Stringer("new_block", m.NewBlock()).
				Msg("previous block does not match with last accept voteproof; moves to syncing")

			return newSyncingSwitchContext(StateConsensus, ivp.Point().Height()-1)
		}

		switch process, err := st.processProposal(ivp); {
		case err != nil:
			return err
		case process == nil:
			return nil
		default:
			return process(st.ctx)
		}
	default:
		l.Debug().Msg("new init voteproof draw; moves to next round")

		go st.nextRound(ivp, lvps.PreviousBlockForNextRound(ivp))

		return nil
	}
}

func (st *ConsensusHandler) newINITVoteproofWithLastACCEPTVoteproof(
	ivp base.INITVoteproof, lvps LastVoteproofs,
) error {
	lavp := lvps.Cap().(base.ACCEPTVoteproof) //nolint:forcetypeassert //...

	l := st.Log().With().
		Dict("init_voteproof", base.VoteproofLog(ivp)).
		Dict("last_accept_voteproof", base.VoteproofLog(lavp)).
		Logger()

	switch expectedheight := lavp.Point().Height() + 1; {
	case ivp.Point().Height() > expectedheight:
		l.Debug().Msg("higher init voteproof; moves to syncing state")

		return newSyncingSwitchContext(StateConsensus, ivp.Point().Height()-1)
	case ivp.Result() == base.VoteResultDraw:
		l.Debug().Msg("new init voteproof draw; moves to next round")

		go st.nextRound(ivp, lvps.PreviousBlockForNextRound(ivp))

		return nil
	default:
		if m := lavp.BallotMajority(); m == nil || !ivp.BallotMajority().PreviousBlock().Equal(m.NewBlock()) {
			// NOTE local stored block is different with other nodes
			l.Debug().
				Stringer("previous_block", ivp.BallotMajority().PreviousBlock()).
				Interface("majority", m).
				Msg("previous block does not match with last accept voteproof; moves to syncing")

			return newSyncingSwitchContext(StateConsensus, ivp.Point().Height()-1)
		}
	}

	switch process, err := st.processProposal(ivp); {
	case err != nil:
		return err
	case process == nil:
		return nil
	default:
		return process(st.ctx)
	}
}

func (st *ConsensusHandler) newACCEPTVoteproofWithLastINITVoteproof(
	avp base.ACCEPTVoteproof, lvps LastVoteproofs,
) error {
	livp := lvps.Cap().(base.INITVoteproof) //nolint:forcetypeassert //...

	switch {
	case avp.Point().Point.Equal(livp.Point().Point): // NOTE expected accept voteproof
		if avp.Result() == base.VoteResultMajority {
			_, err := st.saveBlock(avp)

			return err
		}

		go st.nextRound(avp, lvps.PreviousBlockForNextRound(avp))

		return nil
	case avp.Point().Height() > livp.Point().Height():
	case avp.Result() == base.VoteResultDraw:
		go st.nextRound(avp, lvps.PreviousBlockForNextRound(avp))

		return nil
	}

	return newSyncingSwitchContext(StateConsensus, avp.Point().Height())
}

func (st *ConsensusHandler) newACCEPTVoteproofWithLastACCEPTVoteproof(
	avp base.ACCEPTVoteproof, lvps LastVoteproofs,
) error {
	lavp := lvps.Cap().(base.ACCEPTVoteproof) //nolint:forcetypeassert //...

	l := st.Log().With().
		Dict("last_accept_voteproof", base.VoteproofLog(lavp)).
		Logger()

	switch {
	case avp.Point().Height() > lavp.Point().Height():
		l.Debug().Msg("higher accept voteproof; moves to syncing state")

		return newSyncingSwitchContext(StateConsensus, avp.Point().Height())
	case avp.Result() == base.VoteResultDraw:
		l.Debug().Msg("new accept voteproof draw; moves to next round")

		go st.nextRound(avp, lvps.PreviousBlockForNextRound(avp))

		return nil
	default:
		return newSyncingSwitchContext(StateConsensus, avp.Point().Height())
	}
}

func (st *ConsensusHandler) nextRound(vp base.Voteproof, previousBlock util.Hash) {
	l := st.Log().With().Dict("voteproof", base.VoteproofLog(vp)).Logger()

	started := time.Now()

	if previousBlock == nil {
		l.Debug().Msg("failed to find previous block from last voteproofs; ignore to move next round")

		return
	}

	var sctx switchContext
	var bl base.INITBallot

	switch i, err := st.prepareNextRound(vp, previousBlock); {
	case err == nil:
		if i == nil {
			return
		}

		bl = i
	case errors.As(err, &sctx):
		go st.switchState(sctx)

		return
	default:
		l.Debug().Err(err).Msg("failed to prepare next round; moves to broken state")

		go st.switchState(newBrokenSwitchContext(StateConsensus, err))

		return
	}

	initialWait := time.Nanosecond
	if d := time.Since(started); d < st.policy.WaitPreparingINITBallot() {
		initialWait = st.policy.WaitPreparingINITBallot() - d
	}

	if err := st.prepareINITBallot(bl, initialWait, []util.TimerID{timerIDBroadcastINITBallot}); err != nil {
		l.Error().Err(err).Msg("failed to prepare init ballot for next round")

		return
	}

	l.Debug().Interface("ballot", bl).Msg("init ballot broadcasted for next round")
}

func (st *ConsensusHandler) nextBlock(avp base.ACCEPTVoteproof) {
	point := avp.Point().Point.NextHeight()

	l := st.Log().With().Dict("voteproof", base.VoteproofLog(avp)).Object("point", point).Logger()

	started := time.Now()

	var sctx switchContext
	var bl base.INITBallot

	switch i, err := st.prepareNextBlock(avp, st.nodeInConsensusNodes); {
	case err == nil:
		if i == nil {
			return
		}

		bl = i
	case errors.As(err, &sctx):
		go st.switchState(sctx)

		return
	default:
		l.Debug().Err(err).Msg("failed to prepare next block; moves to broken state")

		go st.switchState(newBrokenSwitchContext(StateConsensus, err))

		return
	}

	initialWait := time.Nanosecond
	if d := time.Since(started); d < st.policy.WaitPreparingINITBallot() {
		initialWait = st.policy.WaitPreparingINITBallot() - d
	}

	if err := st.prepareINITBallot(bl, initialWait, []util.TimerID{
		timerIDBroadcastINITBallot,
		timerIDBroadcastACCEPTBallot,
	}); err != nil {
		l.Error().Err(err).Msg("failed to prepare init ballot for next block")

		return
	}

	l.Debug().Interface("ballot", bl).Msg("next init ballot broadcasted")
}

func (st *ConsensusHandler) saveBlock(avp base.ACCEPTVoteproof) (bool, error) {
	facthash := avp.BallotMajority().Proposal()

	l := st.Log().With().Dict("voteproof", base.VoteproofLog(avp)).Logger()
	ll := l.With().Stringer("fact", facthash).Logger()

	ll.Debug().Msg("expected accept voteproof; trying to save proposal")

	switch err := st.pps.Save(context.Background(), facthash, avp); {
	case err == nil:
		ll.Debug().Msg("processed proposal saved; moves to next block")

		go st.whenNewBlockSaved(avp.Point().Height())
		go st.nextBlock(avp)

		return true, nil
	case errors.Is(err, isaac.ErrProcessorAlreadySaved):
		l.Debug().Msg("already saved")

		return false, nil
	case errors.Is(err, isaac.NotProposalProcessorProcessedError):
		l.Debug().Msg("no processed proposal; moves to syncing state")

		return false, newSyncingSwitchContext(StateConsensus, avp.Point().Height())
	default:
		ll.Error().Err(err).Msg("failed to save proposal; moves to broken state")

		return false, newBrokenSwitchContext(StateConsensus, err)
	}
}

type consensusSwitchContext struct {
	ivp base.INITVoteproof
	baseSwitchContext
}

func newConsensusSwitchContext(from StateType, ivp base.INITVoteproof) consensusSwitchContext {
	return consensusSwitchContext{
		baseSwitchContext: newBaseSwitchContext(from, StateConsensus),
		ivp:               ivp,
	}
}
