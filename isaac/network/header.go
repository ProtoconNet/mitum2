package isaacnetwork

import (
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/hint"
)

var (
	RequestProposalRequestHeaderHint   = hint.MustNewHint("request-proposal-header-v0.0.1")
	ProposalRequestHeaderHint          = hint.MustNewHint("proposal-header-v0.0.1")
	LastSuffrageProofRequestHeaderHint = hint.MustNewHint("last-suffrage-proof-header-v0.0.1")
	SuffrageProofRequestHeaderHint     = hint.MustNewHint("suffrage-proof-header-v0.0.1")
	LastBlockMapRequestHeaderHint      = hint.MustNewHint("last-blockmap-header-v0.0.1")
	BlockMapRequestHeaderHint          = hint.MustNewHint("blockmap-header-v0.0.1")
	BlockMapItemRequestHeaderHint      = hint.MustNewHint("blockmap-item-header-v0.0.1")
)

var ResponseHeaderHint = hint.MustNewHint("response-header-v0.0.1")

type BaseHeader struct {
	prefix string
	hint.BaseHinter
}

func NewBaseHeader(ht hint.Hint, prefix string) BaseHeader {
	return BaseHeader{
		BaseHinter: hint.NewBaseHinter(ht),
		prefix:     prefix,
	}
}

func (h BaseHeader) HandlerPrefix() string {
	return h.prefix
}

type RequestProposalRequestHeader struct {
	proposer base.Address
	BaseHeader
	point base.Point
}

func NewRequestProposalRequestHeader(point base.Point, proposer base.Address) RequestProposalRequestHeader {
	return RequestProposalRequestHeader{
		BaseHeader: NewBaseHeader(RequestProposalRequestHeaderHint, HandlerPrefixRequestProposal),
		point:      point,
		proposer:   proposer,
	}
}

func (h RequestProposalRequestHeader) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid RequestProposalHeader")

	if err := h.BaseHinter.IsValid(h.Hint().Type().Bytes()); err != nil {
		return e(err, "")
	}

	if err := util.CheckIsValid(nil, false, h.point, h.proposer); err != nil {
		return e(err, "")
	}

	return nil
}

func (h RequestProposalRequestHeader) Proposer() base.Address {
	return h.proposer
}

func (h RequestProposalRequestHeader) Point() base.Point {
	return h.point
}

type ProposalRequestHeader struct {
	proposal util.Hash
	BaseHeader
}

func NewProposalRequestHeader(proposal util.Hash) ProposalRequestHeader {
	return ProposalRequestHeader{
		BaseHeader: NewBaseHeader(ProposalRequestHeaderHint, HandlerPrefixProposal),
		proposal:   proposal,
	}
}

func (h ProposalRequestHeader) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid ProposalHeader")

	if err := h.BaseHinter.IsValid(h.Hint().Type().Bytes()); err != nil {
		return e(err, "")
	}

	if err := util.CheckIsValid(nil, false, h.proposal); err != nil {
		return e(err, "")
	}

	return nil
}

func (h ProposalRequestHeader) Proposal() util.Hash {
	return h.proposal
}

type LastSuffrageProofRequestHeader struct {
	state util.Hash
	BaseHeader
}

func NewLastSuffrageProofRequestHeader(state util.Hash) LastSuffrageProofRequestHeader {
	return LastSuffrageProofRequestHeader{
		BaseHeader: NewBaseHeader(LastSuffrageProofRequestHeaderHint, HandlerPrefixLastSuffrageProof),
		state:      state,
	}
}

func (h LastSuffrageProofRequestHeader) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid LastSuffrageProofHeader")

	if err := h.BaseHinter.IsValid(h.Hint().Type().Bytes()); err != nil {
		return e(err, "")
	}

	if h.state != nil {
		if err := h.state.IsValid(nil); err != nil {
			return e(err, "")
		}
	}

	return nil
}

func (h LastSuffrageProofRequestHeader) State() util.Hash {
	return h.state
}

