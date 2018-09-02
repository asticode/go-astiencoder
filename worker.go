package astiencoder

import (
	"github.com/asticode/go-astitools/worker"
)

// Worker represents a worker
type Worker struct {
	c   *Cmds
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
	c := newCmds(e)
	return &Worker{
		c:   c,
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

// Cmds returns the commands
func (w *Worker) Cmds() Cmds {
	return *w.c
}

// AddEventHandler adds an event handler
func (w *Worker) AddEventHandler(h EventHandler) {
	w.ee.addEventHandler(h)
}

// Serve creates and starts the server
func (w *Worker) Serve() {
	s := newServer(w.cfg.Server, w.ee)
	w.AddEventHandler(s)
	w.w.Serve(w.cfg.Server.Addr, s.handler())
}

// SetJobHandler sets the job handler
func (w *Worker) SetJobHandler(h JobHandler) {
	w.e.h = h
}
