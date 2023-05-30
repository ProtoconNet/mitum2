package isaacdatabase

import (
	"bytes"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/storage"
	leveldbstorage "github.com/ProtoconNet/mitum2/storage/leveldb"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/pkg/errors"
	leveldbutil "github.com/syndtr/goleveldb/leveldb/util"
)

type TempLeveldb struct {
	*baseLeveldb
	mp        base.BlockMap // NOTE last blockmap
	sufst     base.State    // NOTE last suffrage state
	policy    base.NetworkPolicy
	proof     base.SuffrageProof
	proofmeta []byte
	proofbody []byte
	mpmeta    []byte // NOTE last blockmap bytes
	mpbody    []byte // NOTE last blockmap bytes
}

func NewTempLeveldbFromPrefix(
	st *leveldbstorage.Storage,
	prefix []byte,
	encs *encoder.Encoders,
	enc encoder.Encoder,
) (*TempLeveldb, error) {
	pst := leveldbstorage.NewPrefixStorage(st, prefix)

	db := &TempLeveldb{
		baseLeveldb: newBaseLeveldb(pst, encs, enc),
	}

	if err := db.loadLastBlockMap(); err != nil {
		return nil, err
	}

	if err := db.loadSuffrageState(); err != nil {
		return nil, err
	}

	if err := db.loadSuffrageProof(); err != nil {
		return nil, err
	}

	if err := db.loadNetworkPolicy(); err != nil {
		return nil, err
	}

	return db, nil
}

func newTempLeveldbFromBlockWriteStorage(wst *LeveldbBlockWrite) (*TempLeveldb, error) {
	e := util.StringErrorFunc("failed new TempLeveldbDatabase from TempLeveldbDatabase")

	var mp base.BlockMap
	var mpmeta, mpb []byte

	switch i, meta, j := wst.blockmaps(); {
	case i == nil:
		return nil, e(nil, "empty blockmap")
	default:
		mp = i
		mpmeta = meta
		mpb = j
	}

	var proof base.SuffrageProof
	var proofmeta, proofbody []byte

	if i, meta, j := wst.proofs(); i != nil {
		proof = i
		proofmeta = meta
		proofbody = j
	}

	sufst := wst.SuffrageState()
	policy := wst.NetworkPolicy()

	return &TempLeveldb{
		baseLeveldb: wst.baseLeveldb,
		mp:          mp,
		mpmeta:      mpmeta,
		mpbody:      mpb,
		sufst:       sufst,
		policy:      policy,
		proof:       proof,
		proofmeta:   proofmeta,
		proofbody:   proofbody,
	}, nil
}

func (db *TempLeveldb) Close() error {
	if err := db.baseLeveldb.Close(); err != nil {
		return err
	}

	db.Lock()
	defer db.Unlock()

	db.clean()

	return nil
}

func (db *TempLeveldb) Remove() error {
	db.Lock()
	defer db.Unlock()

	if db.mp == nil {
		return nil
	}

	if err := db.baseLeveldb.Remove(); err != nil {
		return err
	}

	db.clean()

	return nil
}

func (db *TempLeveldb) Merge() error {
	if err := db.st.Put(leveldbTempMergedKey(db.Height()), nil, nil); err != nil {
		return err
	}

	r := &leveldbutil.Range{
		Start: emptyPrefixStoragePrefixByHeight(leveldbLabelBlockWrite, db.Height()),   //nolint:gomnd //...
		Limit: emptyPrefixStoragePrefixByHeight(leveldbLabelBlockWrite, db.Height()+1), //nolint:gomnd //...
	}

	var lastprefix []byte
	var useless [][]byte

	if err := db.st.Iter(
		r,
		func(key, _ []byte) (bool, error) {
			switch k, err := prefixStoragePrefixFromKey(key); {
			case err != nil:
			case bytes.Equal(k, db.Prefix()):
			case bytes.Equal(k, lastprefix):
			default:
				lastprefix = k

				useless = append(useless, k)
			}

			return true, nil
		},
		false,
	); err != nil {
		return err
	}

	if len(useless) < 1 {
		return nil
	}

	for i := range useless {
		if err := leveldbstorage.RemoveByPrefix(db.st.Storage, useless[i]); err != nil {
			return err
		}
	}

	return nil
}

func (db *TempLeveldb) clean() {
	db.mp = nil
	db.mpmeta = nil
	db.mpbody = nil
	db.sufst = nil
	db.policy = nil
	db.proof = nil
	db.proofmeta = nil
	db.proofbody = nil
}

