package astiencoder

import (
	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Cmds lists all commands you can send to the worker
type Cmds struct {
	e *executer
	w *astiworker.Worker
}

func newCmds(e *executer, w *astiworker.Worker) *Cmds {
	return &Cmds{
		e: e,
		w: w,
	}
}

// ExecJob commands the worker to execute a new job
func (c Cmds) ExecJob(job interface{}) (err error) {
	// Lock executer
	if err = c.e.lock(); err != nil {
		err = errors.Wrap(err, "astiencoder: locking executer failed")
		return
	}

	// Create task
	t := c.w.NewTask()
	go func() {
		// Exec job
		if err := c.e.execJob(c.w.Context(), job); err != nil {
			astilog.Error(errors.Wrapf(err, "astiencoder: executing job %+v failed", job))
		}

		// Unlock executer
		c.e.unlock()

		// Task is done
		t.Done()
	}()
	return
}
