package isaac

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/stretchr/testify/suite"
)

type testLocalParams struct {
	suite.Suite
}

func (t *testLocalParams) TestNew() {
	networkID := base.RandomNetworkID()

	p := DefaultLocalParams(networkID)
	t.NoError(p.IsValid(networkID))

	_ = (interface{})(p).(base.LocalParams)
}

func (t *testLocalParams) TestIsValid() {
	networkID := base.RandomNetworkID()

	t.Run("network id does not match", func() {
		p := DefaultLocalParams(networkID)
		err := p.IsValid(base.RandomNetworkID())
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "network id does not match")
	})

	t.Run("wrong network id", func() {
		wrongnetworkID := make([]byte, base.MaxNetworkIDLength+1)

		p := DefaultLocalParams(wrongnetworkID)
		err := p.IsValid(wrongnetworkID)
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "network id too long")
	})

	t.Run("wrong threshold", func() {
		p := DefaultLocalParams(networkID)
		p.SetThreshold(33)

		err := p.IsValid(networkID)
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "risky threshold")
	})

	t.Run("wrong intervalBroadcastBallot", func() {
		p := DefaultLocalParams(networkID)
		p.SetIntervalBroadcastBallot(-1)

		err := p.IsValid(networkID)
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "wrong duration")
	})

	t.Run("wrong waitPreparingINITBallot", func() {
		p := DefaultLocalParams(networkID)
		p.SetWaitPreparingINITBallot(-1)

		err := p.IsValid(networkID)
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "wrong duration")
	})

	t.Run("wrong timeoutRequestProposal", func() {
		p := DefaultLocalParams(networkID)
		p.SetTimeoutRequestProposal(-1)

		err := p.IsValid(networkID)
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "wrong duration")
	})

	t.Run("wrong maxOperationSize", func() {
		p := DefaultLocalParams(networkID)
		p.SetMaxOperationSize(0)

		err := p.IsValid(networkID)
		t.Error(err)
		t.True(errors.Is(err, util.ErrInvalid))
		t.ErrorContains(err, "wrong maxOperationSize")
	})
}

func TestLocalParams(t *testing.T) {
	suite.Run(t, new(testLocalParams))
}

func TestLocalParamsJSON(tt *testing.T) {
	t := new(encoder.BaseTestEncode)

	networkID := util.UUID().Bytes()
	enc := jsonenc.NewEncoder()

	t.Encode = func() (interface{}, []byte) {
		t.NoError(enc.Add(encoder.DecodeDetail{Hint: LocalParamsHint, Instance: LocalParams{}}))

		p := DefaultLocalParams(networkID)
		p.SetThreshold(base.Threshold(77.7))
		p.SetIntervalBroadcastBallot(time.Second * 33)
		p.SetSameMemberLimit(99)

		b, err := util.MarshalJSON(&p)
		t.NoError(err)

		t.T().Log("marshaled:", string(b))

		return p, b
	}

	t.Decode = func(b []byte) interface{} {
		p := NewLocalParams(networkID)
		t.NoError(enc.Unmarshal(b, p))

		return p
	}
	t.Compare = func(a, b interface{}) {
		ap := a.(*LocalParams)
		bp := b.(*LocalParams)

		t.True(ap.Hint().Equal(bp.Hint()))
		t.True(ap.networkID.Equal(bp.networkID))
		t.Equal(ap.threshold, bp.threshold)
		t.Equal(ap.intervalBroadcastBallot, bp.intervalBroadcastBallot)
		t.Equal(ap.waitPreparingINITBallot, bp.waitPreparingINITBallot)
		t.Equal(ap.timeoutRequestProposal, bp.timeoutRequestProposal)
		t.Equal(ap.syncSourceCheckerInterval, bp.syncSourceCheckerInterval)
		t.Equal(ap.validProposalOperationExpire, bp.validProposalOperationExpire)
		t.Equal(ap.validProposalSuffrageOperationsExpire, bp.validProposalSuffrageOperationsExpire)
		t.Equal(ap.maxOperationSize, bp.maxOperationSize)
		t.Equal(ap.sameMemberLimit, bp.sameMemberLimit)
	}

	suite.Run(tt, t)
}

func TestLocalParamsJSONMissing(tt *testing.T) {
	t := new(encoder.BaseTestEncode)
	t.SetT(tt)

	enc := jsonenc.NewEncoder()

	t.NoError(enc.Add(encoder.DecodeDetail{Hint: LocalParamsHint, Instance: LocalParams{}}))

	p := DefaultLocalParams(util.UUID().Bytes())
	p.SetThreshold(p.Threshold() + 3)
	p.SetIntervalBroadcastBallot(p.IntervalBroadcastBallot() + 3)
	p.SetWaitPreparingINITBallot(p.WaitPreparingINITBallot() + 3)
	p.SetTimeoutRequestProposal(p.TimeoutRequestProposal() + 3)
	p.SetSyncSourceCheckerInterval(p.SyncSourceCheckerInterval() + 3)
	p.SetValidProposalOperationExpire(p.ValidProposalOperationExpire() + 3)
	p.SetValidProposalSuffrageOperationsExpire(p.ValidProposalSuffrageOperationsExpire() + 3)
	p.SetMaxOperationSize(p.MaxOperationSize() + 3)
	p.SetSameMemberLimit(p.SameMemberLimit() + 3)

	b, err := util.MarshalJSON(&p)
	t.NoError(err)

	t.T().Log("marshaled:", string(b))

	var m map[string]interface{}
	t.NoError(util.UnmarshalJSON(b, &m))

	for key := range m {
		if key == "_hint" {
			continue
		}

		nm := map[string]interface{}{}
		for i := range m {
			nm[i] = m[i]
		}

		delete(nm, key)

		mb, err := util.MarshalJSON(nm)
		t.NoError(err)

		var np LocalParams
		t.NoError(enc.Unmarshal(mb, &np))

		ub, err := util.MarshalJSON(&np)
		t.NoError(err)

		var um map[string]interface{}
		t.NoError(util.UnmarshalJSON(ub, &um))

		for i := range m {
			if i == key {
				t.Empty(um[i])

				continue
			}

			t.Equal(nm[i], um[i], "%s: %v != %v", key, nm[i], um[i])
		}
	}
}