package isaacblock

import (
	"encoding/json"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/ProtoconNet/mitum2/util/fixedtree"
	"github.com/ProtoconNet/mitum2/util/hint"
)

type SuffrageProofJSONMarshaler struct {
	Map       base.BlockMap        `json:"map"`
	State     base.State           `json:"state"`
	Voteproof base.ACCEPTVoteproof `json:"voteproof"`
	Proof     fixedtree.Proof      `json:"proof"`
	hint.BaseHinter
}

func (s SuffrageProof) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(SuffrageProofJSONMarshaler{
		BaseHinter: s.BaseHinter,
		Map:        s.m,
		State:      s.st,
		Proof:      s.proof,
		Voteproof:  s.voteproof,
	})
}

type SuffrageProofJSONUnmarshaler struct {
	Map       json.RawMessage `json:"map"`
	State     json.RawMessage `json:"state"`
	Proof     fixedtree.Proof `json:"proof"`
	Voteproof json.RawMessage `json:"voteproof"`
}

func (s *SuffrageProof) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("failed to decode SuffrageProof")

	var u SuffrageProofJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e(err, "")
	}

	if err := encoder.Decode(enc, u.Map, &s.m); err != nil {
		return e(err, "")
	}

	if err := encoder.Decode(enc, u.State, &s.st); err != nil {
		return e(err, "")
	}

	if err := encoder.Decode(enc, u.Voteproof, &s.voteproof); err != nil {
		return e(err, "")
	}

	s.proof = u.Proof

	return nil
}
