package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type testRetry struct {
	suite.Suite
}

func (t *testRetry) TestNoError() {
	var called int
	err := Retry(3, 0, func() error {
		called++

		return nil
	})
	t.NoError(err)
	t.Equal(1, called)
}

func (t *testRetry) TestErrorAndSuccess() {
	var called int
	err := Retry(3, 0, func() error {
		defer func() {
			called++
		}()

		if called == 0 {
			return fmt.Errorf("error")
		}

		return nil
	})
	t.NoError(err)
	t.Equal(2, called)
}

func (t *testRetry) TestError() {
	var called int = 0
	err := Retry(3, 0, func() error {
		defer func() {
			called++
		}()

		return fmt.Errorf("error: %d", called+1)
	})
	t.Contains(err.Error(), "3")
	t.Equal(3, called)
}

func TestRetry(t *testing.T) {
	suite.Run(t, new(testRetry))
}