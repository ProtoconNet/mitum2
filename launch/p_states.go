package launch

import (
	"context"
	"io"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	isaacblock "github.com/spikeekips/mitum/isaac/block"
	isaacdatabase "github.com/spikeekips/mitum/isaac/database"
	isaacnetwork "github.com/spikeekips/mitum/isaac/network"
	isaacstates "github.com/spikeekips/mitum/isaac/states"
	"github.com/spikeekips/mitum/network/quicmemberlist"
	"github.com/spikeekips/mitum/network/quicstream"
	leveldbstorage "github.com/spikeekips/mitum/storage/leveldb"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/logging"
	"github.com/spikeekips/mitum/util/ps"
	"github.com/spikeekips/mitum/util/valuehash"
)

var (
	PNameStatesReady                                = ps.Name("states-ready")
	PNameStates                                     = ps.Name("states")
	PNameBallotbox                                  = ps.Name("ballotbox")
	PNameProposalProcessors                         = ps.Name("proposal-processors")
	PNameStatesSetHandlers                          = ps.Name("states-set-handlers")
	PNameProposerSelector                           = ps.Name("proposer-selector")
	PNameBallotStuckResolver                        = ps.Name("ballot-stuck-resolver")
	BallotboxContextKey                             = util.ContextKey("ballotbox")
	StatesContextKey                                = util.ContextKey("states")
	ProposalProcessorsContextKey                    = util.ContextKey("proposal-processors")
	ProposerSelectorContextKey                      = util.ContextKey("proposer-selector")
	ProposalSelectorContextKey                      = util.ContextKey("proposal-selector")
	WhenNewBlockSavedInSyncingStateFuncContextKey   = util.ContextKey("when-new-block-saved-in-syncing-state-func")
	WhenNewBlockSavedInConsensusStateFuncContextKey = util.ContextKey("when-new-block-saved-in-consensus-state-func")
	WhenNewBlockConfirmedFuncContextKey             = util.ContextKey("when-new-block-confirmed-func")
	BallotStuckResolverContextKey                   = util.ContextKey("ballot-stuck-resolver")
)

func PBallotbox(pctx context.Context) (context.Context, error) {
	var log *logging.Logging
	var local base.LocalNode
	var isaacparams *isaac.Params
	var sp *SuffragePool

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		LocalContextKey, &local,
		ISAACParamsContextKey, &isaacparams,
		SuffragePoolContextKey, &sp,
	); err != nil {
		return pctx, err
	}

	ballotbox := isaacstates.NewBallotbox(local.Address(), isaacparams.Threshold, sp.Height)
	_ = ballotbox.SetCountAfter(isaacparams.WaitPreparingINITBallot())
	_ = ballotbox.SetLogging(log)

	if err := ballotbox.Start(context.Background()); err != nil {
		return pctx, err
	}

	return context.WithValue(pctx, BallotboxContextKey, ballotbox), nil
}

func PBallotStuckResolver(pctx context.Context) (context.Context, error) {
	e := util.StringError("load ballot stuck resolver")

	var log *logging.Logging
	var design NodeDesign
	var local base.LocalNode
	var isaacparams *isaac.Params
	var ballotbox *isaacstates.Ballotbox
	var m *quicmemberlist.Memberlist
	var sp *SuffragePool
	var svf *isaac.SuffrageVoting

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		DesignContextKey, &design,
		LocalContextKey, &local,
		ISAACParamsContextKey, &isaacparams,
		BallotboxContextKey, &ballotbox,
		MemberlistContextKey, &m,
		SuffragePoolContextKey, &sp,
		SuffrageVotingContextKey, &svf,
	); err != nil {
		return pctx, e.Wrap(err)
	}

	findMissingBallotsf := isaacstates.FindMissingBallotsFromBallotboxFunc(
		local.Address(),
		sp.Height,
		ballotbox,
	)

	requestMissingBallotsf := isaacstates.RequestMissingBallots(
		quicstream.NewConnInfo(design.Network.Publish(), design.Network.TLSInsecure),
		m.CallbackBroadcast,
	)

	voteSuffrageVotingf := isaacstates.VoteSuffrageVotingFunc(
		local,
		isaacparams.NetworkID(),
		ballotbox,
		svf,
		sp.Height,
	)

	switch {
	case isaacparams.BallotStuckWait() < isaacparams.WaitPreparingINITBallot():
		return pctx, util.ErrInvalid.Errorf("too short ballot stuck wait; it should be over wait_preparing_init_ballot")
	case isaacparams.BallotStuckWait() < isaacparams.WaitPreparingINITBallot()*2:
		log.Log().Warn().
			Dur("ballot_stuck_wait", isaacparams.BallotStuckWait()).
			Msg("too short ballot stuck wait; proper valud is over 2 * wait_preparing_init_ballot")
	}

	r := isaacstates.NewDefaultBallotStuckResolver(
		isaacparams.BallotStuckWait(),
		time.Second,
		isaacparams.BallotStuckResolveAfter(),
		findMissingBallotsf,
		requestMissingBallotsf,
		voteSuffrageVotingf,
	)

	_ = r.SetLogging(log)

	return context.WithValue(pctx, BallotStuckResolverContextKey, r), nil
}

