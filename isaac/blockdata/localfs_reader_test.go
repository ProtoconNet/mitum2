package blockdata

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/isaac/database"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/tree"
	"github.com/spikeekips/mitum/util/valuehash"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type testBaseLocalBlockDataFS struct {
	isaac.BaseTestBallots
	database.BaseTestDatabase
	root string
}

func (t *testBaseLocalBlockDataFS) SetupSuite() {
	t.BaseTestDatabase.SetupSuite()

	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: BlockDataMapHint, Instance: BlockDataMap{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.INITVoteproofHint, Instance: isaac.INITVoteproof{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.ACCEPTVoteproofHint, Instance: isaac.ACCEPTVoteproof{}}))

	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.DummyOperationFactHint, Instance: isaac.DummyOperationFact{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.DummyOperationHint, Instance: isaac.DummyOperation{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: base.StateFixedTreeNodeHint, Instance: base.StateFixedTreeNode{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: base.OperationFixedTreeNodeHint, Instance: base.OperationFixedTreeNode{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.INITBallotFactHint, Instance: isaac.INITBallotFact{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.ACCEPTBallotFactHint, Instance: isaac.ACCEPTBallotFact{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.INITBallotSignedFactHint, Instance: isaac.INITBallotSignedFact{}}))
	t.NoError(t.Enc.Add(encoder.DecodeDetail{Hint: isaac.ACCEPTBallotSignedFactHint, Instance: isaac.ACCEPTBallotSignedFact{}}))
}

func (t *testBaseLocalBlockDataFS) SetupTest() {
	t.BaseTestBallots.SetupTest()
	t.BaseTestDatabase.SetupTest()

	t.root, _ = os.MkdirTemp("", "mitum-test")
}

func (t *testBaseLocalBlockDataFS) TearDownTest() {
	os.RemoveAll(t.root)
}

func (t *testBaseLocalBlockDataFS) voteproofs(point base.Point) (base.INITVoteproof, base.ACCEPTVoteproof) {
	_, nodes := isaac.NewTestSuffrage(1, t.Local)

	ifact := t.NewINITBallotFact(point, valuehash.RandomSHA256(), valuehash.RandomSHA256())
	ivp, err := t.NewINITVoteproof(ifact, t.Local, nodes)
	t.NoError(err)

	afact := t.NewACCEPTBallotFact(point, valuehash.RandomSHA256(), valuehash.RandomSHA256())
	avp, err := t.NewACCEPTVoteproof(afact, t.Local, nodes)
	t.NoError(err)

	return ivp, avp
}

func (t *testBaseLocalBlockDataFS) walkDirectory(root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.T().Logf("error: %+v", err)

			return err
		}

		if info.IsDir() {
			return nil
		}

		t.T().Log("file:", path)

		return nil
	})
}

type testLocalFSReader struct {
	testBaseLocalBlockDataFS
}

func (t *testLocalFSReader) preparefs(point base.Point) (
	*LocalFSWriter,
	base.ProposalSignedFact,
	[]base.Operation,
	tree.FixedTree,
	[]base.State,
	tree.FixedTree,
	[]base.Voteproof,
) {
	ctx := context.Background()

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.Policy.NetworkID())
	t.NoError(err)

	// NOTE set manifest
	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	t.NoError(fs.SetManifest(ctx, manifest))

	// NOTE set proposal
	pr := isaac.NewProposalSignedFact(isaac.NewProposalFact(point, t.Local.Address(), []util.Hash{valuehash.RandomSHA256()}))
	_ = pr.Sign(t.Local.Privatekey(), t.Policy.NetworkID())
	t.NoError(fs.SetProposal(ctx, pr))

	// NOTE set operations
	ops := make([]base.Operation, 3)
	opstreeg := tree.NewFixedTreeGenerator(uint64(len(ops)))
	for i := range ops {
		fact := isaac.NewDummyOperationFact(util.UUID().Bytes(), valuehash.RandomSHA256())
		op, _ := isaac.NewDummyOperationProcessable(fact, t.Local.Privatekey(), t.Policy.NetworkID())
		ops[i] = op

		node := base.NewOperationFixedTreeNode(uint64(i), op.Fact().Hash(), true, "")

		t.NoError(opstreeg.Add(node))
	}

	for i := range ops {
		t.NoError(fs.SetOperation(context.Background(), i, ops[i]))
	}

	opstree, err := opstreeg.Tree()
	t.NoError(err)

	t.NoError(fs.SetOperationsTree(ctx, opstree))

	// NOTE set states
	stts := make([]base.State, 3)
	sttstreeg := tree.NewFixedTreeGenerator(uint64(len(stts)))
	for i := range stts {
		key := util.UUID().String()
		stts[i] = base.NewBaseState(
			point.Height(),
			key,
			base.NewDummyStateValue(util.UUID().String()),
			valuehash.RandomSHA256(),
			nil,
		)
		node := base.NewStateFixedTreeNode(uint64(i), []byte(key))
		t.NoError(sttstreeg.Add(node))
	}

	for i := range stts {
		t.NoError(fs.SetState(context.Background(), i, stts[i]))
	}

	sttstree, err := sttstreeg.Tree()
	t.NoError(err)

	t.NoError(fs.SetStatesTree(ctx, sttstree))

	// NOTE set voteproofs
	ivp, avp := t.voteproofs(point)
	t.NoError(fs.SetINITVoteproof(ctx, ivp))
	t.NoError(fs.SetACCEPTVoteproof(ctx, avp))

	return fs, pr, ops, opstree, stts, sttstree, []base.Voteproof{ivp, avp}
}

