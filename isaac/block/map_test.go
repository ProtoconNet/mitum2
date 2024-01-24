package isaacblock

import (
	"testing"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/stretchr/testify/suite"
)

type testBlockMap struct {
	suite.Suite
	local     base.Address
	priv      base.Privatekey
	networkID base.NetworkID
}

func (t *testBlockMap) SetupSuite() {
	t.local = base.RandomAddress("")
	t.priv = base.NewMPrivatekey()
	t.networkID = util.UUID().Bytes()
}

func (t *testBlockMap) newitem(ty base.BlockItemType) BlockMapItem {
	return NewBlockMapItem(ty, util.UUID().String())
}

func (t *testBlockMap) newmap() BlockMap {
	m := NewBlockMap()

	for _, i := range []base.BlockItemType{
		base.BlockItemProposal,
		base.BlockItemOperations,
		base.BlockItemOperationsTree,
		base.BlockItemStates,
		base.BlockItemStatesTree,
		base.BlockItemVoteproofs,
	} {
		t.NoError(m.SetItem(t.newitem(i)))
	}

	manifest := base.NewDummyManifest(base.Height(33), valuehash.RandomSHA256())
	m.SetManifest(manifest)
	t.NoError(m.Sign(t.local, t.priv, t.networkID))

	return m
}

func (t *testBlockMap) TestNew() {
	m := t.newmap()
	_ = (interface{})(m).(base.BlockMap)

	t.NoError(m.IsValid(t.networkID))

	t.NotNil(m.Manifest())
}

func (t *testBlockMap) TestInvalid() {
	m := t.newmap()
	t.NoError(m.IsValid(t.networkID))

	t.Run("invalid hinter", func() {
		m := t.newmap()
		m.BaseHinter = hint.NewBaseHinter(base.StringAddressHint)
		err := m.IsValid(t.networkID)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "type does not match")
	})

	t.Run("invalid manifest", func() {
		m := t.newmap()

		manifest := base.NewDummyManifest(base.Height(33), valuehash.RandomSHA256())
		manifest.Invalidf = func([]byte) error {
			return util.ErrInvalid.Errorf("kikiki")
		}

		m.manifest = manifest

		err := m.IsValid(t.networkID)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "kikiki")
	})

	t.Run("proposal not set", func() {
		m := t.newmap()
		m.items.RemoveValue(base.BlockItemProposal)

		err := m.IsValid(t.networkID)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "empty proposal")
	})

	t.Run("empty proposal", func() {
		m := t.newmap()
		m.items.SetValue(base.BlockItemProposal, nil)

		err := m.IsValid(t.networkID)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "empty proposal")
	})

	t.Run("voteproofs not set", func() {
		m := t.newmap()
		m.items.RemoveValue(base.BlockItemVoteproofs)

		err := m.IsValid(t.networkID)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "empty voteproofs")
	})

	t.Run("empty voteproofs", func() {
		m := t.newmap()
		m.items.SetValue(base.BlockItemVoteproofs, nil)

		err := m.IsValid(t.networkID)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "empty voteproofs")
	})

	t.Run("invalid item", func() {
		m := t.newmap()
		m.items.SetValue(base.BlockItemVoteproofs, t.newitem(base.BlockItemType("hehe")))

		err := m.IsValid(t.networkID)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "invalid item found")
		t.ErrorContains(err, "hehe")
	})

	t.Run("invalid signature", func() {
		m := t.newmap()

		err := m.IsValid(util.UUID().Bytes())
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorIs(err, base.ErrSignatureVerification)
	})
}

func (t *testBlockMap) TestSetItem() {
	m := t.newmap()

	t.Run("override", func() {
		olditem, found := m.Item(base.BlockItemProposal)
		t.True(found)
		t.NotNil(olditem)

		newitem := t.newitem(base.BlockItemProposal)
		t.NoError(m.SetItem(newitem))

		t.NotEqual(olditem.Checksum(), newitem.Checksum())

		ritem, found := m.Item(base.BlockItemProposal)
		t.True(found)
		t.NotNil(ritem)

		base.EqualBlockMapItem(t.Assert(), newitem, ritem)
	})

	t.Run("unknown data type", func() {
		newitem := t.newitem(base.BlockItemType("findme"))
		err := m.SetItem(newitem)

		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "unknown block map item type")
	})
}

