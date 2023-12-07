package isaac

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/hint"
	"golang.org/x/sync/singleflight"
)

var (
	LocalFSBlockItemScheme     = "localfs"
	BlockDirectoryHeightFormat = "%021s"
)

type BlockItemReaderFunc func(
	base.Height,
	base.BlockItemType,
	func(BlockItemReader) error,
) (bool, error)

type NewBlockItemReaderFunc func(
	base.BlockItemType,
	encoder.Encoder,
	*util.CompressedReader,
) (BlockItemReader, error)

func BlockItemReadersDecode[T any](
	readers *BlockItemReaders,
	height base.Height,
	t base.BlockItemType,
	f func(ir BlockItemReader) error,
) (target T, _ bool, _ error) {
	irf := blockItemReadersDecodeFunc[T](f)

	switch found, err := readers.Item(height, t, func(ir BlockItemReader) error {
		switch i, err := irf(ir); {
		case err != nil:
			return err
		default:
			target = i

			return nil
		}
	}); {
	case err != nil, !found:
		return target, found, err
	default:
		return target, true, nil
	}
}

func BlockItemReadersDecodeItems[T any](
	readers *BlockItemReaders,
	height base.Height,
	t base.BlockItemType,
	item func(uint64, uint64, T) error,
	f func(ir BlockItemReader) error,
) (count uint64, l []T, found bool, _ error) {
	irf, countf := blockItemReadersDecodeItemsFuncs(item, f)

	switch found, err := readers.Item(height, t, func(ir BlockItemReader) error {
		return irf(ir)
	}); {
	case err != nil, !found:
		return 0, nil, found, err
	default:
		count, l = countf()

		return count, l, true, nil
	}
}

func BlockItemReadersDecodeFromReader[T any](
	readers *BlockItemReaders,
	t base.BlockItemType,
	r io.Reader,
	compressFormat string,
	f func(ir BlockItemReader) error,
) (target T, _ bool, _ error) {
	irf := blockItemReadersDecodeFunc[T](f)

	switch found, err := readers.ItemFromReader(t, r, compressFormat, func(ir BlockItemReader) error {
		switch i, err := irf(ir); {
		case err != nil:
			return err
		default:
			target = i

			return nil
		}
	}); {
	case err != nil, !found:
		return target, found, err
	default:
		return target, true, nil
	}
}

func BlockItemReadersDecodeItemsFromReader[T any](
	readers *BlockItemReaders,
	t base.BlockItemType,
	r io.Reader,
	compressFormat string,
	item func(uint64, uint64, T) error,
	f func(ir BlockItemReader) error,
) (count uint64, l []T, found bool, _ error) {
	irf, countf := blockItemReadersDecodeItemsFuncs(item, f)

	switch found, err := readers.ItemFromReader(t, r, compressFormat, func(ir BlockItemReader) error {
		return irf(ir)
	}); {
	case err != nil, !found:
		return 0, nil, found, err
	default:
		count, l = countf()

		return count, l, true, nil
	}
}

func blockItemReadersDecodeFunc[T any](
	f func(ir BlockItemReader) error,
) func(BlockItemReader) (T, error) {
	if f == nil {
		f = func(ir BlockItemReader) error { return nil } //revive:disable-line:modifies-parameter
	}

	return func(ir BlockItemReader) (target T, _ error) {
		if err := f(ir); err != nil {
			return target, err
		}

		switch i, err := ir.Decode(); {
		case err != nil:
			return target, err
		default:
			v, ok := i.(T)
			if !ok {
				return target, errors.Errorf("expected %T, but %T", target, i)
			}

			return v, nil
		}
	}
}

func blockItemReadersDecodeItemsFuncs[T any](
	item func(uint64, uint64, T) error,
	f func(ir BlockItemReader) error,
) (
	func(ir BlockItemReader) error,
	func() (uint64, []T),
) {
	if item == nil {
		item = func(uint64, uint64, T) error { return nil } //revive:disable-line:modifies-parameter
	}

	if f == nil {
		f = func(ir BlockItemReader) error { return nil } //revive:disable-line:modifies-parameter
	}

	var l []T
	var once sync.Once
	var count uint64

	return func(ir BlockItemReader) error {
			if err := f(ir); err != nil {
				return err
			}

			switch i, err := ir.DecodeItems(func(total, index uint64, v interface{}) error {
				i, ok := v.(T)
				if !ok {
					var target T

					return errors.Errorf("expected %T, but %T", target, v)
				}

				if err := item(total, index, i); err != nil {
					return err
				}

				once.Do(func() {
					l = make([]T, total)
				})

				l[index] = i

				return nil
			}); {
			case err != nil:
				return err
			default:
				count = i

				return nil
			}
		},
		func() (uint64, []T) {
			return count, l[:count]
		}
}

