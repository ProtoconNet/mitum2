package isaac

import (
	"encoding/json"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/hint"
)

type baseBallotJSONMarshaler struct {
	Expels    []base.SuffrageExpelOperation `json:"expels,omitempty"`
	Voteproof base.Voteproof                `json:"voteproof,omitempty"`
	SignFact  base.BallotSignFact           `json:"sign_fact"`
	hint.BaseHinter
}

func (bl baseBallot) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(baseBallotJSONMarshaler{
		BaseHinter: bl.BaseHinter,
		Voteproof:  bl.vp,
		SignFact:   bl.signFact,
		Expels:     bl.expels,
	})
}

type baseBallotJSONUnmarshaler struct {
	Voteproof json.RawMessage   `json:"voteproof"`
	SignFact  json.RawMessage   `json:"sign_fact"`
	Expels    []json.RawMessage `json:"expels,omitempty"`
}

func (bl *baseBallot) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode baseBallot")

	var u baseBallotJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e.Wrap(err)
	}

	if err := encoder.Decode(enc, u.Voteproof, &bl.vp); err != nil {
		return e.Wrap(err)
	}

	if err := encoder.Decode(enc, u.SignFact, &bl.signFact); err != nil {
		return e.Wrap(err)
	}

	bl.expels = make([]base.SuffrageExpelOperation, len(u.Expels))
	for i := range u.Expels {
		if err := encoder.Decode(enc, u.Expels[i], &bl.expels[i]); err != nil {
			return e.Wrap(err)
		}
	}

	return nil
}
