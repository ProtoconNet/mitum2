package isaacblock

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/fixedtree"
	"github.com/spikeekips/mitum/util/hint"
)

var LocalFSWriterHint = hint.MustNewHint("local-block-fs-writer-v0.0.1")

var rHeightDirectory = regexp.MustCompile(`^[\d]{3}$`)

var (
	blockFilenames = map[base.BlockItemType]string{
		base.BlockItemMap:            "map",
		base.BlockItemProposal:       "proposal",
		base.BlockItemOperations:     "operations",
		base.BlockItemOperationsTree: "operations_tree",
		base.BlockItemStates:         "states",
		base.BlockItemStatesTree:     "states_tree",
		base.BlockItemVoteproofs:     "voteproofs",
	}
	BlockTempDirectoryPrefix = "temp"
)

type LocalFSWriter struct {
	vps        [2]base.Voteproof
	local      base.LocalNode
	opsf       util.ChecksumWriter
	stsf       util.ChecksumWriter
	bfiles     *BlockItemFilesMaker
	enc        encoder.Encoder
	root       string
	id         string
	heightbase string
	temp       string
	m          BlockMap
	networkID  base.NetworkID
	hint.BaseHinter
	lenops           uint64
	height           base.Height
	saved            bool
	opsHeaderOnce    sync.Once
	statesHeaderOnce sync.Once
	sync.Mutex
}

func NewLocalFSWriter(
	root string,
	height base.Height,
	jsonenc, enc encoder.Encoder,
	local base.LocalNode,
	networkID base.NetworkID,
) (*LocalFSWriter, error) {
	e := util.StringError("create LocalFSWriter")

	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, e.Wrap(err)
	}

	switch fi, err := os.Stat(abs); {
	case err != nil:
		return nil, e.Wrap(err)
	case !fi.IsDir():
		return nil, e.Errorf("root is not directory")
	}

	id := util.ULID().String()
	temp := filepath.Join(abs, BlockTempDirectoryPrefix, fmt.Sprintf("%d-%s", height, id))

	if err := os.MkdirAll(temp, 0o700); err != nil {
		return nil, e.WithMessage(err, "create temp directory")
	}

	w := &LocalFSWriter{
		BaseHinter: hint.NewBaseHinter(LocalFSWriterHint),
		id:         id,
		root:       abs,
		height:     height,
		enc:        enc,
		local:      local,
		networkID:  networkID,
		heightbase: HeightDirectory(height),
		temp:       temp,
		m:          NewBlockMap(LocalFSWriterHint, enc.Hint()),
		bfiles:     NewBlockItemFilesMaker(jsonenc),
	}

	switch f, err := w.newChecksumWriter(base.BlockItemOperations); {
	case err != nil:
		return nil, e.WithMessage(err, "create operations file")
	default:
		w.opsf = f
	}

	switch f, err := w.newChecksumWriter(base.BlockItemStates); {
	case err != nil:
		return nil, e.WithMessage(err, "create states file")
	default:
		w.stsf = f
	}

	return w, nil
}

func (w *LocalFSWriter) SetProposal(_ context.Context, pr base.ProposalSignFact) error {
	if err := w.writeItem(base.BlockItemProposal, pr); err != nil {
		return errors.Wrap(err, "set proposal in fs writer")
	}

	return nil
}

func (w *LocalFSWriter) SetOperation(_ context.Context, total, _ uint64, op base.Operation) error {
	w.opsHeaderOnce.Do(func() {
		// NOTE the total of operations is not exact number of operations; it is
		// maximum number.
		_ = writeCountHeader(w.opsf, LocalFSWriterHint, w.enc.Hint(), total)
	})

	if err := w.appendfile(w.opsf, op); err != nil {
		return errors.Wrap(err, "set operation")
	}

	atomic.AddUint64(&w.lenops, 1)

	return nil
}