type BlockItemReaders struct {
	*hint.CompatibleSet[NewBlockItemReaderFunc]
	encs             *encoder.Encoders
	bfilescache      *util.GCache[base.Height, base.BlockItemFiles]
	decompressReader util.DecompressReaderFunc
	bfilessg         singleflight.Group
	root             string
}

func NewBlockItemReaders(
	root string,
	encs *encoder.Encoders,
	decompressReader util.DecompressReaderFunc,
) *BlockItemReaders {
	if decompressReader == nil {
		decompressReader = util.DefaultDecompressReaderFunc //revive:disable-line:modifies-parameter
	}

	return &BlockItemReaders{
		CompatibleSet:    hint.NewCompatibleSet[NewBlockItemReaderFunc](8), //nolint:gomnd //...
		root:             root,
		encs:             encs,
		decompressReader: decompressReader,
		bfilescache:      util.NewLFUGCache[base.Height, base.BlockItemFiles](1 << 9), //nolint:gomnd //...
	}
}

func (rs *BlockItemReaders) Root() string {
	return rs.root
}

func (rs *BlockItemReaders) Add(writerhint hint.Hint, v NewBlockItemReaderFunc) error {
	return rs.CompatibleSet.Add(writerhint, v)
}

func (rs *BlockItemReaders) Item(
	height base.Height,
	t base.BlockItemType,
	callback func(BlockItemReader) error,
) (bool, error) {
	var bfile base.BlockItemFile

	switch i, found, err := rs.ItemFile(height, t); {
	case err != nil, !found:
		return found, err
	default:
		bfile = i
	}

	switch f, found, err := rs.readFile(height, bfile); {
	case err != nil, !found:
		return found, err
	default:
		defer func() {
			_ = f.Close()
		}()

		return rs.itemFromReader(t, f, bfile.CompressFormat(), callback)
	}
}

func (rs *BlockItemReaders) ItemFromReader(
	t base.BlockItemType,
	r io.Reader,
	compressFormat string,
	callback func(BlockItemReader) error,
) (bool, error) {
	return rs.itemFromReader(t, r, compressFormat, callback)
}

func (rs *BlockItemReaders) ItemFiles(height base.Height) (base.BlockItemFiles, bool, error) {
	i, err, _ := util.SingleflightDo[[2]interface{}](
		&rs.bfilessg,
		height.String(),
		func() (ii [2]interface{}, _ error) {
			if i, found := rs.bfilescache.Get(height); found {
				return [2]interface{}{i, found}, nil
			}

			switch i, found, err := LoadBlockItemFilesPath(rs.root, height, rs.encs.JSON()); {
			case err != nil:
				return ii, err
			default:
				if found {
					rs.bfilescache.Set(height, i, 0)
				}

				return [2]interface{}{i, found}, nil
			}
		},
	)

	switch {
	case err != nil:
		return nil, false, err
	case !i[1].(bool): //nolint:forcetypeassert //...
		return nil, false, nil
	default:
		return i[0].(base.BlockItemFiles), true, nil //nolint:forcetypeassert //...
	}
}

func (rs *BlockItemReaders) ItemFile(height base.Height, t base.BlockItemType) (base.BlockItemFile, bool, error) {
	switch bfiles, found, err := rs.ItemFiles(height); {
	case err != nil, !found:
		return nil, found, err
	default:
		i, found := bfiles.Item(t)

		return i, found, nil
	}
}

func (rs *BlockItemReaders) readFile(height base.Height, bfile base.BlockItemFile) (*os.File, bool, error) {
	var p string

	switch u := bfile.URI(); u.Scheme {
	case LocalFSBlockItemScheme:
		p = filepath.Join(rs.root, BlockHeightDirectory(height), u.Path)
	case "file":
		p = u.Path
	default:
		return nil, false, nil
	}

	switch i, err := os.Open(p); {
	case os.IsNotExist(err):
		return nil, false, nil
	case err != nil:
		return nil, false, errors.WithStack(err)
	default:
		return i, true, nil
	}
}

func (rs *BlockItemReaders) findBlockReader(f io.Reader, compressFormat string) (
	NewBlockItemReaderFunc,
	encoder.Encoder,
	bool,
	error,
) {
	var dr io.Reader

	switch i, err := rs.decompressReader(compressFormat); {
	case err != nil:
		return nil, nil, false, err
	default:
		j, err := i(f)
		if err != nil {
			return nil, nil, false, err
		}

		dr = j
	}

	switch _, writerhint, enchint, err := LoadBlockItemFileBaseHeader(dr); {
	case errors.Is(err, io.EOF):
		return nil, nil, false, nil
	case err != nil:
		return nil, nil, false, err
	default:
		r, found := rs.Find(writerhint)
		if !found {
			return nil, nil, false, nil
		}

		e, found := rs.encs.Find(enchint)
		if !found {
			return nil, nil, false, nil
		}

		return r, e, true, nil
	}
}

