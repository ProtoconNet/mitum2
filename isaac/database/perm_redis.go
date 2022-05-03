package isaacdatabase

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	redisstorage "github.com/spikeekips/mitum/storage/redis"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	"github.com/spikeekips/mitum/util/valuehash"
	leveldbutil "github.com/syndtr/goleveldb/leveldb/util"
)

var (
	redisSuffrageKeyPrerfix         = "sf"
	redisSuffrageByHeightKeyPrerfix = "sh"
	redisStateKeyPrerfix            = "st"
	redisInStateOperationKeyPrerfix = "ip"
	redisKnownOperationKeyPrerfix   = "kp"
	redisBlockDataMapKeyPrefix      = "mp"
)

var (
	redisZKeySuffragesByHeight = "suffrages_by_height"
	redisZBeginSuffrages       = redisSuffrageKey(base.GenesisHeight)
	redisZEndSuffrages         = fmt.Sprintf("%s-%s", redisSuffrageKeyPrerfix, strings.Repeat("9", 20))
	redisZKeyBlockDataMaps     = "blockdatamaps"
	redisZBeginBlockDataMaps   = redisBlockDataMapKey(base.GenesisHeight)
	redisZEndBlockDataMaps     = fmt.Sprintf("%s-%s", redisBlockDataMapKeyPrefix, strings.Repeat("9", 20))
)

type RedisPermanent struct {
	sync.Mutex
	*baseDatabase
	*basePermanent
	st *redisstorage.Storage
}