func (t *testLocalFSReader) TestNew() {
	ctx := context.Background()

	point := base.RawPoint(33, 44)
	fs, _, _, _, _, _, _ := t.preparefs(point)
	_, err := fs.Save(ctx)
	t.NoError(err)

	r, err := NewLocalFSReader(t.root, point.Height(), t.Enc)
	t.NoError(err)
	t.NotNil(r)

	_ = (interface{})(r).(isaac.BlockDataReader)

	t.Equal(filepath.Join(fs.root, fs.heightbase), r.root)
}

func (t *testLocalFSReader) TestMap() {
	point := base.RawPoint(33, 44)
	fs, _, _, _, _, _, _ := t.preparefs(point)
	_, err := fs.Save(context.Background())
	t.NoError(err)

	r, err := NewLocalFSReader(t.root, point.Height(), t.Enc)
	t.NoError(err)

	m, found, err := r.Map()
	t.NoError(err)
	t.True(found)

	t.NoError(m.IsValid(t.Policy.NetworkID()))
}

func (t *testLocalFSReader) TestReader() {
	point := base.RawPoint(33, 44)
	fs, _, _, _, _, _, _ := t.preparefs(point)
	m, err := fs.Save(context.Background())
	t.NoError(err)

	{
		t.walkDirectory(fs.root)

		b, _ := util.MarshalJSONIndent(m)
		t.T().Log("blockdatamap:", string(b))
	}

	r, err := NewLocalFSReader(t.root, point.Height(), t.Enc)
	t.NoError(err)

	t.Run("unknown", func() {
		f, found, err := r.Reader(base.BlockDataType("findme"))
		t.Error(err)
		t.False(found)
		t.Nil(f)

		t.Contains(err.Error(), "unknown block data type")
	})

	t.Run("all knowns", func() {
		types := []base.BlockDataType{
			base.BlockDataTypeProposal,
			base.BlockDataTypeOperations,
			base.BlockDataTypeOperationsTree,
			base.BlockDataTypeStates,
			base.BlockDataTypeStatesTree,
			base.BlockDataTypeVoteproofs,
		}

		for i := range types {
			f, found, err := r.Reader(types[i])
			t.NoError(err, "type: %q", types[i])
			t.True(found, "type: %q", types[i])
			t.NotNil(f, "type: %q", types[i])
		}
	})

	t.Run("known and found", func() {
		f, found, err := r.Reader(base.BlockDataTypeProposal)
		t.NoError(err)
		t.True(found)
		defer f.Close()

		b, err := io.ReadAll(f)
		t.NoError(err)
		hinter, err := t.Enc.Decode(b)
		t.NoError(err)

		_ = hinter.(base.ProposalSignedFact)
	})

	t.Run("known, but not found", func() {
		// NOTE remove
		fname, _ := BlockDataFileName(base.BlockDataTypeOperations, t.Enc)
		t.NoError(os.Remove(filepath.Join(r.root, fname)))

		f, found, err := r.Reader(base.BlockDataTypeOperations)
		t.NoError(err)
		t.False(found)
		t.Nil(f)
	})
}

