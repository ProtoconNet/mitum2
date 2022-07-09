package isaacoperation

import (
	"testing"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/stretchr/testify/suite"
)

func TestSuffrageCandidateFactEncode(tt *testing.T) {
	t := new(encoder.BaseTestEncode)

	enc := jsonenc.NewEncoder()

	t.Encode = func() (interface{}, []byte) {
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: base.StringAddressHint, Instance: base.StringAddress{}}))
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: base.MPublickeyHint, Instance: base.MPublickey{}}))
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: SuffrageCandidateFactHint, Instance: SuffrageCandidateFact{}}))

		fact := NewSuffrageCandidateFact(
			util.UUID().Bytes(),
			base.RandomAddress(""),
			base.NewMPrivatekey().Publickey(),
		)

		t.NoError(fact.IsValid(nil))

		b, err := enc.Marshal(fact)
		t.NoError(err)

		t.T().Log("marshaled:", string(b))

		return fact, b
	}
	t.Decode = func(b []byte) interface{} {
		i, err := enc.Decode(b)
		t.NoError(err)

		_, ok := i.(SuffrageCandidateFact)
		t.True(ok)

		return i
	}
	t.Compare = func(a, b interface{}) {
		af, ok := a.(SuffrageCandidateFact)
		t.True(ok)
		bf, ok := b.(SuffrageCandidateFact)
		t.True(ok)

		t.NoError(bf.IsValid(nil))

		base.EqualFact(t.Assert(), af, bf)
	}

	suite.Run(tt, t)
}

func TestSuffrageCandidateEncode(tt *testing.T) {
	t := new(encoder.BaseTestEncode)

	enc := jsonenc.NewEncoder()
	networkID := util.UUID().Bytes()

	t.Encode = func() (interface{}, []byte) {
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: base.StringAddressHint, Instance: base.StringAddress{}}))
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: base.MPublickeyHint, Instance: base.MPublickey{}}))
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: SuffrageCandidateFactHint, Instance: SuffrageCandidateFact{}}))
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: SuffrageCandidateHint, Instance: SuffrageCandidate{}}))

		priv := base.NewMPrivatekey()
		fact := NewSuffrageCandidateFact(
			util.UUID().Bytes(),
			base.RandomAddress(""),
			priv.Publickey(),
		)

		op := NewSuffrageCandidate(fact)
		t.NoError(op.Sign(priv, networkID, fact.Address()))

		t.NoError(op.IsValid(networkID))

		b, err := enc.Marshal(op)
		t.NoError(err)

		t.T().Log("marshaled:", string(b))

		return op, b
	}
	t.Decode = func(b []byte) interface{} {
		i, err := enc.Decode(b)
		t.NoError(err)

		_, ok := i.(SuffrageCandidate)
		t.True(ok)

		return i
	}
	t.Compare = func(a, b interface{}) {
		af, ok := a.(SuffrageCandidate)
		t.True(ok)
		bf, ok := b.(SuffrageCandidate)
		t.True(ok)

		t.NoError(bf.IsValid(networkID))

		base.EqualOperation(t.Assert(), af, bf)
	}

	suite.Run(tt, t)
}
