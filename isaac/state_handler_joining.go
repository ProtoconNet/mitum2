package isaac

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/base/ballot"
	"github.com/spikeekips/mitum/base/block"
	"github.com/spikeekips/mitum/base/prprocessor"
	"github.com/spikeekips/mitum/base/seal"
	"github.com/spikeekips/mitum/storage"
	"github.com/spikeekips/mitum/util/localtime"
	"github.com/spikeekips/mitum/util/logging"
)

type StateJoiningHandler struct {
	*BaseStateHandler
	cr base.Round
}

func NewStateJoiningHandler(
	local *Local,
	pps *prprocessor.Processors,
) (*StateJoiningHandler, error) {
	cs := &StateJoiningHandler{
		BaseStateHandler: NewBaseStateHandler(local, pps, base.StateJoining),
	}
	cs.BaseStateHandler.Logging = logging.NewLogging(func(c logging.Context) logging.Emitter {
		return c.Str("module", "consensus-state-joining-handler")
	})

	cs.BaseStateHandler.timers = localtime.NewTimers([]string{TimerIDBroadcastingINITBallot}, false)

	return cs, nil
}

func (cs *StateJoiningHandler) SetLogger(l logging.Logger) logging.Logger {
	_ = cs.Logging.SetLogger(l)
	_ = cs.timers.SetLogger(l)

	return cs.Log()
}

func (cs *StateJoiningHandler) Activate(_ *StateChangeContext) error {
	cs.Lock()
	defer cs.Unlock()

	var avp base.Voteproof // NOTE ACCEPT Voteproof of last block
	switch vp, found, err := cs.local.BlockFS().LastVoteproof(base.StageACCEPT); {
	case !found:
		return storage.NotFoundError.Errorf("last voteproof not found")
	case err != nil:
		return xerrors.Errorf("failed to get last voteproof: %w", err)
	default:
		avp = vp
	}

	cs.activate()

	cs.cr = base.Round(0)

	if err := cs.broadcastINITBallot(cs.cr, avp); err != nil {
		return err
	}

	cs.Log().Debug().Msg("activated")

	return nil
}

func (cs *StateJoiningHandler) Deactivate(_ *StateChangeContext) error {
	cs.Lock()
	defer cs.Unlock()

	cs.deactivate()

	if err := cs.timers.Stop(); err != nil {
		return err
	}

	cs.Log().Debug().Msg("deactivated")

	return nil
}

func (cs *StateJoiningHandler) currentRound() base.Round {
	cs.RLock()
	defer cs.RUnlock()

	return cs.cr
}

func (cs *StateJoiningHandler) setCurrentRound(round base.Round, voteproof base.Voteproof) error {
	cs.Lock()
	defer cs.Unlock()

	if cs.cr == round {
		return nil
	}

	cs.cr = round

	return cs.broadcastINITBallot(cs.cr, voteproof)
}

// NewSeal only cares on INIT ballot and it's Voteproof.
func (cs *StateJoiningHandler) NewSeal(sl seal.Seal) error {
	var blt ballot.Ballot
	var voteproof base.Voteproof
	switch t := sl.(type) {
	case ballot.Proposal:
		return cs.handleProposal(t)
	case ballot.INITBallot:
		blt = t
		voteproof = t.Voteproof()
	case ballot.ACCEPTBallot:
		blt = t
		voteproof = t.Voteproof()
	default:
		cs.Log().Debug().
			Hinted("seal_hint", sl.Hint()).
			Hinted("seal_hash", sl.Hash()).
			Str("seal_signer", sl.Signer().String()).
			Msg("this type of seal will be ignored; in joining only ballots will be handled")

		return nil
	}

	l := loggerWithVoteproof(voteproof, loggerWithBallot(blt, cs.Log()))
	l.Debug().Msg("got voteproof with ballot")

	if blt.Stage() == base.StageINIT {
		switch voteproof.Stage() {
		case base.StageACCEPT:
			return cs.voteproofToStates(voteproof, func() error {
				return cs.handleINITBallotAndACCEPTVoteproof(blt.(ballot.INITBallot), voteproof)
			})
		case base.StageINIT:
			return cs.voteproofToStates(voteproof, func() error {
				return cs.handleINITBallotAndINITVoteproof(blt.(ballot.INITBallot), voteproof)
			})
		default:
			return xerrors.Errorf("invalid Voteproof stage found in init ballot")
		}
	} else if blt.Stage() == base.StageACCEPT {
		switch voteproof.Stage() {
		case base.StageINIT:
			return cs.voteproofToStates(voteproof, func() error {
				return cs.handleACCEPTBallotAndINITVoteproof(blt.(ballot.ACCEPTBallot), voteproof)
			})
		default:
			return xerrors.Errorf("invalid Voteproof stage found in accept ballot")
		}
	}

	return xerrors.Errorf("invalid ballot stage found")
}

