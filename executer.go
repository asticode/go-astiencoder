package astiencoder

import (
	"context"
	"sync"

	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Errors
var (
	ErrExecuterIsBusy = errors.New("astiencoder: executer is busy")
	ErrNoJobHandler   = errors.New("astiencoder: no job handler")
)

// JobHandler handles a job
// Make sure the execution is shut down gracefully when context is cancelled
type JobHandler func(ctx context.Context, t *astiworker.Task, job interface{}, e EventEmitter) error

type executer struct {
	busy bool
	ee   *eventEmitter
	h    JobHandler
	m    *sync.Mutex
	w    *astiworker.Worker
}

func newExecuter(ee *eventEmitter, w *astiworker.Worker) *executer {
	return &executer{
		ee: ee,
		m:  &sync.Mutex{},
		w:  w,
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

func (e *executer) execJob(job interface{}) (err error) {
	// No handler
	if e.h == nil {
		return ErrNoJobHandler
	}

	// Lock executer
	if err = e.lock(); err != nil {
		err = errors.Wrap(err, "astiencoder: locking executer failed")
		return
	}

	// Create task
	t := e.w.NewTask()
	go func() {
		// Handle job
		if err = e.h(e.w.Context(), t, job, e.ee); err != nil {
			astilog.Error(errors.Wrapf(err, "astiencoder: handling job %+v failed", job))
			return
		}

		// Wait for task to be done
		t.Wait()

		// Unlock executer
		e.unlock()

		// Task is done
		t.Done()
	}()
	return
}
