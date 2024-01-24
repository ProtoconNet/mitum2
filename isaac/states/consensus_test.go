package isaacstates

import (
	"context"
	"testing"
	"time"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type baseTestConsensusHandler struct {
	isaac.BaseTestBallots
}

func (t *baseTestConsensusHandler) newargs(previous base.Manifest, suf base.Suffrage) *ConsensusHandlerArgs {
	local := t.Local

	args := NewConsensusHandlerArgs()
	args.ProposalProcessors = isaac.NewProposalProcessors(nil, nil)
	args.GetManifestFunc = func(base.Height) (base.Manifest, error) { return previous, nil }
	args.NodeInConsensusNodesFunc = func(base.Node, base.Height) (base.Suffrage, bool, error) {
		if suf == nil {
			return nil, false, nil
		}

		return suf, suf.ExistsPublickey(local.Address(), local.Publickey()), nil
	}
	args.VoteFunc = func(base.Ballot) (bool, error) { return true, nil }
	args.SuffrageVotingFindFunc = func(context.Context, base.Height, base.Suffrage) ([]base.SuffrageExpelOperation, error) {
		return nil, nil
	}
	args.WaitPreparingINITBallot = t.LocalParams.WaitPreparingINITBallot

	return args
}

func (t *baseTestConsensusHandler) newState(args *ConsensusHandlerArgs) (*ConsensusHandler, func()) {
	newhandler := NewNewConsensusHandlerType(t.LocalParams.NetworkID(), t.Local, args)
	_ = newhandler.SetLogging(logging.TestNilLogging)

	timers, err := util.NewSimpleTimers(3, time.Millisecond*33)
	t.NoError(err)

	i, err := newhandler.new()
	t.NoError(err)

	st := i.(*ConsensusHandler)

	st.bbt = newBallotBroadcastTimers(timers, func(ctx context.Context, bl base.Ballot) error {
		return st.broadcastBallot(ctx, bl)
	}, args.IntervalBroadcastBallot())
	t.NoError(st.bbt.Start(context.Background()))

	return st, func() {
		st.bbt.Stop()

		deferred, err := st.exit(nil)
		t.NoError(err)
		deferred()
	}
}

func (t *baseTestConsensusHandler) newStateWithINITVoteproof(point base.Point, suf base.Suffrage) (
	*ConsensusHandler,
	func(),
	*isaac.DummyProposalProcessor,
	base.INITVoteproof,
) {
	previous := base.NewDummyManifest(point.Height()-1, valuehash.RandomSHA256())

	prpool := t.PRPool
	fact := prpool.GetFact(point)

	args := t.newargs(previous, suf)

	pp := isaac.NewDummyProposalProcessor()
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return nil, errors.Errorf("process error")
	}

	args.ProposalProcessors.SetMakeNew(pp.Make)
	args.ProposalProcessors.SetGetProposal(func(_ context.Context, _ base.Point, facthash util.Hash) (base.ProposalSignFact, error) {
		return prpool.ByHash(facthash)
	})

	args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			pr := prpool.ByPoint(p)
			if pr != nil {
				return pr, nil
			}
			return nil, util.ErrNotFound.WithStack()
		}
	}

	st, closef := t.newState(args)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(base.Ballot) error {
		return nil
	})
	st.switchStateFunc = func(switchContext) error {
		return nil
	}

	nodes := make([]base.LocalNode, suf.Len())
	sn := suf.Nodes()
	for i := range sn {
		nodes[i] = sn[i].(base.LocalNode)
	}

	avp, ivp := t.VoteproofsPair(point.PrevHeight(), point, nil, nil, fact.Hash(), nodes)
	t.True(st.setLastVoteproof(avp))
	t.True(st.setLastVoteproof(ivp))

	return st, closef, pp, ivp
}

type testConsensusHandler struct {
	baseTestConsensusHandler
}