func (cs *StateJoiningHandler) voteproofToStates(voteproof base.Voteproof, callback func() error) error {
	finished := make(chan error)
	go cs.VoteproofToStates(base.VoteproofWithCallback(voteproof, func() error {
		err := callback()

		finished <- err

		return err
	}))

	return <-finished
}

func (cs *StateJoiningHandler) handleProposal(proposal ballot.Proposal) error {
	l := cs.Log().WithLogger(func(ctx logging.Context) logging.Emitter {
		return ctx.Hinted("proposal_hash", proposal.Hash()).
			Hinted("proposal_height", proposal.Height()).
			Hinted("proposal_round", proposal.Round())
	})

	l.Debug().Msg("got proposal")

	return nil
}

func (cs *StateJoiningHandler) handleINITBallotAndACCEPTVoteproof(
	blt ballot.INITBallot, voteproof base.Voteproof,
) error {
	l := loggerWithVoteproof(voteproof, loggerWithBallot(blt, cs.Log()))
	l.Debug().Msg("INIT Ballot + ACCEPT Voteproof")

	var height base.Height
	switch m, found, err := cs.local.Storage().LastManifest(); {
	case !found:
		return storage.NotFoundError.Errorf("last manifest not found")
	case err != nil:
		return err
	default:
		height = m.Height()
	}

	switch d := blt.Height() - (height + 1); {
	case d > 0:
		l.Debug().
			Msgf("Ballot.Height() is higher than expected, %d + 1; moves to syncing", height)

		return cs.ChangeState(base.StateSyncing, voteproof, blt)
	case d == 0:
		l.Debug().Msg("same height; keep waiting another voteproof")

		return nil
	default:
		l.Debug().
			Msgf("Ballot.Height() is lower than expected, %d + 1; ignore it", height)

		return nil
	}
}

func (cs *StateJoiningHandler) handleINITBallotAndINITVoteproof(blt ballot.INITBallot, voteproof base.Voteproof) error {
	l := loggerWithVoteproof(voteproof, loggerWithBallot(blt, cs.Log()))
	l.Debug().Msg("INIT Ballot + INIT Voteproof")

	var manifest block.Manifest
	switch m, found, err := cs.local.Storage().LastManifest(); {
	case !found:
		return storage.NotFoundError.Errorf("last manifest not found")
	case err != nil:
		return err
	default:
		manifest = m
	}

	switch d := blt.Height() - (manifest.Height() + 1); {
	case d == 0:
		if err := checkBlockWithINITVoteproof(manifest, voteproof); err != nil {
			l.Error().Err(err).Msg("expected height, checked voteproof with block")

			return err
		}

		if blt.Round() > cs.currentRound() {
			l.Debug().
				Hinted("current_round", cs.currentRound()).
				Msg("Voteproof.Round() is same or greater than currentRound; use this round")

			if err := cs.setCurrentRound(blt.Round(), voteproof); err != nil {
				return err
			}
		} else {
			l.Debug().Msg("same height; keep waiting another voteproof")
		}

		return nil
	case d > 0:
		l.Debug().
			Msgf("ballotVoteproof.Height() is higher than expected, %d + 1; moves to syncing", manifest.Height())

		return cs.ChangeState(base.StateSyncing, voteproof, blt)
	default:
		l.Debug().
			Msgf("ballotVoteproof.Height() is lower than expected, %d + 1; ignore it", manifest.Height())

		return nil
	}
}

