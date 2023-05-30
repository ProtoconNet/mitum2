package isaacoperation

import (
	"encoding/json"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
)

type GenesisNetworkPolicyFactJSONMarshaler struct {
	Policy base.NetworkPolicy `json:"policy"`
	base.BaseFactJSONMarshaler
}

func (fact GenesisNetworkPolicyFact) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(GenesisNetworkPolicyFactJSONMarshaler{
		BaseFactJSONMarshaler: fact.BaseFact.JSONMarshaler(),
		Policy:                fact.policy,
	})
}

type GenesisNetworkPolicyFactJSONUnmarshaler struct {
	base.BaseFactJSONUnmarshaler
	Policy json.RawMessage `json:"policy"`
}

func (fact *GenesisNetworkPolicyFact) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("failed to decode GenesisNetworkPolicyFact")

	var u GenesisNetworkPolicyFactJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e(err, "")
	}

	fact.BaseFact.SetJSONUnmarshaler(u.BaseFactJSONUnmarshaler)

	if err := encoder.Decode(enc, u.Policy, &fact.policy); err != nil {
		return e(err, "")
	}

	return nil
}