func PStates(pctx context.Context) (context.Context, error) {
	e := util.StringError("prepare states")

	args := isaacstates.NewStatesArgs()

	var log *logging.Logging
	var enc encoder.Encoder
	var devflags DevFlags
	var local base.LocalNode
	var isaacparams *isaac.Params
	var syncSourcePool *isaac.SyncSourcePool
	var pool *isaacdatabase.TempPool
	var m *quicmemberlist.Memberlist

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		EncoderContextKey, &enc,
		DevFlagsContextKey, &devflags,
		LocalContextKey, &local,
		ISAACParamsContextKey, &isaacparams,
		BallotboxContextKey, &args.Ballotbox,
		LastVoteproofsHandlerContextKey, &args.LastVoteproofsHandler,
		SyncSourcePoolContextKey, &syncSourcePool,
		BallotStuckResolverContextKey, &args.BallotStuckResolver,
		PoolDatabaseContextKey, &pool,
		MemberlistContextKey, &m,
	); err != nil {
		return pctx, e.Wrap(err)
	}

	args.IntervalBroadcastBallot = isaacparams.IntervalBroadcastBallot
	args.AllowConsensus = devflags.AllowConsensus

	if vp := args.LastVoteproofsHandler.Last().Cap(); vp != nil {
		last := args.Ballotbox.LastPoint()

		isset := args.Ballotbox.SetLastPointFromVoteproof(vp)

		log.Log().Debug().
			Interface("lastpoint", last).
			Interface("last_voteproof", vp).
			Bool("is_set", isset).
			Msg("last voteproof updated to ballotbox")
	}

	args.IsInSyncSourcePoolFunc = syncSourcePool.IsInFixed
	args.BallotBroadcaster = isaacstates.NewDefaultBallotBroadcaster(
		local.Address(),
		pool,
		func(bl base.Ballot) error {
			ee := util.StringError("broadcast ballot")

			b, err := enc.Marshal(bl)
			if err != nil {
				return ee.Wrap(err)
			}

			id := valuehash.NewSHA256(bl.HashBytes()).String()
			if err := m.CallbackBroadcast(b, id, nil); err != nil {
				return ee.Wrap(err)
			}

			return nil
		},
	)

	states, err := isaacstates.NewStates(isaacparams.NetworkID(), local, args)
	if err != nil {
		return pctx, err
	}

	_ = states.SetLogging(log)

	proposalSelector, err := NewProposalSelector(pctx)
	if err != nil {
		return pctx, e.Wrap(err)
	}

	nctx := context.WithValue(pctx, StatesContextKey, states)
	nctx = context.WithValue(nctx, ProposalSelectorContextKey, proposalSelector)

	return patchStatesArgsForHandover(nctx, args)
}

func PCloseStates(pctx context.Context) (context.Context, error) {
	var states *isaacstates.States
	var ballotbox *isaacstates.Ballotbox

	if err := util.LoadFromContext(pctx,
		StatesContextKey, &states,
		BallotboxContextKey, &ballotbox,
	); err != nil {
		return pctx, err
	}

	if states != nil {
		if err := states.Stop(); err != nil && !errors.Is(err, util.ErrDaemonAlreadyStopped) {
			return pctx, err
		}
	}

	if ballotbox != nil {
		if err := ballotbox.Stop(); err != nil && !errors.Is(err, util.ErrDaemonAlreadyStopped) {
			return pctx, err
		}
	}

	return pctx, nil
}

