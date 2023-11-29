package isaac

import (
	"context"
	"io"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
)

type BlockWriter interface {
	SetOperationsSize(uint64)
	SetProcessResult(
		_ context.Context,
		index uint64,
		ophash, facthash util.Hash,
		instate bool,
		errorreason base.OperationProcessReasonError,
	) error
	SetStates(_ context.Context, index uint64, values []base.StateMergeValue, operation base.Operation) error
	Manifest(_ context.Context, previous base.Manifest) (base.Manifest, error)
	SetINITVoteproof(context.Context, base.INITVoteproof) error
	SetACCEPTVoteproof(context.Context, base.ACCEPTVoteproof) error
	Save(context.Context) (base.BlockMap, error)
	Cancel() error
}

type BlockReader interface {
	BlockMap() (base.BlockMap, bool, error)
	Reader(base.BlockItemType) (io.ReadCloser, bool, error)
	ChecksumReader(base.BlockItemType) (util.ChecksumReader, bool, error)
	Item(base.BlockItemType) (interface{}, bool, error)
	Items(func(base.BlockMapItem, interface{}, bool, error) bool) error
}

type BlockImporter interface {
	WriteMap(base.BlockMap) error
	WriteItem(base.BlockItemType, *util.CompressedReader) error
	Save(context.Context) (func(context.Context) error, error)
	CancelImport(context.Context) error
	Reader() (BlockReader, error)
}
