package isaacstates

import (
	"context"
	"testing"
	"time"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type testNewACCEPTOnINITVoteproofConsensusHandler struct {
	baseTestConsensusHandler
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestExpected() {
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
	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp

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
	confirmedch := make(chan base.Height, 1)
	st.args.WhenNewBlockSaved = func(bm base.BlockMap) {
		newblocksavedch <- bm.Manifest().Height()
	}
	st.args.WhenNewBlockConfirmed = func(height base.Height) {
		confirmedch <- height
	}

	nextpr := t.PRPool.Get(point.NextHeight())

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))

	t.T().Log("wait new block saved")
	var avp base.ACCEPTVoteproof
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case avp = <-savedch:
		base.EqualVoteproof(t.Assert(), nextavp, avp)
		base.EqualVoteproof(t.Assert(), avp, st.lastVoteproofs().ACCEPT())
	}

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait saved callback")

		return
	case height := <-newblocksavedch:
		t.Equal(nextavp.Point().Height(), height)
	}

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait confirmed callback")

		return
	case height := <-confirmedch:
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
		t.Equal(avp.BallotMajority().NewBlock(), rfact.PreviousBlock())
		t.True(nextpr.Fact().Hash().Equal(rfact.Proposal()))
	}
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestNotInConsensus() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()
	st.SetLogging(logging.TestNilLogging)

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
		if bl.Point().Point.Equal(point.NextHeight()) {
			ballotch <- bl
		}

		return nil
	})

	st.args.VoteFunc = func(bl base.Ballot) (bool, error) {
		if bl.Point().Point.Compare(point) == 0 && bl.Point().Stage() == base.StageACCEPT {
			return false, errFailedToVoteNotInConsensus.Errorf("hehehe")
		}

		return true, nil
	}

	sctxch := make(chan switchContext, 1)
	st.switchStateFunc = func(sctx switchContext) error {
		sctxch <- sctx

		return nil
	}

	nextpr := t.PRPool.Get(point.NextHeight())

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))

	t.T().Log("wait new block saved")
	var avp base.ACCEPTVoteproof
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case avp = <-savedch:
		base.EqualVoteproof(t.Assert(), nextavp, avp)
		base.EqualVoteproof(t.Assert(), avp, st.lastVoteproofs().ACCEPT())
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
		t.Equal(avp.BallotMajority().NewBlock(), rfact.PreviousBlock())
		t.True(nextpr.Fact().Hash().Equal(rfact.Proposal()))
	}

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait to switch syncing state")

		return
	case sctx := <-sctxch:
		var ssctx SyncingSwitchContext
		t.ErrorAs(sctx, &ssctx)
		t.Equal(nextavp.Point().Height()-1, ssctx.height)
	}
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestOld() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point.PrevHeight(), point.NextHeight(), nil, fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(nextavp))
	t.Equal(st.lastVoteproofs().ACCEPT().Point().Point, point.PrevHeight())
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestHigher() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point.NextHeight(), point.NextHeight(), nil, fact.Hash(), nil, nodes)
	err = st.newVoteproof(nextavp)
	t.Error(err)
	t.NotNil(st.lastVoteproofs().ACCEPT())

	var nsctx SyncingSwitchContext
	t.ErrorAs(err, &nsctx)
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestDraw() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	nextpr := t.PRPool.Get(point.NextRound())

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		if bl.Point().Point.Compare(point) < 1 {
			return nil
		}

		ballotch <- bl

		return nil
	})

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, fact.Hash(), nil, nodes)
	nextavp.SetResult(base.VoteResultDraw).Finish()

	t.NoError(st.newVoteproof(nextavp))
	t.NotNil(st.lastVoteproofs().ACCEPT())

	t.T().Log("will wait init ballot of next round")

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait next round init ballot")

		return
	case bl := <-ballotch:
		t.Equal(point.NextRound(), bl.Point().Point)

		rbl, ok := bl.(base.INITBallot)
		t.True(ok)

		rfact := rbl.BallotSignFact().BallotFact()
		t.Equal(ivp.BallotMajority().PreviousBlock(), rfact.PreviousBlock())
		t.True(nextpr.Fact().Hash().Equal(rfact.Proposal()))
	}
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestDrawFailedProposalSelectFunc() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}

	st.args.ProposalSelectFunc = func(context.Context, base.Point, util.Hash, time.Duration) (base.ProposalSignFact, error) {
		return nil, errors.Errorf("hahaha")
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

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, fact.Hash(), nil, nodes)
	nextavp.SetResult(base.VoteResultDraw).Finish()

	t.NoError(st.newVoteproof(nextavp))
	t.NotNil(st.lastVoteproofs().ACCEPT())

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait switch context")

		return
	case nsctx := <-sctxch:
		var bsctx baseErrorSwitchContext
		t.ErrorAs(nsctx, &bsctx)
		t.Equal(bsctx.next(), StateBroken)
		t.ErrorContains(bsctx, "hahaha")
	}
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestNotProposalProcessorProcessedError() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}

	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		if avp.Point().Point.Equal(point) {
			return nil, isaac.ErrNotProposalProcessorProcessed.Errorf("hehehe")
		}

		return nil, nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, fact.Hash(), nil, nodes)

	t.T().Log("wait new block saved, but it will be failed; wait to move syncing")

	err = st.newVoteproof(nextavp)

	var ssctx SyncingSwitchContext
	t.ErrorAs(err, &ssctx)
	t.Equal(ssctx.height, point.Height())
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestSaveBlockError() {
	t.Run("no accept voteproof after processing", func() {
		point := base.RawPoint(33, 44)
		suf, nodes := isaac.NewTestSuffrage(2, t.Local)

		st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
		defer closefunc()

		manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())

		fact := t.PRPool.GetFact(point)
		nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)

		processedch := make(chan struct{})
		pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
			defer close(processedch)
			return manifest, nil
		}

		pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
			if avp.Point().Point.Equal(point) {
				return nil, errors.Errorf("hehehe")
			}

			return nil, nil
		}

		sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

		deferred, err := st.enter(StateJoining, sctx)
		t.NoError(err)
		deferred()

		<-processedch

		err = st.newVoteproof(nextavp)

		t.T().Log("wait new block saved, but it will be failed; wait to move broken")

		var bsctx baseErrorSwitchContext
		t.ErrorAs(err, &bsctx)
		t.Equal(bsctx.next(), StateBroken)
		t.ErrorContains(bsctx, "hehehe")
	})

	t.Run("accept voteproof after processing", func() {
		point := base.RawPoint(33, 44)
		suf, nodes := isaac.NewTestSuffrage(2, t.Local)

		st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
		defer closefunc()

		manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())

		fact := t.PRPool.GetFact(point)
		nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)

		savedch := make(chan struct{})
		pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
			return manifest, nil
		}

		pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
			defer close(savedch)
			if avp.Point().Point.Equal(point) {
				return nil, errors.Errorf("hehehe")
			}

			return nil, nil
		}

		sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

		t.True(st.setLastVoteproof(nextavp))

		deferred, err := st.enter(StateJoining, sctx)
		t.NoError(err)
		deferred()

		<-savedch

		t.NoError(st.newVoteproof(nextavp))
	})

	t.Run("save after processing", func() {
		point := base.RawPoint(33, 44)
		suf, nodes := isaac.NewTestSuffrage(2, t.Local)

		st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
		defer closefunc()

		manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())

		fact := t.PRPool.GetFact(point)
		nextavp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)

		savedch := make(chan struct{})
		pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
			return manifest, nil
		}

		pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
			defer close(savedch)

			if avp.Point().Point.Equal(point) {
				return nil, errors.Errorf("hehehe")
			}

			return nil, nil
		}

		sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

		t.True(st.setLastVoteproof(nextavp))

		deferred, err := st.enter(StateJoining, sctx)
		t.NoError(err)
		deferred()

		<-savedch

		issaved, err := st.saveBlock(nextavp)
		t.False(issaved)
		t.NoError(err)
	})
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestHigherAndDraw() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point.NextHeight(), point.NextHeight().NextHeight(), nil, fact.Hash(), nil, nodes)
	nextavp.SetResult(base.VoteResultDraw).Finish()

	err = st.newVoteproof(nextavp)
	t.NotNil(st.lastVoteproofs().ACCEPT())

	var ssctx SyncingSwitchContext
	t.ErrorAs(err, &ssctx)

	t.Equal(nextavp.Point().Height().SafePrev(), ssctx.height)
}

