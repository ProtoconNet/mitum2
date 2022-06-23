package main

import (
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	isaacblock "github.com/spikeekips/mitum/isaac/block"
	isaacstates "github.com/spikeekips/mitum/isaac/states"
	"github.com/spikeekips/mitum/launch"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
)

func (cmd *runCommand) getSuffrageFunc() func(blockheight base.Height) (base.Suffrage, bool, error) {
	return func(blockheight base.Height) (base.Suffrage, bool, error) {
		return isaac.GetSuffrageFromDatabase(cmd.db, blockheight)
	}
}

func (cmd *runCommand) getManifestFunc() func(height base.Height) (base.Manifest, error) {
	return func(height base.Height) (base.Manifest, error) {
		switch m, found, err := cmd.db.BlockMap(height); {
		case err != nil:
			return nil, err
		case !found:
			return nil, nil
		default:
			return m.Manifest(), nil
		}
	}
}

func (cmd *runCommand) proposalMaker() *isaac.ProposalMaker {
	return isaac.NewProposalMaker(
		cmd.local,
		cmd.nodePolicy,
		func(ctx context.Context) ([]util.Hash, error) {
			policy := cmd.db.LastNetworkPolicy()
			n := policy.MaxOperationsInProposal()
			if n < 1 {
				return nil, nil
			}

			hs, err := cmd.pool.NewOperationHashes(
				ctx,
				n,
				func(facthash util.Hash) (bool, error) {
					// FIXME if bad operation and it is failed to be processed;
					// it can be included in next proposal; it should be
					// excluded.
					// FIXME if operation has not enough fact signs, it will
					// ignored. It must be filtered for not this kind of
					// operations.
					switch found, err := cmd.db.ExistsInStateOperation(facthash); {
					case err != nil:
						return false, err
					case !found:
						return false, nil
					}

					return true, nil
				},
			)
			if err != nil {
				return nil, err
			}

			return hs, nil
		},
		cmd.pool,
	)
}

func (cmd *runCommand) proposalSelectorFunc() *isaac.BaseProposalSelector {
	return isaac.NewBaseProposalSelector(
		cmd.local,
		cmd.nodePolicy,
		//isaac.NewBlockBasedProposerSelector( FIXME use
		//	func(height base.Height) (util.Hash, error) {
		//		switch m, err := cmd.getManifest(height); {
		//		case err != nil:
		//			return nil, err
		//		case m == nil:
		//			return nil, nil
		//		default:
		//			return m.Hash(), nil
		//		}
		//	},
		//),
		isaac.NewFixedProposerSelector([]base.Node{cmd.local}), // FIXME remove
		cmd.proposalMaker(),
		cmd.getSuffrage,
		func() []base.Address { return nil },
		func(context.Context, base.Point, base.Address) (
			base.ProposalSignedFact, error,
		) {
			// FIXME set request
			return nil, nil
		},
		cmd.pool,
	)
}

func (cmd *runCommand) getLastManifestFunc() func() (base.Manifest, bool, error) {
	return func() (base.Manifest, bool, error) {
		switch m, found, err := cmd.db.LastBlockMap(); {
		case err != nil || !found:
			return nil, found, err
		default:
			return m.Manifest(), true, nil
		}
	}
}

func (cmd *runCommand) newProposalProcessorFunc(enc encoder.Encoder) newProposalProcessorFunc {
	return func(proposal base.ProposalSignedFact, previous base.Manifest) (
		isaac.ProposalProcessor, error,
	) {
		return isaac.NewDefaultProposalProcessor(
			proposal,
			previous,
			launch.NewBlockWriterFunc(
				cmd.local, networkID, launch.LocalFSDataDirectory(cmd.design.Storage.Base), enc, cmd.db),
			cmd.db.State,
			nil,
			nil,
			cmd.pool.SetLastVoteproofs,
		)
	}
}