func (w *LocalFSWriter) SetOperationsTree(ctx context.Context, tw *fixedtree.Writer) error {
	if _, err := w.setTree(
		ctx,
		base.BlockItemOperationsTree,
		tw,
		func(ctx context.Context, _ uint64) error {
			_ = w.opsf.Close()

			if l := atomic.LoadUint64(&w.lenops); l > 0 {
				if err := w.m.SetItem(NewLocalBlockMapItem(
					base.BlockItemOperations,
					w.opsf.Checksum(),
				)); err != nil {
					return errors.Wrap(err, "set operations")
				}
			}

			return nil
		},
	); err != nil {
		return errors.Wrap(err, "set operations tree")
	}

	return nil
}

func (w *LocalFSWriter) SetState(_ context.Context, total, _ uint64, st base.State) error {
	w.statesHeaderOnce.Do(func() {
		_ = writeCountHeader(w.stsf, LocalFSWriterHint, w.enc.Hint(), total)
	})

	if err := w.appendfile(w.stsf, st); err != nil {
		return errors.Wrap(err, "set state")
	}

	return nil
}

func (w *LocalFSWriter) SetStatesTree(ctx context.Context, tw *fixedtree.Writer) (tr fixedtree.Tree, err error) {
	tr, err = w.setTree(
		ctx,
		base.BlockItemStatesTree,
		tw,
		func(ctx context.Context, _ uint64) error {
			_ = w.stsf.Close()

			if eerr := w.m.SetItem(NewLocalBlockMapItem(
				base.BlockItemStates,
				w.stsf.Checksum(),
			)); eerr != nil {
				return errors.Wrap(eerr, "set states")
			}

			return nil
		},
	)
	if err != nil {
		return tr, errors.Wrap(err, "set states tree")
	}

	return tr, nil
}

func (w *LocalFSWriter) SetManifest(_ context.Context, m base.Manifest) error {
	w.m.SetManifest(m)

	return nil
}

func (w *LocalFSWriter) SetINITVoteproof(_ context.Context, vp base.INITVoteproof) error {
	w.Lock()
	defer w.Unlock()

	w.vps[0] = vp
	if w.vps[1] == nil {
		return nil
	}

	if err := w.saveVoteproofs(); err != nil {
		return errors.Wrap(err, "set init voteproof in fs writer")
	}

	return nil
}

func (w *LocalFSWriter) SetACCEPTVoteproof(_ context.Context, vp base.ACCEPTVoteproof) error {
	w.Lock()
	defer w.Unlock()

	w.vps[1] = vp
	if w.vps[0] == nil {
		return nil
	}

	if err := w.saveVoteproofs(); err != nil {
		return errors.Wrap(err, "set accept voteproof in fs writer")
	}

	return nil
}

func (w *LocalFSWriter) saveVoteproofs() error {
	if _, found := w.m.Item(base.BlockItemVoteproofs); found {
		return nil
	}

	e := util.StringError("save voteproofs ")

	f, err := w.newChecksumWriter(base.BlockItemVoteproofs)
	if err != nil {
		return e.Wrap(err)
	}

	defer func() {
		_ = f.Close()
	}()

	if err := writeBaseHeader(f, baseItemsHeader{Writer: LocalFSWriterHint, Encoder: w.enc.Hint()}); err != nil {
		return e.Wrap(err)
	}

	for i := range w.vps {
		if err := w.appendfile(f, w.vps[i]); err != nil {
			return e.Wrap(err)
		}
	}

	if err := w.m.SetItem(NewLocalBlockMapItem(
		base.BlockItemVoteproofs,
		f.Checksum(),
	)); err != nil {
		return e.Wrap(err)
	}

	_ = w.bfiles.SetItem(base.BlockItemVoteproofs, NewLocalFSBlockItemFile(f.Name(), ""))

	return nil
}

