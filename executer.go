package astiencoder

import (
	"sync"

	"io"

	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Errors
var (
	ErrExecuterIsBusy = errors.New("astiencoder: executer is busy")
)

type executer struct {
	busy  bool
	count int
	e     *eventEmitter
	f     HandleJobFunc
	m     *sync.Mutex
	w     *astiworker.Worker
}

func newExecuter(e *eventEmitter, w *astiworker.Worker) *executer {
	return &executer{
		e: e,
		m: &sync.Mutex{},
		w: w,
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

func (e *executer) inc() int {
	e.m.Lock()
	defer e.m.Unlock()
	e.count++
	return e.count
}

func (e *executer) execJob(j Job) (err error) {
	// No job handle func
	if e.f == nil {
		return errors.New("astiencoder: no job handle func")
	}

	// Lock executer
	if err = e.lock(); err != nil {
		err = errors.Wrap(err, "astiencoder: locking executer failed")
		return
	}

	// Create task
	t := e.w.NewTask()
	go func() {
		// Inc
		count := e.inc()

		// Handle job
		var c io.Closer
		if c, err = e.f(e.w.Context(), j, e.e.emit, t.NewSubTask); err != nil {
			e.e.emit(EventError(errors.Wrapf(err, "astiencoder: execution #%d of job %+v failed", count, j)))
		}

		// Wait for task to be done
		t.Wait()

		// Close
		if c != nil {
			if err = c.Close(); err != nil {
				e.e.emit(EventError(errors.Wrapf(err, "astiencoder: closing execution #%d for job %+v failed", count, j)))
			}
		}

		// Unlock executer
		e.unlock()

		// Task is done
		t.Done()
	}()
	return
}
