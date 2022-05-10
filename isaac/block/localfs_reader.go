package isaacblock

import (
	"bufio"
	"context"
	"crypto/sha256"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/fixedtree"
	"github.com/spikeekips/mitum/util/hint"
)

type LocalFSReader struct {
	root     string
	enc      encoder.Encoder
	mapl     *util.Locked
	readersl *util.LockedMap
	itemsl   *util.LockedMap
}

func NewLocalFSReader(
	baseroot string,
	height base.Height,
	enc encoder.Encoder,
) (*LocalFSReader, error) {
	e := util.StringErrorFunc("failed to NewLocalFSReader")

	heightroot := filepath.Join(baseroot, HeightDirectory(height))
	switch fi, err := os.Stat(filepath.Join(heightroot, blockFSMapFilename(enc))); {
	case err != nil:
		return nil, e(err, "invalid root directory")
	case fi.IsDir():
		return nil, e(nil, "map file is directory")
	}

	return &LocalFSReader{
		root:     heightroot,
		enc:      enc,
		mapl:     util.EmptyLocked(),
		readersl: util.NewLockedMap(),
		itemsl:   util.NewLockedMap(),
	}, nil
}

func (r *LocalFSReader) Map() (base.BlockMap, bool, error) {
	i, err := r.mapl.Get(func() (interface{}, error) {
		var b []byte
		switch f, err := os.Open(filepath.Join(r.root, blockFSMapFilename(r.enc))); {
		case err != nil:
			return nil, err
		default:
			defer func() {
				_ = f.Close()
			}()

			i, err := io.ReadAll(f)
			if err != nil {
				return nil, err
			}

			b = i
		}

		hinter, err := r.enc.Decode(b)
		if err != nil {
			return nil, err
		}

		um, ok := hinter.(base.BlockMap)
		if !ok {
			return nil, errors.Errorf("not blockmap, %T", hinter)
		}

		return um, nil
	})

	e := util.StringErrorFunc("failed to load blockmap")
	switch {
	case err == nil:
		return i.(base.BlockMap), true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	default:
		return nil, false, e(err, "")
	}
}

func (r *LocalFSReader) Reader(t base.BlockMapItemType) (util.ChecksumReader, bool, error) {
	e := util.StringErrorFunc("failed to make reader, %q", t)

	var fpath string
	switch i, err := BlockFileName(t, r.enc); {
	case err != nil:
		return nil, false, e(err, "")
	default:
		fpath = filepath.Join(r.root, i)
	}

	i, _, _ := r.readersl.Get(t, func() (interface{}, error) {
		switch fi, err := os.Stat(fpath); {
		case err != nil:
			return err, nil // nolint:nilerr
		case fi.IsDir():
			return errors.Errorf("not normal file; directory"), nil
		default:
			return nil, nil
		}
	})

	if i != nil {
		switch err, ok := i.(error); {
		case !ok:
			return nil, false, nil
		case os.IsNotExist(err):
			return nil, false, nil
		default:
			return nil, false, e(err, "")
		}
	}

	var f util.ChecksumReader

	rawf, err := os.Open(filepath.Clean(fpath))
	if err == nil {
		cr := util.NewHashChecksumReader(rawf, sha256.New())
		switch {
		case isCompressedBlockMapItemType(t):
			gr, eerr := util.NewGzipReader(cr)
			if eerr != nil {
				err = eerr
			}

			f = util.NewDummyChecksumReader(gr, cr)
		default:
			f = cr
		}
	}

	_ = r.readersl.SetValue(t, err)

	switch {
	case err == nil:
		return f, true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	default:
		return nil, false, e(err, "")
	}
}

func (r *LocalFSReader) Item(t base.BlockMapItemType) (interface{}, bool, error) {
	i, _, _ := r.itemsl.Get(t, func() (interface{}, error) {
		j, found, err := r.item(t)

		return [3]interface{}{j, found, err}, nil
	})

	l := i.([3]interface{})

	var err error
	if l[2] != nil {
		err = l[2].(error)
	}

	return l[0], l[1].(bool), err
}

