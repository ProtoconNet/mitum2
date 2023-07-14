package isaacdatabase

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/storage"
	leveldbstorage "github.com/spikeekips/mitum/storage/leveldb"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/hint"
	"github.com/spikeekips/mitum/util/localtime"
	leveldbutil "github.com/syndtr/goleveldb/leveldb/util"
)

var (
	leveldbLabelBlockWrite = []byte{0x01, 0x01}
	leveldbLabelPermanent  = []byte{0x01, 0x02}
	leveldbLabelPool       = []byte{0x01, 0x03}
	leveldbLabelSyncPool   = []byte{0x01, 0x04}
)

var (
	leveldbKeyPrefixState                   = []byte{0x02, 0x01}
	leveldbKeyPrefixInStateOperation        = []byte{0x02, 0x02}
	leveldbKeyPrefixKnownOperation          = []byte{0x02, 0x03}
	leveldbKeyPrefixProposal                = []byte{0x02, 0x04}
	leveldbKeyPrefixProposalByPoint         = []byte{0x02, 0x05}
	leveldbKeyPrefixBlockMap                = []byte{0x02, 0x06}
	leveldbKeyPrefixNewOperation            = []byte{0x02, 0x07}
	leveldbKeyPrefixNewOperationOrdered     = []byte{0x02, 0x08}
	leveldbKeyPrefixNewOperationOrderedKeys = []byte{0x02, 0x09}
	leveldbKeyPrefixRemovedNewOperation     = []byte{0x02, 0x0a}
	leveldbKeyTempSyncMap                   = []byte{0x02, 0x0c}
	leveldbKeySuffrageProof                 = []byte{0x02, 0x0d}
	leveldbKeySuffrageProofByBlockHeight    = []byte{0x02, 0x0e}
	leveldbKeySuffrageExpelOperation        = []byte{0x02, 0x0f}
	leveldbKeyTempMerged                    = []byte{0x02, 0x10}
	leveldbKeyPrefixBallot                  = []byte{0x02, 0x11}
)

type baseLeveldb struct {
	pst *leveldbstorage.PrefixStorage
	*baseDatabase
	sync.RWMutex
}

func newBaseLeveldb(
	pst *leveldbstorage.PrefixStorage,
	encs *encoder.Encoders,
	enc encoder.Encoder,
) *baseLeveldb {
	return &baseLeveldb{
		baseDatabase: newBaseDatabase(encs, enc),
		pst:          pst,
	}
}

func (db *baseLeveldb) st() (*leveldbstorage.PrefixStorage, error) {
	db.RLock()
	defer db.RUnlock()

	if db.pst == nil {
		return nil, storage.ErrClosed.WithStack()
	}

	return db.pst, nil
}

func (db *baseLeveldb) Prefix() []byte {
	switch pst, err := db.st(); {
	case err != nil:
		return nil
	default:
		return pst.Prefix()
	}
}

func (db *baseLeveldb) Close() error {
	db.Lock()
	defer db.Unlock()

	if db.pst == nil {
		return nil
	}

	if err := db.pst.Close(); err != nil {
		return errors.Wrap(err, "close baseDatabase")
	}

	db.pst = nil
	db.encs = nil
	db.enc = nil

	return nil
}

func (db *baseLeveldb) Remove() error {
	pst, err := db.st()
	if err != nil {
		return err
	}

	return pst.Remove()
}

func (db *baseLeveldb) existsInStateOperation(h util.Hash) (bool, error) {
	pst, err := db.st()
	if err != nil {
		return false, err
	}

	switch found, err := pst.Exists(leveldbInStateOperationKey(h)); {
	case err == nil:
		return found, nil
	default:
		return false, errors.Wrap(err, "check exists instate operation")
	}
}

func (db *baseLeveldb) existsKnownOperation(h util.Hash) (bool, error) {
	pst, err := db.st()
	if err != nil {
		return false, err
	}

	switch found, err := pst.Exists(leveldbKnownOperationKey(h)); {
	case err == nil:
		return found, nil
	default:
		return false, errors.Wrap(err, "check exists known operation")
	}
}

func (db *baseLeveldb) loadLastBlockMap() (m base.BlockMap, enchint hint.Hint, meta []byte, body []byte, err error) {
	e := util.StringError("load last blockmap")

	pst, err := db.st()
	if err != nil {
		return nil, enchint, nil, nil, e.Wrap(err)
	}

	if err = pst.Iter(
		leveldbutil.BytesPrefix(leveldbKeyPrefixBlockMap),
		func(_, b []byte) (bool, error) {
			enchint, meta, body, err = db.readHeader(b)
			if err != nil {
				return false, err
			}

			if err = db.readHinterWithEncoder(enchint, body, &m); err != nil {
				return false, err
			}

			return false, nil
		},
		false,
	); err != nil {
		return nil, enchint, nil, nil, e.Wrap(err)
	}

	return m, enchint, meta, body, nil
}

func (db *baseLeveldb) loadNetworkPolicy() (base.NetworkPolicy, bool, error) {
	e := util.StringError("load suffrage state")

	pst, err := db.st()
	if err != nil {
		return nil, false, e.Wrap(err)
	}

	b, found, err := pst.Get(leveldbStateKey(isaac.NetworkPolicyStateKey))

	switch {
	case err != nil:
		return nil, false, e.Wrap(err)
	case !found:
		return nil, false, nil
	case len(b) < 1:
		return nil, false, nil
	}

	var st base.State
	if err := db.readHinter(b, &st); err != nil {
		return nil, true, e.Wrap(err)
	}

	if !base.IsNetworkPolicyState(st) {
		return nil, true, e.Errorf("not NetworkPolicy state")
	}

	return st.Value().(base.NetworkPolicyStateValue).Policy(), true, nil //nolint:forcetypeassert //...
}

