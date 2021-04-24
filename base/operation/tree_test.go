package operation

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"golang.org/x/xerrors"
)

type testOperationReasonError struct {
	suite.Suite
}

func (t *testOperationReasonError) TestMake() {
	err := NewBaseReasonError("show me").SetData(map[string]interface{}{"a": 1})

	_ = (interface{})(err).(ReasonError)
	_, ok := (interface{})(err).(ReasonError)
	t.True(ok)

	t.Implements((*ReasonError)(nil), err)

	var uerr ReasonError
	t.True(xerrors.As(err, &uerr))
	t.Equal(err.Msg(), uerr.Msg())
	t.Equal(err.data, uerr.Data())
}

func TestOperationReasonError(t *testing.T) {
	suite.Run(t, new(testOperationReasonError))
}
