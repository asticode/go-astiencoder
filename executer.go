package astiencoder

import (
	"sync"

	"github.com/asticode/go-astilog"

	"io"

	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Errors
var (
	ErrExecuterIsBusy = errors.New("astiencoder: executer is busy")
)

// ExecOptions represents execution options
type ExecOptions struct {
	QuitWhenDone bool `json:"quit_when_done"`
}

type executer struct {
	busy  bool
	count int
	e     *eventEmitter
	h     JobHandler
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

func (e *executer) isBusy() bool {
	e.m.Lock()
	defer e.m.Unlock()
	return e.busy
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

func (e *executer) startJob(j Job, o ExecOptions) (err error) {
	// Log
	astilog.Debugf("astiencoder: starting job %+v with exec options %+v", j, o)

	// No job handler
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
		// Inc
		count := e.inc()

		// Handle job
		var c io.Closer
		astilog.Debug("astiencoder: handling job")
		if c, err = e.h.HandleJob(e.w.Context(), j, e.e.emit, t.NewSubTask); err != nil {
			e.e.emit(EventError(errors.Wrapf(err, "astiencoder: execution #%d of job %+v failed", count, j)))
		}

		// Wait for task to be done
		t.Wait()

		// Close
		if c != nil {
			astilog.Debug("astiencoder: closing job")
			if err = c.Close(); err != nil {
				e.e.emit(EventError(errors.Wrapf(err, "astiencoder: closing execution #%d for job %+v failed", count, j)))
			}
		}

		// Stop worker
		if o.QuitWhenDone {
			e.w.Stop()
		}

		// Unlock executer
		e.unlock()

		// Task is done
		t.Done()
	}()
	return
}

func (e *executer) dispatchCmd(c Cmd) {
	// No job handler
	if e.h == nil {
		return
	}

	// Executer is not busy
	if !e.isBusy() {
		return
	}

	// Handle cmd
	if v, ok := e.h.(CmdHandler); ok {
		go v.HandleCmd(c)
	}
}
