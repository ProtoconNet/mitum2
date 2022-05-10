package isaacblock

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/fixedtree"
	"github.com/spikeekips/mitum/util/valuehash"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
	"golang.org/x/sync/semaphore"
)

type DummyBlockFSWriter struct {
	setProposalf        func(context.Context, base.ProposalSignedFact) error
	setOperationf       func(context.Context, uint64, base.Operation) error
	setOperationsTreef  func(context.Context, *fixedtree.Writer) error
	setStatef           func(context.Context, uint64, base.State) error
	setStatesTreef      func(context.Context, *fixedtree.Writer) error
	setManifestf        func(context.Context, base.Manifest) error
	setINITVoteprooff   func(context.Context, base.INITVoteproof) error
	setACCEPTVoteprooff func(context.Context, base.ACCEPTVoteproof) error
	savef               func(context.Context) (base.BlockMap, error)
	cancelf             func() error
}

func (w *DummyBlockFSWriter) SetProposal(ctx context.Context, pr base.ProposalSignedFact) error {
	if w.setProposalf != nil {
		return w.setProposalf(ctx, pr)
	}
	return nil
}

func (w *DummyBlockFSWriter) SetOperation(ctx context.Context, index uint64, op base.Operation) error {
	if w.setOperationf != nil {
		return w.setOperationf(ctx, index, op)
	}
	return nil
}

func (w *DummyBlockFSWriter) SetOperationsTree(ctx context.Context, tw *fixedtree.Writer) error {
	if w.setOperationsTreef != nil {
		return w.setOperationsTreef(ctx, tw)
	}
	return nil
}

func (w *DummyBlockFSWriter) SetState(ctx context.Context, index uint64, st base.State) error {
	if w.setStatef != nil {
		return w.setStatef(ctx, index, st)
	}
	return nil
}

func (w *DummyBlockFSWriter) SetStatesTree(ctx context.Context, tw *fixedtree.Writer) error {
	if w.setStatesTreef != nil {
		return w.setStatesTreef(ctx, tw)
	}
	return nil
}

func (w *DummyBlockFSWriter) SetManifest(ctx context.Context, m base.Manifest) error {
	if w.setManifestf != nil {
		return w.setManifestf(ctx, m)
	}
	return nil
}

func (w *DummyBlockFSWriter) SetINITVoteproof(ctx context.Context, vp base.INITVoteproof) error {
	if w.setINITVoteprooff != nil {
		return w.setINITVoteprooff(ctx, vp)
	}
	return nil
}

func (w *DummyBlockFSWriter) SetACCEPTVoteproof(ctx context.Context, vp base.ACCEPTVoteproof) error {
	if w.setACCEPTVoteprooff != nil {
		return w.setACCEPTVoteprooff(ctx, vp)
	}
	return nil
}

func (w *DummyBlockFSWriter) Save(ctx context.Context) (base.BlockMap, error) {
	if w.savef != nil {
		return w.savef(ctx)
	}
	return nil, nil
}

func (w *DummyBlockFSWriter) Cancel() error {
	if w.cancelf != nil {
		return w.cancelf()
	}
	return nil
}

type testLocalFSWriter struct {
	testBaseLocalBlockFS
}

func (t *testLocalFSWriter) findTempFile(temp string, d base.BlockMapItemType, islist bool) (string, io.Reader, error) {
	fname, err := BlockFileName(d, t.Enc)
	t.NoError(err)

	fpath := filepath.Join(temp, fname)
	f, err := os.Open(fpath)
	if err != nil {
		return fpath, nil, err
	}

	return fpath, f, nil
}

