package states

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/logging"
	"github.com/spikeekips/mitum/util/valuehash"
	"github.com/stretchr/testify/suite"
)

type testConsensusHandler struct {
	baseTestStateHandler
}

func (t *testConsensusHandler) newState() (*ConsensusHandler, func()) {
	st := NewConsensusHandler(
		t.local,
		t.policy,
		nil,
		func(base.Height) base.Suffrage {
			return nil
		},
		newProposalProcessors(nil, nil),
	)
	_ = st.SetLogging(logging.TestNilLogging)
	_ = st.setTimers(util.NewTimers([]util.TimerID{
		timerIDBroadcastINITBallot,
		timerIDPrepareProposal,
	}, false))

	return st, func() {
		deferred, err := st.exit()
		t.NoError(err)
		t.NoError(deferred())
	}
}

func (t *testConsensusHandler) voteproofsPair(prevpoint, point base.Point, pr, nextpr util.Hash, nodes []*LocalNode) (ACCEPTVoteproof, INITVoteproof) {
	if pr == nil {
		pr = valuehash.RandomSHA256()
	}
	if nextpr == nil {
		nextpr = valuehash.RandomSHA256()
	}

	newblock := valuehash.RandomSHA256()

	afact := t.newACCEPTBallotFact(prevpoint, pr, newblock)
	avp, err := t.newACCEPTVoteproof(afact, t.local, nodes)
	t.NoError(err)

	ifact := t.newINITBallotFact(point, newblock, nextpr)
	ivp, err := t.newINITVoteproof(ifact, t.local, nodes)
	t.NoError(err)

	return avp, ivp
}

func (t *testConsensusHandler) TestNew() {
	nodes := t.nodes(3)

	st := NewConsensusHandler(
		t.local,
		t.policy,
		nil,
		func(base.Height) base.Suffrage {
			return nil
		},
		newProposalProcessors(nil, func(context.Context, util.Hash) (base.ProposalFact, error) {
			return nil, util.NotFoundError.Call()
		}),
	)
	_ = st.SetLogging(logging.TestNilLogging)

	defer func() {
		deferred, err := st.exit()
		t.NoError(err)
		t.NoError(deferred())
	}()

	st.switchStateFunc = func(stateSwitchContext) {}

	_ = st.setTimers(util.NewTimers([]util.TimerID{
		timerIDBroadcastINITBallot,
		timerIDPrepareProposal,
	}, false))

	point := base.NewPoint(base.Height(33), base.Round(0))
	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, nodes)

	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())
}

func (t *testConsensusHandler) TestInvalidVoteproofs() {
	nodes := t.nodes(3)

	st := NewConsensusHandler(
		t.local,
		t.policy,
		nil,
		func(base.Height) base.Suffrage {
			return nil
		},
		newProposalProcessors(nil, nil),
	)
	_ = st.setTimers(util.NewTimers(nil, true))

	defer func() {
		deferred, err := st.exit()
		t.NoError(err)
		t.NoError(deferred())
	}()

	t.Run("empty init voteproof", func() {
		sctx := newConsensusSwitchContext(StateJoining, nil)

		deferred, err := st.enter(sctx)
		t.Nil(deferred)
		t.Error(err)
		t.Contains(err.Error(), "empty init voteproof")
	})

	t.Run("draw result of init voteproof", func() {
		point := base.NewPoint(base.Height(33), base.Round(0))
		_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, nodes)
		ivp.SetResult(base.VoteResultDraw)

		sctx := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(sctx)
		t.NotNil(deferred)
		t.NoError(err)
	})

	t.Run("empty majority of init voteproof", func() {
		point := base.NewPoint(base.Height(33), base.Round(0))
		_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, nodes)
		ivp.SetMajority(nil)

		sctx := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(sctx)
		t.NotNil(deferred)
		t.NoError(err)
	})
}