func (t *testConsensusHandler) TestFailedToFetchProposal() {
	point := base.RawPoint(33, 0)

	previous := base.NewDummyManifest(point.Height()-1, valuehash.RandomSHA256())
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	args := t.newargs(previous, suf)
	args.ProposalProcessors = isaac.NewProposalProcessors(nil, func(context.Context, base.Point, util.Hash) (base.ProposalSignFact, error) {
		return nil, util.ErrNotFound.WithStack()
	})
	args.ProposalProcessors.SetRetryLimit(1).SetRetryInterval(1)

	prpool := t.PRPool
	args.ProposalSelectFunc = func(_ context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		return prpool.Get(p), nil
	}

	newhandler := NewNewConsensusHandlerType(t.LocalParams.NetworkID(), t.Local, args)

	i, err := newhandler.new()
	t.NoError(err)

	st := i.(*ConsensusHandler)

	_, ok := (interface{})(st).(handler)
	t.True(ok)

	defer func() {
		deferred, err := st.exit(nil)
		t.NoError(err)
		deferred()
	}()

	st.switchStateFunc = func(switchContext) error { return nil }

	timers, err := util.NewSimpleTimers(3, time.Millisecond*333)
	t.NoError(err)

	st.bbt = newBallotBroadcastTimers(timers, func(ctx context.Context, bl base.Ballot) error {
		return st.broadcastBallot(ctx, bl)
	}, args.IntervalBroadcastBallot())
	t.NoError(st.bbt.Start(context.Background()))

	defer st.bbt.Stop()

	avp, ivp := t.VoteproofsPair(point.PrevHeight(), point, nil, nil, nil, nodes)
	t.True(st.setLastVoteproof(avp))
	t.True(st.setLastVoteproof(ivp))

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		ballotch <- bl

		return nil
	})

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	t.Run("intended wrong accept ballot", func() {
		select {
		case <-time.After(time.Second * 2):
			t.Fail("timeout to wait next round init ballot")

			return
		case bl := <-ballotch:
			t.NoError(bl.IsValid(t.LocalParams.NetworkID()))
			abl, ok := bl.(base.ACCEPTBallot)
			t.True(ok)

			t.Equal(ivp.Point().Point, abl.Point().Point)
		}
	})
}

func (t *testConsensusHandler) TestInvalidVoteproofs() {
	point := base.RawPoint(22, 0)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	t.Run("empty init voteproof", func() {
		st, closef, _, _ := t.newStateWithINITVoteproof(point, suf)
		defer closef()

		sctx, _ := newConsensusSwitchContext(StateJoining, nil)

		deferred, err := st.enter(StateJoining, sctx)
		t.Nil(deferred)
		t.Error(err)
		t.ErrorContains(err, "empty voteproof")
	})

	t.Run("draw result of init voteproof", func() {
		st, closef, _, _ := t.newStateWithINITVoteproof(point, suf)
		defer closef()

		point := base.RawPoint(33, 0)
		avp, ivp := t.VoteproofsPair(point.PrevHeight(), point, nil, nil, nil, nodes)
		ivp.SetResult(base.VoteResultDraw).Finish()
		st.setLastVoteproof(avp)
		st.setLastVoteproof(ivp)

		sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(StateJoining, sctx)
		t.NotNil(deferred)
		t.NoError(err)
	})

	t.Run("empty majority of init voteproof", func() {
		st, closef, _, _ := t.newStateWithINITVoteproof(point, suf)
		defer closef()

		point := base.RawPoint(33, 0)
		avp, ivp := t.VoteproofsPair(point.PrevHeight(), point, nil, nil, nil, nodes)
		ivp.SetMajority(nil).Finish()
		st.setLastVoteproof(avp)
		st.setLastVoteproof(ivp)

		sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(StateJoining, sctx)
		t.NotNil(deferred)
		t.NoError(err)
	})
}

func (t *testConsensusHandler) TestExit() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		ballotch <- bl

		return nil
	})

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferredenter, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferredenter()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait accept ballot")

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.LocalParams.NetworkID()))

		abl, ok := bl.(base.ACCEPTBallot)
		t.True(ok)

		t.Equal(ivp.Point().Point, abl.Point().Point)
		t.True(ivp.BallotMajority().Proposal().Equal(abl.BallotSignFact().BallotFact().Proposal()))
	}

	t.NotNil(st.args.ProposalProcessors.Processor())

	deferredexit, err := st.exit(nil)
	t.NoError(err)
	t.NotNil(deferredexit)

	t.Nil(st.args.ProposalProcessors.Processor())
}

