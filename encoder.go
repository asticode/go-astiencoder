package astiencoder

import (
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Encoder represents a encoder
type Encoder struct {
	cfg Configuration
	e   *executer
	ee  *eventEmitter
	w   *astiworker.Worker
}

// NewEncoder creates a new encoder
func NewEncoder(cfg Configuration) *Encoder {
	aw := astiworker.NewWorker()
	ee := newEventEmitter()
	e := newExecuter(ee, aw)
	return &Encoder{
		cfg: cfg,
		e:   e,
		ee:  ee,
		w:   aw,
	}
}

// Close implements the io.Closer interface
func (e *Encoder) Close() error {
	return nil
}

// Stop stops the encoder
func (e *Encoder) Stop() {
	e.w.Stop()
}

// HandleSignals handles signals
func (e *Encoder) HandleSignals() {
	e.w.HandleSignals()
}

// Wait is a blocking pattern
func (e *Encoder) Wait() {
	e.w.Wait()
}

// AddHandleEventFunc adds an handle event func
func (e *Encoder) AddHandleEventFunc(f HandleEventFunc) {
	e.ee.addHandleEventFunc(f)
}

// SetJobHandler sets the job handler
func (e *Encoder) SetJobHandler(h JobHandler) {
	e.e.h = h
}

// Serve creates and starts the server
func (e *Encoder) Serve() {
	s := newServer(e.cfg.Server, e.ee)
	e.AddHandleEventFunc(s.handleEvent)
	e.w.Serve(e.cfg.Server.Addr, s.handler())
}

// ExecJob executes a job
func (e *Encoder) ExecJob(j Job, o ExecOptions) (err error) {
	if err = e.e.startJob(j, o); err != nil {
		err = errors.Wrapf(err, "astiencoder: starting job %+v with exec options %+v failed", j, o)
		return
	}
	return
}

// DispatchCmd dispatches a cmd
func (e *Encoder) DispatchCmd(c Cmd) {
	e.e.dispatchCmd(c)
}