func PStatesSetHandlers(pctx context.Context) (context.Context, error) { //revive:disable-line:function-length
	e := util.StringError("set states handler")

	var log *logging.Logging
	var local base.LocalNode
	var isaacparams *isaac.Params
	var db isaac.Database
	var states *isaacstates.States
	var nodeinfo *isaacnetwork.NodeInfoUpdater
	var ballotbox *isaacstates.Ballotbox
	var sv *isaac.SuffrageVoting

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		LocalContextKey, &local,
		ISAACParamsContextKey, &isaacparams,
		CenterDatabaseContextKey, &db,
		StatesContextKey, &states,
		NodeInfoContextKey, &nodeinfo,
		BallotboxContextKey, &ballotbox,
		SuffrageVotingContextKey, &sv,
	); err != nil {
		return pctx, e.Wrap(err)
	}

	votef := func(bl base.Ballot) (bool, error) {
		return ballotbox.Vote(bl)
	}

	suffrageVotingFindf := func(ctx context.Context, height base.Height, suf base.Suffrage) (
		[]base.SuffrageExpelOperation, error,
	) {
		return sv.Find(ctx, height, suf)
	}

	getLastManifestf := getLastManifestFunc(db)
	getManifestf := getManifestFunc(db)

	states.SetWhenStateSwitched(func(next isaacstates.StateType) {
		_ = nodeinfo.SetConsensusState(next)
	})

	syncingargs, err := newSyncingHandlerArgs(pctx)
	if err != nil {
		return pctx, e.Wrap(err)
	}

	consensusargs, err := newConsensusHandlerArgs(pctx)
	if err != nil {
		return pctx, e.Wrap(err)
	}

	consensusargs.VoteFunc = votef
	consensusargs.SuffrageVotingFindFunc = suffrageVotingFindf
	consensusargs.GetManifestFunc = getManifestf

	joiningargs, err := newJoiningHandlerArgs(pctx)
	if err != nil {
		return pctx, e.Wrap(err)
	}

	joiningargs.VoteFunc = votef
	joiningargs.SuffrageVotingFindFunc = suffrageVotingFindf
	joiningargs.LastManifestFunc = getLastManifestf

	handoverargs, err := handoverHandlerArgs(pctx)
	if err != nil {
		return pctx, e.Wrap(err)
	}

	handoverargs.VoteFunc = votef
	handoverargs.SuffrageVotingFindFunc = suffrageVotingFindf
	handoverargs.GetManifestFunc = getManifestf

	bootingargs, err := newBootingHandlerArgs(pctx)
	if err != nil {
		return pctx, e.Wrap(err)
	}

	bootingargs.LastManifestFunc = getLastManifestf

	states.
		SetHandler(isaacstates.StateBroken, isaacstates.NewNewBrokenHandlerType(isaacparams.NetworkID(), local)).
		SetHandler(isaacstates.StateStopped, isaacstates.NewNewStoppedHandlerType(isaacparams.NetworkID(), local)).
		SetHandler(isaacstates.StateBooting,
			isaacstates.NewNewBootingHandlerType(isaacparams.NetworkID(), local, bootingargs)).
		SetHandler(isaacstates.StateJoining,
			isaacstates.NewNewJoiningHandlerType(isaacparams.NetworkID(), local, joiningargs)).
		SetHandler(isaacstates.StateConsensus,
			isaacstates.NewNewConsensusHandlerType(isaacparams.NetworkID(), local, consensusargs)).
		SetHandler(isaacstates.StateSyncing,
			isaacstates.NewNewSyncingHandlerType(isaacparams.NetworkID(), local, syncingargs)).
		SetHandler(isaacstates.StateHandover,
			isaacstates.NewNewHandoverHandlerType(isaacparams.NetworkID(), local, handoverargs))

	_ = states.SetLogging(log)

	return pctx, nil
}

func newSyncerFunc(pctx context.Context) (
	func(height base.Height) (isaac.Syncer, error),
	error,
) {
	var db isaac.Database

	if err := util.LoadFromContextOK(pctx,
		CenterDatabaseContextKey, &db,
	); err != nil {
		return nil, err
	}

	newSyncerDeferredf, err := newSyncerDeferredFunc(pctx, db)
	if err != nil {
		return nil, err
	}

	newArgsf, err := newSyncerArgsFunc(pctx)
	if err != nil {
		return nil, err
	}

	return func(height base.Height) (isaac.Syncer, error) {
		e := util.StringError("newSyncer")

		var prev base.BlockMap

		switch m, found, err := db.LastBlockMap(); {
		case err != nil:
			return nil, e.Wrap(isaacstates.ErrUnpromising.Wrap(err))
		case found:
			prev = m
		}

		args, err := newArgsf(height)
		if err != nil {
			return nil, e.Wrap(err)
		}

		syncer := isaacstates.NewSyncer(prev, args)

		go newSyncerDeferredf(height, syncer)

		return syncer, nil
	}, nil
}

func newConsensusHandlerArgs(pctx context.Context) (*isaacstates.ConsensusHandlerArgs, error) {
	var log *logging.Logging
	var isaacparams *isaac.Params
	var ballotbox *isaacstates.Ballotbox
	var db isaac.Database
	var proposalSelector *isaac.BaseProposalSelector
	var pps *isaac.ProposalProcessors
	var nodeinfo *isaacnetwork.NodeInfoUpdater
	var nodeInConsensusNodesf func(base.Node, base.Height) (base.Suffrage, bool, error)

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		ISAACParamsContextKey, &isaacparams,
		BallotboxContextKey, &ballotbox,
		CenterDatabaseContextKey, &db,
		ProposalSelectorContextKey, &proposalSelector,
		ProposalProcessorsContextKey, &pps,
		NodeInfoContextKey, &nodeinfo,
		NodeInConsensusNodesFuncContextKey, &nodeInConsensusNodesf,
	); err != nil {
		return nil, err
	}

	var whenNewBlockSavedf func(base.BlockMap)

	switch err := util.LoadFromContext(pctx, WhenNewBlockSavedInConsensusStateFuncContextKey, &whenNewBlockSavedf); {
	case err != nil:
		return nil, err
	case whenNewBlockSavedf == nil:
		whenNewBlockSavedf = func(base.BlockMap) {}
	}

	defaultWhenNewBlockSavedf := DefaultWhenNewBlockSavedInConsensusStateFunc(log, ballotbox, db, nodeinfo)

	var whenNewBlockConfirmedf func(base.Height)

	switch err := util.LoadFromContext(
		pctx, WhenNewBlockConfirmedFuncContextKey, &whenNewBlockConfirmedf); {
	case err != nil:
		return nil, err
	case whenNewBlockConfirmedf == nil:
		whenNewBlockConfirmedf = func(base.Height) {}
	}

	defaultWhenNewBlockConfirmedf := DefaultWhenNewBlockConfirmedFunc(log)

	args := isaacstates.NewConsensusHandlerArgs()
	args.IntervalBroadcastBallot = isaacparams.IntervalBroadcastBallot
	args.WaitPreparingINITBallot = isaacparams.WaitPreparingINITBallot
	args.NodeInConsensusNodesFunc = nodeInConsensusNodesf
	args.ProposalSelectFunc = proposalSelector.Select
	args.ProposalProcessors = pps
	args.WhenNewBlockSaved = func(bm base.BlockMap) {
		defaultWhenNewBlockSavedf(bm)

		whenNewBlockSavedf(bm)
	}
	args.WhenNewBlockConfirmed = func(height base.Height) {
		defaultWhenNewBlockConfirmedf(height)

		whenNewBlockConfirmedf(height)
	}

	return args, nil
}