func (w *LocalFSWriter) Save(ctx context.Context) (base.BlockMap, error) {
	w.Lock()
	defer w.Unlock()

	if w.saved {
		return w.m, nil
	}

	heightdirectory := filepath.Join(w.root, w.heightbase)

	// NOTE check height directory
	switch _, err := os.Stat(heightdirectory); {
	case err == nil:
		return nil, errors.Errorf("save fs writer; height directory already exists")
	case os.IsNotExist(err):
	default:
		return nil, errors.Errorf("save fs writer; check height directory")
	}

	switch m, err := w.save(ctx, heightdirectory); {
	case err != nil:
		_ = os.RemoveAll(heightdirectory)

		return nil, errors.WithMessage(err, "save fs writer")
	default:
		return m, nil
	}
}

func (w *LocalFSWriter) save(_ context.Context, heightdirectory string) (base.BlockMap, error) {
	if w.opsf != nil {
		_ = w.opsf.Close()

		switch item, found := w.m.Item(base.BlockItemOperations); {
		case !found || item == nil:
			// NOTE remove empty operations file
			_ = os.Remove(filepath.Join(w.temp, w.opsf.Name()))
		default:
			_ = w.bfiles.SetItem(base.BlockItemOperations, NewLocalFSBlockItemFile(w.opsf.Name(), ""))
		}
	}

	if w.stsf != nil {
		_ = w.stsf.Close()

		switch item, found := w.m.Item(base.BlockItemStates); {
		case !found || item == nil:
			// NOTE remove empty states file
			_ = os.Remove(filepath.Join(w.temp, w.stsf.Name()))
		default:
			_ = w.bfiles.SetItem(base.BlockItemStates, NewLocalFSBlockItemFile(w.stsf.Name(), ""))
		}
	}

	if item, found := w.m.Item(base.BlockItemVoteproofs); !found || item == nil {
		return nil, errors.Errorf("empty voteproofs")
	}

	if err := w.saveMap(); err != nil {
		return nil, err
	}

	switch err := os.MkdirAll(filepath.Dir(heightdirectory), 0o700); {
	case err == nil:
	case os.IsExist(err):
	default:
		return nil, errors.WithMessage(err, "create height parent directory")
	}

	if err := os.Rename(w.temp, heightdirectory); err != nil {
		return nil, errors.WithStack(err)
	}

	m := w.m

	if err := w.bfiles.Save(BlockItemFilesPath(w.root, w.height)); err != nil {
		return nil, err
	}

	if err := w.close(); err != nil {
		return nil, err
	}

	w.saved = true

	return m, nil
}

func (w *LocalFSWriter) Cancel() error {
	w.Lock()
	defer w.Unlock()

	if w.opsf != nil {
		_ = w.opsf.Close()
	}

	if w.stsf != nil {
		_ = w.stsf.Close()
	}

	e := util.StringError("cancel fs writer")
	if err := os.RemoveAll(w.temp); err != nil {
		return e.WithMessage(err, "remove temp directory")
	}

	return w.close()
}

func (w *LocalFSWriter) close() error {
	w.vps = [2]base.Voteproof{}
	w.local = nil
	w.opsf = nil
	w.stsf = nil
	w.enc = nil
	w.root = ""
	w.id = ""
	w.heightbase = ""
	w.temp = ""
	w.m = BlockMap{}
	w.lenops = 0

	return nil
}

