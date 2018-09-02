package astiencoder

import (
	"sync"

	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Errors
var (
	ErrExecuterIsBusy = errors.New("astiencoder: executer is busy")
)

type executer struct {
	busy bool
	e   *eventEmitter
	h    JobHandler
	m    *sync.Mutex
	w    *astiworker.Worker
}

func newExecuter(e *eventEmitter, w *astiworker.Worker) *executer {
	return &executer{
		e: e,
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

func (e *executer) execJob(j Job) (err error) {
	// No handler
	if e.h == nil {
		return errors.New("astiencoder: no job handler")
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
		if err = e.h(e.w.Context(), j, e.e.emit, t.NewSubTask); err != nil {
			e.e.emit(EventError(errors.Wrapf(err, "astiencoder: handling job %+v failed", j)))
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
