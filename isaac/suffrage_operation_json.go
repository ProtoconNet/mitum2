package isaac

import (
	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
)

type suffrageExpelFactJSONMarshaler struct {
	Node   base.Address `json:"node"`
	Reason string       `json:"reason"`
	base.BaseFactJSONMarshaler
	Start base.Height `json:"start"`
	End   base.Height `json:"end"`
}

func (fact SuffrageExpelFact) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(suffrageExpelFactJSONMarshaler{
		BaseFactJSONMarshaler: fact.BaseFact.JSONMarshaler(),
		Node:                  fact.node,
		Start:                 fact.start,
		End:                   fact.end,
		Reason:                fact.reason,
	})
}

type suffrageExpelFactJSONUnmarshaler struct {
	Node   string `json:"node"`
	Reason string `json:"reason"`
	base.BaseFactJSONUnmarshaler
	Start base.Height `json:"start"`
	End   base.Height `json:"end"`
}

func (fact *SuffrageExpelFact) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode SuffrageExpelFact")

	var u suffrageExpelFactJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e.Wrap(err)
	}

	fact.BaseFact.SetJSONUnmarshaler(u.BaseFactJSONUnmarshaler)

	switch i, err := base.DecodeAddress(u.Node, enc); {
	case err != nil:
		return e.Wrap(err)
	default:
		fact.node = i
	}

	fact.start = u.Start
	fact.end = u.End
	fact.reason = u.Reason

	return nil
}