func (t *testBlockMap) TestVerify() {
	t.Run("basic", func() {
		m := t.newmap()
		t.NoError(m.IsValid(t.networkID))
	})

	t.Run("update item with same checksum", func() {
		m := t.newmap()
		t.NoError(m.IsValid(t.networkID))

		olditem, found := m.Item(base.BlockItemProposal)
		t.True(found)
		t.NotNil(olditem)

		newitem := NewBlockMapItem(olditem.Type(), olditem.Checksum())
		t.NoError(m.SetItem(newitem))

		t.NoError(m.IsValid(t.networkID))
	})
}

type testBlockMapEncode struct {
	encoder.BaseTestEncode
	enc *jsonenc.Encoder
}

func (t *testBlockMap) TestEncode() {
	tt := new(testBlockMapEncode)

	tt.Encode = func() (interface{}, []byte) {
		tt.enc = jsonenc.NewEncoder()
		t.NoError(tt.enc.Add(encoder.DecodeDetail{Hint: base.MPublickeyHint, Instance: &base.MPublickey{}}))
		t.NoError(tt.enc.Add(encoder.DecodeDetail{Hint: base.StringAddressHint, Instance: base.StringAddress{}}))
		tt.NoError(tt.enc.Add(encoder.DecodeDetail{Hint: base.DummyManifestHint, Instance: base.DummyManifest{}}))
		tt.NoError(tt.enc.Add(encoder.DecodeDetail{Hint: BlockMapHint, Instance: BlockMap{}}))

		m := t.newmap()

		b, err := tt.enc.Marshal(m)
		tt.NoError(err)

		t.T().Log("marshaled:", string(b))

		return m, b
	}
	tt.Decode = func(b []byte) interface{} {
		i, err := tt.enc.Decode(b)
		tt.NoError(err)

		_, ok := i.(BlockMap)
		tt.True(ok)

		return i
	}
	tt.Compare = func(a, b interface{}) {
		af, ok := a.(BlockMap)
		tt.True(ok)
		bf, ok := b.(BlockMap)
		tt.True(ok)

		tt.NoError(bf.IsValid(t.networkID))

		base.EqualBlockMap(tt.Assert(), af, bf)
	}

	suite.Run(t.T(), tt)
}

func TestBlockMap(t *testing.T) {
	suite.Run(t, new(testBlockMap))
}

type testBlockMapItem struct {
	suite.Suite
}

func (t *testBlockMapItem) TestNew() {
	checksum := util.UUID().String()

	item := NewBlockMapItem(base.BlockItemProposal, checksum)
	_ = (interface{})(item).(base.BlockMapItem)

	t.NoError(item.IsValid(nil))

	t.Equal(base.BlockItemProposal, item.Type())
	t.Equal(checksum, item.Checksum())
}

func (t *testBlockMapItem) TestInvalid() {
	t.Run("invalid data type", func() {
		item := NewBlockMapItem(base.BlockItemType("findme"), util.UUID().String())

		err := item.IsValid(nil)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "unknown block map item type")
	})

	t.Run("empty checksum", func() {
		item := NewBlockMapItem(base.BlockItemProposal, "")

		err := item.IsValid(nil)
		t.ErrorIs(err, util.ErrInvalid)
		t.ErrorContains(err, "empty checksum")
	})
}

func TestBlockMapItem(t *testing.T) {
	suite.Run(t, new(testBlockMapItem))
}

type testBlockMapItemEncode struct {
	encoder.BaseTestEncode
	enc *jsonenc.Encoder
}

func (t *testBlockMapItemEncode) SetupTest() {
	t.enc = jsonenc.NewEncoder()
}

func TestBlockMapItemEncode(tt *testing.T) {
	t := new(testBlockMapItemEncode)

	t.Encode = func() (interface{}, []byte) {
		item := NewBlockMapItem(base.BlockItemProposal, util.UUID().String())

		b, err := t.enc.Marshal(item)
		t.NoError(err)

		return item, b
	}
	t.Decode = func(b []byte) interface{} {
		var u BlockMapItem
		t.NoError(t.enc.Unmarshal(b, &u))

		return u
	}
	t.Compare = func(a, b interface{}) {
		af, ok := a.(BlockMapItem)
		t.True(ok)
		bf, ok := b.(BlockMapItem)
		t.True(ok)

		t.NoError(bf.IsValid(nil))

		base.EqualBlockMapItem(t.Assert(), af, bf)
	}

	suite.Run(tt, t)
}
