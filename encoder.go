package astiencoder

import (
	"sync"

	"fmt"

	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Encoder represents an encoder
type Encoder struct {
	b      WorkflowBuilder
	cfg    *Configuration
	ee     *eventEmitter
	m      *sync.Mutex
	w      *astiworker.Worker
	ws     map[string]*Workflow
	wsDone map[string]bool
}

// WorkflowBuilder represents an object that can build a workflow based on a job
type WorkflowBuilder interface {
	BuildWorkflow(j Job, w *Workflow) error
}

// NewEncoder creates a new encoder
func NewEncoder(cfg *Configuration) (e *Encoder) {
	aw := astiworker.NewWorker()
	ee := newEventEmitter()
	e = &Encoder{
		cfg:    cfg,
		ee:     ee,
		m:      &sync.Mutex{},
		w:      aw,
		ws:     make(map[string]*Workflow),
		wsDone: make(map[string]bool),
	}
	ee.addHandler(e.handleEvent)
	return
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

// AddEventHandler adds an event handler
func (e *Encoder) AddEventHandler(h EventHandler) {
	e.ee.addHandler(h)
}

// SetWorkflowBuilder sets the workflow builder
func (e *Encoder) SetWorkflowBuilder(b WorkflowBuilder) {
	e.b = b
}

// Serve creates and starts the server
func (e *Encoder) Serve() {
	s := newServer(e.cfg.Server, e.ee)
	e.AddEventHandler(s.handleEvent)
	e.w.Serve(e.cfg.Server.Addr, s.handler())
}

// NewWorkflow creates a new workflow based on a job
func (e *Encoder) NewWorkflow(name string, j Job) (w *Workflow, err error) {
	// No workflow builder
	if e.b == nil {
		err = errors.New("astiencoder: no workflow builder")
		return
	}

	// Name is already used
	e.m.Lock()
	_, ok := e.ws[name]
	e.m.Unlock()
	if ok {
		err = fmt.Errorf("astiencoder: workflow name %s is already used", name)
		return
	}

	// Create closer
	c := newCloser()

	// Create workflow
	w = newWorkflow(name, e.w.Context(), e.ee.emit, e.w.NewTask, c)

	// Build workflow
	if err = e.b.BuildWorkflow(j, w); err != nil {
		err = errors.Wrapf(err, "astiencoder: building workflow for job %+v failed", j)
		return
	}

	// Store workflow
	e.m.Lock()
	e.ws[name] = w
	e.m.Unlock()
	return
}

func (e *Encoder) handleEvent() (bool, func(Event)) {
	return false, func(evt Event) {
		switch evt.Name {
		case EventNameWorkflowDone:
			e.m.Lock()
			defer e.m.Unlock()
			if _, ok := e.ws[evt.Payload.(string)]; !ok {
				return
			}
			e.wsDone[evt.Payload.(string)] = true
			if e.cfg.Exec.StopWhenWorkflowsAreDone && len(e.ws) == len(e.wsDone) {
				e.Stop()
			}
		}
	}
}