func (t *testNewACCEPTOnINITVoteproofConsensusHandler) TestHigherRoundDraw() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	nextpr := t.PRPool.Get(point.NextRound().NextRound().NextRound())

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		if bl.Point().Point.Compare(point) < 1 {
			return nil
		}

		ballotch <- bl

		return nil
	})

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	nextavp, _ := t.VoteproofsPair(point.NextRound().NextRound(), point.NextHeight(), nil, fact.Hash(), nil, nodes)
	nextavp.SetResult(base.VoteResultDraw).Finish()

	t.NoError(st.newVoteproof(nextavp))
	t.NotNil(st.lastVoteproofs().ACCEPT())

	t.T().Log("will wait init ballot of next round")

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait next round init ballot")

		return
	case bl := <-ballotch:
		t.Equal(nextpr.Point(), bl.Point().Point)

		rbl, ok := bl.(base.INITBallot)
		t.True(ok)

		rfact := rbl.BallotSignFact().BallotFact()
		t.Equal(ivp.BallotMajority().PreviousBlock(), rfact.PreviousBlock())
		t.True(nextpr.Fact().Hash().Equal(rfact.Proposal()))
	}
}

func TestNewACCEPTOnINITVoteproofConsensusHandler(t *testing.T) {
	suite.Run(t, new(testNewACCEPTOnINITVoteproofConsensusHandler))
}