func (t *testLocalFSReader) TestItem() {
	point := base.RawPoint(33, 44)
	fs, pr, ops, opstree, stts, sttstree, vps := t.preparefs(point)

	_, err := fs.Save(context.Background())
	t.NoError(err)

	r, err := NewLocalFSReader(t.root, point.Height(), t.Enc)
	t.NoError(err)

	t.Run("unknown", func() {
		f, found, err := r.Item(base.BlockDataType("findme"))
		t.NoError(err)
		t.False(found)
		t.Nil(f)
	})

	t.Run("all knowns", func() {
		types := []base.BlockDataType{
			base.BlockDataTypeProposal,
			base.BlockDataTypeOperations,
			base.BlockDataTypeOperationsTree,
			base.BlockDataTypeStates,
			base.BlockDataTypeStatesTree,
			base.BlockDataTypeVoteproofs,
		}

		for i := range types {
			v, found, err := r.Item(types[i])
			t.NoError(err, "type: %q", types[i])
			t.True(found, "type: %q", types[i])
			t.NotNil(v, "type: %q", types[i])
		}
	})

	t.Run("proposal", func() {
		v, found, err := r.Item(base.BlockDataTypeProposal)
		t.NoError(err)
		t.True(found)
		t.NotNil(v)

		upr, ok := v.(base.ProposalSignedFact)
		t.True(ok)

		base.EqualProposalSignedFact(t.Assert(), pr, upr)
	})

	t.Run("operations", func() {
		v, found, err := r.Item(base.BlockDataTypeOperations)
		t.NoError(err)
		t.True(found)
		t.NotNil(v)

		uops, ok := v.([]base.Operation)
		t.True(ok)

		t.Equal(len(ops), len(uops))

		sort.Slice(ops, func(i, j int) bool {
			return bytes.Compare(ops[i].Fact().Hash().Bytes(), ops[j].Fact().Hash().Bytes()) > 0
		})
		sort.Slice(uops, func(i, j int) bool {
			return bytes.Compare(uops[i].Fact().Hash().Bytes(), uops[j].Fact().Hash().Bytes()) > 0
		})

		for i := range ops {
			a := ops[i]
			b := uops[i]

			base.EqualOperation(t.Assert(), a, b)
		}
	})

	t.Run("operations tree", func() {
		v, found, err := r.Item(base.BlockDataTypeOperationsTree)
		t.NoError(err)
		t.True(found)
		t.NotNil(v)

		uopstree, ok := v.(tree.FixedTree)
		t.True(ok)

		t.NoError(opstree.Traverse(func(n tree.FixedTreeNode) (bool, error) {
			if i, err := uopstree.Node(n.Index()); err != nil {
				return false, err
			} else if !n.Equal(i) {
				return false, errors.Errorf("not equal")
			}

			return true, nil
		}))
	})

	t.Run("states", func() {
		v, found, err := r.Item(base.BlockDataTypeStates)
		t.NoError(err)
		t.True(found)
		t.NotNil(v)

		ustts, ok := v.([]base.State)
		t.True(ok)

		t.Equal(len(stts), len(ustts))

		sort.Slice(stts, func(i, j int) bool {
			return strings.Compare(stts[i].Key(), stts[j].Key()) > 0
		})
		sort.Slice(ustts, func(i, j int) bool {
			return strings.Compare(ustts[i].Key(), ustts[j].Key()) > 0
		})

		for i := range stts {
			a := stts[i]
			b := ustts[i]

			t.True(base.IsEqualState(a, b))
		}
	})

	t.Run("states tree", func() {
		v, found, err := r.Item(base.BlockDataTypeStatesTree)
		t.NoError(err)
		t.True(found)
		t.NotNil(v)

		usttstree, ok := v.(tree.FixedTree)
		t.True(ok)

		t.NoError(sttstree.Traverse(func(n tree.FixedTreeNode) (bool, error) {
			if i, err := usttstree.Node(n.Index()); err != nil {
				return false, err
			} else if !n.Equal(i) {
				return false, errors.Errorf("not equal")
			}

			return true, nil
		}))
	})

	t.Run("voteproofs", func() {
		v, found, err := r.Item(base.BlockDataTypeVoteproofs)
		t.NoError(err)
		t.True(found)
		t.NotNil(v)

		uvps, ok := v.([]base.Voteproof)
		t.True(ok)
		t.Equal(2, len(uvps))

		base.EqualVoteproof(t.Assert(), vps[0], uvps[0])
		base.EqualVoteproof(t.Assert(), vps[1], uvps[1])
	})
}

func (t *testLocalFSReader) TestWrongChecksum() {
	point := base.RawPoint(33, 44)
	fs, pr, _, _, _, _, _ := t.preparefs(point)

	_, err := fs.Save(context.Background())
	t.NoError(err)

	t.walkDirectory(t.root)

	var root string
	t.Run("load proposal before modifying", func() {
		r, err := NewLocalFSReader(t.root, point.Height(), t.Enc)
		t.NoError(err)

		root = r.root

		v, found, err := r.Item(base.BlockDataTypeProposal)
		t.NoError(err)
		t.True(found)
		t.NotNil(v)

		upr, ok := v.(base.ProposalSignedFact)
		t.True(ok)

		base.EqualProposalSignedFact(t.Assert(), pr, upr)
	})

	// NOTE modify proposal.json
	i, _ := BlockDataFileName(base.BlockDataTypeProposal, t.Enc)
	path := filepath.Join(root, i)
	f, err := os.Open(path)
	t.NoError(err)

	b, err := io.ReadAll(f)
	t.NoError(err)

	b = append(b, '\n')
	nf, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	t.NoError(err)
	_, err = nf.Write(b)
	nf.Close()

	t.Run("load proposal after modifying", func() {
		r, err := NewLocalFSReader(t.root, point.Height(), t.Enc)
		t.NoError(err)

		v, found, err := r.Item(base.BlockDataTypeProposal)
		t.Error(err)
		t.Contains(err.Error(), "checksum mismatch")
		t.True(found)
		t.NotNil(v)
	})
}

func TestLocalFSReader(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(testLocalFSReader))
}