func (t *testConsensusHandler) TestProcessingProposalAfterEntered() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		ballotch <- bl

		return nil
	})

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait accept ballot")

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.LocalParams.NetworkID()))

		abl, ok := bl.(base.ACCEPTBallot)
		t.True(ok)

		t.Equal(ivp.Point().Point, abl.Point().Point)
		t.True(ivp.BallotMajority().Proposal().Equal(abl.BallotSignFact().BallotFact().Proposal()))
	}
}

func (t *testConsensusHandler) TestEnterExpelINITVoteproof() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(2, t.Local)

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	st, closefunc, _, origivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	prpool := t.PRPool
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return prpool.Get(p), nil
		}
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		ballotch <- bl

		return nil
	})

	ivp := origivp.(isaac.INITVoteproof)
	_ = ivp.SetMajority(nil).Finish()

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait accept ballot")

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.LocalParams.NetworkID()))

		abl, ok := bl.(base.INITBallot)
		t.True(ok)

		t.Equal(ivp.Point().Point.NextRound(), abl.Point().Point)
	}
}

func (t *testConsensusHandler) TestEnterExpelACCEPTVoteproof() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	st, closefunc, _, _ := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	prpool := t.PRPool
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return prpool.Get(p), nil
		}
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		ballotch <- bl

		return nil
	})

	avp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, nil, nil, nodes)
	_ = avp.SetMajority(nil).Finish()
	t.True(st.setLastVoteproof(avp))

	sctx, _ := newConsensusSwitchContext(StateJoining, avp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait init ballot")

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.LocalParams.NetworkID()))

		abl, ok := bl.(base.INITBallot)
		t.True(ok)

		t.Equal(avp.Point().Point.NextRound(), abl.Point().Point)
	}
}

func (t *testConsensusHandler) TestFailedProcessingProposalProcessingFailed() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(1, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return nil, errors.Errorf("hahaha")
	}

	var i int
	st.args.ProposalProcessors.SetGetProposal(func(_ context.Context, _ base.Point, facthash util.Hash) (base.ProposalSignFact, error) {
		if i < 1 {
			i++
			return nil, errors.Errorf("findme")
		}

		return t.PRPool.ByHash(facthash)
	})

	sctxch := make(chan switchContext, 1)
	st.switchStateFunc = func(sctx switchContext) error {
		sctxch <- sctx
		return nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait switch context")

		return
	case sctx := <-sctxch:
		t.Equal(StateBroken, sctx.next())
		t.ErrorContains(sctx, "hahaha")
	}
}

func (t *testConsensusHandler) TestProcessingProposalWithACCEPTVoteproof() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	avp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), t.PRPool.Hash(point), nil, nodes)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		st.setLastVoteproof(avp)

		return manifest, nil
	}

	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp
		return nil, nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save block")

		return
	case ravp := <-savedch:
		base.EqualVoteproof(t.Assert(), avp, ravp)
	}
}

func (t *testConsensusHandler) TestProcessingProposalExpelACCEPTVoteproof() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	avp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), nil, nil, nodes)
	avp.SetResult(base.VoteResultDraw).Finish()

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		st.setLastVoteproof(avp)

		return manifest, nil
	}

	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp
		return nil, nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
	case <-savedch:
		t.Fail("to save block should be ignored")
	}

	t.Nil(st.args.ProposalProcessors.Processor())
}

func (t *testConsensusHandler) TestProcessingProposalWithWrongNewBlockACCEPTVoteproof() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	avp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, nil, nil, nodes) // random new block hash

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		st.setLastVoteproof(avp)

		return manifest, nil
	}

	sctxch := make(chan switchContext, 1)
	st.switchStateFunc = func(sctx switchContext) error {
		sctxch <- sctx

		return nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)
	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait to switch syncing state")
	case err := <-sctxch:
		var ssctx SyncingSwitchContext
		t.ErrorAs(err, &ssctx)

		t.Equal(avp.Point().Height(), ssctx.height)

		t.Nil(st.args.ProposalProcessors.Processor())
	}
}