func (r *LocalFSReader) item(t base.BlockMapItemType) (interface{}, bool, error) {
	e := util.StringErrorFunc("failed to load item, %q", t)

	var item base.BlockMapItem
	switch m, found, err := r.Map(); {
	case err != nil || !found:
		return nil, found, e(err, "")
	default:
		if item, found = m.Item(t); !found {
			return nil, false, nil
		}
	}

	var f util.ChecksumReader
	switch i, found, err := r.Reader(t); {
	case err != nil:
		return nil, false, e(err, "")
	case !found:
		return nil, false, nil
	default:
		defer func() {
			_ = f.Close()
		}()

		f = i
	}

	var i interface{}
	var err error
	switch {
	case !isListBlockMapItemType(t):
		i, err = r.loadItem(f)
	default:
		i, err = r.loadItems(item, f)
	}

	switch {
	case err != nil:
		return i, true, e(err, "")
	case item.Checksum() != f.Checksum():
		return i, true, e(nil, "checksum mismatch; item=%q != file=%q", item.Checksum(), f.Checksum())
	default:
		return i, true, err
	}
}

func (r *LocalFSReader) loadItem(f io.Reader) (interface{}, error) {
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	hinter, err := r.enc.Decode(b)
	switch {
	case err != nil:
		return nil, errors.Wrap(err, "")
	default:
		return hinter, nil
	}
}

func (r *LocalFSReader) loadRawItems(
	f io.Reader,
	decode func([]byte) (interface{}, error),
	callback func(uint64, interface{}) error,
) error {
	if decode == nil {
		decode = func(b []byte) (interface{}, error) {
			return r.enc.Decode(b)
		}
	}

	var br *bufio.Reader
	if i, ok := f.(*bufio.Reader); ok {
		br = i
	} else {
		br = bufio.NewReader(f)
	}

	worker := util.NewErrgroupWorker(context.Background(), math.MaxInt32)
	defer worker.Close()

	var index uint64
end:
	for {
		b, err := br.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return errors.Wrap(err, "")
		}

		if len(b) > 0 {
			b := b
			i := index

			if eerr := worker.NewJob(func(ctx context.Context, _ uint64) error {
				v, eerr := decode(b)
				if eerr != nil {
					return errors.Wrap(eerr, "")
				}

				if eerr := callback(i, v); eerr != nil {
					return errors.Wrap(eerr, "")
				}

				return nil
			}); eerr != nil {
				return errors.Wrap(eerr, "")
			}

			index++
		}

		switch {
		case err == nil:
		case errors.Is(err, io.EOF):
			break end
		default:
			return errors.Wrap(err, "")
		}
	}

	worker.Done()
	if err := worker.Wait(); err != nil {
		return errors.Wrap(err, "")
	}

	return nil
}

func (r *LocalFSReader) loadItems(item base.BlockMapItem, f io.Reader) (interface{}, error) {
	switch item.Type() {
	case base.BlockMapItemTypeOperations:
		return r.loadOperations(item, f)
	case base.BlockMapItemTypeOperationsTree:
		return r.loadOperationsTree(item, f)
	case base.BlockMapItemTypeStates:
		return r.loadStates(item, f)
	case base.BlockMapItemTypeStatesTree:
		return r.loadStatesTree(item, f)
	case base.BlockMapItemTypeVoteproofs:
		return r.loadVoteproofs(item, f)
	default:
		return nil, errors.Errorf("unsupported list items, %q", item.Type())
	}
}

func (r *LocalFSReader) loadOperations(item base.BlockMapItem, f io.Reader) ([]base.Operation, error) {
	if item.Num() < 1 {
		return nil, nil
	}

	ops := make([]base.Operation, item.Num())

	if err := r.loadRawItems(f, nil, func(index uint64, v interface{}) error {
		op, ok := v.(base.Operation)
		if !ok {
			return errors.Errorf("not Operation, %T", v)
		}

		ops[index] = op

		return nil
	},
	); err != nil {
		return nil, errors.Wrap(err, "")
	}

	return ops, nil
}