func (cmd *runCommand) states() (*isaacstates.States, error) {
	box := isaacstates.NewBallotbox(cmd.getSuffrage, cmd.nodePolicy.Threshold())
	voteFunc := func(bl base.Ballot) (bool, error) {
		voted, err := box.Vote(bl)
		if err != nil {
			return false, err
		}

		return voted, nil
	}

	pps := isaac.NewProposalProcessors(cmd.newProposalProcessor, cmd.getProposal)
	_ = pps.SetLogging(logging)

	states := isaacstates.NewStates(box)
	_ = states.SetLogging(logging)

	whenNewBlockSaved := func(height base.Height) {
		box.Count()
	}

	syncinghandler := isaacstates.NewSyncingHandler(cmd.local, cmd.nodePolicy, cmd.proposalSelector, cmd.newSyncer)
	syncinghandler.SetWhenFinished(func(height base.Height) { // FIXME set later
	})

	states.
		SetHandler(isaacstates.NewBrokenHandler(cmd.local, cmd.nodePolicy)).
		SetHandler(isaacstates.NewStoppedHandler(cmd.local, cmd.nodePolicy)).
		SetHandler(isaacstates.NewBootingHandler(cmd.local, cmd.nodePolicy, cmd.getLastManifest, cmd.getSuffrage)).
		SetHandler(
			isaacstates.NewJoiningHandler(
				cmd.local, cmd.nodePolicy, cmd.proposalSelector, cmd.getLastManifest, cmd.getSuffrage, voteFunc,
			),
		).
		SetHandler(
			isaacstates.NewConsensusHandler(
				cmd.local, cmd.nodePolicy, cmd.proposalSelector,
				cmd.getManifest, cmd.getSuffrage, voteFunc, whenNewBlockSaved,
				pps,
			)).
		SetHandler(syncinghandler)

	// NOTE load last init, accept voteproof and last majority voteproof
	switch ivp, avp, found, err := cmd.pool.LastVoteproofs(); {
	case err != nil:
		return nil, err
	case !found:
	default:
		_ = states.LastVoteproofsHandler().Set(ivp)
		_ = states.LastVoteproofsHandler().Set(avp)
	}

	return states, nil
}

func (cmd *runCommand) newSyncer(height base.Height) (isaac.Syncer, error) {
	e := util.StringErrorFunc("failed newSyncer")

	// NOTE if no discoveries, moves to broken state
	if len(cmd.SyncNode) < 1 {
		return nil, e(isaacstates.ErrUnpromising.Errorf("syncer needs one or more SyncNode"), "")
	}

	var lastsuffrageproof base.SuffrageProof

	switch proof, found, err := cmd.db.LastSuffrageProof(); {
	case err != nil:
		return nil, e(err, "")
	case found:
		lastsuffrageproof = proof
	}

	var prev base.BlockMap

	switch m, found, err := cmd.db.LastBlockMap(); {
	case err != nil:
		return nil, e(isaacstates.ErrUnpromising.Wrap(err), "")
	case found:
		prev = m
	}

	var tempsyncpool isaac.TempSyncPool

	switch i, err := launch.NewTempSyncPoolDatabase(cmd.design.Storage.Base, height, cmd.encs, cmd.enc); {
	case err != nil:
		return nil, e(isaacstates.ErrUnpromising.Wrap(err), "")
	default:
		tempsyncpool = i
	}

	syncer, err := isaacstates.NewSyncer(
		cmd.design.Storage.Base,
		func(height base.Height) (isaac.BlockWriteDatabase, func(context.Context) error, error) {
			bwdb, err := cmd.db.NewBlockWriteDatabase(height)
			if err != nil {
				return nil, nil, err
			}

			return bwdb,
				func(ctx context.Context) error {
					return launch.MergeBlockWriteToPermanentDatabase(ctx, bwdb, cmd.perm)
				},
				nil
		},
		func(root string, blockmap base.BlockMap, bwdb isaac.BlockWriteDatabase) (isaac.BlockImporter, error) {
			return isaacblock.NewBlockImporter(
				root,
				cmd.encs,
				blockmap,
				bwdb,
				networkID,
			)
		},
		prev,
		cmd.syncerLastBlockMapf(),
		cmd.syncerBlockMapf(),
		cmd.syncerBlockMapItemf(),
		tempsyncpool,
		cmd.setLastVoteproofsf(),
	)
	if err != nil {
		return nil, e(err, "")
	}

	go cmd.newSyncerDeferred(height, syncer, lastsuffrageproof)

	return syncer, nil
}

