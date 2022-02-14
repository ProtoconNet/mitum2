package isaac

import (
	"context"
	"sync"
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

type baseTestConsensusHandler struct {
	baseTestStateHandler
	prpool *proposalPool
}

func (t *baseTestConsensusHandler) SetupTest() {
	t.baseTestStateHandler.SetupTest()

	t.prpool = newProposalPool(func(p base.Point) base.Proposal {
		return t.newProposal(t.local, t.newProposalFact(p, []util.Hash{valuehash.RandomSHA256()}))
	})
}

func (t *baseTestConsensusHandler) newState() (*ConsensusHandler, func()) {
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
		timerIDBroadcastACCEPTBallot,
		timerIDPrepareProposal,
	}, false))

	return st, func() {
		deferred, err := st.exit()
		t.NoError(err)
		t.NoError(deferred())
	}
}

func (t *baseTestConsensusHandler) voteproofsPair(prevpoint, point base.Point, prev, pr, nextpr util.Hash, nodes []*LocalNode) (ACCEPTVoteproof, INITVoteproof) {
	if prev == nil {
		prev = valuehash.RandomSHA256()
	}
	if pr == nil {
		pr = valuehash.RandomSHA256()
	}
	if nextpr == nil {
		nextpr = valuehash.RandomSHA256()
	}

	afact := t.newACCEPTBallotFact(prevpoint, pr, prev)
	avp, err := t.newACCEPTVoteproof(afact, t.local, nodes)
	t.NoError(err)

	ifact := t.newINITBallotFact(point, prev, nextpr)
	ivp, err := t.newINITVoteproof(ifact, t.local, nodes)
	t.NoError(err)

	return avp, ivp
}

type testConsensusHandler struct {
	baseTestConsensusHandler
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

	st.switchStateFunc = func(stateSwitchContext) error { return nil }

	_ = st.setTimers(util.NewTimers([]util.TimerID{
		timerIDBroadcastINITBallot,
		timerIDBroadcastACCEPTBallot,
		timerIDPrepareProposal,
	}, false))

	point := base.NewPoint(base.Height(33), base.Round(0))
	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, nil, nodes)

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
		_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, nil, nodes)
		ivp.SetResult(base.VoteResultDraw)
		ivp.finish()

		sctx := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(sctx)
		t.Nil(deferred)
		t.Error(err)
		t.Contains(err.Error(), "wrong vote result")
	})

	t.Run("empty majority of init voteproof", func() {
		point := base.NewPoint(base.Height(33), base.Round(0))
		_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, nil, nodes)
		ivp.SetMajority(nil)
		ivp.finish()

		sctx := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(sctx)
		t.NotNil(deferred)
		t.NoError(err)
	})
}

func (t *testConsensusHandler) TestExit() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

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

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
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
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

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

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
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
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())
	pp := NewDummyProposalProcessor(manifest)

	st, closefunc := t.newState()
	defer closefunc()

	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		return nil, util.NotFoundError.Errorf("fact not found")
	}

	sctxch := make(chan stateSwitchContext, 1)
	st.switchStateFunc = func(sctx stateSwitchContext) error {
		sctxch <- sctx

		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
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
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

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
	st.switchStateFunc = func(sctx stateSwitchContext) error {
		sctxch <- sctx

		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
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
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

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
	st.switchStateFunc = func(sctx stateSwitchContext) error {
		sctxch <- sctx

		return nil
	}

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
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

func (t *testConsensusHandler) TestProcessingProposalWithACCEPTVoteproof() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
	st, closefunc := t.newState()
	defer closefunc()

	pp := NewDummyProposalProcessor(manifest)
	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	avp, _ := t.voteproofsPair(point, point.Next(), manifest.Hash(), nil, nil, nodes)
	pp.processerr = func() error {
		st.lavp.SetValue(avp)

		return nil
	}

	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.saveerr = func(avp base.ACCEPTVoteproof) error {
		savedch <- avp
		return nil
	}

	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait save block"))

		return
	case ravp := <-savedch:
		base.CompareVoteproof(t.Assert(), avp, ravp)
	}
}