func (t *testLocalFSWriter) TestNew() {
	fs, err := NewLocalFSWriter(t.root, base.Height(33), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	_ = (interface{})(fs).(FSWriter)

	t.T().Logf("root directory: %q", fs.root)
	t.T().Logf("root base directory: %q", fs.heightbase)
	t.T().Logf("temp directory: %q", fs.temp)
}

func (t *testLocalFSWriter) TestSetManifest() {
	point := base.RawPoint(33, 44)

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	t.Nil(fs.m.Manifest())

	t.NoError(fs.SetManifest(context.Background(), manifest))

	base.EqualManifest(t.Assert(), manifest, fs.m.Manifest())
}

func (t *testLocalFSWriter) TestSetProposal() {
	point := base.RawPoint(33, 44)
	pr := isaac.NewProposalSignedFact(isaac.NewProposalFact(point, t.Local.Address(), []util.Hash{valuehash.RandomSHA256()}))
	_ = pr.Sign(t.Local.Privatekey(), t.NodePolicy.NetworkID())

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	t.NoError(fs.SetProposal(context.Background(), pr))

	fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeProposal, false)
	t.NoError(err)
	t.T().Log("temp file:", fpath)
	t.NotNil(f)

	item, found := fs.m.Item(base.BlockMapItemTypeProposal)
	t.True(found)
	t.NoError(item.IsValid(nil))

	// NOTE compare checksum
	t.Run("compare checksum", func() {
		b, err := t.Enc.Marshal(pr)
		t.NoError(err)

		checksum := util.SHA256Checksum(b)

		t.Equal(checksum, item.Checksum())
	})
}

func (t *testLocalFSWriter) TestSave() {
	point := base.RawPoint(33, 44)
	pr := isaac.NewProposalSignedFact(isaac.NewProposalFact(point, t.Local.Address(), []util.Hash{valuehash.RandomSHA256()}))
	_ = pr.Sign(t.Local.Privatekey(), t.NodePolicy.NetworkID())

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	t.NoError(fs.SetManifest(context.Background(), manifest))

	t.NoError(fs.SetProposal(context.Background(), pr))
	ivp, avp := t.voteproofs(point)
	t.NoError(fs.SetINITVoteproof(context.Background(), ivp))
	t.NoError(fs.SetACCEPTVoteproof(context.Background(), avp))

	m, err := fs.Save(context.Background())
	t.NoError(err)
	t.NotNil(m)

	newroot := filepath.Join(fs.root, fs.heightbase)

	{
		t.walkDirectory(newroot)

		b, _ := util.MarshalJSONIndent(m)
		t.T().Log("blockmap:", string(b))
	}

	b, _ := util.MarshalJSONIndent(m)
	t.T().Log("blockmap:", string(b))

	t.Run("operations(tree) should be empty in map", func() {
		_, found := m.Item(base.BlockMapItemTypeOperations)
		t.False(found)
		_, found = m.Item(base.BlockMapItemTypeOperationsTree)
		t.False(found)
	})

	t.Run("states(tree) should be empty in map", func() {
		_, found := m.Item(base.BlockMapItemTypeStates)
		t.False(found)
		_, found = m.Item(base.BlockMapItemTypeStatesTree)
		t.False(found)
	})

	checkfile := func(d base.BlockMapItemType) {
		fname, err := BlockFileName(d, t.Enc)
		t.NoError(err)
		fi, err := os.Stat(filepath.Join(newroot, fname))
		t.NoError(err)
		t.False(fi.IsDir())
	}

	t.Run("check save directory", func() {
		fi, err := os.Stat(newroot)
		t.NoError(err)
		t.True(fi.IsDir())

		checkfile(base.BlockMapItemTypeProposal)
		checkfile(base.BlockMapItemTypeVoteproofs)
	})

	t.Run("check map file", func() {
		fname := blockFSMapFilename(t.Enc)
		fpath := filepath.Join(newroot, fname)
		f, err := os.Open(fpath)
		t.NoError(err)

		b, err := io.ReadAll(f)
		t.NoError(err)

		hinter, err := t.Enc.Decode(b)
		t.NoError(err)

		um, ok := hinter.(base.BlockMap)
		t.True(ok)

		base.EqualBlockMap(t.Assert(), fs.m, um)
	})
}

