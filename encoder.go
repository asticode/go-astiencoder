package astiencoder

import (
	"context"
	"sync"

	"fmt"

	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Errors
var (
	ErrWorkflowNotFound = errors.New("workflow.not.found")
)

// Encoder represents an encoder
type Encoder struct {
	cfg       *Configuration
	e         *exposer
	ee        *EventEmitter
	m         *sync.Mutex
	w         *astiworker.Worker
	ws        map[string]*Workflow
	wsStarted map[string]bool
}

// NewEncoder creates a new encoder
func NewEncoder(cfg *Configuration) (e *Encoder) {
	aw := astiworker.NewWorker()
	ee := NewEventEmitter()
	e = &Encoder{
		cfg:       cfg,
		ee:        ee,
		m:         &sync.Mutex{},
		w:         aw,
		ws:        make(map[string]*Workflow),
		wsStarted: make(map[string]bool),
	}
	e.e = newExposer(e)
	ee.AddHandler(e.handleEvent, EventHandlerOptions{})
	return
}

// Close implements the io.Closer interface
func (e *Encoder) Close() error {
	return nil
}

// Context returns the encoder context
func (e *Encoder) Context() context.Context {
	return e.w.Context()
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
func (e *Encoder) AddEventHandler(h EventHandler, o EventHandlerOptions) {
	e.ee.AddHandler(h, o)
}

// Serve creates and starts the server
func (e *Encoder) Serve(h ServerCustomHandler) (err error) {
	var s *server
	if s, err = newServer(e.cfg.Server, e.e); err != nil {
		err = errors.Wrap(err, "astiencoder: creating new server failed")
		return
	}
	e.AddEventHandler(s.handleEvent, EventHandlerOptions{})
	e.w.Serve(e.cfg.Server.Addr, s.handler(h))
	return
}

// NewWorkflow creates a new workflow
func (e *Encoder) NewWorkflow(name string) (w *Workflow, err error) {
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
	w = NewWorkflow(name, e.ee, e.w.NewTask, c)

	// Store workflow
	e.m.Lock()
	e.ws[name] = w
	e.m.Unlock()
	return
}

// Workflow returns a specific workflow
func (e *Encoder) Workflow(name string) (w *Workflow, err error) {
	e.m.Lock()
	defer e.m.Unlock()
	var ok bool
	if w, ok = e.ws[name]; !ok {
		err = ErrWorkflowNotFound
		return
	}
	return
}

func (e *Encoder) handleEvent(evt Event) {
	switch evt.Name {
	case EventNameWorkflowStarted:
		e.m.Lock()
		defer e.m.Unlock()
		if _, ok := e.ws[evt.Payload.(string)]; !ok {
			return
		}
		e.wsStarted[evt.Payload.(string)] = true
	case EventNameWorkflowStopped:
		e.m.Lock()
		defer e.m.Unlock()
		delete(e.wsStarted, evt.Payload.(string))
		if e.cfg.Exec.StopWhenWorkflowsAreStopped && len(e.wsStarted) == 0 {
			e.Stop()
		}
	}
}
