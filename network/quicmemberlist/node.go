package quicmemberlist

import (
	"encoding/json"
	"net"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/network/quicstream"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/hint"
	"github.com/spikeekips/mitum/util/localtime"
)

var (
	NodeHint     = hint.MustNewHint("memberlist-node-v0.0.1")
	NodeMetaHint = hint.MustNewHint("memberlist-node-meta-v0.0.1")
)

type Node interface {
	util.IsValider
	quicstream.ConnInfo
	Name() string
	Address() base.Address
	Publickey() base.Publickey
	PublishConnInfo() quicstream.ConnInfo
	JoinedAt() time.Time
	Meta() NodeMeta
	MetaBytes() []byte
}

type BaseNode struct {
	joinedAt time.Time
	addr     *net.UDPAddr
	name     string
	metab    []byte
	hint.BaseHinter
	publishconninfo quicstream.ConnInfo
	meta            NodeMeta
}

func NewNode(name string, addr *net.UDPAddr, meta NodeMeta) (BaseNode, error) {
	metab, err := util.MarshalJSON(meta)
	if err != nil {
		return BaseNode{}, errors.WithMessage(err, "failed to create Node")
	}

	return BaseNode{
		BaseHinter: hint.NewBaseHinter(NodeHint),
		name:       name,
		addr:       addr,
		joinedAt:   localtime.UTCNow(),
		meta:       meta,
		metab:      metab,
	}, nil
}

func newNodeFromMemberlist(node *memberlist.Node, enc encoder.Encoder) (BaseNode, error) {
	e := util.StringErrorFunc("failed to make Node from memberlist.Node")

	var meta NodeMeta

	if err := encoder.Decode(enc, node.Meta, &meta); err != nil {
		return BaseNode{}, e(err, "failed to decode NodeMeta")
	}

	addr, _ := convertNetAddr(node)

	return NewNode(node.Name, addr.(*net.UDPAddr), meta) //nolint:forcetypeassert // ...
}

func (n BaseNode) IsValid([]byte) error {
	e := util.ErrInvalid.Errorf("invalid BaseNode")

	if err := n.BaseHinter.IsValid(NodeHint.Type().Bytes()); err != nil {
		return e.Wrap(err)
	}

	if err := util.CheckIsValid(nil, false,
		util.DummyIsValider(func([]byte) error {
			if n.joinedAt.IsZero() {
				return errors.Errorf("empty joined at time")
			}

			return nil
		}),
		util.DummyIsValider(func([]byte) error {
			if len(n.name) < 1 {
				return errors.Errorf("empty name")
			}

			return nil
		}),
		util.DummyIsValider(func([]byte) error {
			if len(n.metab) < 1 {
				return errors.Errorf("empty meta")
			}

			return nil
		}),
		n.meta,
	); err != nil {
		return e.Wrap(err)
	}

	return nil
}

func (n BaseNode) String() string {
	return n.publishconninfo.String()
}

func (n BaseNode) Name() string {
	return n.name
}

func (n BaseNode) Addr() net.Addr {
	return n.addr
}

func (n BaseNode) UDPAddr() *net.UDPAddr {
	return n.addr
}

func (n BaseNode) TLSInsecure() bool {
	return n.meta.TLSInsecure()
}

func (n BaseNode) JoinedAt() time.Time {
	return n.joinedAt
}

func (n BaseNode) Meta() NodeMeta {
	return n.meta
}

func (n BaseNode) Address() base.Address {
	return n.meta.Address()
}

func (n BaseNode) Publickey() base.Publickey {
	return n.meta.Publickey()
}

func (n *BaseNode) CheckPublishConnInfo() (quicstream.ConnInfo, error) {
	var err error

	if n.publishconninfo == nil {
		switch addr, eerr := net.ResolveUDPAddr("udp", n.meta.Publish()); {
		case err != nil:
			err = errors.Wrap(eerr, "invalid publish address")

			n.publishconninfo = quicstream.NewBaseConnInfo(nil, false)
		default:
			n.publishconninfo = quicstream.NewBaseConnInfo(addr, n.meta.TLSInsecure())
		}
	}

	return n.publishconninfo, err
}

func (n BaseNode) PublishConnInfo() quicstream.ConnInfo {
	return n.publishconninfo
}

func (n BaseNode) MetaBytes() []byte {
	return n.metab
}