func (t *testConsensusHandler) TestWithBallotbox() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(0, t.Local)

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)
	t.LocalParams.SetThreshold(base.Threshold(100))

	box := NewBallotbox(
		t.Local.Address(),
		func() base.Threshold { return t.LocalParams.Threshold() },
		func(base.Height) (base.Suffrage, bool, error) {
			return suf, true, nil
		},
	)

	testctx, testdone := context.WithCancel(context.Background())

	st, closef, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer func() {
		testdone()

		closef()
	}()

	manifests := util.NewSingleLockedMap[base.Height, base.Manifest]()
	getmanifest := func(height base.Height) base.Manifest {
		var m base.Manifest

		_ = manifests.GetOrCreate(
			height,
			func(i base.Manifest, _ bool) error {
				m = i

				return nil
			},
			func() (base.Manifest, error) {
				manifest := base.NewDummyManifest(height, valuehash.RandomSHA256())

				return manifest, nil
			},
		)

		return m
	}

	processdelay := time.Millisecond * 100
	pp.Processerr = func(_ context.Context, fact base.ProposalFact, _ base.INITVoteproof) (base.Manifest, error) {
		<-time.After(processdelay)

		return getmanifest(fact.Point().Height()), nil
	}

	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp

		return nil, nil
	}

	st.args.VoteFunc = func(bl base.Ballot) (bool, error) {
		voted, err := box.Vote(bl)
		if err != nil {
			return false, errors.WithStack(err)
		}

		return voted, nil
	}

	prpool := t.PRPool
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		var pr base.ProposalSignFact

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-testctx.Done():
			return nil, testctx.Err()
		default:
			pr = prpool.Get(p)

		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-testctx.Done():
			return nil, testctx.Err()
		default:
			return pr, nil
		}
	}

	go func() {
	end:
		for {
			select {
			case <-testctx.Done():
				break end
			case vp := <-box.Voteproof():
				_ = st.newVoteproof(vp)
			}
		}
	}()

	target := point
	for range make([]struct{}, 33) {
		target = target.NextHeight()
	}

	wait := processdelay * time.Duration((target.Height()-point.Height()).Int64()*10)
	after := time.After(wait)
	t.T().Logf("> trying to create blocks up to %q; will wait %q", target, wait)

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

end:
	for {
		select {
		case <-after:
			t.Fail("failed to wait new blocks")
		case avp := <-savedch:
			t.T().Logf("new block saved: %q", avp.Point())

			if avp.Point().Point.Equal(target) {
				t.T().Logf("< all new blocks saved, %q", target)

				break end
			}
		}
	}
}

func (t *testConsensusHandler) TestEmptySuffrageNextBlock() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	st.args.NodeInConsensusNodesFunc = func(_ base.Node, height base.Height) (base.Suffrage, bool, error) {
		switch {
		case height <= point.Height().SafePrev():
			return suf, true, nil
		default:
			return nil, false, nil
		}
	}

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}
	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp
		return nil, nil
	}

	prpool := t.PRPool
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return prpool.Get(p), nil
		}
	}

	sctxch := make(chan switchContext, 1)
	st.switchStateFunc = func(sctx switchContext) error {
		sctxch <- sctx

		return nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	avp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), t.PRPool.Hash(point), t.PRPool.Hash(point.NextHeight()), nodes)
	t.NoError(st.newVoteproof(avp))

	t.T().Log("wait new block saved")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case <-savedch:
	}

	t.T().Log("wait to switch syncing state")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait")

		return
	case sctx := <-sctxch:
		var ssctx SyncingSwitchContext
		t.ErrorAs(sctx, &ssctx)
		t.Equal(point.Height(), ssctx.height)
	}
}