func newJoiningHandlerArgs(pctx context.Context) (*isaacstates.JoiningHandlerArgs, error) {
	var isaacparams *isaac.Params
	var proposalSelector *isaac.BaseProposalSelector
	var nodeInConsensusNodesf func(base.Node, base.Height) (base.Suffrage, bool, error)

	if err := util.LoadFromContextOK(pctx,
		ISAACParamsContextKey, &isaacparams,
		ProposalSelectorContextKey, &proposalSelector,
		NodeInConsensusNodesFuncContextKey, &nodeInConsensusNodesf,
	); err != nil {
		return nil, err
	}

	joinMemberlistf, err := joinMemberlistForJoiningeHandlerFunc(pctx)
	if err != nil {
		return nil, err
	}

	leaveMemberlistf, err := leaveMemberlistForHandlerFunc(pctx)
	if err != nil {
		return nil, err
	}

	args := isaacstates.NewJoiningHandlerArgs()
	args.NodeInConsensusNodesFunc = nodeInConsensusNodesf
	args.ProposalSelectFunc = proposalSelector.Select
	args.JoinMemberlistFunc = joinMemberlistf
	args.LeaveMemberlistFunc = leaveMemberlistf
	args.IntervalBroadcastBallot = isaacparams.IntervalBroadcastBallot
	args.WaitFirstVoteproof = func() time.Duration {
		return isaacparams.IntervalBroadcastBallot()*2 + isaacparams.WaitPreparingINITBallot()
	}
	args.WaitPreparingINITBallot = isaacparams.WaitPreparingINITBallot

	return args, nil
}

func newBootingHandlerArgs(pctx context.Context) (*isaacstates.BootingHandlerArgs, error) {
	var nodeInConsensusNodesf func(base.Node, base.Height) (base.Suffrage, bool, error)

	if err := util.LoadFromContextOK(pctx,
		NodeInConsensusNodesFuncContextKey, &nodeInConsensusNodesf,
	); err != nil {
		return nil, err
	}

	args := isaacstates.NewBootingHandlerArgs()
	args.NodeInConsensusNodesFunc = nodeInConsensusNodesf

	return args, nil
}

func newSyncingHandlerArgs(pctx context.Context) (*isaacstates.SyncingHandlerArgs, error) {
	var log *logging.Logging
	var isaacparams *isaac.Params
	var db isaac.Database
	var ballotbox *isaacstates.Ballotbox
	var nodeinfo *isaacnetwork.NodeInfoUpdater
	var nodeInConsensusNodesf func(base.Node, base.Height) (base.Suffrage, bool, error)

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		ISAACParamsContextKey, &isaacparams,
		CenterDatabaseContextKey, &db,
		BallotboxContextKey, &ballotbox,
		NodeInfoContextKey, &nodeinfo,
		NodeInConsensusNodesFuncContextKey, &nodeInConsensusNodesf,
	); err != nil {
		return nil, err
	}

	newsyncerf, err := newSyncerFunc(pctx)
	if err != nil {
		return nil, err
	}

	joinMemberlistf, err := joinMemberlistForSyncingHandlerFunc(pctx)
	if err != nil {
		return nil, err
	}

	leaveMemberlistf, err := leaveMemberlistForHandlerFunc(pctx)
	if err != nil {
		return nil, err
	}

	whenNewBlockSavedf := func(base.Height) {}
	if err = util.LoadFromContext(pctx,
		WhenNewBlockSavedInSyncingStateFuncContextKey, &whenNewBlockSavedf); err != nil {
		return nil, err
	}

	defaultWhenNewBlockSavedf := DefaultWhenNewBlockSavedInSyncingStateFunc(log, db, nodeinfo)

	args := isaacstates.NewSyncingHandlerArgs()
	args.WaitStuckInterval = func() time.Duration {
		return isaacparams.IntervalBroadcastBallot()*2 + isaacparams.WaitPreparingINITBallot()
	}
	args.WaitPreparingINITBallot = isaacparams.WaitPreparingINITBallot
	args.NodeInConsensusNodesFunc = nodeInConsensusNodesf
	args.NewSyncerFunc = newsyncerf
	args.WhenReachedTopFunc = func(base.Height) {
		ballotbox.Count()
	}
	args.JoinMemberlistFunc = joinMemberlistf
	args.LeaveMemberlistFunc = leaveMemberlistf
	args.WhenNewBlockSavedFunc = func(height base.Height) {
		defaultWhenNewBlockSavedf(height)

		whenNewBlockSavedf(height)
	}

	return args, nil
}

