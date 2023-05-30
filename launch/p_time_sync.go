package launch

import (
	"context"
	"time"

	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/localtime"
	"github.com/ProtoconNet/mitum2/util/logging"
	"github.com/ProtoconNet/mitum2/util/ps"
)

var DefaultTimeSyncerInterval = time.Minute * 10

var (
	PNameTimeSyncer      = ps.Name("time-syncer")
	TimeSyncerContextKey = util.ContextKey("time-syncer")
)

func PStartTimeSyncer(pctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to prepare time syncer")

	var log *logging.Logging
	var design NodeDesign

	if err := util.LoadFromContextOK(pctx,
		LoggingContextKey, &log,
		DesignContextKey, &design,
	); err != nil {
		return pctx, e(err, "")
	}

	if len(design.TimeServer) < 1 {
		log.Log().Debug().Msg("no time server given")

		return pctx, nil
	}

	ts, err := localtime.NewTimeSyncer(design.TimeServer, design.TimeServerPort, DefaultTimeSyncerInterval)
	if err != nil {
		return pctx, e(err, "")
	}

	_ = ts.SetLogging(log)

	if err := ts.Start(context.Background()); err != nil {
		return pctx, e(err, "")
	}

	return context.WithValue(pctx, TimeSyncerContextKey, ts), nil
}

func PCloseTimeSyncer(pctx context.Context) (context.Context, error) {
	e := util.StringErrorFunc("failed to stop time syncer")

	var ts *localtime.TimeSyncer

	switch err := util.LoadFromContext(pctx, TimeSyncerContextKey, &ts); {
	case err != nil:
		return pctx, e(err, "")
	case ts == nil:
		return pctx, nil
	default:
		if err := ts.Stop(); err != nil {
			return pctx, e(err, "")
		}

		return pctx, nil
	}
}
