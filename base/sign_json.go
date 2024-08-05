package base

import (
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/localtime"
)

type BaseSignJSONMarshaler struct {
	SignedAt  localtime.Time `json:"signed_at"`
	Signer    Publickey      `json:"signer"`
	Signature Signature      `json:"signature"`
}

func (si BaseSign) JSONMarshaler() BaseSignJSONMarshaler {
	return BaseSignJSONMarshaler{
		Signer:    si.signer,
		Signature: si.signature,
		SignedAt:  localtime.New(si.signedAt),
	}
}

func (si BaseSign) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(si.JSONMarshaler())
}

type baseSignJSONUnmarshaler struct {
	SignedAt  localtime.Time `json:"signed_at"`
	Signer    string         `json:"signer"`
	Signature Signature      `json:"signature"`
}

func (si *BaseSign) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("faied to decode BaseSign")

	var u baseSignJSONUnmarshaler
	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e.Wrap(err)
	}

	signer, err := DecodePublickeyFromString(u.Signer, enc)
	if err != nil {
		return e.Wrap(err)
	}

	si.signer = signer
	si.signature = u.Signature
	si.signedAt = u.SignedAt.Time

	return nil
}

type BaseNodeSignJSONMarshaler struct {
	Node Address `json:"node"`
	BaseSignJSONMarshaler
}

func (si BaseNodeSign) JSONMarshaler() BaseNodeSignJSONMarshaler {
	return BaseNodeSignJSONMarshaler{
		BaseSignJSONMarshaler: si.BaseSign.JSONMarshaler(),
		Node:                  si.node,
	}
}

func (si BaseNodeSign) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(si.JSONMarshaler())
}

type baseNodeSignJSONUnmarshaler struct {
	Node string `json:"node"`
}

func (si *BaseNodeSign) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode BaseNodeSign")

	var u baseNodeSignJSONUnmarshaler
	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e.Wrap(err)
	}

	switch ad, err := DecodeAddress(u.Node, enc); {
	case err != nil:
		return e.WithMessage(err, "decode node address")
	default:
		si.node = ad
	}

	if err := si.BaseSign.DecodeJSON(b, enc); err != nil {
		return e.Wrap(err)
	}

	return nil
}
