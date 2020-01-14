package mitum

import (
	"sync"

	"github.com/spikeekips/mitum/errors"
	"github.com/spikeekips/mitum/logging"
)

var (
	DaemonAlreadyStartedError = errors.NewError("daemon already started")
	DaemonAlreadyStoppedError = errors.NewError("daemon already stopped")
)

type Daemon struct {
	sync.RWMutex
	*logging.Logger
	fn           func(chan struct{}) error
	stoppingChan chan struct{}
	stopChan     chan struct{}
	stoppingWait *sync.WaitGroup
}

func NewDaemon(fn func(chan struct{}) error) *Daemon {
	dm := &Daemon{
		Logger:   logging.NewLogger(nil),
		fn:       fn,
		stopChan: make(chan struct{}),
	}

	return dm
}

func (dm *Daemon) Start() error {
	dm.Log().Debug().Msg("trying to start")

	if dm.IsStarted() {
		return DaemonAlreadyStartedError
	}

	{
		dm.Lock()
		dm.stopChan = make(chan struct{})
		dm.stoppingChan = make(chan struct{}, 2)

		dm.stoppingWait = &sync.WaitGroup{}
		dm.stoppingWait.Add(1)
		dm.Unlock()
	}

	go dm.kill()

	go func() {
		if err := dm.fn(dm.stopChan); err != nil {
			dm.Log().Error().Err(err).Msg("occurred in daemon function")
		}
		dm.stoppingChan <- struct{}{}
	}()

	dm.Log().Debug().Msg("started")
	return nil
}

func (dm *Daemon) kill() {
	<-dm.stoppingChan
	dm.stoppingWait.Done()

	dm.Lock()
	dm.stopChan = nil
	dm.stoppingChan = nil
	dm.Unlock()
}

func (dm *Daemon) Stop() error {
	dm.Log().Debug().Msg("trying to stop")

	if dm.IsStopped() {
		return DaemonAlreadyStoppedError
	}

	dm.stopChan <- struct{}{}
	dm.stoppingWait.Wait()

	dm.Lock()
	dm.stopChan = nil
	dm.stoppingChan = nil
	dm.stoppingWait = nil
	dm.Unlock()

	dm.Log().Debug().Msg("stopped")
	return nil
}

func (dm *Daemon) IsStarted() bool {
	dm.RLock()
	defer dm.RUnlock()

	return dm.stoppingChan != nil
}

func (dm *Daemon) IsStopped() bool {
	dm.RLock()
	defer dm.RUnlock()

	return dm.stoppingChan == nil
}
