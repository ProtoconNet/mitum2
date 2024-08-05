package isaac

import (
	"testing"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/ProtoconNet/mitum2/util/valuehash"
	"github.com/stretchr/testify/suite"
)

type testProposalFact struct {
	suite.Suite
}

func (t *testProposalFact) proposal() ProposalFact {
	pr := NewProposalFact(
		base.RawPoint(33, 44),
		base.RandomAddress("pr"),
		valuehash.RandomSHA256(),
		[][2]util.Hash{{valuehash.RandomSHA256(), valuehash.RandomSHA256()}},
	)
	_ = (interface{})(pr).(base.ProposalFact)

	return pr
}

func (t *testProposalFact) TestNew() {
	pr := t.proposal()
	t.NoError(pr.IsValid(nil))

	_ = (interface{})(pr).(base.ProposalFact)
}

func (t *testProposalFact) TestEmptyHash() {
	pr := t.proposal()
	pr.SetHash(nil)

	err := pr.IsValid(nil)
	t.Error(err)
	t.ErrorIs(err, util.ErrInvalid)
}

func (t *testProposalFact) TestWrongHash() {
	pr := t.proposal()
	pr.SetHash(valuehash.RandomSHA256())

	err := pr.IsValid(nil)
	t.Error(err)
	t.ErrorIs(err, util.ErrInvalid)
}

func (t *testProposalFact) TestWrongPoint() {
	pr := t.proposal()
	pr.point = base.ZeroPoint

	err := pr.IsValid(nil)
	t.Error(err)
	t.ErrorIs(err, util.ErrInvalid)
}

func (t *testProposalFact) TestDuplicatedOperations() {
	op := valuehash.RandomSHA256()
	pr := NewProposalFact(
		base.RawPoint(33, 44),
		base.RandomAddress("pr"),
		valuehash.RandomSHA256(),
		[][2]util.Hash{
			{valuehash.RandomSHA256(), valuehash.RandomSHA256()},
			{op, valuehash.RandomSHA256()},
			{valuehash.RandomSHA256(), valuehash.RandomSHA256()},
			{op, valuehash.RandomSHA256()},
		})

	err := pr.IsValid(nil)
	t.Error(err)
	t.ErrorIs(err, util.ErrInvalid)
	t.ErrorContains(err, "duplicated operation found")
}

func TestProposalFact(t *testing.T) {
	suite.Run(t, new(testProposalFact))
}

type testProposalFactEncode struct {
	encoder.BaseTestEncode
	enc *jsonenc.Encoder
}

func (t *testProposalFactEncode) SetupTest() {
	t.enc = jsonenc.NewEncoder()

	t.NoError(t.enc.Add(encoder.DecodeDetail{Hint: base.StringAddressHint, Instance: base.StringAddress{}}))
	t.NoError(t.enc.Add(encoder.DecodeDetail{Hint: ProposalFactHint, Instance: ProposalFact{}}))
}

func TestProposalFactJSON(tt *testing.T) {
	t := new(testProposalFactEncode)

	t.Encode = func() (interface{}, []byte) {
		pr := NewProposalFact(base.RawPoint(33, 44),
			base.RandomAddress("pr"),
			valuehash.RandomSHA256(),
			[][2]util.Hash{{valuehash.RandomSHA256(), valuehash.RandomSHA256()}})

		b, err := t.enc.Marshal(&pr)
		t.NoError(err)

		return pr, b
	}
	t.Decode = func(b []byte) interface{} {
		i, err := t.enc.Decode(b)
		t.NoError(err)

		_, ok := i.(ProposalFact)
		t.True(ok)

		return i
	}
	t.Compare = func(a, b interface{}) {
		af, ok := a.(ProposalFact)
		t.True(ok)
		bf, ok := b.(ProposalFact)
		t.True(ok)

		t.NoError(bf.IsValid(nil))

		base.EqualProposalFact(t.Assert(), af, bf)
	}

	suite.Run(tt, t)
}