func newSyncerArgsFunc(pctx context.Context) (func(base.Height) (isaacstates.SyncerArgs, error), error) {
	var log *logging.Logging
	var encs *encoder.Encoders
	var enc encoder.Encoder
	var devflags DevFlags
	var design NodeDesign
	var params *LocalParams
	var client *isaacnetwork.QuicstreamClient
	var st *leveldbstorage.Storage
	var db isaac.Database
	var syncSourcePool *isaac.SyncSourcePool
	var lvps *isaac.LastVoteproofsHandler
	var nodeinfo *isaacnetwork.NodeInfoUpdater

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		EncodersContextKey, &encs,
		EncoderContextKey, &enc,
		DevFlagsContextKey, &devflags,
		DesignContextKey, &design,
		LocalParamsContextKey, &params,
		QuicstreamClientContextKey, &client,
		LeveldbStorageContextKey, &st,
		CenterDatabaseContextKey, &db,
		SyncSourcePoolContextKey, &syncSourcePool,
		LastVoteproofsHandlerContextKey, &lvps,
		NodeInfoContextKey, &nodeinfo,
	); err != nil {
		return nil, err
	}

	isaacparams := params.ISAAC

	setLastVoteproofsfFromBlockReaderf, err := setLastVoteproofsfFromBlockReaderFunc(lvps)
	if err != nil {
		return nil, err
	}

	removePrevBlockf, err := removePrevBlockFunc(pctx)
	if err != nil {
		return nil, err
	}

	var whenNewBlockSavedInSyncingStatef func(base.Height)

	switch err = util.LoadFromContext(
		pctx, WhenNewBlockSavedInSyncingStateFuncContextKey, &whenNewBlockSavedInSyncingStatef); {
	case err != nil:
		return nil, err
	case whenNewBlockSavedInSyncingStatef == nil:
		whenNewBlockSavedInSyncingStatef = func(base.Height) {}
	}

	defaultWhenNewBlockSavedInSyncingStatef := DefaultWhenNewBlockSavedInSyncingStateFunc(log, db, nodeinfo)

	var whenNewBlockConfirmedf func(base.Height)

	switch err = util.LoadFromContext(
		pctx, WhenNewBlockConfirmedFuncContextKey, &whenNewBlockConfirmedf); {
	case err != nil:
		return nil, err
	case whenNewBlockConfirmedf == nil:
		whenNewBlockConfirmedf = func(base.Height) {}
	}

	defaultWhenNewBlockConfirmedf := DefaultWhenNewBlockConfirmedFunc(log)

	return func(height base.Height) (args isaacstates.SyncerArgs, _ error) {
		var tempsyncpool isaac.TempSyncPool

		switch i, err := isaacdatabase.NewLeveldbTempSyncPool(height, st, encs, enc); {
		case err != nil:
			return args, isaacstates.ErrUnpromising.Wrap(err)
		default:
			tempsyncpool = i
		}

		newclient := client.Clone()

		conninfocache, _ := util.NewShardedMap[base.Height, quicstream.ConnInfo](1 << 9) //nolint:gomnd //...

		args = isaacstates.NewSyncerArgs()
		args.LastBlockMapFunc = syncerLastBlockMapFunc(newclient, isaacparams, syncSourcePool)
		args.LastBlockMapTimeout = params.MISC.TimeoutRequest()
		args.BlockMapFunc = syncerBlockMapFunc(newclient, params, syncSourcePool, conninfocache, devflags.DelaySyncer)
		args.TempSyncPool = tempsyncpool
		args.WhenStoppedFunc = func() error {
			conninfocache.Close()

			return newclient.Close()
		}
		args.RemovePrevBlockFunc = removePrevBlockf
		args.NewImportBlocksFunc = func(
			ctx context.Context,
			from, to base.Height,
			batchlimit int64,
			blockMapf func(context.Context, base.Height) (base.BlockMap, bool, error),
		) error {
			return isaacblock.ImportBlocks(
				ctx,
				from, to,
				batchlimit,
				blockMapf,
				syncerBlockMapItemFunc(newclient, conninfocache, params.MISC.TimeoutRequest),
				func(blockmap base.BlockMap) (isaac.BlockImporter, error) {
					bwdb, err := db.NewBlockWriteDatabase(blockmap.Manifest().Height())
					if err != nil {
						return nil, err
					}

					return isaacblock.NewBlockImporter(
						LocalFSDataDirectory(design.Storage.Base),
						encs,
						blockmap,
						bwdb,
						func(context.Context) error {
							return db.MergeBlockWriteDatabase(bwdb)
						},
						isaacparams.NetworkID(),
					)
				},
				setLastVoteproofsfFromBlockReaderf,
				func(context.Context) error {
					defaultWhenNewBlockSavedInSyncingStatef(to)

					if c := to.SafePrev(); c >= from {
						defaultWhenNewBlockConfirmedf(c)
						whenNewBlockConfirmedf(c)
					}

					whenNewBlockSavedInSyncingStatef(to)

					return db.MergeAllPermanent()
				},
			)
		}

		return args, nil
	}, nil
}