func (t *testLocalFSWriter) TestSaveAgain() {
	point := base.RawPoint(33, 44)
	pr := isaac.NewProposalSignedFact(isaac.NewProposalFact(point, t.Local.Address(), []util.Hash{valuehash.RandomSHA256()}))
	_ = pr.Sign(t.Local.Privatekey(), t.NodePolicy.NetworkID())

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	t.NoError(fs.SetManifest(context.Background(), manifest))

	t.NoError(fs.SetProposal(context.Background(), pr))
	ivp, avp := t.voteproofs(point)
	t.NoError(fs.SetINITVoteproof(context.Background(), ivp))
	t.NoError(fs.SetACCEPTVoteproof(context.Background(), avp))

	m, err := fs.Save(context.Background())
	t.NoError(err)
	t.NotNil(m)

	t.Run("save again", func() {
		fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
		t.NoError(err)

		manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
		t.NoError(fs.SetManifest(context.Background(), manifest))

		t.NoError(fs.SetProposal(context.Background(), pr))
		ivp, avp := t.voteproofs(point)
		t.NoError(fs.SetINITVoteproof(context.Background(), ivp))
		t.NoError(fs.SetACCEPTVoteproof(context.Background(), avp))

		m, err := fs.Save(context.Background())
		t.Error(err)
		t.Nil(m)
		t.ErrorContains(err, "height directory already exists")
	})
}

func (t *testLocalFSWriter) TestCancel() {
	point := base.RawPoint(33, 44)
	pr := isaac.NewProposalSignedFact(isaac.NewProposalFact(point, t.Local.Address(), []util.Hash{valuehash.RandomSHA256()}))
	_ = pr.Sign(t.Local.Privatekey(), t.NodePolicy.NetworkID())

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	manifest := base.NewDummyManifest(point.Height(), valuehash.RandomSHA256())
	t.NoError(fs.SetManifest(context.Background(), manifest))

	t.NoError(fs.SetProposal(context.Background(), pr))
	ivp, avp := t.voteproofs(point)
	t.NoError(fs.SetINITVoteproof(context.Background(), ivp))
	t.NoError(fs.SetACCEPTVoteproof(context.Background(), avp))

	t.NoError(fs.Cancel())

	t.Run("check temp directory", func() {
		fi, err := os.Stat(fs.temp)
		t.True(os.IsNotExist(err))
		t.Nil(fi)
	})
}

func (t *testLocalFSWriter) TestSetACCEPTVoteproof() {
	point := base.RawPoint(33, 44)

	ivp, avp := t.voteproofs(point)
	t.Run("both", func() {
		fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
		t.NoError(err)

		t.NoError(fs.SetINITVoteproof(context.Background(), ivp))
		t.NoError(fs.SetACCEPTVoteproof(context.Background(), avp))

		fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeVoteproofs, false)
		t.NoError(err)
		t.T().Log("temp file:", fpath)
		t.NotNil(f)

		item, found := fs.m.Item(base.BlockMapItemTypeVoteproofs)
		t.True(found)
		t.NoError(item.IsValid(nil))
	})

	t.Run("without init", func() {
		fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
		t.NoError(err)

		t.NoError(fs.SetACCEPTVoteproof(context.Background(), avp))

		fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeVoteproofs, false)
		t.Error(err)
		t.T().Log("temp file:", fpath)
		t.Nil(f)

		item, found := fs.m.Item(base.BlockMapItemTypeVoteproofs)
		t.False(found)
		t.Nil(item)
	})

	t.Run("without accept", func() {
		fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
		t.NoError(err)

		t.NoError(fs.SetINITVoteproof(context.Background(), ivp))

		fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeVoteproofs, false)
		t.Error(err)
		t.T().Log("temp file:", fpath)
		t.Nil(f)

		item, found := fs.m.Item(base.BlockMapItemTypeVoteproofs)
		t.False(found)
		t.Nil(item)
	})
}

