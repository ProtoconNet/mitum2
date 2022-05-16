package isaacnetwork

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	isaacdatabase "github.com/spikeekips/mitum/isaac/database"
	"github.com/spikeekips/mitum/network/quicstream"
	"github.com/spikeekips/mitum/network/quictransport"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/valuehash"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type testQuicstreamHandlers struct {
	isaacdatabase.BaseTestDatabase
	isaac.BaseTestBallots
}

func (t *testQuicstreamHandlers) SetupTest() {
	t.BaseTestDatabase.SetupTest()
	t.BaseTestBallots.SetupTest()
}

func (t *testQuicstreamHandlers) SetupSuite() {
	t.BaseTestDatabase.SetupSuite()

	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: RequestProposalRequestHeaderHint, Instance: RequestProposalRequestHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: ProposalHeaderHint, Instance: ProposalRequestHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: SuffrageProofHeaderHint, Instance: SuffrageProofRequestHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: LastBlockMapHeaderHint, Instance: LastBlockMapRequestHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: BlockMapHeaderHint, Instance: BlockMapRequestHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: BlockMapItemHeaderHint, Instance: BlockMapItemRequestHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.SuffrageCandidateHint, Instance: isaac.SuffrageCandidate{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: ErrorResponseHeaderHint, Instance: ErrorResponseHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: OKResponseHeaderHint, Instance: OKResponseHeader{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.DummySuffrageProofHint, Instance: isaac.DummySuffrageProof{}}))
}

func (t *testQuicstreamHandlers) TestClient() {
	c := newBaseNetworkClient(t.Encs, t.Enc, nil)

	_ = (interface{})(c).(isaac.NetworkClient)
}

func (t *testQuicstreamHandlers) writef(prefix string, handler quicstream.Handler) baseNetworkClientWriteFunc {
	return func(ctx context.Context, ci quictransport.ConnInfo, f quicstream.ClientWriteFunc) (io.ReadCloser, error) {
		r := bytes.NewBuffer(nil)
		if err := f(r); err != nil {
			return nil, errors.Wrap(err, "")
		}

		uprefix, err := quicstream.ReadPrefix(r)
		if err != nil {
			return nil, errors.Wrap(err, "")
		}

		if !bytes.Equal(uprefix, quicstream.HashPrefix(prefix)) {
			return nil, errors.Errorf("unknown request, %q", prefix)
		}

		w := bytes.NewBuffer(nil)
		if err := handler(nil, r, w); err != nil {
			return nil, errors.Wrap(err, "failed to handle request")
		}

		return io.NopCloser(w), nil
	}
}

func (t *testQuicstreamHandlers) TestRequestProposal() {
	pool := t.NewPool()
	defer pool.Close()

	proposalMaker := isaac.NewProposalMaker(
		t.Local,
		t.NodePolicy,
		func(context.Context) ([]util.Hash, error) {
			return []util.Hash{valuehash.RandomSHA256(), valuehash.RandomSHA256()}, nil
		},
		pool,
	)

	handlers := NewQuicstreamHandlers(t.Local, t.Encs, t.Enc, pool, proposalMaker, nil, nil, nil, nil)

	ci := quictransport.NewBaseConnInfo(nil, true)
	c := newBaseNetworkClient(t.Encs, t.Enc, t.writef(HandlerPrefixRequestProposal, handlers.RequestProposal))

	t.Run("local is proposer", func() {
		point := base.RawPoint(33, 1)
		pr, found, err := c.RequestProposal(context.Background(), ci, point, t.Local.Address())
		t.NoError(err)
		t.True(found)

		t.Equal(point, pr.Point())
		t.True(t.Local.Address().Equal(pr.ProposalFact().Proposer()))
		t.NoError(base.IsValidProposalSignedFact(pr, t.NodePolicy.NetworkID()))
	})

	t.Run("local is not proposer", func() {
		point := base.RawPoint(33, 1)
		proposer := base.RandomAddress("")
		pr, found, err := c.RequestProposal(context.Background(), ci, point, proposer)
		t.NoError(err)
		t.False(found)
		t.Nil(pr)
	})
}

