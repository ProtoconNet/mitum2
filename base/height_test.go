package base

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum/util"
	"github.com/stretchr/testify/suite"
)

type testHeight struct {
	suite.Suite
}

func (t *testHeight) TestNew() {
	h10 := Height(10)
	t.Equal(int64(10), int64(h10))
}

func (t *testHeight) TestInt64() {
	h10 := Height(10)
	t.Equal(int64(10), h10.Int64())
}

func (t *testHeight) TestInvalid() {
	h10 := Height(10)
	t.NoError(h10.IsValid(nil))

	hu1 := NilHeight
	err := hu1.IsValid(nil)
	t.True(errors.Is(err, util.InvalidError))
}

func TestHeight(t *testing.T) {
	suite.Run(t, new(testHeight))
}