func (w *LocalFSWriter) setTree(
	ctx context.Context,
	treetype base.BlockItemType,
	tw *fixedtree.Writer,
	newjob util.ContextWorkerCallback,
) (tr fixedtree.Tree, _ error) {
	e := util.StringError("set tree, %q", treetype)

	worker, err := util.NewErrgroupWorker(ctx, math.MaxInt8)
	if err != nil {
		return tr, e.Wrap(err)
	}

	defer worker.Close()

	tf, err := w.newChecksumWriter(treetype)
	if err != nil {
		return tr, e.WithMessage(err, "create tree file, %q", treetype)
	}

	defer func() {
		_ = tf.Close()
	}()

	if err := writeTreeHeader(tf, LocalFSWriterHint, w.enc.Hint(), uint64(tw.Len()), tw.Hint()); err != nil {
		return tr, e.Wrap(err)
	}

	if newjob != nil {
		if err := worker.NewJob(newjob); err != nil {
			return tr, e.Wrap(err)
		}
	}

	if err := tw.Write(func(index uint64, n fixedtree.Node) error {
		return worker.NewJob(func(ctx context.Context, _ uint64) error {
			b, err := marshalIndexedTreeNode(w.enc, index, n)
			if err != nil {
				return err
			}

			return w.writefile(tf, append(b, '\n'))
		})
	}); err != nil {
		return tr, e.Wrap(err)
	}

	worker.Done()

	if err := worker.Wait(); err != nil {
		return tr, e.Wrap(err)
	}

	_ = tf.Close()

	switch i, err := tw.Tree(); {
	case err != nil:
		return tr, e.Wrap(err)
	default:
		tr = i
	}

	if err := w.m.SetItem(NewLocalBlockMapItem(treetype, tf.Checksum())); err != nil {
		return tr, e.Wrap(err)
	}

	_ = w.bfiles.SetItem(treetype, NewLocalFSBlockItemFile(tf.Name(), ""))

	return tr, nil
}

func (w *LocalFSWriter) saveMap() error {
	e := util.StringError("filed to save map")

	// NOTE sign blockmap by local node
	if err := w.m.Sign(w.local.Address(), w.local.Privatekey(), w.networkID); err != nil {
		return e.Wrap(err)
	}

	if err := w.writeItem(base.BlockItemMap, w.m); err != nil {
		return errors.Wrap(err, "blockmap in fs writer")
	}

	return nil
}

func (w *LocalFSWriter) filename(t base.BlockItemType) (filename string, temppath string, err error) {
	f, err := BlockFileName(t, w.enc.Hint().Type().String())
	if err != nil {
		return "", "", err
	}

	return f, filepath.Join(w.temp, f), nil
}

func (w *LocalFSWriter) writeItem(t base.BlockItemType, i interface{}) error {
	cw, err := w.newChecksumWriter(t)
	if err != nil {
		return err
	}

	defer func() {
		_ = cw.Close()
	}()

	if err := writeBaseHeader(cw, baseItemsHeader{Writer: LocalFSWriterHint, Encoder: w.enc.Hint()}); err != nil {
		return err
	}

	if err := w.writefileonce(cw, i); err != nil {
		return err
	}

	_ = cw.Close()

	_ = w.bfiles.SetItem(t, NewLocalFSBlockItemFile(cw.Name(), ""))

	if t != base.BlockItemMap {
		return w.m.SetItem(NewLocalBlockMapItem(t, cw.Checksum()))
	}

	return nil
}

func (w *LocalFSWriter) writefileonce(f io.Writer, i interface{}) error {
	return w.enc.StreamEncoder(f).Encode(i)
}

func (w *LocalFSWriter) appendfile(f io.Writer, i interface{}) error {
	b, err := w.enc.Marshal(i)
	if err != nil {
		return err
	}

	return w.writefile(f, append(b, '\n'))
}

func (*LocalFSWriter) writefile(f io.Writer, b []byte) error {
	_, err := f.Write(b)

	return errors.WithStack(err)
}

func (w *LocalFSWriter) newChecksumWriter(t base.BlockItemType) (util.ChecksumWriter, error) {
	fname, temppath, _ := w.filename(t)

	var f io.WriteCloser

	switch i, err := os.OpenFile(temppath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600); { //nolint:gosec //...
	case err != nil:
		return nil, errors.Wrapf(err, "open file, %q", temppath)
	default:
		f = i
	}

	if isCompressedBlockMapItemType(t) {
		switch i, err := util.NewGzipWriter(f, gzip.BestSpeed); {
		case err != nil:
			return nil, err
		default:
			f = i
		}
	}

	return util.NewHashChecksumWriterWithWriter(fname, f, sha256.New()), nil
}