func syncerLastBlockMapFunc(
	client *isaacnetwork.QuicstreamClient,
	params *isaac.Params,
	syncSourcePool *isaac.SyncSourcePool,
) isaacstates.SyncerLastBlockMapFunc {
	f := func(
		ctx context.Context, manifest util.Hash, ci quicstream.ConnInfo,
	) (_ base.BlockMap, updated bool, _ error) {
		switch m, updated, err := client.LastBlockMap(ctx, ci, manifest); {
		case err != nil, !updated:
			return m, updated, err
		default:
			if err := m.IsValid(params.NetworkID()); err != nil {
				return m, updated, err
			}

			return m, updated, nil
		}
	}

	return func(ctx context.Context, manifest util.Hash) (base.BlockMap, bool, error) {
		ml := util.EmptyLocked[base.BlockMap]()

		numnodes := 3 // NOTE choose top 3 sync nodes

		if err := isaac.DistributeWorkerWithSyncSourcePool(
			context.Background(),
			syncSourcePool,
			numnodes,
			uint64(numnodes),
			nil,
			func(_ context.Context, i, _ uint64, nci isaac.NodeConnInfo) error {
				ci, err := nci.ConnInfo()
				if err != nil {
					return err
				}

				m, updated, err := f(ctx, manifest, ci)
				switch {
				case err != nil:
					return err
				case !updated:
					return nil
				}

				_, err = ml.Set(func(v base.BlockMap, _ bool) (base.BlockMap, error) {
					switch {
					case v == nil,
						m.Manifest().Height() > v.Manifest().Height():

						return m, nil
					default:
						return nil, util.ErrLockedSetIgnore.Errorf("old BlockMap")
					}
				})

				return err
			},
		); err != nil {
			return nil, false, err
		}

		switch v, _ := ml.Value(); {
		case v == nil:
			return nil, false, nil
		default:
			return v, true, nil
		}
	}
}

func syncerBlockMapFunc( //revive:disable-line:cognitive-complexity
	client *isaacnetwork.QuicstreamClient,
	params *LocalParams,
	syncSourcePool *isaac.SyncSourcePool,
	conninfocache util.LockedMap[base.Height, quicstream.ConnInfo],
	devdelay time.Duration,
) isaacblock.ImportBlocksBlockMapFunc {
	f := func(ctx context.Context, height base.Height, ci quicstream.ConnInfo) (base.BlockMap, bool, error) {
		if devdelay > 0 {
			<-time.After(devdelay) // NOTE for testing
		}

		cctx, cancel := context.WithTimeout(ctx, params.MISC.TimeoutRequest())
		defer cancel()

		switch m, found, err := client.BlockMap(cctx, ci, height); {
		case err != nil, !found:
			return m, found, err
		default:
			if err := m.IsValid(params.ISAAC.NetworkID()); err != nil {
				return m, true, err
			}

			return m, true, nil
		}
	}

	return func(ctx context.Context, height base.Height) (m base.BlockMap, found bool, _ error) {
		err := util.Retry(
			ctx,
			func() (bool, error) {
				numnodes := 3 // NOTE choose top 3 sync nodes
				result := util.EmptyLocked[[2]interface{}]()

				_ = isaac.ErrGroupWorkerWithSyncSourcePool(
					ctx,
					syncSourcePool,
					numnodes,
					uint64(numnodes),
					func(ctx context.Context, i, _ uint64, nci isaac.NodeConnInfo) error {
						ci, err := nci.ConnInfo()
						if err != nil {
							return err
						}

						switch a, b, err := f(ctx, height, ci); {
						case err != nil:
							return err
						case !b:
							return errors.Errorf("not found")
						default:
							_, _ = result.Set(func(i [2]interface{}, isempty bool) ([2]interface{}, error) {
								if !isempty {
									return [2]interface{}{}, errors.Errorf("already set")
								}

								_ = conninfocache.SetValue(height, ci)

								return [2]interface{}{a, b}, nil
							})

							return nil
						}
					},
				)

				i, isempty := result.Value()
				if isempty {
					return true, nil
				}

				m, found = i[0].(base.BlockMap), i[1].(bool) //nolint:forcetypeassert //...

				return false, nil
			},
			-1,
			time.Second,
		)

		return m, found, err
	}
}