func NewRedisPermanent(
	st *redisstorage.Storage,
	encs *encoder.Encoders,
	enc encoder.Encoder,
) (*RedisPermanent, error) {
	db := &RedisPermanent{
		baseDatabase: newBaseDatabase(
			encs,
			enc,
		),
		basePermanent: newBasePermanent(),
		st:            st,
	}

	if err := db.loadLastBlockDataMap(); err != nil {
		return nil, err
	}

	if err := db.loadLastSuffrage(); err != nil {
		return nil, err
	}

	if err := db.loadNetworkPolicy(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *RedisPermanent) Close() error {
	if err := db.st.Close(); err != nil {
		return errors.Wrap(err, "failed to close RedisPermanentDatabase")
	}

	return nil
}

func (db *RedisPermanent) Clean() error {
	if err := db.st.Clean(context.Background()); err != nil {
		return errors.Wrap(err, "failed to clean redis PermanentDatabase")
	}

	return nil
}

func (db *RedisPermanent) Suffrage(height base.Height) (base.State, bool, error) {
	e := util.StringErrorFunc("failed to get suffrage by block height")

	switch m, found, err := db.LastMap(); {
	case err != nil:
		return nil, false, e(err, "")
	case !found:
		return nil, false, nil
	case height > m.Manifest().Height():
		return nil, false, nil
	}

	switch st, found, err := db.LastSuffrage(); {
	case err != nil:
		return nil, false, e(err, "")
	case !found:
		return nil, false, nil
	case height == st.Height():
		return st, true, nil
	}

	var key string
	if err := db.st.ZRangeArgs(
		context.Background(),
		redis.ZRangeArgs{
			Key:   redisZKeySuffragesByHeight,
			Start: "[" + redisZBeginSuffrages,
			Stop:  "[" + redisSuffrageKey(height),
			ByLex: true,
			Rev:   true,
			Count: 1,
		},
		func(i string) (bool, error) {
			key = i

			return false, nil
		},
	); err != nil {
		return nil, false, e(err, "")
	}

	if len(key) < 1 {
		return nil, false, nil
	}

	switch b, found, err := db.st.Get(context.Background(), key); {
	case err != nil:
		return nil, false, err
	case !found:
		return nil, false, nil
	default:
		st, err := db.decodeSuffrage(b)
		if err != nil {
			return nil, false, e(err, "")
		}
		return st, true, nil
	}
}

func (db *RedisPermanent) SuffrageByHeight(suffrageHeight base.Height) (base.State, bool, error) {
	e := util.StringErrorFunc("failed to get suffrage by height")

	switch st, found, err := db.LastSuffrage(); {
	case err != nil:
		return nil, false, e(err, "")
	case !found:
		return nil, false, nil
	case suffrageHeight > st.Value().(base.SuffrageStateValue).Height():
		return nil, false, nil
	case suffrageHeight == st.Value().(base.SuffrageStateValue).Height():
		return st, true, nil
	}

	switch b, found, err := db.st.Get(context.Background(), redisSuffrageByHeightKey(suffrageHeight)); {
	case err != nil:
		return nil, false, err
	case !found:
		return nil, false, nil
	default:
		st, err := db.decodeSuffrage(b)
		if err != nil {
			return nil, false, e(err, "")
		}
		return st, true, nil
	}
}

func (db *RedisPermanent) State(key string) (base.State, bool, error) {
	e := util.StringErrorFunc("failed to get state")

	switch b, found, err := db.st.Get(context.Background(), redisStateKey(key)); {
	case err != nil:
		return nil, false, e(err, "")
	case !found:
		return nil, false, nil
	default:
		i, err := db.decodeState(b)
		if err != nil {
			return nil, false, e(err, "")
		}

		return i, true, nil
	}
}

func (db *RedisPermanent) ExistsInStateOperation(h util.Hash) (bool, error) {
	e := util.StringErrorFunc("failed to check instate operation")

	switch found, err := db.st.Exists(context.Background(), redisInStateOperationKey(h)); {
	case err != nil:
		return false, e(err, "")
	default:
		return found, nil
	}
}

func (db *RedisPermanent) ExistsKnownOperation(h util.Hash) (bool, error) {
	e := util.StringErrorFunc("failed to check known operation")

	switch found, err := db.st.Exists(context.Background(), redisKnownOperationKey(h)); {
	case err != nil:
		return false, e(err, "")
	default:
		return found, nil
	}
}

func (db *RedisPermanent) Map(height base.Height) (base.BlockDataMap, bool, error) {
	e := util.StringErrorFunc("failed to load blockdatamap")

	switch m, found, err := db.LastMap(); {
	case err != nil:
		return nil, false, e(err, "")
	case found:
		return m, true, nil
	}

	switch b, found, err := db.st.Get(context.Background(), redisBlockDataMapKey(height)); {
	case err != nil:
		return nil, false, e(err, "")
	case !found:
		return nil, false, nil
	default:
		m, err := db.decodeBlockDataMap(b)
		if err != nil {
			return nil, false, e(err, "")
		}

		return m, true, nil
	}
}

func (db *RedisPermanent) MergeTempDatabase(ctx context.Context, temp isaac.TempDatabase) error {
	db.Lock()
	defer db.Unlock()

	if i, _ := db.mp.Value(); i != nil && i.(base.BlockDataMap).Manifest().Height() >= temp.Height() {
		return nil
	}

	e := util.StringErrorFunc("failed to merge TempDatabase")

	switch t := temp.(type) {
	case *TempLeveldb:
		mp, sufstt, err := db.mergeTempDatabaseFromLeveldb(ctx, t)
		if err != nil {
			return e(err, "")
		}

		_ = db.mp.SetValue(mp)
		if sufstt != nil {
			_ = db.sufstt.SetValue(sufstt)
		}

		if t.policy != nil {
			_ = db.policy.SetValue(t.policy)
		}

		return nil
	default:
		return e(nil, "unknown temp database, %T", temp)
	}
}

func (db *RedisPermanent) mergeTempDatabaseFromLeveldb(ctx context.Context, temp *TempLeveldb) (
	base.BlockDataMap, base.State, error,
) {
	e := util.StringErrorFunc("failed to merge LeveldbTempDatabase")

	var mp base.BlockDataMap
	switch i, err := temp.Map(); {
	case err != nil:
		return nil, nil, e(err, "")
	default:
		mp = i
	}

	var sufstt base.State
	var sufsv base.SuffrageStateValue
	switch st, found, err := temp.Suffrage(); {
	case err != nil:
		return nil, nil, e(err, "")
	case found:
		sufstt = st
		sufsv = st.Value().(base.SuffrageStateValue)
	}

	worker := util.NewErrgroupWorker(ctx, math.MaxInt32)
	defer worker.Close()

	// NOTE merge operations
	if err := worker.NewJob(func(ctx context.Context, jobid uint64) error {
		if err := db.mergeOperationsTempDatabaseFromLeveldb(ctx, temp); err != nil {
			return errors.Wrap(err, "failed to merge operations")
		}

		return nil
	}); err != nil {
		return nil, nil, e(err, "")
	}

	// NOTE merge states
	if err := worker.NewJob(func(ctx context.Context, jobid uint64) error {
		bsufst, err := db.mergeStatesTempDatabaseFromLeveldb(ctx, temp)
		if err != nil {
			return errors.Wrap(err, "failed to merge states")
		}

		// NOTE merge suffrage state
		if sufsv != nil && len(bsufst) > 0 {
			if err := db.mergeSuffrageStateTempDatabaseFromLeveldb(ctx, temp, sufsv, bsufst); err != nil {
				return errors.Wrap(err, "failed to merge suffrage state")
			}
		}

		return nil
	}); err != nil {
		return nil, nil, e(err, "")
	}

	// NOTE merge blockdatamap
	if err := worker.NewJob(func(ctx context.Context, jobid uint64) error {
		if err := db.mergeBlockDataMapTempDatabaseFromLeveldb(ctx, temp); err != nil {
			return errors.Wrap(err, "failed to merge blockdatamap")
		}

		return nil
	}); err != nil {
		return nil, nil, e(err, "")
	}

	worker.Done()
	if err := worker.Wait(); err != nil {
		return nil, nil, e(err, "")
	}

	return mp, sufstt, nil
}

func (db *RedisPermanent) mergeOperationsTempDatabaseFromLeveldb(
	ctx context.Context, temp *TempLeveldb,
) error {
	e := util.StringErrorFunc("failed to merge operations from temp")

	if err := temp.st.Iter(
		leveldbutil.BytesPrefix(leveldbKeyPrefixInStateOperation),
		func(_, b []byte) (bool, error) {
			if err := db.st.Set(ctx, redisInStateOperationKey(valuehash.Bytes(b)), b); err != nil {
				return false, err
			}

			return true, nil
		}, true); err != nil {
		return e(err, "")
	}

	if err := temp.st.Iter(
		leveldbutil.BytesPrefix(leveldbKeyPrefixKnownOperation),
		func(_, b []byte) (bool, error) {
			if err := db.st.Set(ctx, redisKnownOperationKey(valuehash.Bytes(b)), b); err != nil {
				return false, err
			}

			return true, nil
		}, true); err != nil {
		return e(err, "")
	}

	return nil
}

func (db *RedisPermanent) mergeStatesTempDatabaseFromLeveldb(
	ctx context.Context, temp *TempLeveldb,
) ([]byte, error) {
	var bsufst []byte
	if err := temp.st.Iter(
		leveldbutil.BytesPrefix(leveldbKeyPrefixState),
		func(key, b []byte) (bool, error) {
			if err := db.st.Set(ctx, redisStateKeyFromLeveldb(key), b); err != nil {
				return false, err
			}

			if bytes.Equal(key, leveldbSuffrageStateKey) {
				bsufst = b
			}

			return true, nil
		}, true); err != nil {
		return nil, err
	}

	return bsufst, nil
}

func (db *RedisPermanent) mergeSuffrageStateTempDatabaseFromLeveldb(
	ctx context.Context,
	temp *TempLeveldb,
	sufsv base.SuffrageStateValue,
	bsufst []byte,
) error {
	z := redis.ZAddArgs{
		NX:      true,
		Members: []redis.Z{{Score: 0, Member: redisSuffrageKey(temp.Height())}},
	}
	if err := db.st.ZAddArgs(ctx, redisZKeySuffragesByHeight, z); err != nil {
		return errors.Wrap(err, "failed to zadd suffrage by block height")
	}

	if err := db.st.Set(ctx, redisSuffrageKey(temp.Height()), bsufst); err != nil {
		return errors.Wrap(err, "failed to set suffrage")
	}

	if err := db.st.Set(ctx, redisSuffrageByHeightKey(sufsv.Height()), bsufst); err != nil {
		return errors.Wrap(err, "failed to set suffrage by height")
	}

	return nil
}

func (db *RedisPermanent) mergeBlockDataMapTempDatabaseFromLeveldb(
	ctx context.Context, temp *TempLeveldb,
) error {
	switch b, found, err := temp.st.Get(leveldbKeyPrefixBlockDataMap); {
	case err != nil || !found:
		return errors.Wrap(err, "failed to get blockdatamap from TempDatabase")
	default:
		key := redisBlockDataMapKey(temp.Height())
		z := redis.ZAddArgs{
			NX:      true,
			Members: []redis.Z{{Score: 0, Member: key}},
		}
		if err := db.st.ZAddArgs(ctx, redisZKeyBlockDataMaps, z); err != nil {
			return errors.Wrap(err, "failed to zadd blockdatamap by block height")
		}

		if err := db.st.Set(ctx, key, b); err != nil {
			return errors.Wrap(err, "failed to set blockdatamap")
		}

		return nil
	}
}

func (db *RedisPermanent) loadLastBlockDataMap() error {
	e := util.StringErrorFunc("failed to load last blockdatamap")

	b, found, err := db.loadLast(redisZKeyBlockDataMaps, redisZBeginBlockDataMaps, redisZEndBlockDataMaps)

	switch {
	case err != nil:
		return e(err, "")
	case !found:
		return nil
	default:
		m, err := db.decodeBlockDataMap(b)
		if err != nil {
			return e(err, "")
		}

		_ = db.mp.SetValue(m)
	}

	return nil
}

func (db *RedisPermanent) loadLastSuffrage() error {
	e := util.StringErrorFunc("failed to load last suffrage state")

	b, found, err := db.loadLast(redisZKeySuffragesByHeight, redisZBeginSuffrages, redisZEndSuffrages)

	switch {
	case err != nil:
		return e(err, "")
	case !found:
		return nil
	default:
		sufstt, err := db.decodeSuffrage(b)
		if err != nil {
			return e(err, "")
		}
		_ = db.sufstt.SetValue(sufstt)

		return nil
	}
}

func (db *RedisPermanent) loadNetworkPolicy() error {
	e := util.StringErrorFunc("failed to load last network policy")

	switch st, found, err := db.State(isaac.NetworkPolicyStateKey); {
	case err != nil:
		return e(err, "")
	case !found:
		return nil
	default:
		if !base.IsNetworkPolicyState(st) {
			return e(nil, "not NetworkPolicy state: %T", st)
		}

		_ = db.policy.SetValue(st.Value().(base.NetworkPolicyStateValue).Policy())

		return nil
	}
}

func (db *RedisPermanent) loadLast(zkey, begin, end string) ([]byte, bool, error) {
	var key string
	if err := db.st.ZRangeArgs(
		context.Background(),
		redis.ZRangeArgs{
			Key:   zkey,
			Start: "[" + begin,
			Stop:  "[" + end,
			ByLex: true,
			Rev:   true,
			Count: 1,
		},
		func(i string) (bool, error) {
			key = i

			return false, nil
		},
	); err != nil {
		return nil, false, errors.Wrap(err, "")
	}

	if len(key) < 1 {
		return nil, false, nil
	}

	switch b, found, err := db.st.Get(context.Background(), key); {
	case err != nil:
		return nil, false, errors.Wrap(err, "")
	case !found:
		return nil, false, nil
	default:
		return b, true, nil
	}
}

func redisSuffrageKey(height base.Height) string {
	return fmt.Sprintf("%s-%021d", redisSuffrageKeyPrerfix, height)
}

func redisSuffrageByHeightKey(suffrageheight base.Height) string {
	return fmt.Sprintf("%s-%021d", redisSuffrageByHeightKeyPrerfix, suffrageheight)
}

func redisStateKey(key string) string {
	return fmt.Sprintf("%s-%s", redisStateKeyPrerfix, key)
}

func redisInStateOperationKey(h util.Hash) string {
	return fmt.Sprintf("%s-%s", redisInStateOperationKeyPrerfix, h.String())
}

func redisKnownOperationKey(h util.Hash) string {
	return fmt.Sprintf("%s-%s", redisKnownOperationKeyPrerfix, h.String())
}

func redisStateKeyFromLeveldb(b []byte) string {
	return redisStateKey(string(b[2:]))
}

func redisBlockDataMapKey(height base.Height) string {
	return fmt.Sprintf("%s-%021d", redisBlockDataMapKeyPrefix, height)
}