func HeightDirectory(height base.Height) string {
	h := height.String()
	if height < 0 {
		h = strings.ReplaceAll(h, "-", "_")
	}

	p := fmt.Sprintf(BlockDirectoryHeightFormat, h)

	sl := make([]string, 7)
	var i int

	for {
		e := (i * 3) + 3 //nolint:gomnd //...
		if e > len(p) {
			e = len(p)
		}

		s := p[i*3 : e]
		if len(s) < 1 {
			break
		}

		sl[i] = s

		if len(s) < 3 { //nolint:gomnd //...
			break
		}

		i++
	}

	return "/" + strings.Join(sl, "/")
}

func HeightFromDirectory(s string) (base.Height, error) {
	hs := strings.Replace(s, "/", "", -1)

	h, err := base.ParseHeightString(hs)
	if err != nil {
		return base.NilHeight, errors.WithMessage(err, "wrong directory for height")
	}

	return h, nil
}

func FindHighestDirectory(root string) (highest string, found bool, _ error) {
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", false, errors.WithStack(err)
	}

	switch highest, found, err = findHighestDirectory(abs); {
	case err != nil:
		return highest, found, err
	case !found:
		return highest, found, nil
	default:
		return highest, found, nil
	}
}

func FindLastHeightFromLocalFS(
	readers *Readers, networkID base.NetworkID,
) (last base.Height, found bool, _ error) {
	e := util.StringError("find last height from localfs")

	baseroot := readers.Root()
	last = base.NilHeight

	switch h, found, err := FindHighestDirectory(baseroot); {
	case err != nil:
		return last, false, e.Wrap(err)
	case !found:
		return last, false, nil
	default:
		rel, err := filepath.Rel(baseroot, h)
		if err != nil {
			return last, false, e.Wrap(err)
		}

		height, err := HeightFromDirectory(rel)
		if err != nil {
			return last, false, nil
		}

		last = height

		switch i, found, err := ReadersDecode[base.BlockMap](readers, height, base.BlockItemMap, nil); {
		case err != nil, !found:
			return last, found, e.Wrap(err)
		default:
			if err := i.IsValid(networkID); err != nil {
				return last, false, e.Wrap(err)
			}

			return last, true, nil
		}
	}
}

func findHighestDirectory(root string) (string, bool, error) {
	var highest string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		switch {
		case err != nil:
			return errors.WithStack(err)
		case !info.IsDir():
			return nil
		}

		var names []string

		switch files, err := os.ReadDir(path); {
		case err != nil:
			return errors.WithStack(err)
		default:
			var foundsubs bool
			filtered := util.FilterSlice(files, func(f fs.DirEntry) bool {
				switch {
				case !f.IsDir(), !rHeightDirectory.MatchString(f.Name()):
					return false
				default:
					if !foundsubs {
						foundsubs = true
					}

					return true
				}
			})

			if !foundsubs {
				highest = path

				return util.ErrNotFound.WithStack()
			}

			names = make([]string, len(filtered))
			for i := range filtered {
				names[i] = filtered[i].Name()
			}

			sort.Slice(names, func(i, j int) bool {
				return strings.Compare(names[i], names[j]) > 0
			})
		}

		switch a, found, err := findHighestDirectory(filepath.Join(path, names[0])); {
		case err != nil:
			if !errors.Is(err, util.ErrNotFound) {
				return err
			}
		case !found:
		default:
			highest = a
		}

		return util.ErrNotFound.WithStack()
	})

	switch {
	case err == nil, errors.Is(err, util.ErrNotFound):
		return highest, highest != "", nil
	default:
		return highest, false, errors.WithStack(err)
	}
}

func BlockFileName(t base.BlockItemType, hinttype string) (string, error) {
	name, found := blockFilenames[t]
	if !found {
		return "", errors.Errorf("unknown block map item type, %q", t)
	}

	ext := fileExtFromEncoder(hinttype)
	if isListBlockMapItemType(t) {
		ext = listFileExtFromEncoder(hinttype)
	}

	if isCompressedBlockMapItemType(t) {
		ext += ".gz"
	}

	return fmt.Sprintf("%s%s", name, ext), nil
}