type SuffrageProofRequestHeader struct {
	BaseHeader
	suffrageheight base.Height
}

func NewSuffrageProofRequestHeader(suffrageheight base.Height) SuffrageProofRequestHeader {
	return SuffrageProofRequestHeader{
		BaseHeader:     NewBaseHeader(SuffrageProofRequestHeaderHint, HandlerPrefixSuffrageProof),
		suffrageheight: suffrageheight,
	}
}

func (h SuffrageProofRequestHeader) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid SuffrageProofHeader")

	if err := h.BaseHinter.IsValid(h.Hint().Type().Bytes()); err != nil {
		return e(err, "")
	}

	if err := util.CheckIsValid(nil, false, h.suffrageheight); err != nil {
		return e(err, "")
	}

	return nil
}

func (h SuffrageProofRequestHeader) Height() base.Height {
	return h.suffrageheight
}

type LastBlockMapRequestHeader struct {
	manifest util.Hash
	BaseHeader
}

func NewLastBlockMapRequestHeader(manifest util.Hash) LastBlockMapRequestHeader {
	return LastBlockMapRequestHeader{
		BaseHeader: NewBaseHeader(LastBlockMapRequestHeaderHint, HandlerPrefixLastBlockMap),
		manifest:   manifest,
	}
}

func (h LastBlockMapRequestHeader) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid LastLastBlockMapHeader")

	if err := h.BaseHinter.IsValid(h.Hint().Type().Bytes()); err != nil {
		return e(err, "")
	}

	if err := util.CheckIsValid(nil, true, h.manifest); err != nil {
		return e(err, "")
	}

	return nil
}

func (h LastBlockMapRequestHeader) Manifest() util.Hash {
	return h.manifest
}

type BlockMapRequestHeader struct {
	BaseHeader
	height base.Height
}

func NewBlockMapRequestHeader(height base.Height) BlockMapRequestHeader {
	return BlockMapRequestHeader{
		BaseHeader: NewBaseHeader(BlockMapRequestHeaderHint, HandlerPrefixBlockMap),
		height:     height,
	}
}

func (h BlockMapRequestHeader) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid LastBlockMapHeader")

	if err := h.BaseHinter.IsValid(h.Hint().Type().Bytes()); err != nil {
		return e(err, "")
	}

	if err := util.CheckIsValid(nil, false, h.height); err != nil {
		return e(err, "")
	}

	return nil
}

func (h BlockMapRequestHeader) Height() base.Height {
	return h.height
}

type BlockMapItemRequestHeader struct {
	item base.BlockMapItemType
	BaseHeader
	height base.Height
}

func NewBlockMapItemRequestHeader(height base.Height, item base.BlockMapItemType) BlockMapItemRequestHeader {
	return BlockMapItemRequestHeader{
		BaseHeader: NewBaseHeader(BlockMapItemRequestHeaderHint, HandlerPrefixBlockMapItem),
		height:     height,
		item:       item,
	}
}

func (h BlockMapItemRequestHeader) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid LastBlockMapItemHeader")

	if err := h.BaseHinter.IsValid(h.Hint().Type().Bytes()); err != nil {
		return e(err, "")
	}

	if err := util.CheckIsValid(nil, false, h.height, h.item); err != nil {
		return e(err, "")
	}

	return nil
}

func (h BlockMapItemRequestHeader) Height() base.Height {
	return h.height
}

func (h BlockMapItemRequestHeader) Item() base.BlockMapItemType {
	return h.item
}

type ResponseHeader struct {
	err error
	BaseHeader
	ok bool
}

func NewResponseHeader(ok bool, err error) ResponseHeader {
	return ResponseHeader{
		BaseHeader: NewBaseHeader(ResponseHeaderHint, ""),
		ok:         ok,
		err:        err,
	}
}

func (r ResponseHeader) OK() bool {
	return r.ok
}

func (r ResponseHeader) Err() error {
	return r.err
}
