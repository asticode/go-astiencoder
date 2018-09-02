package astiencoder

import (
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Worker represents a worker
type Worker struct {
	cfg Configuration
	e   *executer
	ee  *eventEmitter
	w   *astiworker.Worker
}

// NewWorker creates a new worker
func NewWorker(cfg Configuration) (w *Worker) {
	aw := astiworker.NewWorker()
	ee := newEventEmitter()
	e := newExecuter(ee, aw)
	return &Worker{
		cfg: cfg,
		e:   e,
		ee:  ee,
		w:   aw,
	}
}

// Close implements the io.Closer interface
func (w *Worker) Close() error {
	return nil
}

// Stop stops the worker
func (w *Worker) Stop() {
	w.w.Stop()
}

// HandleSignals handles signals
func (w *Worker) HandleSignals() {
	w.w.HandleSignals()
}

// Wait is a blocking pattern
func (w *Worker) Wait() {
	w.w.Wait()
}

// AddHandleEventFunc adds an handle event func
func (w *Worker) AddHandleEventFunc(f HandleEventFunc) {
	w.ee.addHandleEventFunc(f)
}

// SetJobHandler sets the job handler
func (w *Worker) SetJobHandler(h JobHandler) {
	w.e.h = h
}

// Serve creates and starts the server
func (w *Worker) Serve() {
	s := newServer(w.cfg.Server, w.ee)
	w.AddHandleEventFunc(s.handleEvent)
	w.w.Serve(w.cfg.Server.Addr, s.handler())
}

// ExecJob executes a job
func (w *Worker) ExecJob(j Job) (err error) {
	if err = w.e.execJob(j); err != nil {
		err = errors.Wrapf(err, "astiencoder: executing job %+v failed", j)
		return
	}
	return
}

// DispatchCmd dispatches a cmd
func (w *Worker) DispatchCmd(c Cmd) {
	w.e.dispatchCmd(c)
}