func CleanBlockTempDirectory(root string) error {
	d := filepath.Join(filepath.Clean(root), BlockTempDirectoryPrefix)
	if err := os.RemoveAll(d); err != nil {
		return errors.Wrap(err, "remove block temp directory")
	}

	return nil
}

func RemoveBlockFromLocalFS(root string, height base.Height) (bool, error) {
	heightdirectory := filepath.Join(root, HeightDirectory(height))

	switch _, err := os.Stat(heightdirectory); {
	case errors.Is(err, os.ErrNotExist):
		return false, errors.WithMessagef(err, "height directory, %q does not exist", heightdirectory)
	case err != nil:
		return false, errors.WithMessagef(err, "check height directory, %q", heightdirectory)
	default:
		if err := os.RemoveAll(heightdirectory); err != nil {
			return false, errors.WithMessagef(err, "remove %q", heightdirectory)
		}
	}

	if err := os.Remove(BlockItemFilesPath(root, height)); err != nil {
		return false, errors.WithMessagef(err, "files.json")
	}

	return true, nil
}

func RemoveBlocksFromLocalFS(root string, height base.Height) (bool, error) {
	switch {
	case height < base.GenesisHeight:
		return false, nil
	case height < base.GenesisHeight+1:
		if err := os.RemoveAll(root); err != nil {
			return false, errors.WithMessage(err, "clean localfs")
		}

		return true, nil
	}

	var top base.Height

	switch i, found, err := FindHighestDirectory(root); {
	case err != nil:
		return false, err
	case !found:
		return false, nil
	default:
		rel, err := filepath.Rel(root, i)
		if err != nil {
			return false, errors.WithStack(err)
		}

		switch h, err := HeightFromDirectory(rel); {
		case err != nil:
			return false, err
		case height > h:
			return false, nil
		default:
			top = h
		}
	}

	for i := top; i >= height; i-- {
		if removed, err := RemoveBlockFromLocalFS(root, i); err != nil {
			return removed, err
		}
	}

	return true, nil
}

func fileExtFromEncoder(hinttype string) string {
	switch {
	case strings.Contains(strings.ToLower(hinttype), "json"):
		return ".json"
	default:
		return ".b" // NOTE means b(ytes)
	}
}

func listFileExtFromEncoder(hinttype string) string {
	switch {
	case strings.Contains(strings.ToLower(hinttype), "json"):
		return ".ndjson"
	default:
		return ".blist"
	}
}

func isListBlockMapItemType(t base.BlockItemType) bool {
	switch t {
	case base.BlockItemOperations,
		base.BlockItemOperationsTree,
		base.BlockItemStates,
		base.BlockItemStatesTree,
		base.BlockItemVoteproofs:
		return true
	default:
		return false
	}
}

func isCompressedBlockMapItemType(t base.BlockItemType) bool {
	switch t {
	case base.BlockItemProposal,
		base.BlockItemOperations,
		base.BlockItemOperationsTree,
		base.BlockItemStates,
		base.BlockItemStatesTree:
		return true
	default:
		return false
	}
}

func marshalIndexedTreeNode(enc encoder.Encoder, index uint64, n fixedtree.Node) ([]byte, error) {
	b, err := enc.Marshal(n)
	if err != nil {
		return nil, err
	}

	return util.ConcatBytesSlice([]byte(fmt.Sprintf("%d,", index)), b), nil
}

type indexedTreeNode struct {
	Node  fixedtree.Node
	Index uint64
}

func unmarshalIndexedTreeNode(enc encoder.Encoder, b []byte, ht hint.Hint) (in indexedTreeNode, _ error) {
	e := util.StringError("unmarshal indexed tree node")

	bf := bytes.NewBuffer(b)
	defer bf.Reset()

	switch i, err := bf.ReadBytes(','); {
	case err != nil:
		return in, e.Wrap(err)
	case len(i) < 2:
		return in, e.Errorf("find index string")
	default:
		index, err := strconv.ParseUint(string(i[:len(i)-1]), 10, 64)
		if err != nil {
			return in, e.Wrap(err)
		}

		in.Index = index
	}

	left, err := io.ReadAll(bf)
	if err != nil {
		return in, e.Wrap(err)
	}

	switch i, err := enc.DecodeWithHint(left, ht); {
	case err != nil:
		return in, err
	default:
		j, ok := i.(fixedtree.Node)
		if !ok {
			return in, errors.Errorf("expected fixedtree.Node, but %T", i)
		}

		in.Node = j

		return in, nil
	}
}