func (t *testLocalFSWriter) TestSetOperations() {
	point := base.RawPoint(33, 44)

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	ops := make([]base.Operation, 33)
	opstreeg, err := fixedtree.NewWriter(base.OperationFixedtreeHint, 33)
	t.NoError(err)
	for i := range ops {
		fact := isaac.NewDummyOperationFact(util.UUID().Bytes(), valuehash.RandomSHA256())
		op, _ := isaac.NewDummyOperation(fact, t.Local.Privatekey(), t.NodePolicy.NetworkID())
		ops[i] = op

		node := base.NewOperationFixedtreeNode(op.Fact().Hash(), true, "")

		t.NoError(opstreeg.Add(uint64(i), node))
	}

	ctx := context.Background()
	sem := semaphore.NewWeighted(int64(len(ops)))

	for i := range ops {
		if err := sem.Acquire(ctx, 1); err != nil {
			panic(err)
		}

		i := uint64(i)
		op := ops[i]
		go func() {
			defer sem.Release(1)

			if err := fs.SetOperation(context.Background(), i, op); err != nil {
				panic(err)
			}
		}()
	}

	if err := sem.Acquire(ctx, int64(len(ops))); err != nil {
		panic(err)
	}

	t.NoError(fs.SetOperationsTree(ctx, opstreeg))

	t.Run("operations file", func() {
		fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeOperations, true)
		t.NoError(err)
		t.T().Log("temp file:", fpath)
		t.NotNil(f)

		item, found := fs.m.Item(base.BlockMapItemTypeOperations)
		t.True(found)
		t.NoError(item.IsValid(nil))

		// NOTE compare checksum
		b, err := io.ReadAll(f)
		t.NoError(err)

		checksum := util.SHA256Checksum(b)

		t.Equal(checksum, item.Checksum())
	})

	t.Run("operations tree file", func() {
		fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeOperationsTree, true)
		t.NoError(err)
		t.T().Log("temp file:", fpath)
		t.NotNil(f)

		item, found := fs.m.Item(base.BlockMapItemTypeOperationsTree)
		t.True(found)
		t.NoError(item.IsValid(nil))

		// NOTE compare checksum
		b, err := io.ReadAll(f)
		t.NoError(err)

		checksum := util.SHA256Checksum(b)

		t.Equal(checksum, item.Checksum())
	})
}

func (t *testLocalFSWriter) TestSetStates() {
	point := base.RawPoint(33, 44)

	fs, err := NewLocalFSWriter(t.root, point.Height(), t.Enc, t.Local, t.NodePolicy.NetworkID())
	t.NoError(err)

	stts := make([]base.State, 33)
	sttstreeg, err := fixedtree.NewWriter(base.StateFixedtreeHint, 33)
	t.NoError(err)
	for i := range stts {
		key := util.UUID().String()
		stts[i] = base.NewBaseState(
			point.Height(),
			key,
			base.NewDummyStateValue(util.UUID().String()),
			valuehash.RandomSHA256(),
			nil,
		)
		node := fixedtree.NewBaseNode(key)
		t.NoError(sttstreeg.Add(uint64(i), node))
	}

	ctx := context.Background()
	sem := semaphore.NewWeighted(int64(len(stts)))

	for i := range stts {
		if err := sem.Acquire(ctx, 1); err != nil {
			panic(err)
		}

		i := uint64(i)
		st := stts[i]
		go func() {
			defer sem.Release(1)

			if err := fs.SetState(context.Background(), i, st); err != nil {
				panic(err)
			}
		}()
	}

	if err := sem.Acquire(ctx, int64(len(stts))); err != nil {
		panic(err)
	}

	t.NoError(fs.SetStatesTree(ctx, sttstreeg))

	t.Run("states file", func() {
		fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeStates, true)
		t.NoError(err)
		t.T().Log("temp file:", fpath)
		t.NotNil(f)

		item, found := fs.m.Item(base.BlockMapItemTypeStates)
		t.True(found)
		t.NoError(item.IsValid(nil))

		// NOTE compare checksum
		b, err := io.ReadAll(f)
		t.NoError(err)

		checksum := util.SHA256Checksum(b)

		t.Equal(checksum, item.Checksum())
	})

	t.Run("states tree file", func() {
		fpath, f, err := t.findTempFile(fs.temp, base.BlockMapItemTypeStatesTree, true)
		t.NoError(err)
		t.T().Log("temp file:", fpath)
		t.NotNil(f)

		item, found := fs.m.Item(base.BlockMapItemTypeStatesTree)
		t.True(found)
		t.NoError(item.IsValid(nil))

		// NOTE compare checksum
		b, err := io.ReadAll(f)
		t.NoError(err)

		checksum := util.SHA256Checksum(b)

		t.Equal(checksum, item.Checksum())
	})
}

func TestLocalFSWriter(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(testLocalFSWriter))
}