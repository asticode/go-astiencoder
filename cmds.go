package astiencoder

import (
	"github.com/pkg/errors"
)

// Cmds lists all commands you can send to the worker
type Cmds struct {
	e *executer
}

func newCmds(e *executer) *Cmds {
	return &Cmds{
		e: e,
	}
}

// ExecJob commands the worker to execute a new job
func (c Cmds) ExecJob(job interface{}) (err error) {
	if err = c.e.execJob(job); err != nil {
		err = errors.Wrapf(err, "astiencoder: executing job %+v failed", job)
		return
	}
	return
}
