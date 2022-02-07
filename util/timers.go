package util

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/spikeekips/mitum/util/logging"
)

// Timers handles the multiple timers and controls them selectively.
type Timers struct {
	*logging.Logging
	sync.RWMutex
	timers   map[ /* timer id */ TimerID]Timer
	allowNew bool // if allowNew is true, new timer can be added.
}

func NewTimers(ids []TimerID, allowNew bool) *Timers {
	timers := map[TimerID]Timer{}
	for _, id := range ids {
		timers[id] = nil
	}

	return &Timers{
		Logging: logging.NewLogging(func(c zerolog.Context) zerolog.Context {
			return c.Str("module", "timers")
		}),
		timers:   timers,
		allowNew: allowNew,
	}
}

func (ts *Timers) SetLogging(l *logging.Logging) *logging.Logging {
	ts.Lock()
	defer ts.Unlock()

	for id := range ts.timers {
		timer := ts.timers[id]
		if timer == nil {
			continue
		}

		if i, ok := timer.(logging.SetLogging); ok {
			_ = i.SetLogging(l)
		}
	}

	return ts.Logging.SetLogging(l)
}

// Start of Timers does nothing
func (*Timers) Start() error {
	return nil
}

// Stop of Timers will stop all the timers
func (ts *Timers) Stop() error {
	ts.Lock()
	defer ts.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(ts.timers))

	for id := range ts.timers {
		timer := ts.timers[id]
		if timer == nil {
			wg.Done()
			continue
		}

		go func(t Timer) {
			defer wg.Done()

			if err := t.Stop(); err != nil {
				ts.Log().Error().Err(err).Stringer("timer", t.ID()).Msg("failed to stop timer")
			}
		}(timer)
	}

	wg.Wait()

	ts.timers = map[TimerID]Timer{}

	return nil
}

func (ts *Timers) ResetTimer(id TimerID) error {
	ts.RLock()
	defer ts.RUnlock()

	switch t, found := ts.timers[id]; {
	case !found:
		return errors.Errorf("timer, %q not found", id)
	case t == nil:
		return errors.Errorf("timer, %q not running", id)
	default:
		return t.Reset()
	}
}

// SetTimer sets the timer with id
func (ts *Timers) SetTimer(timer Timer) error {
	ts.Lock()
	defer ts.Unlock()

	if _, found := ts.timers[timer.ID()]; !found {
		if !ts.allowNew {
			return errors.Errorf("not allowed to add new timer: %s", timer.ID())
		}
	}

	if existing := ts.timers[timer.ID()]; existing != nil && existing.IsStarted() {
		if err := existing.Stop(); err != nil {
			return errors.Wrapf(err, "failed to stop timer, %q", timer.ID())
		}
	}

	ts.timers[timer.ID()] = timer

	if timer != nil {
		if l, ok := ts.timers[timer.ID()].(logging.SetLogging); ok {
			_ = l.SetLogging(ts.Logging)
		}
	}

	return nil
}

// StartTimers starts timers with the given ids, before starting timers, stops
// the other timers if stopOthers is true.
func (ts *Timers) StartTimers(ids []TimerID, stopOthers bool) error {
	ts.Lock()
	defer ts.Unlock()

	sids := make([]string, len(ids))
	for i := range ids {
		sids[i] = ids[i].String()
	}

	if stopOthers {
		var stopIDs []TimerID
		for id := range ts.timers {
			if InStringSlice(id.String(), sids) {
				continue
			}
			stopIDs = append(stopIDs, id)
		}

		if len(stopIDs) > 0 {
			if err := ts.stopTimers(stopIDs); err != nil {
				return errors.Wrap(err, "failed to start timers")
			}
		}
	}

	callback := func(t Timer) {
		if t.IsStarted() {
			return
		}

		if err := t.Start(); err != nil {
			ts.Log().Error().Err(err).Stringer("timer", t.ID()).Msg("failed to start timer")
		}
	}

	return ts.traverse(callback, ids)
}

func (ts *Timers) StopTimers(ids []TimerID) error {
	ts.Lock()
	defer ts.Unlock()

	return ts.stopTimers(ids)
}

func (ts *Timers) StopTimersAll() error {
	ts.Lock()
	defer ts.Unlock()

	ids := make([]TimerID, len(ts.timers))

	var i int
	for id := range ts.timers {
		ids[i] = id
		i++
	}

	if len(ids) < 1 {
		return nil
	}

	return ts.stopTimers(ids)
}

func (ts *Timers) Started() []TimerID {
	ts.RLock()
	defer ts.RUnlock()

	var started []TimerID
	for id := range ts.timers {
		timer := ts.timers[id]
		if timer != nil && ts.timers[id].IsStarted() {
			started = append(started, id)
		}
	}

	return started
}

func (ts *Timers) IsTimerStarted(id TimerID) bool {
	ts.RLock()
	defer ts.RUnlock()

	switch timer, found := ts.timers[id]; {
	case !found:
		return false
	case timer == nil:
		return false
	default:
		return timer.IsStarted()
	}
}

func (ts *Timers) stopTimers(ids []TimerID) error {
	callback := func(t Timer) {
		if !t.IsStarted() {
			return
		}

		if err := t.Stop(); err != nil {
			ts.Log().Error().Err(err).Stringer("timer", t.ID()).Msg("failed to stop timer")
		}
	}

	if err := ts.traverse(callback, ids); err != nil {
		return errors.Wrap(err, "failed to stop timers")
	}

	for _, id := range ids {
		ts.timers[id] = nil
	}

	return nil
}

func (ts *Timers) checkExists(ids []TimerID) error {
	for _, id := range ids {
		if _, found := ts.timers[id]; !found {
			return errors.Errorf("timer not found: %s", id)
		}
	}

	return nil
}

func (ts *Timers) traverse(callback func(Timer), ids []TimerID) error {
	if err := ts.checkExists(ids); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(ids))

	for _, id := range ids {
		go func(id TimerID) {
			defer wg.Done()

			timer := ts.timers[id]
			if timer == nil {
				return
			}

			callback(timer)
		}(id)
	}

	wg.Wait()

	return nil
}