type testNewACCEPTOnACCEPTVoteproofConsensusHandler struct {
	baseTestConsensusHandler
}

func (t *testNewACCEPTOnACCEPTVoteproofConsensusHandler) TestHigerHeight() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return manifest, nil
	}
	savedch := make(chan base.ACCEPTVoteproof, 1)
	pp.Saveerr = func(_ context.Context, avp base.ACCEPTVoteproof) (base.BlockMap, error) {
		savedch <- avp
		return nil, nil
	}

	_ = t.PRPool.Get(point.NextHeight())
	fact := t.PRPool.GetFact(point)

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	avp, _ := t.VoteproofsPair(point, point.NextHeight(), manifest.Hash(), fact.Hash(), nil, nodes)
	t.NoError(st.newVoteproof(avp))

	t.T().Log("wait new block saved")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case ravp := <-savedch:
		base.EqualVoteproof(t.Assert(), avp, ravp)
		base.EqualVoteproof(t.Assert(), ravp, st.lastVoteproofs().ACCEPT())
	}

	t.T().Log("new accept voteproof; higher height")

	newavp, _ := t.VoteproofsPair(point.NextHeight().NextHeight(), point.NextHeight(), nil, nil, nil, nodes)
	err = st.newVoteproof(newavp)

	var ssctx SyncingSwitchContext
	t.ErrorAs(err, &ssctx)
	t.Equal(ssctx.height, newavp.Point().Height())
}

func (t *testNewACCEPTOnACCEPTVoteproofConsensusHandler) TestDrawAndHigherHeight() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	_ = t.PRPool.Get(point.NextRound())

	nextprch := make(chan base.Point, 1)
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			pr := t.PRPool.ByPoint(p)
			if pr != nil {
				if p.Equal(point.NextRound()) {
					nextprch <- p
				}

				return pr, nil
			}
			return nil, util.ErrNotFound.WithStack()
		}
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	avp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, fact.Hash(), nil, nodes)
	avp.SetResult(base.VoteResultDraw).Finish()

	t.NoError(st.newVoteproof(avp))

	t.T().Log("wait new block saved")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case p := <-nextprch:
		t.Equal(point.NextRound(), p)
	}

	t.T().Log("new accept voteproof; higher height")

	newavp, _ := t.VoteproofsPair(point.NextHeight(), point.NextHeight().NextHeight(), nil, nil, nil, nodes)
	err = st.newVoteproof(newavp)

	var ssctx SyncingSwitchContext
	t.ErrorAs(err, &ssctx)
	t.Equal(ssctx.height, newavp.Point().Height())
}

