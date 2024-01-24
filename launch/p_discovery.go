package launch

import (
	"context"

	"github.com/ProtoconNet/mitum2/network/quicstream"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/ps"
)

var (
	PNameDiscoveryFlag      = ps.Name("discovery-flag")
	DiscoveryFlagContextKey = util.ContextKey("discovery-flag")
	DiscoveryContextKey     = util.ContextKey("discovery")
)

func PDiscoveryFlag(pctx context.Context) (context.Context, error) {
	e := util.StringError("prepare discovery flag")

	var flag []ConnInfoFlag
	if err := util.LoadFromContextOK(pctx, DiscoveryFlagContextKey, &flag); err != nil {
		return pctx, e.Wrap(err)
	}

	discoveries := util.EmptyLocked[[]quicstream.ConnInfo]()

	if len(flag) > 0 {
		v := make([]quicstream.ConnInfo, len(flag))

		for i := range flag {
			v[i] = flag[i].ConnInfo()
		}

		_ = discoveries.SetValue(v)
	}

	return context.WithValue(pctx, DiscoveryContextKey, discoveries), nil
}

func GetDiscoveriesFromLocked(l *util.Locked[[]quicstream.ConnInfo]) []quicstream.ConnInfo {
	switch i, isempty := l.Value(); {
	case isempty:
		return nil
	default:
		return i
	}
}
