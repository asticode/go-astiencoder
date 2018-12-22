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
	EventTypeContinued         = "continued"
	EventTypePaused            = "paused"
	EventTypeStarted           = "started"
	EventTypeStopped           = "stopped"
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
			astilog.Debugf("astiencoder: node %s is started", e.Payload.(Node).Metadata().Name)
		case EventNameNodeStopped:
			astilog.Debugf("astiencoder: node %s is stopped", e.Payload.(Node).Metadata().Name)
		case EventNameWorkflowStarted:
			astilog.Debugf("astiencoder: workflow %s is started", e.Payload.(*Workflow).Name())
		case EventNameWorkflowStopped:
			astilog.Debugf("astiencoder: workflow %s is stopped", e.Payload.(*Workflow).Name())
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

// EventGenerator represents an object capable of generating an event based on its type
type EventGenerator interface {
	Event(eventType string) Event
}

// EventGeneratorNode represents a node event generator
type EventGeneratorNode struct {
	n Node
}

// NewEventGeneratorNode creates a new node event generator
func NewEventGeneratorNode(n Node) *EventGeneratorNode {
	return &EventGeneratorNode{n: n}
}

// Event implements the EventGenerator interface
func (g EventGeneratorNode) Event(eventType string) Event {
	switch eventType {
	case EventTypeContinued:
		return Event{Name: EventNameNodeContinued, Payload: g.n}
	case EventTypePaused:
		return Event{Name: EventNameNodePaused, Payload: g.n}
	case EventTypeStarted:
		return Event{Name: EventNameNodeStarted, Payload: g.n}
	case EventTypeStopped:
		return Event{Name: EventNameNodeStopped, Payload: g.n}
	default:
		return Event{}
	}
}

// EventGeneratorWorkflow represents a workflow event generator
type EventGeneratorWorkflow struct {
	w *Workflow
}

// NewEventGeneratorWorkflow creates a new workflow event generator
func NewEventGeneratorWorkflow(w *Workflow) *EventGeneratorWorkflow {
	return &EventGeneratorWorkflow{w: w}
}

// Event implements the EventGenerator interface
func (g EventGeneratorWorkflow) Event(eventType string) Event {
	switch eventType {
	case EventTypeContinued:
		return Event{Name: EventNameWorkflowContinued, Payload: g.w}
	case EventTypePaused:
		return Event{Name: EventNameWorkflowPaused, Payload: g.w}
	case EventTypeStarted:
		return Event{Name: EventNameWorkflowStarted, Payload: g.w}
	case EventTypeStopped:
		return Event{Name: EventNameWorkflowStopped, Payload: g.w}
	default:
		return Event{}
	}
}