func (db *TempLeveldb) Height() base.Height {
	if db.mp == nil {
		return base.NilHeight
	}

	return db.mp.Manifest().Height()
}

func (db *TempLeveldb) SuffrageHeight() base.Height {
	if db.sufst == nil {
		return base.NilHeight
	}

	return db.sufst.Value().(base.SuffrageNodesStateValue).Height() //nolint:forcetypeassert //...
}

func (db *TempLeveldb) LastBlockMap() (base.BlockMap, bool, error) {
	if db.mp == nil {
		return nil, false, storage.ErrNotFound.Errorf("blockmap not found")
	}

	return db.mp, true, nil
}

func (db *TempLeveldb) BlockMapBytes() (enchint hint.Hint, meta, body []byte, _ error) {
	return db.enc.Hint(), db.mpmeta, db.mpbody, nil //nolint:forcetypeassert //...
}

func (db *TempLeveldb) SuffrageProof() (base.SuffrageProof, bool, error) {
	if db.proof == nil {
		return nil, false, nil
	}

	return db.proof, true, nil
}

func (db *TempLeveldb) LastSuffrageProofBytes() (enchint hint.Hint, meta, body []byte, found bool, err error) {
	if db.proof == nil {
		return enchint, nil, nil, false, nil
	}

	return db.enc.Hint(), db.proofmeta, db.proofbody, true, nil
}

func (db *TempLeveldb) NetworkPolicy() base.NetworkPolicy {
	return db.policy
}

func (db *TempLeveldb) State(key string) (st base.State, found bool, err error) {
	found, err = db.getRecord(leveldbStateKey(key), db.st.Get, &st)

	return st, found, err
}

func (db *TempLeveldb) StateBytes(key string) (enchint hint.Hint, meta, body []byte, found bool, err error) {
	return db.getRecordBytes(leveldbStateKey(key), db.st.Get)
}

func (db *TempLeveldb) ExistsInStateOperation(h util.Hash) (bool, error) {
	return db.existsInStateOperation(h)
}

func (db *TempLeveldb) ExistsKnownOperation(h util.Hash) (bool, error) {
	return db.existsKnownOperation(h)
}

func (db *TempLeveldb) isMerged() (bool, error) {
	return db.st.Exists(leveldbTempMergedKey(db.Height()))
}

func (db *TempLeveldb) loadLastBlockMap() error {
	switch m, enchint, meta, body, err := db.baseLeveldb.loadLastBlockMap(); {
	case err != nil:
		return err
	case m == nil:
		return util.ErrNotFound.Errorf("last BlockMap not found")
	default:
		enc := db.encs.Find(enchint)
		if enc == nil {
			return errors.Errorf("encoder not found, %q", enchint)
		}

		db.enc = enc
		db.mp = m
		db.mpmeta = meta
		db.mpbody = body

		return nil
	}
}

func (db *TempLeveldb) loadSuffrageState() error {
	e := util.StringErrorFunc("failed to load suffrage state")

	switch b, found, err := db.st.Get(leveldbStateKey(isaac.SuffrageStateKey)); {
	case err != nil:
		return e(err, "")
	case !found:
		return nil
	default:
		st, err := db.decodeSuffrage(b)
		if err != nil {
			return e(err, "")
		}

		db.sufst = st

		return nil
	}
}

func (db *TempLeveldb) loadSuffrageProof() error {
	e := util.StringErrorFunc("failed to load SuffrageProof")

	if err := db.st.Iter(
		leveldbutil.BytesPrefix(leveldbKeySuffrageProof),
		func(_ []byte, b []byte) (bool, error) {
			enchint, meta, body, err := db.readHeader(b)
			if err != nil {
				return false, err
			}

			var proof base.SuffrageProof
			if err := db.readHinterWithEncoder(enchint, body, &proof); err != nil {
				return false, err
			}

			db.proof = proof
			db.proofmeta = meta
			db.proofbody = body

			return false, nil
		},
		false,
	); err != nil {
		return e(err, "")
	}

	return nil
}

func (db *TempLeveldb) loadNetworkPolicy() error {
	switch policy, found, err := db.baseLeveldb.loadNetworkPolicy(); {
	case err != nil:
		return err
	case !found:
		return nil
	default:
		db.policy = policy

		return nil
	}
}