func (t *testNewACCEPTOnACCEPTVoteproofConsensusHandler) TestDrawAndHigherRound() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	_ = t.PRPool.Get(point.NextRound())

	nextprch := make(chan base.Point, 1)
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			pr := t.PRPool.ByPoint(p)
			if pr != nil {
				if p.Equal(point.NextRound()) {
					nextprch <- p
				}

				return pr, nil
			}
			return nil, util.ErrNotFound.WithStack()
		}
	}

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	avp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, fact.Hash(), nil, nodes)
	avp.SetResult(base.VoteResultDraw).Finish()

	t.NoError(st.newVoteproof(avp))

	t.T().Log("wait new block saved")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait save proposal processor")

		return
	case p := <-nextprch:
		t.Equal(point.NextRound(), p)
	}

	t.T().Log("new accept voteproof; higher height")

	newavp, _ := t.VoteproofsPair(point.NextRound(), point.NextHeight(), nil, nil, nil, nodes)
	err = st.newVoteproof(newavp)

	var ssctx SyncingSwitchContext
	t.ErrorAs(err, &ssctx)
	t.Equal(ssctx.height, newavp.Point().Height())
}

func (t *testNewACCEPTOnACCEPTVoteproofConsensusHandler) TestDrawAndDrawAgain() {
	point := base.RawPoint(33, 44)
	suf, nodes := isaac.NewTestSuffrage(2, t.Local)

	st, closefunc, pp, ivp := t.newStateWithINITVoteproof(point, suf)
	defer closefunc()

	t.LocalParams.SetWaitPreparingINITBallot(time.Nanosecond)

	pp.Processerr = func(context.Context, base.ProposalFact, base.INITVoteproof) (base.Manifest, error) {
		return base.NewDummyManifest(point.Height(), valuehash.RandomSHA256()), nil
	}

	_ = t.PRPool.Get(point.NextRound())
	nextpr := t.PRPool.Get(point.NextRound().NextRound())

	newprch := make(chan base.Point, 1)
	st.args.ProposalSelectFunc = func(ctx context.Context, p base.Point, _ util.Hash, _ time.Duration) (base.ProposalSignFact, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			pr := t.PRPool.ByPoint(p)
			if pr != nil {
				if p.Equal(point.NextRound()) {
					newprch <- p
				}

				return pr, nil
			}
			return nil, util.ErrNotFound.WithStack()
		}
	}

	ballotch := make(chan base.Ballot, 1)
	st.ballotBroadcaster = NewDummyBallotBroadcaster(t.Local.Address(), func(bl base.Ballot) error {
		if bl.Point().Point.Compare(point.NextRound().NextRound()) == 0 {
			ballotch <- bl
		}

		return nil
	})

	sctx, _ := newConsensusSwitchContext(StateJoining, ivp)

	deferred, err := st.enter(StateJoining, sctx)
	t.NoError(err)
	deferred()

	fact := t.PRPool.GetFact(point)
	avp, _ := t.VoteproofsPair(point, point.NextHeight(), nil, fact.Hash(), nil, nodes)
	avp.SetResult(base.VoteResultDraw).Finish()

	t.NoError(st.newVoteproof(avp))

	t.T().Log("wait new block saved")
	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait")

		return
	case p := <-newprch:
		t.Equal(point.NextRound(), p)
	}

	t.T().Log("new accept voteproof; higher height")

	newavp, _ := t.VoteproofsPair(point.NextRound(), point.NextHeight(), nil, nil, nil, nodes)
	newavp.SetResult(base.VoteResultDraw).Finish()

	t.NoError(st.newVoteproof(newavp))

	select {
	case <-time.After(time.Second * 2):
		t.Fail("timeout to wait accept ballot")

		return
	case bl := <-ballotch:
		t.NoError(bl.IsValid(t.LocalParams.NetworkID()))

		rbl, ok := bl.(base.INITBallot)
		t.True(ok)

		t.Equal(nextpr.Point(), bl.Point().Point)

		rfact := rbl.BallotSignFact().BallotFact()
		t.True(nextpr.Fact().Hash().Equal(rfact.Proposal()))
	}
}

func TestNewACCEPTOnACCEPTVoteproofConsensusHandler(t *testing.T) {
	suite.Run(t, new(testNewACCEPTOnACCEPTVoteproofConsensusHandler))
}
