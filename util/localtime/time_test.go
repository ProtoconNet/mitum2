package localtime

import (
	"testing"
	"time"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type testTime struct {
	suite.Suite
}

func (t *testTime) loadTime(s string) Time {
	a, err := util.ParseRFC3339(s)
	t.NoError(err)

	return New(a)
}

func (t *testTime) TestNew() {
	now := Now()
	tm := New(now)

	t.True(now.Equal(tm.Time))
}

func (t *testTime) TestNormalizeToUTC() {
	tm, err := util.ParseRFC3339("2009-11-10T10:00:00.001+09:00")
	t.NoError(err)

	_, offset := tm.Zone()
	t.Equal(32400, offset)

	_, offset = util.NormalizeTime(tm).Zone()

	t.Equal(0, offset)
}

func (t *testTime) TestBytesNormalize() {
	s0 := "2009-11-10T23:00:00.00101010Z"
	s1 := "2009-11-10T23:00:00.001Z"

	a := t.loadTime(s0)
	b := t.loadTime(s1)

	t.Equal(a.Bytes(), b.Bytes())
}

func (t *testTime) TestNormalizeCases() {
	cases := []struct {
		name     string
		s        string
		expected string
	}{
		{
			name:     "nano",
			s:        "2009-11-10T23:00:00.00101010Z",
			expected: "2009-11-10T23:00:00.00101010Z",
		},
		{
			name:     "different nano",
			s:        "2009-11-10T23:00:00.00101010Z",
			expected: "2009-11-10T23:00:00.001Z",
		},
		{
			name:     "no nano",
			s:        "2009-11-10T23:00:00Z",
			expected: "2009-11-10T23:00:00Z",
		},
		{
			name:     "different timezone, but same",
			s:        "2009-11-10T01:00:00.001Z",
			expected: "2009-11-10T10:00:00.001+09:00",
		},
	}

	for i, c := range cases {
		i := i
		c := c
		if !t.Run(
			c.name,
			func() {
				a, err := util.ParseRFC3339(c.s)
				t.NoError(err)

				b, err := util.ParseRFC3339(c.expected)
				t.NoError(err)

				an := util.NormalizeTime(a)
				bn := util.NormalizeTime(b)

				t.True(bn.Equal(an), "%d: %v; %v != %v", i, c.name, bn.String(), an.String())
			},
		) {
			break
		}
	}
}

func (t *testTime) TestWithin() {
	cases := []struct {
		name     string
		base     string
		target   string
		d        time.Duration
		expected bool
	}{
		{
			name:     "zero duration; same",
			base:     "2009-11-10T23:00:00.00101010Z",
			target:   "2009-11-10T23:00:00.00101010Z",
			d:        0,
			expected: true,
		},
		{
			name:     "zero duration; not same",
			base:     "2009-11-10T23:00:00.00101010Z",
			target:   "2009-11-10T23:00:01.00101010Z",
			d:        0,
			expected: false,
		},
		{
			name:     "negative duration; same",
			base:     "2009-11-10T23:00:00.00101010Z",
			target:   "2009-11-10T23:00:00.00101010Z",
			d:        -1,
			expected: true,
		},
		{
			name:     "negative duration; not same",
			base:     "2009-11-10T23:00:00.00101010Z",
			target:   "2009-11-10T23:00:01.00101010Z",
			d:        -1,
			expected: false,
		},
		{
			name:     "ok #0",
			base:     "2009-11-10T23:00:00.00101010Z",
			target:   "2009-11-10T23:00:01.00101010Z",
			d:        time.Second,
			expected: true,
		},
		{
			name:     "ok #1",
			base:     "2009-11-10T23:00:01.00101010Z",
			target:   "2009-11-10T23:00:00.00101010Z",
			d:        time.Second,
			expected: true,
		},
		{
			name:     "not ok #0",
			base:     "2009-11-10T23:00:00.00101010Z",
			target:   "2009-11-10T23:00:02.00101010Z",
			d:        time.Second,
			expected: false,
		},
		{
			name:     "not ok #1",
			base:     "2009-11-10T23:00:02.00101010Z",
			target:   "2009-11-10T23:00:00.00101010Z",
			d:        time.Second,
			expected: false,
		},
	}

	for i, c := range cases {
		i := i
		c := c
		if !t.Run(
			c.name,
			func() {
				base, err := util.ParseRFC3339(c.base)
				t.NoError(err)
				target, err := util.ParseRFC3339(c.target)
				t.NoError(err)

				bn := util.NormalizeTime(base)
				tn := util.NormalizeTime(target)

				r := Within(bn, tn, c.d)

				t.Equal(c.expected, r, "%d: %v; %v, %v, %v", i, c.name, bn.String(), tn.String(), c.d)
			},
		) {
			break
		}
	}
}

func TestTime(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(testTime))
}