func syncerBlockMapItemFunc(
	client *isaacnetwork.QuicstreamClient,
	conninfocache util.LockedMap[base.Height, quicstream.ConnInfo],
	requestTimeoutf func() time.Duration,
) isaacblock.ImportBlocksBlockMapItemFunc {
	nrequestTimeoutf := func() time.Duration {
		return isaac.DefaultTimeoutRequest
	}

	if requestTimeoutf != nil {
		nrequestTimeoutf = requestTimeoutf
	}

	return func(ctx context.Context, height base.Height, item base.BlockMapItemType) (
		reader io.Reader, closef func() error, found bool, _ error,
	) {
		e := util.StringError("fetch blockmap item")

		var ci quicstream.ConnInfo

		switch i, cfound := conninfocache.Value(height); {
		case !cfound:
			return nil, nil, false, e.Errorf("conninfo not found")
		default:
			ci = i
		}

		cctx, ctxcancel := context.WithTimeout(ctx, nrequestTimeoutf())
		defer ctxcancel()

		r, cancel, found, err := client.BlockMapItem(cctx, ci, height, item)
		if err != nil {
			return nil, nil, false, e.Wrap(err)
		}

		return r, cancel, found, err
	}
}

func setLastVoteproofsfFromBlockReaderFunc(
	lvps *isaac.LastVoteproofsHandler,
) (func(isaac.BlockReader) error, error) {
	return func(reader isaac.BlockReader) error {
		switch v, found, err := reader.Item(base.BlockMapItemTypeVoteproofs); {
		case err != nil:
			return err
		case !found:
			return errors.Errorf("voteproofs not found at last")
		default:
			vps := v.([]base.Voteproof) //nolint:forcetypeassert //...

			_ = lvps.Set(vps[0].(base.INITVoteproof))   //nolint:forcetypeassert //...
			_ = lvps.Set(vps[1].(base.ACCEPTVoteproof)) //nolint:forcetypeassert //...

			return nil
		}
	}, nil
}

func joinMemberlistForSyncingHandlerFunc(pctx context.Context) (
	func(context.Context, base.Suffrage) error,
	error,
) {
	var long *LongRunningMemberlistJoin
	if err := util.LoadFromContextOK(pctx, LongRunningMemberlistJoinContextKey, &long); err != nil {
		return nil, err
	}

	return func(ctx context.Context, _ base.Suffrage) error {
		donech := long.Join()
		if donech == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "join")
		case <-donech:
			return nil
		}
	}, nil
}

func joinMemberlistForJoiningeHandlerFunc(pctx context.Context) (
	func(context.Context, base.Suffrage) error,
	error,
) {
	var long *LongRunningMemberlistJoin
	if err := util.LoadFromContextOK(pctx, LongRunningMemberlistJoinContextKey, &long); err != nil {
		return nil, err
	}

	return func(ctx context.Context, _ base.Suffrage) error {
		_ = long.Join()

		return nil
	}, nil
}

func leaveMemberlistForHandlerFunc(pctx context.Context) (
	func(time.Duration) error,
	error,
) {
	var long *LongRunningMemberlistJoin
	var m *quicmemberlist.Memberlist

	if err := util.LoadFromContextOK(pctx,
		MemberlistContextKey, &m,
		LongRunningMemberlistJoinContextKey, &long,
	); err != nil {
		return nil, err
	}

	return func(timeout time.Duration) error {
		_ = long.Cancel()

		switch {
		case !m.IsJoined():
			return nil
		default:
			return m.Leave(timeout)
		}
	}, nil
}

func getLastManifestFunc(db isaac.Database) func() (base.Manifest, bool, error) {
	return func() (base.Manifest, bool, error) {
		switch m, found, err := db.LastBlockMap(); {
		case err != nil || !found:
			return nil, found, err
		default:
			return m.Manifest(), true, nil
		}
	}
}

func getManifestFunc(db isaac.Database) func(height base.Height) (base.Manifest, error) {
	return func(height base.Height) (base.Manifest, error) {
		switch m, found, err := db.BlockMap(height); {
		case err != nil:
			return nil, err
		case !found:
			return nil, nil
		default:
			return m.Manifest(), nil
		}
	}
}

func DefaultWhenNewBlockSavedInSyncingStateFunc(
	log *logging.Logging,
	db isaac.Database,
	nodeinfo *isaacnetwork.NodeInfoUpdater,
) func(base.Height) {
	return func(height base.Height) {
		log.Log().Debug().
			Interface("height", height).
			Stringer("state", isaacstates.StateSyncing).
			Msg("new block saved")

		_ = UpdateNodeInfoWithNewBlock(db, nodeinfo)
	}
}

func DefaultWhenNewBlockSavedInConsensusStateFunc(
	log *logging.Logging,
	ballotbox *isaacstates.Ballotbox,
	db isaac.Database,
	nodeinfo *isaacnetwork.NodeInfoUpdater,
) func(base.BlockMap) {
	return func(bm base.BlockMap) {
		log.Log().Debug().
			Interface("blockmap", bm).
			Interface("height", bm.Manifest().Height()).
			Stringer("state", isaacstates.StateConsensus).
			Msg("new block saved")

		ballotbox.Count()

		_ = UpdateNodeInfoWithNewBlock(db, nodeinfo)
	}
}

