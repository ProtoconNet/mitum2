package isaacoperation

import (
	"encoding/json"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
)

type baseNetworkPolicyFactJSONMarshaler struct {
	Policy base.NetworkPolicy `json:"policy"`
	base.BaseFactJSONMarshaler
}

func (fact baseNetworkPolicyFact) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(baseNetworkPolicyFactJSONMarshaler{
		BaseFactJSONMarshaler: fact.BaseFact.JSONMarshaler(),
		Policy:                fact.policy,
	})
}

type baseNetworkPolicyFactJSONUnmarshaler struct {
	base.BaseFactJSONUnmarshaler
	Policy json.RawMessage `json:"policy"`
}

func (fact *baseNetworkPolicyFact) DecodeJSON(b []byte, enc encoder.Encoder) error {
	e := util.StringError("decode baseNetworkPolicyFact")

	var u baseNetworkPolicyFactJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e.Wrap(err)
	}

	fact.BaseFact.SetJSONUnmarshaler(u.BaseFactJSONUnmarshaler)

	if err := encoder.Decode(enc, u.Policy, &fact.policy); err != nil {
		return e.Wrap(err)
	}

	return nil
}
