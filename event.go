package astiencoder

import (
	"fmt"
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
func EventError(target interface{}, err error) Event {
	return Event{
		Name:    EventNameError,
		Payload: err,
		Target:  target,
	}
}

// EventHandler represents an object that can handle events
type EventHandler interface {
	HandleEvent(e Event)
}

// LoggerEventHandler represents a logger event handler
type LoggerEventHandler struct{}

// NewLoggerEventHandler creates a new event handler
func NewLoggerEventHandler() *LoggerEventHandler {
	return &LoggerEventHandler{}
}

// Handle implements the EventHandler interface
func (h *LoggerEventHandler) HandleEvent(e Event) {
	switch e.Name {
	case EventNameError:
		var t string
		if v, ok := e.Target.(Node); ok{
			t = v.Metadata().Name
		} else if v, ok := e.Target.(*Workflow); ok {
			t = v.Name()
		} else if e.Target != nil {
			t = fmt.Sprintf("%p", e.Target)
		}
		if len(t) > 0 {
			t = "(" + t + ")"
		}
		astilog.Errorf("%s%s", e.Payload.(error), t)
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

// EventCallback represents an event callback
type EventCallback func(e Event) (deleteListener bool)

// CallbackEventHandler represents a callback event handler
type CallbackEventHandler struct {
	// Indexed by target then by event name then by listener idx
	// We use a map[int]Listener so that deletion is as smooth as possible
	// It means it doesn't store listeners in order
	cs  map[interface{}]map[string]map[int]EventCallback
	idx int
	m   *sync.Mutex
}

// NewCallbackEventHandler creates a new callback event handler
func NewCallbackEventHandler() *CallbackEventHandler {
	return &CallbackEventHandler{
		cs: make(map[interface{}]map[string]map[int]EventCallback),
		m:  &sync.Mutex{},
	}
}

// Add adds a new callback
func (h *CallbackEventHandler) Add(target interface{}, eventName string, c EventCallback) {
	h.m.Lock()
	defer h.m.Unlock()
	if _, ok := h.cs[target]; !ok {
		h.cs[target] = make(map[string]map[int]EventCallback)
	}
	if _, ok := h.cs[target][eventName]; !ok {
		h.cs[target][eventName] = make(map[int]EventCallback)
	}
	h.idx++
	h.cs[target][eventName][h.idx] = c
}

func (h *CallbackEventHandler) del(target interface{}, eventName string, idx int) {
	h.m.Lock()
	defer h.m.Unlock()
	if _, ok := h.cs[target]; !ok {
		return
	}
	if _, ok := h.cs[target][eventName]; !ok {
		return
	}
	delete(h.cs[target][eventName], idx)
}

func (h *CallbackEventHandler) callbacks(target interface{}, eventName string) (cs map[int]EventCallback) {
	h.m.Lock()
	defer h.m.Unlock()
	cs = map[int]EventCallback{}
	if _, ok := h.cs[target]; !ok {
		return
	}
	if _, ok := h.cs[target][eventName]; !ok {
		return
	}
	for k, v := range h.cs[target][eventName] {
		cs[k] = v
	}
	return
}

// HandleEvent implements the EventHandler interface
func (h *CallbackEventHandler) HandleEvent(e Event) {
	for idx, c := range h.callbacks(e.Target, e.Name) {
		if c(e) {
			h.del(e.Target, e.Name, idx)
		}
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
