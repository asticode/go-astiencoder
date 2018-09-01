package astiencoder

import (
	"github.com/asticode/go-astilog"

	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

var (
	ErrEncoderIsBusy = errors.New("astiencoder: encoder is busy")
)

type dispatcher struct {
	e *encoder
	s *server
	w *astiworker.Worker
}

func newDispatcher() *dispatcher {
	return &dispatcher{}
}

func (d *dispatcher) init(e *encoder, s *server, w *astiworker.Worker) {
	d.e = e
	d.s = s
	d.w = w
}

func (d *dispatcher) dispatchJob(j Job) (err error) {
	// Encoder is busy
	if d.e.isBusy() {
		err = ErrEncoderIsBusy
		return
	}

	// Create task
	t := d.w.NewTask()
	go func() {
		// Exec job
		if err := d.e.execJob(d.w.Context(), j); err != nil {
			astilog.Error(errors.Wrapf(err, "astiencoder: executing job %+v failed", j))
		}

		// Task is done
		t.Done()
	}()
	return
}
