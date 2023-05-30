package isaacdatabase

import (
	"github.com/ProtoconNet/mitum2/base"
	leveldbstorage "github.com/ProtoconNet/mitum2/storage/leveldb"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/pkg/errors"
	leveldbutil "github.com/syndtr/goleveldb/leveldb/util"
)

type LeveldbTempSyncPool struct {
	*baseLeveldb
}

func NewLeveldbTempSyncPool(
	height base.Height,
	st *leveldbstorage.Storage,
	encs *encoder.Encoders,
	enc encoder.Encoder,
) (*LeveldbTempSyncPool, error) {
	return newLeveldbTempSyncPool(height, st, encs, enc), nil
}

func newLeveldbTempSyncPool(
	height base.Height,
	st *leveldbstorage.Storage,
	encs *encoder.Encoders,
	enc encoder.Encoder,
) *LeveldbTempSyncPool {
	pst := leveldbstorage.NewPrefixStorage(st, newPrefixStoragePrefixByHeight(leveldbLabelSyncPool, height))

	return &LeveldbTempSyncPool{
		baseLeveldb: newBaseLeveldb(pst, encs, enc),
	}
}

func (db *LeveldbTempSyncPool) BlockMap(height base.Height) (m base.BlockMap, found bool, _ error) {
	switch b, found, err := db.st.Get(leveldbTempSyncMapKey(height)); {
	case err != nil:
		return nil, false, err
	case !found:
		return nil, false, nil
	case len(b) < 1:
		return nil, false, nil
	default:
		if err := db.readHinter(b, &m); err != nil {
			return nil, false, err
		}

		return m, true, nil
	}
}

func (db *LeveldbTempSyncPool) SetBlockMap(m base.BlockMap) error {
	b, _, err := db.marshal(m, nil)
	if err != nil {
		return err
	}

	return db.st.Put(leveldbTempSyncMapKey(m.Manifest().Height()), b, nil)
}

func (db *LeveldbTempSyncPool) Cancel() error {
	e := util.StringErrorFunc("failed to cancel temp sync pool")

	if err := func() error {
		db.Lock()
		defer db.Unlock()

		if db.st == nil {
			return nil
		}

		r := leveldbutil.BytesPrefix(db.st.Prefix())

		_, err := leveldbstorage.BatchRemove(db.st.Storage, r, 333) //nolint:gomnd //...

		return err
	}(); err != nil {
		return e(err, "")
	}

	if err := db.Close(); err != nil {
		return e(err, "")
	}

	return nil
}

func CleanSyncPool(st *leveldbstorage.Storage) error {
	r := leveldbutil.BytesPrefix(leveldbLabelSyncPool)

	if _, err := leveldbstorage.BatchRemove(st, r, 333); err != nil { //nolint:gomnd //...
		return errors.WithMessage(err, "failed to clean syncpool database")
	}

	return nil
}