func (t *testConsensusHandler) TestProcessingProposalWithDrawACCEPTVoteproof() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
	st, closefunc := t.newState()
	defer closefunc()

	pp := NewDummyProposalProcessor(manifest)
	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	avp, _ := t.voteproofsPair(point, point.Next(), manifest.Hash(), nil, nil, nodes)
	avp.SetMajority(nil)
	avp.SetResult(base.VoteResultDraw)
	avp.finish()

	pp.processerr = func() error {
		st.lavp.SetValue(avp)

		return nil
	}

	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.saveerr = func(avp base.ACCEPTVoteproof) error {
		savedch <- avp
		return nil
	}

	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	select {
	case <-time.After(time.Second * 2):
	case <-savedch:
		t.NoError(errors.Errorf("to save block should be ignored"))
	}

	t.True(st.pps.isClose())
}

func (t *testConsensusHandler) TestProcessingProposalWithWrongNewBlockACCEPTVoteproof() {
	nodes := t.nodes(3)

	point := base.NewPoint(base.Height(33), base.Round(44))
	fact := t.newProposalFact(point, []util.Hash{valuehash.RandomSHA256()})

	manifest := base.NewDummyManifest(fact.Point().Height(), valuehash.RandomSHA256())

	_, ivp := t.voteproofsPair(point.Decrease(), point, nil, nil, fact.Hash(), nodes)
	st, closefunc := t.newState()
	defer closefunc()

	pp := NewDummyProposalProcessor(manifest)
	st.pps.makenew = pp.make
	st.pps.getfact = func(_ context.Context, facthash util.Hash) (base.ProposalFact, error) {
		if facthash.Equal(fact.Hash()) {
			return fact, nil
		}

		return nil, util.NotFoundError.Errorf("fact not found")
	}

	avp, _ := t.voteproofsPair(point, point.Next(), nil, nil, nil, nodes) // random new block hash
	pp.processerr = func() error {
		st.lavp.SetValue(avp)

		return nil
	}

	sctxch := make(chan stateSwitchContext, 1)
	st.switchStateFunc = func(sctx stateSwitchContext) error {
		sctxch <- sctx

		return nil
	}

	sctx := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(sctx)
	t.NoError(err)
	t.NoError(deferred())

	select {
	case <-time.After(time.Second * 2):
		t.NoError(errors.Errorf("timeout to wait to switch syncing state"))
	case nsctx := <-sctxch:
		var ssctx syncingSwitchContext
		t.True(errors.As(nsctx, &ssctx))

		t.Equal(avp.Point().Height(), ssctx.height)
	}

	t.True(st.pps.isClose())
}

func TestConsensusHandler(t *testing.T) {
	suite.Run(t, new(testConsensusHandler))
}

type proposalPool struct { // BLOCK apply
	sync.RWMutex
	p           map[base.Point]base.Proposal
	newproposal func(base.Point) base.Proposal
}

func newProposalPool(
	newproposal func(base.Point) base.Proposal,
) *proposalPool {
	return &proposalPool{
		p:           map[base.Point]base.Proposal{},
		newproposal: newproposal,
	}
}

func (p *proposalPool) hash(point base.Point) util.Hash {
	pr := p.get(point)

	return pr.SignedFact().Fact().Hash()
}

func (p *proposalPool) get(point base.Point) base.Proposal {
	p.Lock()
	defer p.Unlock()

	if pr, found := p.p[point]; found {
		return pr
	}

	pr := p.newproposal(point)

	p.p[point] = pr

	return pr
}

func (p *proposalPool) byHash(h util.Hash) base.Proposal {
	p.RLock()
	defer p.RUnlock()

	for i := range p.p {
		pr := p.p[i]
		if pr.SignedFact().Fact().Hash().Equal(h) {
			return pr
		}

	}

	return nil
}