func (t *testQuicstreamHandlers) TestProposal() {
	pool := t.NewPool()
	defer pool.Close()

	proposalMaker := isaac.NewProposalMaker(
		t.Local,
		t.NodePolicy,
		func(context.Context) ([]util.Hash, error) {
			return []util.Hash{valuehash.RandomSHA256(), valuehash.RandomSHA256()}, nil
		},
		pool,
	)

	point := base.RawPoint(33, 1)
	pr, err := proposalMaker.New(context.Background(), point)
	t.NoError(err)
	_, err = pool.SetProposal(pr)
	t.NoError(err)

	handlers := NewQuicstreamHandlers(t.Local, t.Encs, t.Enc, pool, proposalMaker, nil, nil, nil, nil)

	ci := quictransport.NewBaseConnInfo(nil, true)
	c := newBaseNetworkClient(t.Encs, t.Enc, t.writef(HandlerPrefixProposal, handlers.Proposal))

	t.Run("found", func() {
		pr, found, err := c.Proposal(context.Background(), ci, pr.Fact().Hash())
		t.NoError(err)
		t.True(found)

		t.Equal(point, pr.Point())
		t.True(t.Local.Address().Equal(pr.ProposalFact().Proposer()))
		t.NoError(base.IsValidProposalSignedFact(pr, t.NodePolicy.NetworkID()))
	})

	t.Run("unknown", func() {
		pr, found, err := c.Proposal(context.Background(), ci, valuehash.RandomSHA256())
		t.NoError(err)
		t.False(found)
		t.Nil(pr)
	})

	t.Run("nil proposal fact hash", func() {
		pr, found, err := c.Proposal(context.Background(), ci, nil)
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "invalid ProposalHeader")
		t.False(found)
		t.Nil(pr)
	})
}

func (t *testQuicstreamHandlers) TestSuffrageProof() {
	handlers := NewQuicstreamHandlers(t.Local, t.Encs, t.Enc, nil, nil, nil, nil, nil, nil)

	ci := quictransport.NewBaseConnInfo(nil, true)
	c := newBaseNetworkClient(t.Encs, t.Enc, t.writef(HandlerPrefixSuffrageProof, handlers.SuffrageProof))

	t.Run("found", func() {
		st, _ := t.SuffrageState(base.Height(33), base.Height(11), nil)
		proof := isaac.NewDummySuffrageProof()
		proof = proof.SetState(st)

		handlers.suffrageProof = func(state util.Hash) (isaac.SuffrageProof, bool, error) {
			if !state.Equal(st.Hash()) {
				return nil, false, nil
			}

			return proof, true, nil
		}

		rproof, found, err := c.SuffrageProof(context.Background(), ci, st.Hash())
		t.NoError(err)
		t.True(found)
		t.NotNil(rproof)

		t.True(base.IsEqualState(proof.State(), rproof.State()))
	})

	t.Run("nil state", func() {
		handlers.suffrageProof = func(state util.Hash) (isaac.SuffrageProof, bool, error) {
			return nil, true, nil
		}

		_, _, err := c.SuffrageProof(context.Background(), ci, nil)
		t.Error(err)
		t.ErrorContains(err, "invalid")
	})

	t.Run("not found", func() {
		handlers.suffrageProof = func(state util.Hash) (isaac.SuffrageProof, bool, error) {
			return nil, false, nil
		}

		proof, found, err := c.SuffrageProof(context.Background(), ci, valuehash.RandomSHA256())
		t.NoError(err)
		t.False(found)
		t.Nil(proof)
	})
}

func (t *testQuicstreamHandlers) TestLastBlockMap() {
	handlers := NewQuicstreamHandlers(t.Local, t.Encs, t.Enc, nil, nil, nil, nil, nil, nil)

	ci := quictransport.NewBaseConnInfo(nil, true)
	c := newBaseNetworkClient(t.Encs, t.Enc, t.writef(HandlerPrefixLastBlockMap, handlers.LastBlockMap))

	t.Run("nil and updated", func() {
		m := base.NewDummyManifest(base.Height(33), valuehash.RandomSHA256())
		mp := base.NewDummyBlockMap(m)

		handlers.lastBlockMap = func(manifest util.Hash) (base.BlockMap, bool, error) {
			if manifest != nil && manifest.Equal(m.Hash()) {
				return nil, false, nil
			}

			return mp, true, nil
		}

		rmp, updated, err := c.LastBlockMap(context.Background(), ci, nil)
		t.NoError(err)
		t.True(updated)
		t.NotNil(rmp)

		base.EqualBlockMap(t.Assert(), mp, rmp)
	})

	t.Run("not nil and not updated", func() {
		m := base.NewDummyManifest(base.Height(33), valuehash.RandomSHA256())
		mp := base.NewDummyBlockMap(m)

		handlers.lastBlockMap = func(manifest util.Hash) (base.BlockMap, bool, error) {
			if manifest != nil && manifest.Equal(m.Hash()) {
				return nil, false, nil
			}

			return mp, true, nil
		}

		rmp, updated, err := c.LastBlockMap(context.Background(), ci, m.Hash())
		t.NoError(err)
		t.False(updated)
		t.Nil(rmp)
	})

	t.Run("not found", func() {
		handlers.lastBlockMap = func(manifest util.Hash) (base.BlockMap, bool, error) {
			return nil, false, nil
		}

		rmp, updated, err := c.LastBlockMap(context.Background(), ci, valuehash.RandomSHA256())
		t.NoError(err)
		t.False(updated)
		t.Nil(rmp)
	})
}

