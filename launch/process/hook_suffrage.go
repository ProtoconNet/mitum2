package process

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/launch/config"
	"github.com/spikeekips/mitum/launch/pm"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
)

type HookHandlerSuffrage func(context.Context, map[string]interface{}) (config.Suffrage, error)

var DefaultHookHandlersSuffrage = map[string]HookHandlerSuffrage{
	"fixed-suffrage": SuffrageHandlerFixedProposer,
	"roundrobin":     SuffrageHandlerRoundrobin,
}

func HookSuffrageFunc(handlers map[string]HookHandlerSuffrage) pm.ProcessFunc {
	return func(ctx context.Context) (context.Context, error) {
		var conf config.LocalNode
		var sc map[string]interface{}
		if err := config.LoadConfigContextValue(ctx, &conf); err != nil {
			return nil, err
		} else {
			sc = conf.Source()
		}

		var m map[string]interface{}
		var st string
		if n, err := config.ParseMap(sc, "suffrage", true); err != nil {
			return ctx, err
		} else if n == nil {
			//
		} else if t, err := config.ParseType(n, true); err != nil {
			return ctx, err
		} else {
			st = t
			m = n
		}

		var sf config.Suffrage
		if len(st) < 1 {
			if i, err := SuffrageHandlerRoundrobin(ctx, nil); err != nil {
				return ctx, err
			} else {
				sf = i
			}
		} else if h, found := handlers[st]; !found {
			return nil, xerrors.Errorf("unknown suffrage found, %s", st)
		} else if i, err := h(ctx, m); err != nil {
			return nil, err
		} else {
			sf = i
		}

		if err := conf.SetSuffrage(sf); err != nil {
			return ctx, err
		} else {
			return ctx, nil
		}
	}
}

func SuffrageHandlerFixedProposer(ctx context.Context, m map[string]interface{}) (config.Suffrage, error) {
	var enc *jsonenc.Encoder
	if err := config.LoadJSONEncoderContextValue(ctx, &enc); err != nil {
		return nil, err
	}

	var proposer base.Address
	if i, found := m["proposer"]; !found {
		return nil, xerrors.Errorf("proposer not set for fixed-suffrage")
	} else if a, err := parseAddress(i, enc); err != nil {
		return nil, xerrors.Errorf("invalid proposer address for fixed-suffrage: %w", err)
	} else {
		proposer = a
	}

	var nodes []base.Address
	if i, found := m["nodes"]; found {
		if l, ok := i.([]interface{}); !ok {
			return nil, xerrors.Errorf("invalid nodes list, %T", i)
		} else {
			nodes = make([]base.Address, len(l))
			for j := range l {
				if a, err := parseAddress(l[j], enc); err != nil {
					return nil, xerrors.Errorf("invalid node address for fixed-suffrage: %w", err)
				} else {
					nodes[j] = a
				}
			}
		}
	}

	return config.NewFixedSuffrage(proposer, nodes)
}

func SuffrageHandlerRoundrobin(_ context.Context, m map[string]interface{}) (config.Suffrage, error) {
	var numberOfActing uint
	if i, found := m["number-of-acting"]; !found {
		numberOfActing = isaac.DefaultPolicyNumberOfActingSuffrageNodes
	} else {
		switch n := i.(type) {
		case int:
			numberOfActing = uint(n)
		case uint:
			numberOfActing = n
		default:
			return nil, xerrors.Errorf("invalid type for number-of-acting, %T", i)
		}
	}

	return config.NewRoundrobinSuffrage(numberOfActing), nil
}

func parseAddress(i interface{}, enc *jsonenc.Encoder) (base.Address, error) {
	if s, ok := i.(string); !ok {
		return nil, xerrors.Errorf("not address string, not %T", i)
	} else if address, err := base.DecodeAddressFromString(enc, s); err != nil {
		return nil, xerrors.Errorf("invalid address: %w", err)
	} else {
		return address, err
	}
}
