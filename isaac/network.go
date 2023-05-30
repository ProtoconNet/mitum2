package isaac

import (
	"context"
	"io"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/network"
	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util"
)

// revive:disable:line-length-limit
type NetworkClient interface { //nolint:interfacebloat //..
	Request(context.Context, quicstream.UDPConnInfo, NetworkHeader, io.Reader) (NetworkResponseHeader, interface{}, func() error, error)
	Operation(_ context.Context, _ quicstream.UDPConnInfo, operationhash util.Hash) (base.Operation, bool, error)
	SendOperation(context.Context, quicstream.UDPConnInfo, base.Operation) (bool, error)
	RequestProposal(_ context.Context, connInfo quicstream.UDPConnInfo, point base.Point, propser base.Address) (base.ProposalSignFact, bool, error)
	Proposal(_ context.Context, connInfo quicstream.UDPConnInfo, facthash util.Hash) (base.ProposalSignFact, bool, error)
	LastSuffrageProof(_ context.Context, connInfo quicstream.UDPConnInfo, state util.Hash) (lastheight base.Height, _ base.SuffrageProof, updated bool, _ error)
	SuffrageProof(_ context.Context, connInfo quicstream.UDPConnInfo, suffrageheight base.Height) (_ base.SuffrageProof, found bool, _ error)
	LastBlockMap(_ context.Context, _ quicstream.UDPConnInfo, manifest util.Hash) (_ base.BlockMap, updated bool, _ error)
	BlockMap(context.Context, quicstream.UDPConnInfo, base.Height) (_ base.BlockMap, updated bool, _ error)
	BlockMapItem(context.Context, quicstream.UDPConnInfo, base.Height, base.BlockMapItemType) (io.ReadCloser, func() error, bool, error)
	NodeChallenge(_ context.Context, _ quicstream.UDPConnInfo, _ base.NetworkID, _ base.Address, _ base.Publickey, input []byte) (base.Signature, error)
	SuffrageNodeConnInfo(context.Context, quicstream.UDPConnInfo) ([]NodeConnInfo, error)
	SyncSourceConnInfo(context.Context, quicstream.UDPConnInfo) ([]NodeConnInfo, error)
	State(_ context.Context, _ quicstream.UDPConnInfo, key string, _ util.Hash) (base.State, bool, error)
	ExistsInStateOperation(_ context.Context, _ quicstream.UDPConnInfo, facthash util.Hash) (bool, error)
	SendBallots(context.Context, quicstream.UDPConnInfo, []base.BallotSignFact) error
}

// revive:enable:line-length-limit

type NetworkHeader interface {
	util.IsValider
	HandlerPrefix() string
}

type NetworkResponseContentType string

var (
	NetworkResponseHinterContentType NetworkResponseContentType
	NetworkResponseRawContentType    NetworkResponseContentType = "raw"
)

type NetworkResponseHeader interface {
	NetworkHeader
	Err() error
	OK() bool
	Type() NetworkResponseContentType
}

type NodeConnInfo interface {
	base.Node
	network.ConnInfo
	UDPConnInfo() (quicstream.UDPConnInfo, error)
}
