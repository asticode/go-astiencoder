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
	EventNameNodeStats         = "node.stats"
	EventNameNodeStopped       = "node.stopped"
	EventNameWorkflowContinued = "workflow.continued"
	EventNameWorkflowPaused    = "workflow.paused"
	EventNameWorkflowStarted   = "workflow.started"
	EventNameWorkflowStats     = "workflow.stats"
	EventNameWorkflowStopped   = "workflow.stopped"
	EventTypeContinued         = "continued"
	EventTypePaused            = "paused"
	EventTypeStarted           = "started"
	EventTypeStats             = "stats"
	EventTypeStopped           = "stopped"
)

// Event is an event coming out of the encoder
type Event struct {
	Name    string
	Payload interface{}
	Target  interface{}
}

// EventError returns an error event
func EventError(err error) Event {
	return Event{
		Name:    EventNameError,
		Payload: err,
	}
}

// EventHandler represents an object that can handle events
type EventHandler interface {
	HandleEvent(e Event)
}

// LoggerEventHandler represents then logger event handler
type LoggerEventHandler struct{}

// NewLoggerEventHandler creates a new event handler
func NewLoggerEventHandler() *LoggerEventHandler {
	return &LoggerEventHandler{}
}

// Handle implements the EventHandler interface
func (h *LoggerEventHandler) HandleEvent(e Event) {
	switch e.Name {
	case EventNameError:
		astilog.Error(e.Payload.(error))
	case EventNameNodeStarted:
		astilog.Debugf("astiencoder: node %s is started", e.Target.(Node).Metadata().Name)
	case EventNameNodeStopped:
		astilog.Debugf("astiencoder: node %s is stopped", e.Target.(Node).Metadata().Name)
	case EventNameWorkflowStarted:
		astilog.Debugf("astiencoder: workflow %s is started", e.Target.(*Workflow).Name())
	case EventNameWorkflowStopped:
		astilog.Debugf("astiencoder: workflow %s is stopped", e.Target.(*Workflow).Name())
	}
}

// EventEmitter represents an object capable of emitting events
type EventEmitter interface {
	Emit(e Event)
}

// DefaultEventEmitter represents the default event emitter
type DefaultEventEmitter struct {
	hs []EventHandler
	m  *sync.Mutex
}

// NewDefaultEventEmitter creates a new default event emitter
func NewDefaultEventEmitter() *DefaultEventEmitter {
	return &DefaultEventEmitter{m: &sync.Mutex{}}
}

// AddHandler adds a new handler
func (e *DefaultEventEmitter) AddHandler(h EventHandler) {
	e.m.Lock()
	defer e.m.Unlock()
	e.hs = append(e.hs, h)
}

// Emit emits an event
func (e *DefaultEventEmitter) Emit(evt Event) {
	e.m.Lock()
	defer e.m.Unlock()
	for _, h := range e.hs {
		h.HandleEvent(evt)
	}
}

// EventGenerator represents an object capable of generating an event based on its type
type EventGenerator interface {
	Event(eventType string, payload interface{}) Event
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
func (g EventGeneratorNode) Event(eventType string, payload interface{}) Event {
	switch eventType {
	case EventTypeContinued:
		return Event{Name: EventNameNodeContinued, Target: g.n}
	case EventTypePaused:
		return Event{Name: EventNameNodePaused, Target: g.n}
	case EventTypeStarted:
		return Event{Name: EventNameNodeStarted, Target: g.n}
	case EventTypeStats:
		return Event{Name: EventNameNodeStats, Payload: payload, Target: g.n}
	case EventTypeStopped:
		return Event{Name: EventNameNodeStopped, Target: g.n}
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
func (g EventGeneratorWorkflow) Event(eventType string, payload interface{}) Event {
	switch eventType {
	case EventTypeContinued:
		return Event{Name: EventNameWorkflowContinued, Target: g.w}
	case EventTypePaused:
		return Event{Name: EventNameWorkflowPaused, Target: g.w}
	case EventTypeStarted:
		return Event{Name: EventNameWorkflowStarted, Target: g.w}
	case EventTypeStats:
		return Event{Name: EventNameWorkflowStats, Payload: payload, Target: g.w}
	case EventTypeStopped:
		return Event{Name: EventNameWorkflowStopped, Target: g.w}
	default:
		return Event{}
	}
}