func (t *testConsensusHandler) TestOutOfSuffrage() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)
	newsuf, _ := isaac.NewTestSuffrage(2)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	st.args.NodeInConsensusNodesFunc = func(_ base.Node, height base.Height) (base.Suffrage, bool, error) {
		if height == point.Height().SafePrev() {
			return suf, true, nil
		}

		return newsuf, newsuf.ExistsPublickey(t.Local.Address(), t.Local.Publickey()), nil
	}

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}
	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp
		return nil, nil
	}

	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return t.PRPool.Get(p), nil
		}
	}

	sctxch := make(chan switchContext, 1)
	st.switchStateFunc = func(sctx switchContext) error {
		sctxch <- sctx

		return nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	avp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), t.PRPool.Hash(point), t.PRPool.Hash(point.NextHeight()), nodes)
	t.NoError(st.newVoteproof(avp), "%+v", err)

	t.T().Log("wait new block saved")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case <-savedch:
	}

	t.T().Log("wait to switch syncing state")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait to switch state syncing")

		return
	case sctx := <-sctxch:
		var ssctx SyncingSwitchContext
		t.ErrorAs(sctx, &ssctx)
		t.Equal(point.Height(), ssctx.height)
	}
}

func (t *testConsensusHandler) TestEnterButEmptySuffrage() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(2)

	st, closefunc, _, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()
	st.args.NodeInConsensusNodesFunc = func(base.Node, base.Height) (base.Suffrage, bool, error) {
		return nil, false, nil
	}

	sctxch := make(chan switchContext, 1)
	st.switchStateFunc = func(sctx switchContext) error {
		sctxch <- sctx

		return nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	_, err := st.enter(StateJoining, sctx)
	t.Error(err)
	t.ErrorContains(err, "empty suffrage of init voteproof")
}

func (t *testConsensusHandler) TestEnterButNotInSuffrage() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(2)

	st, closefunc, _, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	sctxch := make(chan switchContext, 1)
	st.switchStateFunc = func(sctx switchContext) error {
		sctxch <- sctx

		return nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	_, err := st.enter(StateJoining, sctx)

	var ssctx SyncingSwitchContext
	t.ErrorAs(err, &ssctx)
	t.Equal(point.Height().SafePrev(), ssctx.height)
}

func (t *testConsensusHandler) TestNewVoteproofButNotAllowConsensus() {
	t.Run("from voteproof", func() {
		point := base.RawPoint(33, 44)
		suf, nodes := isaac.NewTestSuffrage(2, t.Local)

		st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
		defer closefunc()

		st.args.NodeInConsensusNodesFunc = func(base.Node, base.Height) (base.Suffrage, bool, error) {
			return suf, true, nil
		}

		manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
		pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
			return manifest, nil
		}

		prpool := t.PRPool
		st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return prpool.Get(p), nil
			}
		}

		sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(StateJoining, sctx)
		t.NoError(err)
		deferred()

		avp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), t.PRPool.Hash(point), t.PRPool.Hash(point.NextHeight()), nodes)

		t.True(st.allowedConsensus())
		st.setAllowConsensus(false)

		err = st.newVoteproof(avp)

		var ssctx SyncingSwitchContext
		t.ErrorAs(err, &ssctx)
		t.Equal(ssctx.next(), StateSyncing)
		t.Equal(point.Height(), ssctx.height)
	})

	t.Run("from channel", func() {
		point := base.RawPoint(33, 44)
		suf, _ := isaac.NewTestSuffrage(2, t.Local)

		st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
		defer closefunc()

		st.args.NodeInConsensusNodesFunc = func(base.Node, base.Height) (base.Suffrage, bool, error) {
			return suf, true, nil
		}

		sctxch := make(chan switchContext, 1)
		st.switchStateFunc = func(sctx switchContext) error {
			sctxch <- sctx

			return nil
		}

		manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
		pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
			return manifest, nil
		}

		prpool := t.PRPool
		st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return prpool.Get(p), nil
			}
		}

		sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(StateJoining, sctx)
		t.NoError(err)
		deferred()

		t.True(st.allowedConsensus())

		st.setAllowConsensus(false)

		select {
		case <-time.After(time.Second * 2):
			t.Fail("timeout to wait switch context")

			return
		case sctx := <-sctxch:
			var ssctx SyncingSwitchContext
			t.ErrorAs(sctx, &ssctx)
			t.Equal(ssctx.next(), StateSyncing)
			t.Equal(base.GenesisHeight, ssctx.height)
		}
	})
}