func writeBaseHeader(f io.Writer, hs interface{}) error {
	switch b, err := util.MarshalJSON(hs); {
	case err != nil:
		return err
	default:
		var s strings.Builder
		_, _ = s.WriteString("# ")
		_, _ = s.WriteString(string(b))
		_, _ = s.WriteString("\n")

		if _, err := f.Write([]byte(s.String())); err != nil {
			return errors.WithStack(err)
		}

		return nil
	}
}

type baseItemsHeader struct {
	Writer  hint.Hint `json:"writer"`
	Encoder hint.Hint `json:"encoder"`
}

type countItemsHeader struct {
	baseItemsHeader
	Count uint64 `json:"count"`
}

type treeItemsHeader struct {
	Tree hint.Hint `json:"tree"`
	countItemsHeader
}

func writeCountHeader(f io.Writer, writer, enc hint.Hint, count uint64) error {
	return writeBaseHeader(f, countItemsHeader{
		baseItemsHeader: baseItemsHeader{Writer: writer, Encoder: enc},
		Count:           count,
	})
}

func writeTreeHeader(f io.Writer, writer, enc hint.Hint, count uint64, tree hint.Hint) error {
	return writeBaseHeader(f, treeItemsHeader{
		countItemsHeader: countItemsHeader{
			baseItemsHeader: baseItemsHeader{Writer: writer, Encoder: enc},
			Count:           count,
		},
		Tree: tree,
	})
}

func readItemsHeader(f io.Reader) (br *bufio.Reader, _ []byte, _ error) {
	if i, ok := f.(*bufio.Reader); ok {
		br = i
	} else {
		br = bufio.NewReader(f)
	}

	for {
		var iseof bool
		var b []byte

		switch i, err := br.ReadBytes('\n'); {
		case errors.Is(err, io.EOF):
			iseof = true
		case err != nil:
			return br, nil, errors.WithStack(err)
		default:
			b = i
		}

		if !bytes.HasPrefix(b, []byte("# ")) {
			if iseof {
				return br, nil, io.EOF
			}

			continue
		}

		return br, b[2:], nil
	}
}

func loadItemsHeader(f io.Reader, v interface{}) (br *bufio.Reader, _ error) {
	switch br, b, err := readItemsHeader(f); {
	case err != nil:
		return br, err
	default:
		if err := util.UnmarshalJSON(b, v); err != nil {
			return br, err
		}

		return br, nil
	}
}

func loadBaseHeader(f io.Reader) (
	_ *bufio.Reader,
	writer, enc hint.Hint,
	_ error,
) {
	var u baseItemsHeader

	switch i, err := loadItemsHeader(f, &u); {
	case err != nil:
		return nil, writer, enc, err
	default:
		return i, u.Writer, u.Encoder, nil
	}
}

func loadCountHeader(f io.Reader) (
	_ *bufio.Reader,
	writer, enc hint.Hint,
	_ uint64,
	_ error,
) {
	var u countItemsHeader

	switch i, err := loadItemsHeader(f, &u); {
	case err != nil:
		return nil, writer, enc, 0, err
	default:
		return i, u.Writer, u.Encoder, u.Count, nil
	}
}

func loadTreeHeader(f io.Reader) (
	_ *bufio.Reader,
	writer, enc hint.Hint,
	_ uint64,
	tree hint.Hint,
	_ error,
) {
	var u treeItemsHeader

	switch i, err := loadItemsHeader(f, &u); {
	case err != nil:
		return nil, writer, enc, 0, tree, err
	default:
		return i, u.Writer, u.Encoder, u.Count, u.Tree, nil
	}
}
