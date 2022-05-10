package isaacnetwork

import (
	"context"
	"math"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/network/quictransport"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/logging"
)

var MinIntervalSuffrageChecker = time.Second

type SuffrageCheckerCallback func(context.Context, base.SuffrageInfo)

type SuffrageChecker struct {
	*logging.Logging
	*util.ContextDaemon
	discoveries                []quictransport.ConnInfo
	lastSuffrage               isaac.LastSuffrageFunc
	minIntervalSuffrageChecker time.Duration
	interval                   time.Duration
	info                       *util.Locked
	ncbs                       *util.Locked
}

func NewSuffrageChecker(
	discoveries []quictransport.ConnInfo,
	initialInfo base.SuffrageInfo,
	lastSuffrage isaac.LastSuffrageFunc,
) (*SuffrageChecker, error) {
	if len(discoveries) < 1 {
		return nil, errors.Errorf("empty discoveries")
	}

	s := &SuffrageChecker{
		Logging: logging.NewLogging(func(zctx zerolog.Context) zerolog.Context {
			return zctx.Str("module", "suffrage-checker")
		}),
		discoveries:                discoveries,
		lastSuffrage:               lastSuffrage,
		info:                       util.NewLocked(initialInfo),
		ncbs:                       util.EmptyLocked(),
		minIntervalSuffrageChecker: MinIntervalSuffrageChecker,
	}

	s.ContextDaemon = util.NewContextDaemon("suffrage-checker", s.start)

	return s, nil
}

func (s *SuffrageChecker) Check(ctx context.Context) (base.SuffrageInfo, bool, error) {
	return s.check(ctx)
}

func (s *SuffrageChecker) SuffrageInfo() base.SuffrageInfo {
	return s.suffrageInfo()
}

func (s *SuffrageChecker) suffrageInfo() base.SuffrageInfo {
	switch i, isnil := s.info.Value(); {
	case isnil, i == nil:
		return nil
	default:
		return i.(base.SuffrageInfo)
	}
}

func (s *SuffrageChecker) AddCallback(cb SuffrageCheckerCallback) *SuffrageChecker {
	_, _ = s.ncbs.Set(func(i interface{}) (interface{}, error) {
		var ncbs []SuffrageCheckerCallback
		if i != nil {
			ncbs = i.([]SuffrageCheckerCallback)
		}

		ncbs = append(ncbs, cb)

		return ncbs, nil
	})

	return s
}

func (s *SuffrageChecker) start(ctx context.Context) error {
	defer s.Log().Debug().Msg("stopped")

	if s.interval < s.minIntervalSuffrageChecker {
		s.interval = s.minIntervalSuffrageChecker
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

end:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			switch info, updated, err := s.check(ctx); {
			case err != nil:
				s.Log().Error().Err(err).Msg("failed to check new suffrage")

				continue end
			case updated:
				s.notify(ctx, info)
			}
		}
	}
}

func (s *SuffrageChecker) check(ctx context.Context) (base.SuffrageInfo, bool, error) {
	var updated bool
	i, err := s.info.Set(func(i interface{}) (interface{}, error) {
		var oldinfo base.SuffrageInfo
		if i != nil {
			oldinfo = i.(base.SuffrageInfo)
		}

		switch newinfo, err := s.find(ctx, oldinfo); {
		case err != nil:
			return nil, errors.Wrap(err, "")
		case newinfo == nil:
			return oldinfo, nil
		default:
			updated = oldinfo == nil || newinfo.Height() > oldinfo.Height()

			return newinfo, nil
		}
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "")
	}

	n := i.(base.SuffrageInfo)

	if updated {
		s.Log().Debug().Interface("suffrage_info", n).Msg("new suffrage info found")
	}

	return n, updated, nil
}

func (s *SuffrageChecker) find(ctx context.Context, oldinfo base.SuffrageInfo) (base.SuffrageInfo, error) {
	e := util.StringErrorFunc("failed to check")

	if len(s.discoveries) < 1 {
		return nil, nil
	}

	worker := util.NewDistributeWorker(ctx, math.MaxInt32, nil)
	defer worker.Close()

	newinfo := util.NewLocked(oldinfo)

	for i := range s.discoveries {
		conninfo := s.discoveries[i]

		if err := worker.NewJob(func(ctx context.Context, _ uint64) error {
			l := s.Log().With().Interface("conninfo", conninfo).Logger()

			rinfo, found, err := s.lastSuffrage(ctx, conninfo)
			switch {
			case err != nil:
				l.Error().Err(err).Msg("failed to check last suffrage info from remote node")

				return err
			case !found:
				err = util.NotFoundError.Errorf("no last suffrage info")

				l.Error().Err(err).Msg("failed to check last suffrage info from remote node")

				return err
			}

			_, _ = newinfo.Set(func(i interface{}) (interface{}, error) {
				if i == nil {
					return rinfo, nil
				}

				old := i.(base.SuffrageInfo)
				if rinfo.Height() > old.Height() {
					return rinfo, nil
				}

				return i, nil
			})

			return nil
		}); err != nil {
			return nil, e(err, "")
		}
	}

	worker.Done()

	if err := worker.Wait(); err != nil {
		return nil, e(err, "")
	}

	switch i, isnil := newinfo.Value(); {
	case isnil, i == nil:
		return nil, nil
	default:
		return i.(base.SuffrageInfo), nil
	}
}

func (s *SuffrageChecker) notifyCallbacks() []SuffrageCheckerCallback {
	switch i, isnil := s.ncbs.Value(); {
	case isnil, i == nil:
		return nil
	default:
		return i.([]SuffrageCheckerCallback)
	}
}

func (s *SuffrageChecker) notify(ctx context.Context, info base.SuffrageInfo) {
	l := s.Log().With().Interface("suffrage_info", info).Logger()
	l.Debug().Msg("new suffrage found")

	ncbs := s.notifyCallbacks()
	if len(ncbs) < 1 {
		return
	}

	worker := util.NewDistributeWorker(ctx, math.MaxInt32, nil)
	defer worker.Close()

	for i := range ncbs {
		cb := ncbs[i]

		if err := worker.NewJob(func(ctx context.Context, _ uint64) error {
			cb(ctx, info)

			return nil
		}); err != nil {
			l.Error().Err(err).Msg("callback failed")

			return
		}
	}

	worker.Done()
	if err := worker.Wait(); err != nil {
		l.Error().Err(err).Msg("callback failed")

		return
	}

	l.Debug().Msg("new suffrage updated")
}