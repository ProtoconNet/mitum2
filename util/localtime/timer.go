package localtime

import (
	"time"

	"github.com/spikeekips/mitum/util"
)

var StopTimerError = util.NewError("stop timer")

type TimerID string

func (ti TimerID) String() string {
	return string(ti)
}

type Timer interface {
	util.Daemon
	IsStarted() bool
	ID() TimerID
	SetInterval(func(int) time.Duration) Timer
	Reset() error
}
