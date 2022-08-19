package isaac

import (
	"encoding/json"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/hint"
)

var (
	NetworkPolicyHint           = hint.MustNewHint("network-policy-v0.0.1")
	NetworkPolicyStateValueHint = hint.MustNewHint("network-policy-state-value-v0.0.1")

	DefaultMaxOperationsInProposal uint64 = 333
	DefaultMaxSuffrageSize         uint64 = 33

	// NOTE suffrage candidate can be approved within lifespan height; almost 15
	// days(based on 5 second for one block)
	DefaultSuffrageCandidateLifespan base.Height = 1 << 18
)

type NetworkPolicy struct {
	suffrageCandidateLimiterRule base.SuffrageCandidateLimiterRule
	util.DefaultJSONMarshaled
	hint.BaseHinter
	maxOperationsInProposal   uint64
	suffrageCandidateLifespan base.Height
	maxSuffrageSize           uint64
}

func DefaultNetworkPolicy() NetworkPolicy {
	return NetworkPolicy{
		BaseHinter:                   hint.NewBaseHinter(NetworkPolicyHint),
		maxOperationsInProposal:      DefaultMaxOperationsInProposal,
		suffrageCandidateLifespan:    DefaultSuffrageCandidateLifespan,
		suffrageCandidateLimiterRule: NewFixedSuffrageCandidateLimiterRule(1),
		maxSuffrageSize:              DefaultMaxSuffrageSize,
	}
}

func (p NetworkPolicy) IsValid([]byte) error {
	e := util.ErrInvalid.Errorf("invalid NetworkPolicy")

	if err := p.BaseHinter.IsValid(NetworkPolicyHint.Type().Bytes()); err != nil {
		return e.Wrap(err)
	}

	if p.maxOperationsInProposal < 1 {
		return e.Errorf("under zero maxOperationsInProposal")
	}

	if p.maxSuffrageSize < 1 {
		return e.Errorf("under zero maxSuffrageSize")
	}

	switch err := p.suffrageCandidateLifespan.IsValid(nil); {
	case err != nil:
		return e.Wrapf(err, "invalid SuffrageCandidateLifespan")
	case p.suffrageCandidateLifespan <= base.GenesisHeight:
		return e.Errorf("zero SuffrageCandidateLifespan")
	}

	if p.suffrageCandidateLimiterRule == nil {
		return e.Errorf("empty SuffrageCandidateLimiterRule")
	}

	if err := p.suffrageCandidateLimiterRule.IsValid(nil); err != nil {
		return e.Wrap(err)
	}

	return nil
}

func (p NetworkPolicy) HashBytes() []byte {
	var rule []byte

	if p.suffrageCandidateLimiterRule != nil {
		rule = p.suffrageCandidateLimiterRule.HashBytes()
	}

	return util.ConcatBytesSlice(
		util.Uint64ToBytes(p.maxOperationsInProposal),
		p.suffrageCandidateLifespan.Bytes(),
		util.Uint64ToBytes(p.maxSuffrageSize),
		rule,
	)
}

func (p NetworkPolicy) MaxOperationsInProposal() uint64 {
	return p.maxOperationsInProposal
}

func (p NetworkPolicy) SuffrageCandidateLifespan() base.Height {
	return p.suffrageCandidateLifespan
}

func (p NetworkPolicy) SuffrageCandidateLimiterRule() base.SuffrageCandidateLimiterRule {
	return p.suffrageCandidateLimiterRule
}

func (p NetworkPolicy) MaxSuffrageSize() uint64 {
	return p.maxSuffrageSize
}

type networkPolicyJSONMarshaler struct {
	// revive:disable-next-line:line-length-limit
	SuffrageCandidateLimiterRule base.SuffrageCandidateLimiterRule `json:"suffrage_candidate_limiter"` //nolint:tagliatelle //...
	hint.BaseHinter
	MaxOperationsInProposal   uint64      `json:"max_operations_in_proposal"`
	SuffrageCandidateLifespan base.Height `json:"suffrage_candidate_lifespan"`
	MaxSuffrageSize           uint64      `json:"max_suffrage_size"`
}

func (p NetworkPolicy) MarshalJSON() ([]byte, error) {
	return util.MarshalJSON(networkPolicyJSONMarshaler{
		BaseHinter:                   p.BaseHinter,
		MaxOperationsInProposal:      p.maxOperationsInProposal,
		SuffrageCandidateLifespan:    p.suffrageCandidateLifespan,
		SuffrageCandidateLimiterRule: p.suffrageCandidateLimiterRule,
		MaxSuffrageSize:              p.maxSuffrageSize,
	})
}

type networkPolicyJSONUnmarshaler struct {
	SuffrageCandidateLimiterRule json.RawMessage `json:"suffrage_candidate_limiter"` //nolint:tagliatelle //...
	MaxOperationsInProposal      uint64          `json:"max_operations_in_proposal"`
	SuffrageCandidateLifespan    base.Height     `json:"suffrage_candidate_lifespan"`
	MaxSuffrageSize              uint64          `json:"max_suffrage_size"`
}

func (p *NetworkPolicy) DecodeJSON(b []byte, enc *jsonenc.Encoder) error {
	e := util.StringErrorFunc("failed to unmarshal NetworkPolicy")

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

	return nil
}

type NetworkPolicyStateValue struct {
	policy base.NetworkPolicy
	hint.BaseHinter
}

func NewNetworkPolicyStateValue(policy base.NetworkPolicy) NetworkPolicyStateValue {
	return NetworkPolicyStateValue{
		BaseHinter: hint.NewBaseHinter(NetworkPolicyStateValueHint),
		policy:     policy,
	}
}

func (s NetworkPolicyStateValue) HashBytes() []byte {
	return s.policy.HashBytes()
}

func (s NetworkPolicyStateValue) IsValid([]byte) error {
	e := util.StringErrorFunc("invalid NetworkPolicyStateValue")
	if err := s.BaseHinter.IsValid(NetworkPolicyStateValueHint.Type().Bytes()); err != nil {
		return e(err, "")
	}

	if err := util.CheckIsValid(nil, false, s.policy); err != nil {
		return e(err, "")
	}

	return nil
}

func (s NetworkPolicyStateValue) Policy() base.NetworkPolicy {
	return s.policy
}