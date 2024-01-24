//go:build test && redis
// +build test,redis

package isaacdatabase

import (
	"context"
	"testing"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	redisstorage "github.com/ProtoconNet/mitum2/storage/redis"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
)

type testRedisPermanent struct {
	testCommonPermanent
}

func TestRedisPermanent(tt *testing.T) {
	t := new(testRedisPermanent)

	mredis := miniredis.RunT(tt)
	defer mredis.Close()

	moptions := &redis.Options{
		Network: "tcp",
		Addr:    mredis.Addr(),
	}

	t.newDB = func() isaac.PermanentDatabase {
		st, err := redisstorage.NewStorage(context.Background(), moptions, util.UUID().String())
		t.NoError(err)

		db, err := NewRedisPermanent(st, t.Encs, t.Enc, 0)
		t.NoError(err)

		return db
	}

	t.newFromDB = func(db isaac.PermanentDatabase) (isaac.PermanentDatabase, error) {
		return NewRedisPermanent(db.(*RedisPermanent).st, t.Encs, t.Enc, 0)
	}

	t.setState = func(perm isaac.PermanentDatabase, st base.State) error {
		db := perm.(*RedisPermanent)

		e := util.StringError("failed to set state")

		b, err := EncodeFrameState(db.enc, st)
		if err != nil {
			return e.Wrap(err)
		}

		if err := db.st.Set(context.TODO(), redisStateKey(st.Key()), b); err != nil {
			return e.WithMessage(err, "failed to put state")
		}

		return nil
	}

	suite.Run(tt, t)
}
