package launch

import (
	"net"
	"testing"
	"time"

	"github.com/ProtoconNet/mitum2/base"
	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util/encoder"
	jsonenc "github.com/ProtoconNet/mitum2/util/encoder/json"
	"github.com/stretchr/testify/suite"
	"golang.org/x/time/rate"
)

func TestRateLimiterRuleEncode(tt *testing.T) {
	t := new(suite.Suite)
	t.SetT(tt)

	cases := []struct {
		name   string
		a      string
		result RateLimiterRule
		err    string
	}{
		{name: "example", a: "33/3s", result: NewRateLimiterRule(time.Second*3, 33)},
		{name: "nolimit", a: "nolimit", result: NoLimitRateLimiterRule()},
		{name: "limit all", a: "0", result: LimitRateLimiterRule()},
		{name: "empty", a: "  ", err: "empty"},
		{name: "missing burst", a: "/3s", err: "burst"},
		{name: "missing duration", a: "3/", err: "duration"},
		{name: "missing sep", a: "33s", err: "invalid format"},
		{name: "invalid second unit", a: "33/s", result: NewRateLimiterRule(time.Second, 33)},
		{name: "invalid millisecond unit", a: "33/ms", result: NewRateLimiterRule(time.Millisecond, 33)},
	}

	for i, c := range cases {
		i := i
		c := c

		t.Run(c.name, func() {
			var r RateLimiterRule
			err := r.UnmarshalText([]byte(c.a))

			if len(c.err) > 0 {
				t.Error(err, "%d: %v", i, c.name)
				t.ErrorContains(err, c.err, "%d: %v", i, c.name)

				return
			}

			t.EqualExportedValues(c.result, r, "%d: %v; %q != %q", i, c.name, c.result, r)
		})
	}
}

func TestNetRateLimiterRuleSetEncode(tt *testing.T) {
	t := new(encoder.BaseTestEncode)

	enc := jsonenc.NewEncoder()

	newipnet := func() *net.IPNet {
		addr := quicstream.RandomUDPAddr()

		ipnet := &net.IPNet{IP: addr.IP, Mask: net.CIDRMask(24, 32)}
		_, i, _ := net.ParseCIDR(ipnet.String())

		return i
	}

	t.Encode = func() (interface{}, []byte) {
		rs := NewNetRateLimiterRuleSet()
		rs.
			Add(
				newipnet(),
				NewRateLimiterRuleMap(
					&RateLimiterRule{Limit: rate.Every(time.Second * 22), Burst: 11},
					map[string]RateLimiterRule{
						"a": NewRateLimiterRule(time.Second*33, 44),
						"b": NoLimitRateLimiterRule(),
						"c": LimitRateLimiterRule(),
					},
				),
			).
			Add(
				newipnet(),
				NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
					"d": NewRateLimiterRule(time.Second*55, 66),
					"e": NoLimitRateLimiterRule(),
					"f": LimitRateLimiterRule(),
				}),
			)

		b, err := enc.Marshal(rs)
		t.NoError(err)

		t.T().Log("marshaled:", string(b))

		return rs, b
	}

	t.Decode = func(b []byte) interface{} {
		var rs NetRateLimiterRuleSet
		t.NoError(enc.Unmarshal(b, &rs))

		return rs
	}

	t.Compare = func(a interface{}, b interface{}) {
		ars, ok := a.(NetRateLimiterRuleSet)
		t.True(ok)
		brs, ok := b.(NetRateLimiterRuleSet)
		t.True(ok)

		t.Equal(len(ars.ipnets), len(brs.ipnets))
		for i := range ars.ipnets {
			t.Equal(ars.ipnets[i].String(), brs.ipnets[i].String())
		}

		t.Equal(len(ars.rules), len(brs.rules))
		for i := range ars.rules {
			am := ars.rules[i]
			bm := brs.rules[i]

			t.Equal(len(am.m), len(bm.m))
			if am.d != nil || bm.d != nil {
				t.EqualExportedValues(*am.d, *bm.d)
			}

			for j := range am.m {
				t.EqualExportedValues(am.m[j], bm.m[j])
			}
		}
	}

	suite.Run(tt, t)
}

func TestNodeRateLimiterRuleSetEncode(tt *testing.T) {
	t := new(encoder.BaseTestEncode)
	t.SetT(tt)

	enc := jsonenc.NewEncoder()

	t.Encode = func() (interface{}, []byte) {
		rs := NewNodeRateLimiterRuleSet(
			map[string]RateLimiterRuleMap{
				base.RandomAddress("").String(): NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
					"a": NewRateLimiterRule(time.Second*33, 44),
					"b": NoLimitRateLimiterRule(),
					"c": LimitRateLimiterRule(),
				}),
				base.RandomAddress("").String(): NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
					"d": NewRateLimiterRule(time.Second*55, 66),
					"e": NoLimitRateLimiterRule(),
					"f": LimitRateLimiterRule(),
				}),
			},
		)

		b, err := enc.Marshal(rs)
		t.NoError(err)

		t.T().Log("marshaled:", string(b))

		return rs, b
	}

	t.Decode = func(b []byte) interface{} {
		var rs NodeRateLimiterRuleSet
		t.NoError(enc.Unmarshal(b, &rs))

		return rs
	}

	t.Compare = func(a interface{}, b interface{}) {
		ars, ok := a.(NodeRateLimiterRuleSet)
		t.True(ok)
		brs, ok := b.(NodeRateLimiterRuleSet)
		t.True(ok)

		t.Equal(len(ars.rules), len(brs.rules))
		for i := range ars.rules {
			am := ars.rules[i]
			bm := brs.rules[i]

			t.Equal(len(am.m), len(bm.m))

			for j := range am.m {
				t.EqualExportedValues(am.m[j], bm.m[j])
			}
		}
	}

	suite.Run(tt, t)
}

func TestSuffrageRateLimiterRuleSetEncode(tt *testing.T) {
	t := new(encoder.BaseTestEncode)
	t.SetT(tt)

	enc := jsonenc.NewEncoder()

	t.Encode = func() (interface{}, []byte) {
		rs := NewSuffrageRateLimiterRuleSet(
			NewRateLimiterRuleMap(nil, map[string]RateLimiterRule{
				"a": NewRateLimiterRule(time.Second*33, 44),
				"b": NoLimitRateLimiterRule(),
				"c": NoLimitRateLimiterRule(),
				"d": NewRateLimiterRule(time.Second*55, 66),
				"e": NoLimitRateLimiterRule(),
				"f": LimitRateLimiterRule(),
			}),
		)

		b, err := enc.Marshal(rs)
		t.NoError(err)

		t.T().Log("marshaled:", string(b))

		return rs, b
	}

	t.Decode = func(b []byte) interface{} {
		var rs *SuffrageRateLimiterRuleSet
		t.NoError(enc.Unmarshal(b, &rs))

		return rs
	}

	t.Compare = func(a interface{}, b interface{}) {
		ars, ok := a.(*SuffrageRateLimiterRuleSet)
		t.True(ok, "%T", a)
		brs, ok := b.(*SuffrageRateLimiterRuleSet)
		t.True(ok, "%T", b)

		t.Equal(len(ars.rules.m), len(brs.rules.m))
		for i := range ars.rules.m {
			t.EqualExportedValues(ars.rules.m[i], brs.rules.m[i])
		}
	}

	suite.Run(tt, t)
}