func DefaultWhenNewBlockConfirmedFunc(log *logging.Logging) func(base.Height) {
	return func(height base.Height) {
		log.Log().Debug().Interface("height", height).Msg("new block confirmed")
	}
}

func newSyncerDeferredFunc(pctx context.Context, db isaac.Database) (
	func(base.Height, *isaacstates.Syncer),
	error,
) {
	var log *logging.Logging

	if err := util.LoadFromContextOK(pctx, LoggingContextKey, &log); err != nil {
		return nil, err
	}

	return func(height base.Height, syncer *isaacstates.Syncer) {
		l := log.Log().With().Str("module", "new-syncer").Logger()

		if err := db.MergeAllPermanent(); err != nil {
			l.Error().Err(err).Msg("failed to merge temps")

			return
		}

		log.Log().Debug().Msg("SuffrageProofs built")

		_ = syncer.Add(height)

		l.Debug().Interface("height", height).Msg("new syncer created")
	}, nil
}

func removePrevBlockFunc(pctx context.Context) (func(base.Height) (bool, error), error) {
	var db isaac.Database
	var design NodeDesign
	var sp *SuffragePool

	if err := util.LoadFromContextOK(pctx,
		DesignContextKey, &design,
		CenterDatabaseContextKey, &db,
		SuffragePoolContextKey, &sp,
	); err != nil {
		return nil, err
	}

	return func(height base.Height) (bool, error) {
		// NOTE remove from database
		switch removed, err := db.RemoveBlocks(height); {
		case err != nil:
			return false, err
		case !removed:
			return false, nil
		}

		switch p, found, err := db.LastSuffrageProof(); {
		case err != nil || !found:
			return false, err
		case height <= p.Map().Manifest().Height():
			sp.Purge() // NOTE purge suffrage cache
		}

		// NOTE remove from localfs
		switch removed, err := isaacblock.RemoveBlocksFromLocalFS(design.Storage.Base, height); {
		case err != nil:
			return false, err
		case !removed:
			return false, nil
		}

		return true, nil
	}, nil
}

func handoverHandlerArgs(pctx context.Context) (*isaacstates.HandoverHandlerArgs, error) {
	var log *logging.Logging
	var isaacparams *isaac.Params
	var ballotbox *isaacstates.Ballotbox
	var db isaac.Database
	var proposalSelector *isaac.BaseProposalSelector
	var pps *isaac.ProposalProcessors
	var nodeinfo *isaacnetwork.NodeInfoUpdater
	var nodeInConsensusNodesf func(base.Node, base.Height) (base.Suffrage, bool, error)

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		ISAACParamsContextKey, &isaacparams,
		BallotboxContextKey, &ballotbox,
		CenterDatabaseContextKey, &db,
		ProposalSelectorContextKey, &proposalSelector,
		ProposalProcessorsContextKey, &pps,
		NodeInfoContextKey, &nodeinfo,
		NodeInConsensusNodesFuncContextKey, &nodeInConsensusNodesf,
	); err != nil {
		return nil, err
	}

	var whenNewBlockSavedf func(base.BlockMap)

	switch err := util.LoadFromContext(pctx, WhenNewBlockSavedInConsensusStateFuncContextKey, &whenNewBlockSavedf); {
	case err != nil:
		return nil, err
	case whenNewBlockSavedf == nil:
		whenNewBlockSavedf = func(base.BlockMap) {}
	}

	var whenNewBlockConfirmedf func(base.Height)

	switch err := util.LoadFromContext(
		pctx, WhenNewBlockConfirmedFuncContextKey, &whenNewBlockConfirmedf); {
	case err != nil:
		return nil, err
	case whenNewBlockConfirmedf == nil:
		whenNewBlockConfirmedf = func(base.Height) {}
	}

	defaultWhenNewBlockSavedf := DefaultWhenNewBlockSavedInConsensusStateFunc(log, ballotbox, db, nodeinfo)
	defaultWhenNewBlockConfirmedf := DefaultWhenNewBlockConfirmedFunc(log)

	args := isaacstates.NewHandoverHandlerArgs()
	args.IntervalBroadcastBallot = isaacparams.IntervalBroadcastBallot
	args.WaitPreparingINITBallot = isaacparams.WaitPreparingINITBallot
	args.NodeInConsensusNodesFunc = nodeInConsensusNodesf
	args.ProposalSelectFunc = proposalSelector.Select
	args.ProposalProcessors = pps
	args.WhenNewBlockSaved = func(bm base.BlockMap) {
		defaultWhenNewBlockSavedf(bm)

		whenNewBlockSavedf(bm)
	}
	args.WhenNewBlockConfirmed = func(height base.Height) {
		defaultWhenNewBlockConfirmedf(height)

		whenNewBlockConfirmedf(height)
	}

	return args, nil
}