func (t *testConsensusHandler) TestExit() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	ballotch := make(chan base.Ballot, 1)
	st.broadcastBallotFunc = func(bl base.Ballot, tolocal bool) error {
		ballotch <- bl

		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferredenter, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferredenter())

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait accept ballot"))

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.policy.NetworkID()))

		abl, ok := bl.(base.ACCEPTBallot)
		t.True(ok)

		t.Equal(ivp.Point().Point, abl.Point().Point)
		t.True(ivp.BallotMajority().Proposal().Equal(abl.BallotSignedFact().BallotFact().Proposal()))
	}

	t.NotNil(st.pps.p)

	deferredexit, err := st.exit()
	t.NoError(err)
	t.NotNil(deferredexit)

	t.Nil(st.pps.p)
}

func (t *testConsensusHandler) TestProcessingProposalAfterEntered() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	ballotch := make(chan base.Ballot, 1)
	st.broadcastBallotFunc = func(bl base.Ballot, tolocal bool) error {
		ballotch <- bl

		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait accept ballot"))

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.policy.NetworkID()))

		abl, ok := bl.(base.ACCEPTBallot)
		t.True(ok)

		t.Equal(ivp.Point().Point, abl.Point().Point)
		t.True(ivp.BallotMajority().Proposal().Equal(abl.BallotSignedFact().BallotFact().Proposal()))
	}
}

func (t *testConsensusHandler) TestFailedProcessingProposalFetchFactFailed() {
	nodes := t.nodes(2)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		return nil, util.NotFoundError.Errorf("fact not found")
	}

	sctxch := make(chan stateSwitchContext, 1)
	st.switchStateFunc = func(sctx stateSwitchContext) {
		sctxch <- sctx
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait switch context"))

		return
	case sctx := <-sctxch:
		t.Equal(StateConsensus, sctx.from())
		t.Equal(StateBroken, sctx.next())
		t.Contains(sctx.Error(), "failed to get proposal fact")
	}
}

func (t *testConsensusHandler) TestFailedProcessingProposalProcessingFailed() {
	nodes := t.nodes(2)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)
	pp.processerr = func() error {
		return errors.Errorf("hahaha")
	}

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make

	var i int
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if i < 1 {
			i++
			return nil, RetryProposalProcessorError.Errorf("findme")
		}

		return fact, nil
	}

	sctxch := make(chan stateSwitchContext, 1)
	st.switchStateFunc = func(sctx stateSwitchContext) {
		sctxch <- sctx
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait switch context"))

		return
	case sctx := <-sctxch:
		t.Equal(StateConsensus, sctx.from())
		t.Equal(StateBroken, sctx.next())
		t.Contains(sctx.Error(), "hahaha")
	}
}

func (t *testConsensusHandler) TestFailedProcessingProposalProcessingFailedRetry() {
	nodes := t.nodes(2)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())

	var c int64
	pp := NewDummyProposalProcessor(manifest)
	pp.processerr = func() error {
		if i := atomic.LoadInt64(&c); i < 1 {
			atomic.AddInt64(&c, 1)

			return RetryProposalProcessorError.Errorf("findme")
		}

		return errors.Errorf("hahaha")
	}

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make

	var g int64
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if i := atomic.LoadInt64(&g); i < 1 {
			atomic.AddInt64(&g, 1)

			return nil, RetryProposalProcessorError.Errorf("findme")
		}

		return fact, nil
	}

	sctxch := make(chan stateSwitchContext, 1)
	st.switchStateFunc = func(sctx stateSwitchContext) {
		sctxch <- sctx
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	select {
	case <-time.After(time.Second * 5):
		t.NoError(errors.Errorf("timeout to wait switch context"))

		return
	case sctx := <-sctxch:
		t.Equal(StateConsensus, sctx.from())
		t.Equal(StateBroken, sctx.next())
		t.Contains(sctx.Error(), "hahaha")
	}
}