func (n BaseNode) MarshalZerologObject(e *zerolog.Event) {
	e.
		Str("name", n.name).
		Stringer("address", n.meta.Address()).
		Stringer("address", n.addr).
		Bool("tls_insecure", n.meta.TLSInsecure()).
		Time("joined_at", n.joinedAt)
}

func (n BaseNode) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string
		Address  string
		JoinedAt time.Time `json:"joined_at"`
		Meta     NodeMeta
	}{
		Name:     n.name,
		Address:  n.addr.String(),
		JoinedAt: n.joinedAt,
		Meta:     n.meta,
	})
}

func (n *BaseNode) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	var u struct {
		Name     string
		Address  string
		JoinedAt time.Time `json:"joined_at"`
		Meta     json.RawMessage
	}

	e := util.StringErrorFunc("failed to unmarshal Node")
	if err := json.Unmarshal(b, &u); err != nil {
		return e(err, "")
	}

	var meta NodeMeta
	if err := meta.DecodeJSON(u.Meta, enc); err != nil {
		return e(err, "failed to decode NodeMeta")
	}

	addr, err := net.ResolveUDPAddr("udp", u.Address)
	if err != nil {
		return e(err, "")
	}

	n.name = u.Name
	n.addr = addr
	n.joinedAt = u.JoinedAt
	n.meta = meta

	return nil
}

type NodeMeta struct {
	address   base.Address
	publickey base.Publickey
	publish   string
	hint.BaseHinter
	tlsinsecure bool
}

func NewNodeMeta(address base.Address, publickey base.Publickey, publish string, tlsinsecure bool) NodeMeta {
	return NodeMeta{
		BaseHinter:  hint.NewBaseHinter(NodeMetaHint),
		address:     address,
		publickey:   publickey,
		publish:     publish,
		tlsinsecure: tlsinsecure,
	}
}

func (n NodeMeta) IsValid([]byte) error {
	e := util.ErrInvalid.Errorf("invalid NodeMeta")

	if err := n.BaseHinter.IsValid(NodeMetaHint.Type().Bytes()); err != nil {
		return e.Wrap(err)
	}

	if err := util.CheckIsValid(nil, false,
		n.address,
		n.publickey,
		util.DummyIsValider(func([]byte) error {
			if len(n.publish) < 1 {
				return errors.Errorf("empty publish")
			}

			switch host, port, err := net.SplitHostPort(n.publish); {
			case err != nil:
				return errors.WithStack(err)
			case len(host) < 1:
				return errors.Errorf("empty host")
			case len(port) < 1:
				return errors.Errorf("empty port")
			}

			return nil
		}),
	); err != nil {
		return e.Wrap(err)
	}

	return nil
}

func (n NodeMeta) Address() base.Address {
	return n.address
}

func (n NodeMeta) Publickey() base.Publickey {
	return n.publickey
}

func (n NodeMeta) Publish() string {
	return n.publish
}

func (n NodeMeta) TLSInsecure() bool {
	return n.tlsinsecure
}

type nodeMetaJSONMmarshaler struct {
	Address   base.Address   `json:"address"`
	Publickey base.Publickey `json:"publickey"`
	Publish   string         `json:"publish"`
	hint.BaseHinter
	TLSInsecure bool `json:"tls_insecure"`
}

func (n NodeMeta) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(nodeMetaJSONMmarshaler{
		BaseHinter:  n.BaseHinter,
		Address:     n.address,
		Publickey:   n.publickey,
		Publish:     n.publish,
		TLSInsecure: n.tlsinsecure,
	})
}

type nodeMetaJSONUnmarshaler struct {
	Address     string `json:"address"`
	Publickey   string `json:"publickey"`
	Publish     string `json:"publish"`
	TLSInsecure bool   `json:"tls_insecure"`
}

func (n *NodeMeta) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("failed to decode NodeMta")

	var u nodeMetaJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e(err, "")
	}

	switch i, err := base.DecodeAddress(u.Address, enc); {
	case err != nil:
		return e(err, "failed to decode node")
	default:
		n.address = i
	}

	switch i, err := base.DecodePublickeyFromString(u.Publickey, enc); {
	case err != nil:
		return e(err, "failed to decode publickey")
	default:
		n.publickey = i
	}

	n.publish = u.Publish
	n.tlsinsecure = u.TLSInsecure

	return nil
}