func (t *testQuicstreamHandlers) TestBlockMap() {
	handlers := NewQuicstreamHandlers(t.Local, t.Encs, t.Enc, nil, nil, nil, nil, nil, nil)

	ci := quictransport.NewBaseConnInfo(nil, true)
	c := newBaseNetworkClient(t.Encs, t.Enc, t.writef(HandlerPrefixBlockMap, handlers.BlockMap))

	t.Run("found", func() {
		m := base.NewDummyManifest(base.Height(33), valuehash.RandomSHA256())
		mp := base.NewDummyBlockMap(m)

		handlers.blockMap = func(height base.Height) (base.BlockMap, bool, error) {
			if height != m.Height() {
				return nil, false, nil
			}

			return mp, true, nil
		}

		rmp, found, err := c.BlockMap(context.Background(), ci, m.Height())
		t.NoError(err)
		t.True(found)
		t.NotNil(rmp)

		base.EqualBlockMap(t.Assert(), mp, rmp)
	})

	t.Run("not found", func() {
		handlers.blockMap = func(height base.Height) (base.BlockMap, bool, error) {
			return nil, false, nil
		}

		rmp, found, err := c.BlockMap(context.Background(), ci, base.Height(33))
		t.NoError(err)
		t.False(found)
		t.Nil(rmp)
	})

	t.Run("error", func() {
		handlers.blockMap = func(height base.Height) (base.BlockMap, bool, error) {
			return nil, false, errors.Errorf("hehehe")
		}

		_, found, err := c.BlockMap(context.Background(), ci, base.Height(33))
		t.Error(err)
		t.False(found)

		t.ErrorContains(err, "hehehe")
	})
}

func (t *testQuicstreamHandlers) TestBlockMapItem() {
	handlers := NewQuicstreamHandlers(t.Local, t.Encs, t.Enc, nil, nil, nil, nil, nil, nil)

	ci := quictransport.NewBaseConnInfo(nil, true)
	c := newBaseNetworkClient(t.Encs, t.Enc, t.writef(HandlerPrefixBlockMapItem, handlers.BlockMapItem))

	t.Run("known item", func() {
		height := base.Height(33)
		item := base.BlockMapItemTypeVoteproofs

		body := util.UUID().Bytes()
		r := bytes.NewBuffer(body)

		handlers.blockMapItem = func(h base.Height, i base.BlockMapItemType) (io.ReadCloser, bool, error) {
			if h != height {
				return nil, false, nil
			}

			if i != item {
				return nil, false, nil
			}

			return io.NopCloser(r), true, nil
		}

		rr, found, err := c.BlockMapItem(context.Background(), ci, height, item)
		t.NoError(err)
		t.True(found)
		t.NotNil(rr)

		rb, err := io.ReadAll(rr)
		t.NoError(err)

		t.Equal(body, rb)
	})

	t.Run("unknown item", func() {
		handlers.blockMapItem = func(h base.Height, i base.BlockMapItemType) (io.ReadCloser, bool, error) {
			return nil, false, nil
		}

		rr, found, err := c.BlockMapItem(context.Background(), ci, base.Height(33), base.BlockMapItemTypeVoteproofs)
		t.NoError(err)
		t.False(found)
		t.Nil(rr)
	})
}

func TestQuicstreamHandlers(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain"),
	)

	suite.Run(t, new(testQuicstreamHandlers))
}
