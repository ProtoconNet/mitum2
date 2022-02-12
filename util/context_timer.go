package util

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/util/logging"
)

var ignoreError = NewError("ignore context timer error")

var contextTimerPool = sync.Pool{
	New: func() interface{} {
		return new(ContextTimer)
	},
}

var (
	ContextTimerPoolGet = func() *ContextTimer {
		return contextTimerPool.Get().(*ContextTimer)
	}
	ContextTimerPoolPut = func(ct *ContextTimer) {
		ct.Lock()
		ct.Logging = nil
		ct.ContextDaemon = nil
		ct.id = TimerID("")
		ct.interval = nil
		ct.callback = nil
		ct.c = 0
		ct.Unlock()

		contextTimerPool.Put(ct)
	}
)

type ContextTimer struct {
	sync.RWMutex
	*logging.Logging
	*ContextDaemon
	id       TimerID
	interval func(int) time.Duration
	callback func(int) (bool, error)
	c        int
}

func NewContextTimer(id TimerID, interval time.Duration, callback func(int) (bool, error)) *ContextTimer {
	ct := ContextTimerPoolGet()
	ct.RWMutex = sync.RWMutex{}
	ct.Logging = logging.NewLogging(func(c zerolog.Context) zerolog.Context {
		return c.Str("module", "context-timer").Stringer("id", id)
	})
	ct.id = id
	ct.interval = func(int) time.Duration {
		return interval
	}
	ct.callback = callback
	ct.ContextDaemon = NewContextDaemon("timer-"+string(id), ct.start)

	return ct
}

func (ct *ContextTimer) ID() TimerID {
	return ct.id
}

func (ct *ContextTimer) SetInterval(f func(int) time.Duration) Timer {
	ct.Lock()
	defer ct.Unlock()

	ct.interval = f

	return ct
}

func (ct *ContextTimer) Reset() error {
	ct.Lock()
	defer ct.Unlock()

	ct.c = 0

	return nil
}

func (ct *ContextTimer) SetLogging(l *logging.Logging) *logging.Logging {
	_ = ct.ContextDaemon.SetLogging(l)

	return ct.Logging.SetLogging(l)
}

func (ct *ContextTimer) Stop() error {
	errch := make(chan error)
	go func() {
		errch <- ct.ContextDaemon.Stop()
	}()

	select {
	case err := <-errch:
		return errors.Wrap(err, "failed to stop ContextTimer")
	case <-time.After(time.Millisecond * 300):
		break
	}

	go func() {
		<-errch
		ContextTimerPoolPut(ct)
	}()

	return nil
}

func (ct *ContextTimer) count() int {
	ct.RLock()
	defer ct.RUnlock()

	return ct.c
}

func (ct *ContextTimer) start(ctx context.Context) error {
	if err := ct.Reset(); err != nil {
		return errors.Wrap(err, "failed to start ContextTimer")
	}

end:
	for {
		select {
		case <-ctx.Done():
			break end
		default:
			if err := ct.prepareCallback(ctx); err != nil {
				switch {
				case errors.Is(err, StopTimerError):
				case errors.Is(err, ignoreError):
				default:
					ct.Log().Debug().Err(err).Msg("timer got error; timer will be stopped")
				}

				break end
			}
		}
	}

	return nil
}

func (ct *ContextTimer) finishCallback(count int) {
	ct.Lock()
	defer ct.Unlock()

	if ct.c == count {
		ct.c++
	}
}

func (ct *ContextTimer) prepareCallback(ctx context.Context) error {
	ct.RLock()
	intervalfunc := ct.interval
	callback := ct.callback
	ct.RUnlock()

	if intervalfunc == nil || callback == nil {
		return ignoreError.Errorf("empty interval or callback")
	}

	count := ct.count()
	interval := intervalfunc(count)
	if interval < time.Nanosecond {
		return errors.Errorf("invalid interval; too narrow, %v", interval)
	}

	return ct.waitAndRun(ctx, interval, callback, count)
}

func (ct *ContextTimer) waitAndRun(
	ctx context.Context,
	interval time.Duration,
	callback func(int) (bool, error),
	count int,
) error {
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(interval):
	}

	if keep, err := callback(count); err != nil {
		return errors.Wrap(err, "failed to callback in ContextTimer")
	} else if !keep {
		return StopTimerError.Call()
	}

	ct.finishCallback(count)

	return nil
}