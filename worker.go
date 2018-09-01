package astiencoder

import (
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/go-astiws"
	"github.com/pkg/errors"
)

// Worker represents a worker
type Worker struct {
	d *dispatcher
	e *encoder
	m *astiws.Manager
	s *server
	*astiworker.Worker
}

// NewWorker creates a new worker
func NewWorker(c Configuration) (w *Worker) {
	d := newDispatcher()
	m := astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 8192})
	aw := astiworker.NewWorker()
	s := newServer(c.Server, d, m, aw)
	e := newEncoder(d)
	d.init(e, s, aw)
	return &Worker{
		d:      d,
		e:      e,
		m:      m,
		s:      s,
		Worker: aw,
	}
}

// Close implements the io.Closer interface
func (w *Worker) Close() error {
	return nil
}

// Serve starts the server
func (w *Worker) Serve() {
	w.s.serve()
}

// DispatchJob dispatches a job
func (w *Worker) DispatchJob(j Job) (err error) {
	if err = w.d.dispatchJob(j); err != nil {
		err = errors.Wrap(err, "astiencoder: dispatching job failed")
		return
	}
	return
}
