package base

import (
	"encoding/json"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/pkg/errors"
)

type BaseOperationJSONMarshaler struct {
	Hash  util.Hash `json:"hash"`
	Fact  Fact      `json:"fact"`
	Signs []Sign    `json:"signs"`
	hint.BaseHinter
}

func (op BaseOperation) JSONMarshaler() BaseOperationJSONMarshaler {
	return BaseOperationJSONMarshaler{
		BaseHinter: op.BaseHinter,
		Hash:       op.h,
		Fact:       op.fact,
		Signs:      op.signs,
	}
}

func (op BaseOperation) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(op.JSONMarshaler())
}

type BaseOperationJSONUnmarshaler struct {
	Hash  valuehash.HashDecoder `json:"hash"`
	Fact  json.RawMessage       `json:"fact"`
	Signs []json.RawMessage     `json:"signs"`
}

func (op *BaseOperation) decodeJSON(b []byte, enc encoder.Encoder, u *BaseOperationJSONUnmarshaler) error {
	if err := enc.Unmarshal(b, u); err != nil {
		return err
	}

	op.h = u.Hash.Hash()

	if err := encoder.Decode(enc, u.Fact, &op.fact); err != nil {
		return errors.WithMessage(err, "decode fact")
	}

	return nil
}

func (op *BaseOperation) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode BaseOperation")

	var u BaseOperationJSONUnmarshaler

	if err := op.decodeJSON(b, enc, &u); err != nil {
		return e.Wrap(err)
	}

	op.signs = make([]Sign, len(u.Signs))

	for i := range u.Signs {
		var ub BaseSign
		if err := ub.DecodeJSON(u.Signs[i], enc); err != nil {
			return e.WithMessage(err, "decode sign")
		}

		op.signs[i] = ub
	}

	return nil
}

func (op BaseNodeOperation) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(op.JSONMarshaler())
}

func (op *BaseNodeOperation) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode BaseNodeOperation")

	var u BaseOperationJSONUnmarshaler

	if err := op.decodeJSON(b, enc, &u); err != nil {
		return e.Wrap(err)
	}

	op.signs = make([]Sign, len(u.Signs))

	for i := range u.Signs {
		var ub BaseNodeSign
		if err := ub.DecodeJSON(u.Signs[i], enc); err != nil {
			return e.WithMessage(err, "decode sign")
		}

		op.signs[i] = ub
	}

	return nil
}