func (t *testConsensusHandler) TestSendBallotForHandoverBrokerX() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()
	st.SetLogging(logging.TestNilLogging)

	sendch := make(chan base.Ballot, 1)

	connInfo := quicstream.RandomConnInfo()
	brokerargs := NewHandoverXBrokerArgs(t.Local, t.LocalParams.NetworkID())
	brokerargs.SendMessageFunc = func(_ context.Context, _ quicstream.ConnInfo, msg HandoverMessage) error {
		switch md, ok := msg.(HandoverMessageData); {
		case !ok:
			return nil
		case md.DataType() != HandoverMessageDataTypeBallot:
		default:
			ballot, ok := md.Data().(base.Ballot)
			if !ok {
				return nil
			}

			if point.NextHeight().Equal(ballot.Point().Point) {
				sendch <- ballot
			}
		}

		return nil
	}

	broker := NewHandoverXBroker(context.Background(), brokerargs, connInfo)
	st.handoverXBrokerFunc = func() *HandoverXBroker {
		return broker
	}

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}

	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp
		return nil, nil
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		if p := bl.Point(); p.Point.Equal(point.NextHeight()) && p.Stage() == base.StageACCEPT {
			ballotch <- bl
		}

		return nil
	})

	prpool := t.PRPool
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return prpool.Get(p), nil
		}
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	nextavp, nextivp := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), t.PRPool.Hash(point), t.PRPool.Hash(point.NextHeight()), nodes)
	t.NoError(st.newVoteproof(nextavp))

	t.T().Log("wait new block saved")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case <-savedch:
	}

	t.T().Log("new init voteproof")

	t.NoError(st.newVoteproof(nextivp))

	t.T().Log("wait next accept ballot")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait next accept ballot")

		return
	case bl := <-ballotch:
		t.Equal(point.NextHeight(), bl.Point().Point)
	}

	t.T().Log("wait to send ballot for handover x")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait next accept ballot")

		return
	case bl := <-sendch:
		t.Equal(point.NextHeight(), bl.Point().Point)
	}
}

func (t *testConsensusHandler) TestEmptyProposalINITUnderEmptyProposalNoBlock() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()
	st.SetLogging(logging.TestNilLogging)

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		return base.NewDummyBlockMap(base.NewDummyManifest(avp.Point().Height(), avp.BallotMajority().NewBlock())), nil
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		if bl.Point().Point.Equal(point.NextHeight()) {
			ballotch <- bl
		}

		return nil
	})

	newblocksavedch := make(chan base.Height, 1)
	st.args.WhenNewBlockSaved = func(bm base.BlockMap) {
		newblocksavedch <- bm.Manifest().Height()
	}

	st.args.IsEmptyProposalNoBlockFunc = func() bool {
		return true
	}
	st.args.IsEmptyProposalFunc = func(context.Context, base.ProposalSignFact) (bool, error) {
		return true, nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	_ = t.PRPool.Get(point.NextHeight())

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait saved callback")

		return
	case height := <-newblocksavedch:
		t.Equal(nextavp.Point().Height(), height)
	}

	t.T().Log("wait next empty prooposal init ballot")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait next init ballot")

		return
	case bl := <-ballotch:
		t.Equal(point.NextHeight(), bl.Point().Point)

		rbl, ok := bl.(base.INITBallot)
		t.True(ok)

		rfact := rbl.BallotSignFact().BallotFact()
		_, ok = rfact.(isaac.EmptyProposalINITBallotFact)
		t.True(ok)
	}
}