func leveldbStateKey(key string) []byte {
	return util.ConcatBytesSlice(leveldbKeyPrefixState, []byte(key))
}

func leveldbInStateOperationKey(h util.Hash) []byte {
	return util.ConcatBytesSlice(leveldbKeyPrefixInStateOperation, h.Bytes())
}

func leveldbKnownOperationKey(h util.Hash) []byte {
	return util.ConcatBytesSlice(leveldbKeyPrefixKnownOperation, h.Bytes())
}

func leveldbProposalKey(h util.Hash) []byte {
	return util.ConcatBytesSlice(leveldbKeyPrefixProposal, h.Bytes())
}

func leveldbProposalPointKey(point base.Point, proposer base.Address, previousBlock util.Hash) []byte {
	var pb, bb []byte
	if proposer != nil {
		pb = proposer.Bytes()
	}

	if previousBlock != nil {
		bb = previousBlock.Bytes()
	}

	return util.ConcatBytesSlice(
		leveldbKeyPrefixProposalByPoint,
		point.Bytes(),
		[]byte("-"),
		pb,
		bb,
	)
}

func leveldbBlockMapKey(height base.Height) []byte {
	return util.ConcatBytesSlice(
		leveldbKeyPrefixBlockMap,
		height.Bytes(),
	)
}

func leveldbNewOperationOrderedKey(operationhash util.Hash) []byte {
	return util.ConcatBytesSlice(
		leveldbKeyPrefixNewOperationOrdered,
		util.Int64ToBytes(localtime.Now().UnixNano()),
		operationhash.Bytes(),
	)
}

func leveldbNewOperationOrderedKeyPrefix(prefix []byte) []byte {
	return util.ConcatBytesSlice(
		leveldbKeyPrefixNewOperationOrdered,
		prefix,
	)
}

func leveldbNewOperationKeysKey(operationhash util.Hash) []byte {
	return util.ConcatBytesSlice(
		leveldbKeyPrefixNewOperationOrderedKeys,
		operationhash.Bytes(),
	)
}

func leveldbNewOperationKey(operationhash util.Hash) []byte {
	return util.ConcatBytesSlice(leveldbKeyPrefixNewOperation, operationhash.Bytes())
}

func leveldbRemovedNewOperationPrefixWithHeight(height base.Height) []byte {
	return util.ConcatBytesSlice(
		leveldbKeyPrefixRemovedNewOperation,
		height.Bytes(),
	)
}

func leveldbRemovedNewOperationKey(height base.Height, operationhash util.Hash) []byte {
	return util.ConcatBytesSlice(
		leveldbRemovedNewOperationPrefixWithHeight(height),
		operationhash.Bytes(),
	)
}

func leveldbTempSyncMapKey(height base.Height) []byte {
	return util.ConcatBytesSlice(
		leveldbKeyTempSyncMap,
		height.Bytes(),
	)
}

func leveldbSuffrageProofKey(suffrageheight base.Height) []byte {
	return util.ConcatBytesSlice(
		leveldbKeySuffrageProof,
		suffrageheight.Bytes(),
	)
}

func leveldbSuffrageProofByBlockHeightKey(height base.Height) []byte {
	return util.ConcatBytesSlice(
		leveldbKeySuffrageProofByBlockHeight,
		height.Bytes(),
	)
}

func leveldbSuffrageExpelOperation(fact base.SuffrageExpelFact) []byte {
	return util.ConcatBytesSlice(leveldbKeySuffrageExpelOperation, fact.ExpelEnd().Bytes(), fact.Hash().Bytes())
}

func leveldbBallotKey(point base.StagePoint, isSuffrageConfirm bool) []byte { // revive:disable-line:flag-parameter
	s := []byte("-")
	if isSuffrageConfirm {
		s = []byte("+")
	}

	return util.ConcatBytesSlice(leveldbKeyPrefixBallot, point.Bytes(), s)
}

func heightFromleveldbKey(b, prefix []byte) (base.Height, error) {
	e := util.StringError("parse height from leveldbBlockMapKey")

	if len(b) < len(prefix)+8 {
		return base.NilHeight, e.Errorf("too short")
	}

	h, err := base.ParseHeightBytes(b[len(prefix) : len(prefix)+8])
	if err != nil {
		return base.NilHeight, e.Wrap(err)
	}

	return h, nil
}

func leveldbTempMergedKey(height base.Height) []byte {
	return util.ConcatBytesSlice(
		leveldbKeyTempMerged,
		height.Bytes(),
	)
}

func offsetFromLeveldbOperationOrderedKey(b []byte) ([]byte, error) {
	switch l := len(leveldbKeyPrefixNewOperationOrdered); {
	case len(b) <= l:
		return nil, errors.Errorf("not enough")
	default:
		return b[l : l+8], nil
	}
}

func offsetRangeLeveldbOperationOrderedKey(offset []byte) *leveldbutil.Range {
	r := leveldbutil.BytesPrefix(leveldbKeyPrefixNewOperationOrdered)

	if offset == nil {
		return r
	}

	start := leveldbutil.BytesPrefix(leveldbNewOperationOrderedKeyPrefix(offset)).Limit

	limit := make([]byte, len(start))
	copy(limit, leveldbutil.BytesPrefix(leveldbKeyPrefixNewOperationOrdered).Limit)

	r = &leveldbutil.Range{
		Start: start,
		Limit: limit,
	}

	return r
}
