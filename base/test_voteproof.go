//go:build test
// +build test

package base

import (
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/ProtoconNet/mitum2/util/localtime"
	"github.com/stretchr/testify/assert"
)

func EqualVoteproof(t *assert.Assertions, a, b Voteproof) {
	if a == nil {
		t.Equal(a, b)
		return
	}

	aht := a.(hint.Hinter).Hint()
	bht := b.(hint.Hinter).Hint()
	t.True(aht.Equal(bht), "Hint does not match")

	t.Equal(a.HashBytes(), b.HashBytes())
	t.True(localtime.New(a.FinishedAt()).Equal(localtime.New(b.FinishedAt())))
	t.Equal(a.Point(), b.Point())
	t.Equal(a.Result(), b.Result())

	EqualBallotFact(t, a.Majority(), b.Majority())

	t.Equal(len(a.SignFacts()), len(b.SignFacts()))

	as := a.SignFacts()
	bs := b.SignFacts()
	for i := range as {
		EqualBallotSignFact(t, as[i], bs[i])
	}

	t.Equal(a.Threshold(), b.Threshold())
}
