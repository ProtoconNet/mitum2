package isaac

import (
	"encoding/json"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/hint"
)

type networkPolicyJSONMarshaler struct {
	// revive:disable-next-line:line-length-limit
	SuffrageCandidateLimiterRule base.SuffrageCandidateLimiterRule `json:"suffrage_candidate_limiter"` //nolint:tagliatelle //...
	hint.BaseHinter
	MaxOperationsInProposal   uint64      `json:"max_operations_in_proposal"`
	SuffrageCandidateLifespan base.Height `json:"suffrage_candidate_lifespan"`
	MaxSuffrageSize           uint64      `json:"max_suffrage_size"`
	SuffrageExpelLifespan     base.Height `json:"suffrage_expel_lifespan"`
}

func (p NetworkPolicy) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(networkPolicyJSONMarshaler{
		BaseHinter:                   p.BaseHinter,
		MaxOperationsInProposal:      p.maxOperationsInProposal,
		SuffrageCandidateLifespan:    p.suffrageCandidateLifespan,
		SuffrageCandidateLimiterRule: p.suffrageCandidateLimiterRule,
		MaxSuffrageSize:              p.maxSuffrageSize,
		SuffrageExpelLifespan:        p.suffrageExpelLifespan,
	})
}

type networkPolicyJSONUnmarshaler struct {
	SuffrageCandidateLimiterRule json.RawMessage `json:"suffrage_candidate_limiter"` //nolint:tagliatelle //...
	MaxOperationsInProposal      uint64          `json:"max_operations_in_proposal"`
	SuffrageCandidateLifespan    base.Height     `json:"suffrage_candidate_lifespan"`
	MaxSuffrageSize              uint64          `json:"max_suffrage_size"`
	SuffrageExpelLifespan        base.Height     `json:"suffrage_expel_lifespan"`
}

func (p *NetworkPolicy) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("unmarshal NetworkPolicy")

	var u networkPolicyJSONUnmarshaler
	if err := util.UnmarshalJSON(b, &u); err != nil {
		return e(err, "")
	}

	if err := encoder.Decode(enc, u.SuffrageCandidateLimiterRule, &p.suffrageCandidateLimiterRule); err != nil {
		return e(err, "")
	}

	p.maxOperationsInProposal = u.MaxOperationsInProposal
	p.suffrageCandidateLifespan = u.SuffrageCandidateLifespan
	p.maxSuffrageSize = u.MaxSuffrageSize
	p.suffrageExpelLifespan = u.SuffrageExpelLifespan

	return nil
}

type NetworkPolicyStateValueJSONMarshaler struct {
	Policy base.NetworkPolicy `json:"policy"`
	hint.BaseHinter
}

func (s NetworkPolicyStateValue) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(NetworkPolicyStateValueJSONMarshaler{
		BaseHinter: s.BaseHinter,
		Policy:     s.policy,
	})
}

type NetworkPolicyStateValueJSONUnmarshaler struct {
	Policy json.RawMessage `json:"policy"`
}

func (s *NetworkPolicyStateValue) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("decode NetworkPolicyStateValue")

	var u NetworkPolicyStateValueJSONUnmarshaler
	if err := enc.Unmarshal(b, &u); err != nil {
		return e(err, "")
	}

	if err := encoder.Decode(enc, u.Policy, &s.policy); err != nil {
		return e(err, "")
	}

	return nil
}
