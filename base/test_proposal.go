//go:build test
// +build test

package base

import (
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func EqualProposalSignFact(t *assert.Assertions, a, b ProposalSignFact) {
	switch {
	case a == nil && b == nil:
		return
	case a == nil || b == nil:
		t.NoError(errors.Errorf("empty"))

		return
	}

	EqualSignFact(t, a, b)

	aht := a.(hint.Hinter).Hint()
	bht := b.(hint.Hinter).Hint()
	t.True(aht.Equal(bht), "Hint does not match")

	EqualProposalFact(t, a.ProposalFact(), b.ProposalFact())
}

func EqualProposalFact(t *assert.Assertions, a, b ProposalFact) {
	switch {
	case a == nil && b == nil:
		return
	case a == nil || b == nil:
		t.NoError(errors.Errorf("empty"))

		return
	}

	EqualFact(t, a, b)
	t.Equal(a.Point(), b.Point())
	t.True(a.Proposer().Equal(b.Proposer()))

	t.Equal(len(a.Operations()), len(b.Operations()))

	aop := a.Operations()
	bop := b.Operations()
	for i := range aop {
		t.True(aop[i].Equal(bop[i]))
	}
	t.True(util.TimeEqual(a.ProposedAt(), b.ProposedAt()))
}
