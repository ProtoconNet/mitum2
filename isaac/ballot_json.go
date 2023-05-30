package isaac

import (
	"encoding/json"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/ProtoconNet/mitum2/util/hint"
)

type baseBallotJSONMarshaler struct {
	Withdraws []base.SuffrageWithdrawOperation `json:"withdraws,omitempty"`
	Voteproof base.Voteproof                   `json:"voteproof,omitempty"`
	SignFact  base.BallotSignFact              `json:"sign_fact"`
	hint.BaseHinter
}

func (bl baseBallot) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(baseBallotJSONMarshaler{
		BaseHinter: bl.BaseHinter,
		Voteproof:  bl.vp,
		SignFact:   bl.signFact,
		Withdraws:  bl.withdraws,
	})
}

type baseBallotJSONUnmarshaler struct {
	Voteproof json.RawMessage   `json:"voteproof"`
	SignFact  json.RawMessage   `json:"sign_fact"`
	Withdraws []json.RawMessage `json:"withdraws,omitempty"`
}

func (bl *baseBallot) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("failed to decode baseBallot")

	var u baseBallotJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e(err, "")
	}

	if err := encoder.Decode(enc, u.Voteproof, &bl.vp); err != nil {
		return e(err, "")
	}

	if err := encoder.Decode(enc, u.SignFact, &bl.signFact); err != nil {
		return e(err, "")
	}

	bl.withdraws = make([]base.SuffrageWithdrawOperation, len(u.Withdraws))
	for i := range u.Withdraws {
		if err := encoder.Decode(enc, u.Withdraws[i], &bl.withdraws[i]); err != nil {
			return e(err, "")
		}
	}

	return nil
}
