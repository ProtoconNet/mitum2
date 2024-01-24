package isaacnetwork

import (
	"net"
	"testing"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/network"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/stretchr/testify/suite"
)

type testNodeConnInfo struct {
	isaac.BaseTestBallots
}

func (t *testNodeConnInfo) TestNew() {
	node := base.NewBaseNode(isaac.NodeHint, base.NewMPrivatekey().Publickey(), base.RandomAddress("local-"))

	nci, err := NewNodeConnInfo(node, "127.0.0.1:1234", true)
	t.NoError(err)

	t.NoError(nci.IsValid(nil))

	_ = (interface{})(nci).(base.Node)
	_ = (interface{})(nci).(network.ConnInfo)
}

func (t *testNodeConnInfo) TestInvalid() {
	t.Run("wrong ip", func() {
		node := base.NewBaseNode(isaac.NodeHint, base.NewMPrivatekey().Publickey(), base.RandomAddress("local-"))

		_, err := NewNodeConnInfo(node, "1.2.3.500:1234", true)
		t.Error(err)

		var dnserr *net.DNSError
		t.ErrorAs(err, &dnserr)
	})

	t.Run("dns error", func() {
		node := base.NewBaseNode(isaac.NodeHint, base.NewMPrivatekey().Publickey(), base.RandomAddress("local-"))

		_, err := NewNodeConnInfo(node, "a.b.c.d:1234", true)
		t.Error(err)

		var dnserr *net.DNSError
		t.ErrorAs(err, &dnserr)
	})

	t.Run("empty host", func() {
		node := base.NewBaseNode(isaac.NodeHint, base.NewMPrivatekey().Publickey(), base.RandomAddress("local-"))

		_, err := NewNodeConnInfo(node, ":1234", true)
		t.Error(err)
		t.ErrorIs(err, util.ErrInvalid)
	})

	t.Run("empty port", func() {
		node := base.NewBaseNode(isaac.NodeHint, base.NewMPrivatekey().Publickey(), base.RandomAddress("local-"))

		_, err := NewNodeConnInfo(node, "a.b.c.d", true)
		t.Error(err)
		t.ErrorIs(err, util.ErrInvalid)
	})
}

func TestNodeConnInfo(t *testing.T) {
	suite.Run(t, new(testNodeConnInfo))
}

func TestNodeConnInfoEncode(t *testing.T) {
	tt := new(encoder.BaseTestEncode)

	enc := jsonenc.NewEncoder()

	tt.Encode = func() (interface{}, []byte) {
		tt.NoError(enc.Add(encoder.DecodeDetail{Hint: base.StringAddressHint, Instance: base.StringAddress{}}))
		tt.NoError(enc.Add(encoder.DecodeDetail{Hint: base.MPublickeyHint, Instance: &base.MPublickey{}}))
		tt.NoError(enc.Add(encoder.DecodeDetail{Hint: NodeConnInfoHint, Instance: NodeConnInfo{}}))

		node := base.RandomNode()

		nc, err := NewNodeConnInfo(node, "1.2.3.4:4321", true)
		tt.NoError(err)
		_ = (interface{})(nc).(isaac.NodeConnInfo)

		b, err := enc.Marshal(nc)
		tt.NoError(err)

		tt.T().Log("marshaled:", string(b))

		return nc, b
	}
	tt.Decode = func(b []byte) interface{} {
		var u NodeConnInfo

		tt.NoError(encoder.Decode(enc, b, &u))

		return u
	}
	tt.Compare = func(a interface{}, b interface{}) {
		ap := a.(NodeConnInfo)
		bp := b.(NodeConnInfo)

		tt.True(base.IsEqualNode(ap, bp))

		tt.Equal(ap.String(), bp.String())
	}

	suite.Run(t, tt)
}