func (rs *BlockItemReaders) itemFromReader(
	t base.BlockItemType,
	r io.Reader,
	compressFormat string,
	callback func(BlockItemReader) error,
) (bool, error) {
	var br io.Reader
	var resetf func() error

	switch rt := r.(type) {
	case io.Seeker:
		br = r
		resetf = func() error {
			_, err := rt.Seek(0, 0)
			return errors.WithStack(err)
		}
	default:
		i := util.NewBufferedResetReader(r)
		defer func() {
			_ = i.Close()
		}()

		resetf = func() error {
			i.Reset()

			return nil
		}

		br = i
	}

	var readerf NewBlockItemReaderFunc
	var enc encoder.Encoder

	switch i, e, found, err := rs.findBlockReader(br, compressFormat); {
	case err != nil, !found:
		return found, err
	default:
		if err := resetf(); err != nil {
			return false, err
		}

		readerf = i
		enc = e
	}

	var cr *util.CompressedReader

	switch i, err := util.NewCompressedReader(br, compressFormat, rs.decompressReader); {
	case err != nil:
		return true, err
	default:
		defer func() {
			_ = i.Close()
		}()

		cr = i
	}

	switch i, err := readerf(t, enc, cr); {
	case err != nil:
		return true, err
	default:
		return true, callback(i)
	}
}

func BlockItemDecodeLineItems(
	f io.Reader,
	decode func([]byte) (interface{}, error),
	callback func(uint64, interface{}) error,
) (count uint64, _ error) {
	var br *bufio.Reader
	if i, ok := f.(*bufio.Reader); ok {
		br = i
	} else {
		br = bufio.NewReader(f)
	}

	var index uint64
end:
	for {
		b, err := br.ReadBytes('\n')

		switch {
		case err != nil && !errors.Is(err, io.EOF):
			return 0, errors.WithStack(err)
		case len(b) < 1,
			bytes.HasPrefix(b, []byte("# ")):
		default:
			v, eerr := decode(b)
			if eerr != nil {
				return 0, errors.WithStack(eerr)
			}

			if eerr := callback(index, v); eerr != nil {
				return 0, eerr
			}

			index++
		}

		switch {
		case err == nil:
		case errors.Is(err, io.EOF):
			break end
		default:
			return 0, errors.WithStack(err)
		}
	}

	return index, nil
}

func BlockItemDecodeLineItemsWithWorker(
	f io.Reader,
	num uint64,
	decode func([]byte) (interface{}, error),
	callback func(uint64, interface{}) error,
) (count uint64, _ error) {
	worker, err := util.NewErrgroupWorker(context.Background(), int64(num))
	if err != nil {
		return 0, err
	}

	defer worker.Close()

	switch i, err := BlockItemDecodeLineItems(f, decode, func(index uint64, v interface{}) error {
		return worker.NewJob(func(ctx context.Context, _ uint64) error {
			return callback(index, v)
		})
	}); {
	case err != nil:
		return 0, err
	default:
		worker.Done()

		return i, worker.Wait()
	}
}

func LoadBlockItemFilesPath(
	root string,
	height base.Height,
	jsonenc encoder.Encoder,
) (bfiles base.BlockItemFiles, found bool, _ error) {
	switch i, err := os.Open(BlockItemFilesPath(root, height)); {
	case os.IsNotExist(err):
		return nil, false, nil
	case err != nil:
		return nil, false, errors.WithStack(err)
	default:
		defer func() {
			_ = i.Close()
		}()

		if err := encoder.DecodeReader(jsonenc, i, &bfiles); err != nil {
			return nil, false, err
		}

		return bfiles, true, nil
	}
}

func BlockHeightDirectory(height base.Height) string {
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

type BlockItemFileBaseItemsHeader struct {
	Writer  hint.Hint `json:"writer"`
	Encoder hint.Hint `json:"encoder"`
}

func LoadBlockItemFileBaseHeader(f io.Reader) (
	_ *bufio.Reader,
	writer, enc hint.Hint,
	_ error,
) {
	var u BlockItemFileBaseItemsHeader

	switch i, err := LoadBlockItemFileHeader(f, &u); {
	case err != nil:
		return nil, writer, enc, err
	default:
		return i, u.Writer, u.Encoder, nil
	}
}

func ReadBlockItemFileHeader(f io.Reader) (br *bufio.Reader, _ []byte, _ error) {
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

func LoadBlockItemFileHeader(f io.Reader, v interface{}) (br *bufio.Reader, _ error) {
	switch br, b, err := ReadBlockItemFileHeader(f); {
	case err != nil:
		return br, err
	default:
		if err := util.UnmarshalJSON(b, v); err != nil {
			return br, err
		}

		return br, nil
	}
}

func BlockItemFilesPath(root string, height base.Height) string {
	return filepath.Join(
		filepath.Dir(filepath.Join(root, BlockHeightDirectory(height))),
		base.BlockItemFilesName(height),
	)
}