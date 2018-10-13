package astiencoder

import (
	"sync"

	"github.com/asticode/go-astilog"
)

// Default event names
var (
	EventNameError             = "error"
	EventNameNodeContinued     = "node.continued"
	EventNameNodePaused        = "node.paused"
	EventNameNodeStarted       = "node.started"
	EventNameNodeStopped       = "node.stopped"
	EventNameStats             = "stats"
	EventNameWorkflowContinued = "workflow.continued"
	EventNameWorkflowPaused    = "workflow.paused"
	EventNameWorkflowStarted   = "workflow.started"
	EventNameWorkflowStopped   = "workflow.stopped"
)

// Event is an event coming out of the encoder
type Event struct {
	Name    string      `json:"name"`
	Payload interface{} `json:"payload,omitempty"`
}

// EventError returns an error event
func EventError(err error) Event {
	return Event{
		Name:    EventNameError,
		Payload: err,
	}
}

// LoggerEventHandler is the logger event handler
var LoggerEventHandler = EventHandlerOptions{
	Blocking: true,
	Handler: func(e Event) {
		switch e.Name {
		case EventNameError:
			astilog.Error(e.Payload.(error))
		case EventNameNodeStarted:
			astilog.Debugf("astiencoder: node %s is started", e.Payload.(string))
		case EventNameNodeStopped:
			astilog.Debugf("astiencoder: node %s is stopped", e.Payload.(string))
		case EventNameWorkflowStarted:
			astilog.Debugf("astiencoder: workflow %s is started", e.Payload.(string))
		case EventNameWorkflowStopped:
			astilog.Debugf("astiencoder: workflow %s is stopped", e.Payload.(string))
		}
	},
}

// EventHandler returns a method that can handle events coming out of the encoder
type EventHandler func(e Event)

// EventHandlerOptions represents event handler options
type EventHandlerOptions struct {
	Blocking bool
	Handler  EventHandler
}

// EventEmitter represents an object capable of emitting events
type EventEmitter struct {
	hs []EventHandlerOptions
	m  *sync.Mutex
}

// NewEventEmitter creates a new event emitter
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{m: &sync.Mutex{}}
}

// AddHandler adds a new handler
func (e *EventEmitter) AddHandler(o EventHandlerOptions) {
	e.m.Lock()
	defer e.m.Unlock()
	e.hs = append(e.hs, o)
}

// Emit emits a new event
func (e *EventEmitter) Emit(evt Event) {
	e.m.Lock()
	defer e.m.Unlock()
	for _, h := range e.hs {
		if h.Blocking {
			h.Handler(evt)
		} else {
			go h.Handler(evt)
		}
	}
}
