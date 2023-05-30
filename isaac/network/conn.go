package isaacnetwork

import (
	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/isaac"
	"github.com/ProtoconNet/mitum2/network/quicmemberlist"
	"github.com/ProtoconNet/mitum2/util"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/ProtoconNet/mitum2/util/hint"
)

var NodeConnInfoHint = hint.MustNewHint("node-conninfo-v0.0.1")

type NodeConnInfo struct {
	quicmemberlist.NamedConnInfo
	base.BaseNode
}

func NewNodeConnInfo(node base.BaseNode, addr string, tlsinsecure bool) NodeConnInfo {
	node.BaseHinter = node.BaseHinter.SetHint(NodeConnInfoHint).(hint.BaseHinter) //nolint:forcetypeassert //...

	return NodeConnInfo{
		BaseNode:      node,
		NamedConnInfo: quicmemberlist.NewNamedConnInfo(addr, tlsinsecure),
	}
}

func NewNodeConnInfoFromMemberlistNode(node quicmemberlist.Node) NodeConnInfo {
	return NewNodeConnInfo(
		isaac.NewNode(node.Publickey(), node.Address()),
		node.Publish().Addr().String(),
		node.Publish().TLSInsecure(),
	)
}

func (n NodeConnInfo) IsValid([]byte) error {
	e := util.ErrInvalid.Errorf("invalid NodeConnInfo")

	if err := n.BaseNode.BaseHinter.IsValid(NodeConnInfoHint.Type().Bytes()); err != nil {
		return e.Wrap(err)
	}

	if err := n.BaseNode.IsValid(nil); err != nil {
		return e.Wrap(err)
	}

	if err := n.NamedConnInfo.IsValid(nil); err != nil {
		return e.Wrap(err)
	}

	return nil
}

type connInfoJSONMarshaler struct {
	ConnInfo quicmemberlist.NamedConnInfo `json:"conn_info"`
}

func (n NodeConnInfo) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(struct {
		connInfoJSONMarshaler
		base.BaseNodeJSONMarshaler
		hint.BaseHinter
	}{
		BaseHinter: n.BaseHinter,
		BaseNodeJSONMarshaler: base.BaseNodeJSONMarshaler{
			Address:   n.BaseNode.Address(),
			Publickey: n.BaseNode.Publickey(),
		},
		connInfoJSONMarshaler: connInfoJSONMarshaler{
			ConnInfo: n.NamedConnInfo,
		},
	})
}

func (n *NodeConnInfo) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("failed to decode NodeConnInfo")

	if err := n.BaseNode.DecodeJSON(b, enc); err != nil {
		return e(err, "")
	}

	var u connInfoJSONMarshaler

	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e(err, "")
	}

	n.NamedConnInfo = u.ConnInfo

	return nil
}