func (cs *StateJoiningHandler) handleACCEPTBallotAndINITVoteproof(
	blt ballot.ACCEPTBallot,
	voteproof base.Voteproof,
) error {
	l := loggerWithVoteproof(voteproof, loggerWithBallot(blt, cs.Log()))
	l.Debug().Msg("ACCEPT Ballot + INIT Voteproof")

	var manifest block.Manifest
	switch m, found, err := cs.local.Storage().LastManifest(); {
	case !found:
		return storage.NotFoundError.Errorf("last manifest not found")
	case err != nil:
		return err
	default:
		manifest = m
	}

	switch d := blt.Height() - (manifest.Height() + 1); {
	case d == 0:
		if err := checkBlockWithINITVoteproof(manifest, voteproof); err != nil {
			l.Error().Err(err).Msg("expected height, checked voteproof with block")

			return err
		}

		// NOTE expected ACCEPT Ballot received, so will process Proposal of
		// INIT Voteproof and broadcast new ACCEPT Ballot.
		var newBlock block.Block
		if blk, err := cs.processACCEPTBallotAndINITVoteproof(blt, voteproof); err != nil {
			return err
		} else {
			newBlock = blk
		}

		ab := NewACCEPTBallotV0(cs.local.Node().Address(), newBlock, voteproof)
		if err := SignSeal(&ab, cs.local); err != nil {
			cs.Log().Error().Err(err).Msg("failed to sign ACCEPTBallot; will keep trying")

			return err
		} else {
			al := loggerWithBallot(ab, l)
			cs.BroadcastSeal(ab)

			al.Debug().Msg("ACCEPTBallot was broadcasted")
		}

		return nil
	case d > 0:
		l.Debug().
			Msgf("Ballot.Height() is higher than expected, %d + 1; moves to syncing", manifest.Height())

		return cs.ChangeState(base.StateSyncing, voteproof, blt)
	default:
		l.Debug().
			Msgf("Ballot.Height() is lower than expected, %d + 1; ignore it", manifest.Height())

		return nil
	}
}

// NewVoteproof receives Voteproof.
func (cs *StateJoiningHandler) NewVoteproof(voteproof base.Voteproof) error {
	l := loggerWithVoteproof(voteproof, cs.Log())

	switch voteproof.Stage() {
	case base.StageACCEPT:
		// NOTE ACCEPT Voteproof is next block of local, but do nothing.
		return nil
	case base.StageINIT:
		return cs.handleINITVoteproof(voteproof)
	default:
		err := xerrors.Errorf("unknown stage Voteproof received")
		l.Error().Err(err).Msg("invalid voteproof found")
		return err
	}
}

func (cs *StateJoiningHandler) handleINITVoteproof(voteproof base.Voteproof) error {
	l := loggerWithLocal(cs.local, loggerWithVoteproof(voteproof, cs.Log()))

	l.Debug().Msg("expected height; moves to consensus state")

	return cs.ChangeState(base.StateConsensus, voteproof, nil)
}

func (cs *StateJoiningHandler) broadcastINITBallot(round base.Round, voteproof base.Voteproof) error {
	if timer, err := cs.TimerBroadcastingINITBallot(
		func(i int) time.Duration {
			// NOTE at first time, wait enough time for incoming ballot
			if i < 1 {
				return cs.local.Policy().IntervalBroadcastingINITBallot() * 3
			} else {
				return cs.local.Policy().IntervalBroadcastingINITBallot()
			}
		},
		round,
		voteproof,
	); err != nil {
		return err
	} else if err := cs.timers.SetTimer(TimerIDBroadcastingINITBallot, timer); err != nil {
		return err
	}

	// NOTE starts to keep broadcasting INIT Ballot
	if err := cs.timers.StartTimers([]string{TimerIDBroadcastingINITBallot}, true); err != nil {
		return err
	} else {
		return nil
	}
}

func (cs *StateJoiningHandler) processACCEPTBallotAndINITVoteproof(
	blt ballot.ACCEPTBallot,
	voteproof base.Voteproof,
) (block.Block, error) {
	l := loggerWithVoteproof(voteproof, loggerWithBallot(blt, cs.Log()))

	// NOTE expected ACCEPT Ballot received, so will process Proposal of
	// INIT Voteproof and broadcast new ACCEPT Ballot.
	var proposal ballot.Proposal
	if pr, err := cs.findProposal(blt.Proposal()); err != nil {
		return nil, err
	} else {
		timespan := cs.local.Policy().TimespanValidBallot()
		if pr.SignedAt().Before(voteproof.FinishedAt().Add(timespan * -1)) {
			return nil, xerrors.Errorf(
				"Proposal was sent before Voteproof; SignedAt=%s now=%s timespan=%s",
				pr.SignedAt(), voteproof.FinishedAt(), timespan,
			)
		}

		proposal = pr
	}

	timeout := cs.local.Policy().TimeoutProcessProposal()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	l.Debug().Dur("timeout", timeout).Msg("trying to prepare block")

	if result := <-cs.pps.NewProposal(ctx, proposal, voteproof); result.Err != nil {
		l.Debug().Err(result.Err).Msg("tried to process Proposal, but it is not yet received")

		return nil, result.Err
	} else if err := cs.SetLastINITVoteproof(voteproof); err != nil {
		return nil, err
	} else {
		return result.Block, nil
	}
}
