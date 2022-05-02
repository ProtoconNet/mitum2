package isaacoperation

import (
	"context"
	"errors"
	"testing"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/stretchr/testify/suite"
)

type testGenesisNetworkPolicyFact struct {
	suite.Suite
}

func (t *testGenesisNetworkPolicyFact) TestNew() {
	policy := isaac.DefaultNetworkPolicy()
	policy.SetMaxOperationsInProposal(33)

	fact := NewGenesisNetworkPolicyFact(policy)
	t.NoError(fact.IsValid(nil))
}

func TestGenesisNetworkPolicyFact(t *testing.T) {
	suite.Run(t, new(testGenesisNetworkPolicyFact))
}

func TestGenesisNetworkPolicyFactEncode(tt *testing.T) {
	t := new(encoder.BaseTestEncode)

	enc := jsonenc.NewEncoder()

	t.Encode = func() (interface{}, []byte) {
		policy := isaac.DefaultNetworkPolicy()
		policy.SetMaxOperationsInProposal(33)

		fact := NewGenesisNetworkPolicyFact(policy)

		b, err := enc.Marshal(fact)
		t.NoError(err)

		t.T().Log("marshaled:", string(b))

		return fact, b
	}
	t.Decode = func(b []byte) interface{} {
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: isaac.NetworkPolicyHint, Instance: isaac.NetworkPolicy{}}))
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: GenesisNetworkPolicyFactHint, Instance: GenesisNetworkPolicyFact{}}))

		i, err := enc.Decode(b)
		t.NoError(err)

		_, ok := i.(GenesisNetworkPolicyFact)
		t.True(ok)

		return i
	}
	t.Compare = func(a, b interface{}) {
		af, ok := a.(GenesisNetworkPolicyFact)
		t.True(ok)
		bf, ok := b.(GenesisNetworkPolicyFact)
		t.True(ok)

		t.NoError(bf.IsValid(nil))

		base.EqualFact(t.Assert(), af, bf)
	}

	suite.Run(tt, t)
}

type testGenesisNetworkPolicy struct {
	suite.Suite
	priv      base.Privatekey
	networkID base.NetworkID
}

func (t *testGenesisNetworkPolicy) SetupSuite() {
	t.priv = base.NewMPrivatekey()
	t.networkID = util.UUID().Bytes()
}

func (t *testGenesisNetworkPolicy) TestNew() {
	policy := isaac.DefaultNetworkPolicy()
	policy.SetMaxOperationsInProposal(33)

	fact := NewGenesisNetworkPolicyFact(policy)
	t.NoError(fact.IsValid(nil))

	op := NewGenesisNetworkPolicy(fact)
	t.NoError(op.Sign(t.priv, t.networkID))

	t.NoError(op.IsValid(t.networkID))

	_ = (interface{})(op).(base.Operation)
}

func (t *testGenesisNetworkPolicy) TestIsValid() {
	t.Run("invalid fact", func() {
		policy := isaac.DefaultNetworkPolicy()
		policy.SetMaxOperationsInProposal(0)

		fact := NewGenesisNetworkPolicyFact(policy)
		err := fact.IsValid(nil)
		t.Error(err)
		t.True(errors.Is(err, util.InvalidError))
		t.ErrorContains(err, "under zero maxOperationsInProposal")

		op := NewGenesisNetworkPolicy(fact)
		t.NoError(op.Sign(t.priv, t.networkID))

		err = op.IsValid(t.networkID)
		t.Error(err)
		t.True(errors.Is(err, util.InvalidError))
		t.ErrorContains(err, "under zero maxOperationsInProposal")
	})

	t.Run("multiple signed", func() {
		policy := isaac.DefaultNetworkPolicy()
		policy.SetMaxOperationsInProposal(33)

		fact := NewGenesisNetworkPolicyFact(policy)
		t.NoError(fact.IsValid(nil))

		op := NewGenesisNetworkPolicy(fact)
		t.NoError(op.Sign(t.priv, t.networkID))
		t.NoError(op.Sign(base.NewMPrivatekey(), t.networkID))

		t.Equal(2, len(op.Signed()))

		err := op.IsValid(t.networkID)
		t.Error(err)
		t.True(errors.Is(err, util.InvalidError))
		t.ErrorContains(err, "multiple signed found")
	})
}

func (t *testGenesisNetworkPolicy) TestPreProcess() {
	policy := isaac.DefaultNetworkPolicy()
	policy.SetMaxOperationsInProposal(33)

	fact := NewGenesisNetworkPolicyFact(policy)
	t.NoError(fact.IsValid(nil))

	op := NewGenesisNetworkPolicy(fact)
	t.NoError(op.Sign(t.priv, t.networkID))

	reason, err := op.PreProcess(context.Background(), nil)
	t.NoError(err)
	t.Nil(reason)
}

func (t *testGenesisNetworkPolicy) TestProcess() {
	policy := isaac.DefaultNetworkPolicy()
	policy.SetMaxOperationsInProposal(33)

	fact := NewGenesisNetworkPolicyFact(policy)
	t.NoError(fact.IsValid(nil))

	op := NewGenesisNetworkPolicy(fact)
	t.NoError(op.Sign(t.priv, t.networkID))

	values, reason, err := op.Process(context.Background(), nil)
	t.NoError(err)
	t.Nil(reason)

	t.Equal(1, len(values))

	value := values[0]
	t.Equal(isaac.NetworkPolicyStateKey, value.Key())

	expected := isaac.NewNetworkPolicyStateValue(policy)

	t.True(expected.Equal(value.Value()))
}

func TestGenesisNetworkPolicy(t *testing.T) {
	suite.Run(t, new(testGenesisNetworkPolicy))
}