func (t *testConsensusHandler) TestEmptyProposalINITUnderEmptyProposalNoBlockButExpels() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()
	st.SetLogging(logging.TestNilLogging)

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		return base.NewDummyBlockMap(base.NewDummyManifest(avp.Point().Height(), avp.BallotMajority().NewBlock())), nil
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		if bl.Point().Point.Equal(point.NextHeight()) {
			ballotch <- bl
		}

		return nil
	})

	newblocksavedch := make(chan base.Height, 1)
	st.args.WhenNewBlockSaved = func(bm base.BlockMap) {
		newblocksavedch <- bm.Manifest().Height()
	}

	st.args.IsEmptyProposalNoBlockFunc = func() bool {
		return true
	}
	st.args.IsEmptyProposalFunc = func(context.Context, base.ProposalSignFact) (bool, error) {
		return true, nil
	}
	expels := t.Expels(point.NextHeight().Height(), []base.Address{nodes[0].Address()}, nodes[1:])
	st.args.SuffrageVotingFindFunc = func(context.Context, base.Height, base.Suffrage) (
		[]base.SuffrageExpelOperation, error,
	) {
		return expels, nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	_ = t.PRPool.Get(point.NextHeight())

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait saved callback")

		return
	case height := <-newblocksavedch:
		t.Equal(nextavp.Point().Height(), height)
	}

	t.T().Log("wait next init ballot")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait next init ballot")

		return
	case bl := <-ballotch:
		t.Equal(point.NextHeight(), bl.Point().Point)

		rbl, ok := bl.(base.INITBallot)
		t.True(ok)

		rfact := rbl.BallotSignFact().BallotFact()
		_, ok = rfact.(isaac.INITBallotFact)
		t.True(ok)
	}
}

func (t *testConsensusHandler) TestEmptyProposalINITUnderEmptyProposalNoBlockButNotEmpty() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()
	st.SetLogging(logging.TestNilLogging)

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		return base.NewDummyBlockMap(base.NewDummyManifest(avp.Point().Height(), avp.BallotMajority().NewBlock())), nil
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		if bl.Point().Point.Equal(point.NextHeight()) {
			ballotch <- bl
		}

		return nil
	})

	newblocksavedch := make(chan base.Height, 1)
	st.args.WhenNewBlockSaved = func(bm base.BlockMap) {
		newblocksavedch <- bm.Manifest().Height()
	}

	st.args.IsEmptyProposalNoBlockFunc = func() bool {
		return true
	}
	st.args.IsEmptyProposalFunc = func(context.Context, base.ProposalSignFact) (bool, error) {
		return false, nil // <-- not empty
	}
	expels := t.Expels(point.NextHeight().Height(), []base.Address{nodes[0].Address()}, nodes[1:])
	st.args.SuffrageVotingFindFunc = func(context.Context, base.Height, base.Suffrage) (
		[]base.SuffrageExpelOperation, error,
	) {
		return expels, nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	_ = t.PRPool.Get(point.NextHeight())

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait saved callback")

		return
	case height := <-newblocksavedch:
		t.Equal(nextavp.Point().Height(), height)
	}

	t.T().Log("wait next init ballot")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait next init ballot")

		return
	case bl := <-ballotch:
		t.Equal(point.NextHeight(), bl.Point().Point)

		rbl, ok := bl.(base.INITBallot)
		t.True(ok)

		rfact := rbl.BallotSignFact().BallotFact()
		_, ok = rfact.(isaac.INITBallotFact)
		t.True(ok)
	}
}

func (t *testConsensusHandler) TestProposalProcessorEmptyOperations() {
	point := base.RawPoint(33, 44)
	suf, _ := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return nil, isaac.ErrProposalProcessorEmptyOperations.Errorf("showme")
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		ballotch <- bl

		return nil
	})

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait accept ballot")

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.LocalParams.NetworkID()))

		abl, ok := bl.(base.ACCEPTBallot)
		t.True(ok)

		_, ok = abl.BallotSignFact().Fact().(isaac.EmptyOperationsACCEPTBallotFact)
		t.True(ok)

		t.Equal(ivp.Point().Point, abl.Point().Point)
		t.True(ivp.BallotMajority().Proposal().Equal(abl.BallotSignFact().BallotFact().Proposal()))
	}
}

func TestConsensusHandler(t *testing.T) {
	suite.Run(t, new(testConsensusHandler))
}
