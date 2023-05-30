//go:build test
// +build test

package base

import (
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func EqualLocalParams(t *assert.Assertions, a, b LocalParams) {
	switch {
	case a == nil && b == nil:
		return
	case a == nil || b == nil:
		t.NoError(errors.Errorf("empty"))

		return
	}

	aht := a.(hint.Hinter).Hint()
	bht := b.(hint.Hinter).Hint()
	t.True(aht.Equal(bht), "Hint does not match: %q != %q", aht, bht)

	t.Equal(a.NetworkID(), b.NetworkID())
	t.Equal(a.Threshold(), b.Threshold())
}