func (r *LocalFSReader) loadOperationsTree(item base.BlockMapItem, f io.Reader) (fixedtree.Tree, error) {
	tr, err := r.loadTree(item, f, func(i interface{}) (fixedtree.Node, error) {
		node, ok := i.(base.OperationFixedtreeNode)
		if !ok {
			return nil, errors.Errorf("not OperationFixedtreeNode, %T", i)
		}

		return node, nil
	})
	if err != nil {
		return fixedtree.Tree{}, errors.Wrap(err, "failed to load OperationsTree")
	}

	return tr, nil
}

func (r *LocalFSReader) loadStates(item base.BlockMapItem, f io.Reader) ([]base.State, error) {
	if item.Num() < 1 {
		return nil, nil
	}

	sts := make([]base.State, item.Num())

	if err := r.loadRawItems(f, nil, func(index uint64, v interface{}) error {
		st, ok := v.(base.State)
		if !ok {
			return errors.Errorf("expected State, but %T", v)
		}

		sts[index] = st

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "")
	}

	return sts, nil
}

func (r *LocalFSReader) loadStatesTree(item base.BlockMapItem, f io.Reader) (fixedtree.Tree, error) {
	tr, err := r.loadTree(item, f, func(i interface{}) (fixedtree.Node, error) {
		node, ok := i.(fixedtree.Node)
		if !ok {
			return nil, errors.Errorf("not StateFixedtreeNode, %T", i)
		}

		return node, nil
	})
	if err != nil {
		return fixedtree.Tree{}, errors.Wrap(err, "failed to load StatesTree")
	}

	return tr, nil
}

func (r *LocalFSReader) loadVoteproofs(item base.BlockMapItem, f io.Reader) ([]base.Voteproof, error) {
	if item.Num() < 1 {
		return nil, nil
	}

	e := util.StringErrorFunc("failed to load voteproofs")

	vps := make([]base.Voteproof, 2)

	if err := r.loadRawItems(f, nil, func(_ uint64, v interface{}) error {
		switch t := v.(type) {
		case base.INITVoteproof:
			vps[0] = t
		case base.ACCEPTVoteproof:
			vps[1] = t
		default:
			return errors.Errorf("not Operation, %T", v)
		}

		return nil
	}); err != nil {
		return nil, e(err, "")
	}

	if vps[0] == nil || vps[1] == nil {
		return nil, e(nil, "missing")
	}

	return vps, nil
}

func (r *LocalFSReader) loadTree(
	item base.BlockMapItem,
	f io.Reader,
	callback func(interface{}) (fixedtree.Node, error),
) (tr fixedtree.Tree, err error) {
	if item.Num() < 1 {
		return tr, nil
	}

	e := util.StringErrorFunc("failed to load tree")

	br := bufio.NewReader(f)
	ht, err := r.loadTreeHint(br)
	if err != nil {
		return tr, e(err, "")
	}

	nodes := make([]fixedtree.Node, item.Num())
	if tr, err = fixedtree.NewTree(ht, nodes); err != nil {
		return tr, e(err, "")
	}

	if err := r.loadRawItems(
		br,
		func(b []byte) (interface{}, error) {
			return unmarshalIndexedTreeNode(r.enc, b, ht)
		},
		func(_ uint64, v interface{}) error {
			in := v.(indexedTreeNode)
			n, err := callback(in.Node)
			if err != nil {
				return errors.Wrap(err, "")
			}

			if err := tr.Set(in.Index, n); err != nil {
				return errors.Wrap(err, "")
			}

			return nil
		},
	); err != nil {
		return tr, e(err, "")
	}

	return tr, nil
}

func (*LocalFSReader) loadTreeHint(br *bufio.Reader) (hint.Hint, error) {
end:
	for {
		s, err := br.ReadString('\n')
		switch {
		case err != nil:
			return hint.Hint{}, errors.Wrap(err, "")
		case len(s) < 1:
			continue end
		}

		ht, err := hint.ParseHint(s)
		if err != nil {
			return hint.Hint{}, errors.Wrap(err, "failed to load tree hint")
		}

		return ht, nil
	}
}