func (cmd *runCommand) newSyncerDeferred(
	height base.Height,
	syncer *isaacstates.Syncer,
	lastsuffrageproof base.SuffrageProof,
) {
	l := log.With().Str("module", "new-syncer").Logger()

	if err := cmd.db.MergeAllPermanent(); err != nil {
		l.Error().Err(err).Msg("failed to merge temps")

		return
	}

	var lastsuffragestate base.State
	if lastsuffrageproof != nil {
		lastsuffragestate = lastsuffrageproof.State()
	}

	if _, err := cmd.suffrageStateBuilder.Build(context.Background(), lastsuffragestate); err != nil {
		l.Error().Err(err).Msg("suffrage state builder failed")

		return
	}

	log.Debug().Msg("SuffrageProofs built")

	err := syncer.Start()
	if err != nil {
		l.Error().Err(err).Msg("syncer stopped")

		return
	}

	_ = syncer.Add(height)

	l.Debug().Interface("height", height).Msg("new syncer created")
}

func (cmd *runCommand) getProposalFunc() func(_ context.Context, facthash util.Hash) (base.ProposalSignedFact, error) {
	return func(_ context.Context, facthash util.Hash) (base.ProposalSignedFact, error) {
		switch pr, found, err := cmd.pool.Proposal(facthash); {
		case err != nil:
			return nil, err
		case !found:
			// FIXME if not found, request to remote node
			return nil, nil
		default:
			return pr, nil
		}
	}
}

func (cmd *runCommand) syncerLastBlockMapf() isaacstates.SyncerLastBlockMapFunc {
	return func(ctx context.Context, manifest util.Hash) (_ base.BlockMap, updated bool, _ error) {
		discovery := cmd.Discovery[0]

		ci, err := discovery.ConnInfo()
		if err != nil {
			return nil, false, err
		}

		switch m, updated, err := cmd.client.LastBlockMap(ctx, ci, manifest); {
		case err != nil, !updated:
			return m, updated, err
		default:
			if err := m.IsValid(networkID); err != nil {
				return m, updated, err
			}

			return m, updated, nil
		}
	}
}

func (cmd *runCommand) syncerBlockMapf() isaacstates.SyncerBlockMapFunc {
	return func(ctx context.Context, height base.Height) (base.BlockMap, bool, error) {
		// FIXME use multiple discoveries
		sn := cmd.SyncNode[0]

		ci, err := sn.ConnInfo()
		if err != nil {
			return nil, false, err
		}

		switch m, found, err := cmd.client.BlockMap(ctx, ci, height); {
		case err != nil, !found:
			return m, found, err
		default:
			if err := m.IsValid(networkID); err != nil {
				return m, found, err
			}

			return m, found, nil
		}
	}
}

func (cmd *runCommand) syncerBlockMapItemf() isaacstates.SyncerBlockMapItemFunc {
	return func(
		ctx context.Context, height base.Height, item base.BlockMapItemType,
	) (io.ReadCloser, func() error, bool, error) {
		sn := cmd.SyncNode[0]

		ci, err := sn.ConnInfo()
		if err != nil {
			return nil, nil, false, err
		}

		r, cancel, found, err := cmd.client.BlockMapItem(ctx, ci, height, item)

		return r, cancel, found, err
	}
}

func (cmd *runCommand) setLastVoteproofsf() func(isaac.BlockReader) error {
	return func(reader isaac.BlockReader) error {
		switch v, found, err := reader.Item(base.BlockMapItemTypeVoteproofs); {
		case err != nil:
			return err
		case !found:
			return errors.Errorf("voteproofs not found at last")
		default:
			vps := v.([]base.Voteproof)           //nolint:forcetypeassert //...
			if err := cmd.pool.SetLastVoteproofs( //nolint:forcetypeassert //...
				vps[0].(base.INITVoteproof),
				vps[1].(base.ACCEPTVoteproof),
			); err != nil {
				return err
			}

			return nil
		}
	}
}
