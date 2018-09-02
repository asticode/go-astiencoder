package astiencoder

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

// Errors
var (
	ErrExecuterIsBusy = errors.New("astiencoder: executer is busy")
	ErrNoJobHandler   = errors.New("astiencoder: no job handler")
)

// JobHandler handles a job
// Make sure the execution is shut down gracefully when context is cancelled
type JobHandler func(ctx context.Context, job interface{}, e EventEmitter) error

type executer struct {
	busy bool
	ee   *eventEmitter
	h    JobHandler
	m    *sync.Mutex
}

func newExecuter(ee *eventEmitter) *executer {
	return &executer{
		ee: ee,
		m:  &sync.Mutex{},
	}
}

func (e *executer) lock() error {
	e.m.Lock()
	defer e.m.Unlock()
	if e.busy {
		return ErrExecuterIsBusy
	}
	e.busy = true
	return nil
}

func (e *executer) unlock() {
	e.m.Lock()
	defer e.m.Unlock()
	e.busy = false
}

func (e *executer) execJob(ctx context.Context, job interface{}) (err error) {
	// No handler
	if e.h == nil {
		return ErrNoJobHandler
	}

	// Handle job
	if err = e.h(ctx, job, e.ee); err != nil {
		err = errors.Wrapf(err, "astiencoder: handling job %+v failed", job)
		return
	}
	return
}
