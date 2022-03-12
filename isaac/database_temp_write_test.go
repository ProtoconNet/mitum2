package isaac

import (
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/storage"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/valuehash"
	"github.com/stretchr/testify/suite"
)

type testWODatabase struct {
	baseTestHandler
	baseTestDatabase
}

func (t *testWODatabase) SetupTest() {
	t.baseTestHandler.SetupTest()
	t.baseTestDatabase.SetupTest()
}

func (t *testWODatabase) TestNew() {
	t.Run("valid", func() {
		wst, err := NewTempWODatabase(base.Height(33), t.root, t.encs, t.enc)
		t.NoError(err)

		_ = (interface{})(wst).(BlockWriteDatabase)
	})

	t.Run("root exists", func() {
		_, err := NewTempWODatabase(base.Height(33), t.root, t.encs, t.enc)
		t.Error(err)
		t.Contains(err.Error(), "failed batch leveldb storage")
	})
}

func (t *testWODatabase) TestSetManifest() {
	height := base.Height(33)

	wst := t.newMemWO(height)
	defer wst.Close()

	m := base.NewDummyManifest(height, valuehash.RandomSHA256())

	t.NoError(wst.SetManifest(m))
	t.NoError(wst.Write())

	rst, err := wst.TempDatabase()
	t.NoError(err)

	t.Run("Manifest", func() {
		rm, err := rst.Manifest()
		t.NoError(err)

		base.EqualManifest(t.Assert(), m, rm)
	})
}

func (t *testWODatabase) TestSetStates() {
	height := base.Height(33)
	_, nodes := t.locals(3)

	sv := NewSuffrageStateValue(
		base.Height(33),
		valuehash.RandomSHA256(),
		nodes,
	)

	_ = (interface{})(sv).(base.SuffrageStateValue)

	sufstt := base.NewBaseState(
		height,
		util.UUID().String(),
		sv,
	)
	sufstt.SetOperations([]util.Hash{valuehash.RandomSHA256(), valuehash.RandomSHA256(), valuehash.RandomSHA256()})

	stts := t.states(height, 3)
	stts = append(stts, sufstt)

	m := base.NewDummyManifest(height, valuehash.RandomSHA256())

	wst := t.newMemWO(height)
	defer wst.Close()

	t.NoError(wst.SetManifest(m))
	t.NoError(wst.SetStates(stts))
	t.NoError(wst.Write())

	rst, err := wst.TempDatabase()
	t.NoError(err)

	t.Run("check suffrage", func() {
		rstt, found, err := rst.Suffrage()
		t.NotNil(rstt)
		t.True(found)
		t.NoError(err)

		t.True(base.IsEqualState(sufstt, rstt))
	})

	t.Run("check states", func() {
		for i := range stts {
			stt := stts[i]

			rstt, found, err := rst.State(stt.Key())
			t.NotNil(rstt)
			t.True(found)
			t.NoError(err)

			t.True(base.IsEqualState(stt, rstt))
		}
	})

	t.Run("check unknown states", func() {
		rstt, found, err := rst.State(util.UUID().String())
		t.Nil(rstt)
		t.False(found)
		t.NoError(err)
	})
}

func (t *testWODatabase) TestSetOperations() {
	wst := t.newMemWO(base.Height(33))
	defer wst.Close()

	ops := make([]util.Hash, 33)
	for i := range ops {
		ops[i] = valuehash.RandomSHA256()
	}

	t.NoError(wst.SetOperations(ops))

	m := base.NewDummyManifest(base.Height(33), valuehash.RandomSHA256())
	t.NoError(wst.SetManifest(m))
	t.NoError(wst.Write())

	rst, err := wst.TempDatabase()
	t.NoError(err)

	t.Run("check operation exists", func() {
		for i := range ops {
			found, err := rst.ExistsOperation(ops[i])
			t.NoError(err)
			t.True(found)
		}
	})

	t.Run("check unknown operation", func() {
		found, err := rst.ExistsOperation(valuehash.RandomSHA256())
		t.NoError(err)
		t.False(found)
	})
}

func (t *testWODatabase) TestRemove() {
	height := base.Height(33)

	wst := t.newWO(height)
	defer wst.Close()

	t.T().Log("check root directory created")
	fi, err := os.Stat(t.root)
	t.NoError(err)
	t.True(fi.IsDir())

	m := base.NewDummyManifest(height, valuehash.RandomSHA256())

	t.NoError(wst.SetManifest(m))
	t.NoError(wst.Write())

	t.NoError(wst.Remove())

	t.T().Log("check root directory removed")
	_, err = os.Stat(t.root)
	t.True(os.IsNotExist(err))

	t.T().Log("remove again")
	err = wst.Remove()
	t.True(errors.Is(err, storage.ConnectionError))
}

func TestWODatabase(t *testing.T) {
	suite.Run(t, new(testWODatabase))
}