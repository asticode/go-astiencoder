package astiencoder

import (
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/go-astiws"
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
	s := newServer(c.Server, d, m)
	e := newEncoder(d)
	d.init(e, s)
	return &Worker{
		d:      d,
		e:      e,
		m:      m,
		s:      s,
		Worker: astiworker.NewWorker(),
	}
}

// Close implements the io.Closer interface
func (w *Worker) Close() error {
	return nil
}

// Start starts the worker
func (w *Worker) Start() (err error) {
	// Start the server
	w.s.start(w.Worker.Serve)
	return
}
