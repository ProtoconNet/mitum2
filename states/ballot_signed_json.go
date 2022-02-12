package states

import (
	"encoding/json"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/hint"
)

type baseBallotSignedFactJSONMarshaler struct {
	hint.BaseHinter
	Fact   base.BallotFact `json:"fact"`
	Node   base.Address    `json:"node"`
	Signed base.BaseSigned `json:"signed"`
}

type baseBallotSignedFactJSONUnmarshaler struct {
	hint.BaseHinter
	Fact   json.RawMessage `json:"fact"`
	Node   string          `json:"node"`
	Signed json.RawMessage `json:"signed"`
}

func (sf baseBallotSignedFact) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(baseBallotSignedFactJSONMarshaler{
		BaseHinter: sf.BaseHinter,
		Fact:       sf.fact,
		Node:       sf.node,
		Signed:     sf.signed,
	})
}

func (sf *baseBallotSignedFact) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("failed to decode BaseBallotSignedFact")

	var u baseBallotSignedFactJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e(err, "")
	}

	switch i, err := enc.Decode(u.Fact); {
	case err != nil:
		return e(err, "failed to decode fact")
	default:
		j, ok := i.(base.BallotFact)
		if !ok {
			return e(err, "decoded fact not BallotFact, %T", i)
		}

		sf.fact = j
	}

	switch i, err := base.DecodeAddressFromString(u.Node, enc); {
	case err != nil:
		return e(err, "failed to decode address")
	default:
		sf.node = i
	}

	if err := sf.signed.DecodeJSON(u.Signed, enc); err != nil {
		return e(err, "failed to decode signed")
	}

	return nil
}