func (t *testConsensusHandler) TestExpectedACCEPTVoteproof() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.saveerr = func(avp base.ACCEPTVoteproof) error {
		savedch <- avp
		return nil
	}

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	st.broadcastBallotFunc = func(bl base.Ballot, tolocal bool) error {
		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	nextavp, _ := t.voteproofsPair(point, point.Next(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait save proposal processor"))

		return
	case avp := <-savedch:
		base.CompareVoteproof(t.Assert(), nextavp, avp)
		base.CompareVoteproof(t.Assert(), avp, st.lastACCEPTVoteproof())
	}
}

func (t *testConsensusHandler) TestNotExpectedACCEPTVoteproofOldVoteproof() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	st.broadcastBallotFunc = func(bl base.Ballot, tolocal bool) error {
		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	nextavp, _ := t.voteproofsPair(point.Decrease(), point.Next(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))
	t.Nil(st.lastACCEPTVoteproof())
}

func (t *testConsensusHandler) TestNotExpectedACCEPTVoteproofHigerVoteproof() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	st.broadcastBallotFunc = func(bl base.Ballot, tolocal bool) error {
		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	nextavp, _ := t.voteproofsPair(point.Next(), point.Next(), fact.Hash(), nil, nodes)
	err = st.newVoteproof(nextavp)
	t.Error(err)
	t.Nil(st.lastACCEPTVoteproof())

	var nsctx syncingSwitchContext
	t.True(errors.As(err, &nsctx))
}

func (t *testConsensusHandler) TestExpectedACCEPTVoteproofButDraw() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()
	st.SetLogging(logging.TestLogging)

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	pr := t.newProposal(t.local, NewProposalFact(point.NextRound(), nil))
	st.proposalSelector = DummyProposalSelector(func(point base.Point) (base.Proposal, error) {
		return pr, nil
	})

	ballotch := make(chan base.Ballot, 1)
	st.broadcastBallotFunc = func(bl base.Ballot, tolocal bool) error {
		if bl.Point().Point.Compare(point) < 1 {
			return nil
		}

		ballotch <- bl

		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	nextavp, _ := t.voteproofsPair(point, point.Next(), fact.Hash(), nil, nodes)
	nextavp.SetMajority(nil)
	nextavp.SetResult(base.VoteResultDraw)

	t.NoError(st.newVoteproof(nextavp))
	t.Nil(st.lastACCEPTVoteproof())

	t.T().Log("will wait init ballot of next round")

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait next round init ballot"))

		return
	case bl := <-ballotch:
		t.Equal(point.NextRound(), bl.Point().Point)

		rbl, ok := bl.(base.INITBallot)
		t.True(ok)

		rfact := rbl.BallotSignedFact().BallotFact()
		t.Equal(ivp.BallotMajority().PreviousBlock(), rfact.PreviousBlock())
		t.True(pr.SignedFact().Fact().Hash().Equal(rfact.Proposal()))
	}
}

func (t *testConsensusHandler) TestExpectedDrawACCEPTVoteproofFailedProposalSelector() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, nil)

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	st.proposalSelector = DummyProposalSelector(func(point base.Point) (base.Proposal, error) {
		return nil, errors.Errorf("hahaha")
	})

	st.broadcastBallotFunc = func(bl base.Ballot, tolocal bool) error {
		return nil
	}

	sctxch := make(chan stateSwitchContext, 1)
	st.switchStateFunc = func(sctx stateSwitchContext) {
		sctxch <- sctx
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, fact.Hash(), nodes)
	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	nextavp, _ := t.voteproofsPair(point, point.Next(), fact.Hash(), nil, nodes)
	nextavp.SetMajority(nil)
	nextavp.SetResult(base.VoteResultDraw)

	t.NoError(st.newVoteproof(nextavp))
	t.Nil(st.lastACCEPTVoteproof())

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait switch context"))

		return
	case nsctx := <-sctxch:
		var bsctx brokenSwitchContext
		t.True(errors.As(nsctx, &bsctx))
		t.Contains(bsctx.Error(), "hahaha")
	}
}

// BLOCK test expected accept voteproof and saved, moves to next block
// BLOCK test expected accept voteproof, but NotProposalProcessorProcessedError, mvoes to syncing state
// BLOCK test expected accept voteproof, but failed, mvoes to broken state

func TestConsensusHandler(t *testing.T) {
	suite.Run(t, new(testConsensusHandler))
}