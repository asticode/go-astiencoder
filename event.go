package astiencoder

import (
	"fmt"
	"sync"

	"github.com/asticode/go-astilog"
	"sort"
)

// Default event names
var (
	EventNameError             = "astiencoder.error"
	EventNameNodeContinued     = "astiencoder.node.continued"
	EventNameNodePaused        = "astiencoder.node.paused"
	EventNameNodeStarted       = "astiencoder.node.started"
	EventNameNodeStats         = "astiencoder.node.stats"
	EventNameNodeStopped       = "astiencoder.node.stopped"
	EventNameWorkflowContinued = "astiencoder.workflow.continued"
	EventNameWorkflowPaused    = "astiencoder.workflow.paused"
	EventNameWorkflowStarted   = "astiencoder.workflow.started"
	EventNameWorkflowStats     = "astiencoder.workflow.stats"
	EventNameWorkflowStopped   = "astiencoder.workflow.stopped"
	EventTypeContinued         = "continued"
	EventTypePaused            = "paused"
	EventTypeStarted           = "started"
	EventTypeStats             = "stats"
	EventTypeStopped           = "stopped"
)

// Event defaults
const (
	eventDefaultEventName = ""
	eventDefaultTarget    = "default"
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

// EventHandler represents an event handler
type EventHandler struct {
	// Indexed by target then by event name then by listener idx
	// We use a map[int]Listener so that deletion is as smooth as possible
	cs  map[interface{}]map[string]map[int]EventCallback
	idx int
	m   *sync.Mutex
}

// EventCallback represents an event callback
type EventCallback func(e Event) (deleteListener bool)

// NewEventHandler creates a new event handler
func NewEventHandler() *EventHandler {
	return &EventHandler{
		cs: make(map[interface{}]map[string]map[int]EventCallback),
		m:  &sync.Mutex{},
	}
}

// Add adds a new callback for a specific target and event name
func (h *EventHandler) Add(target interface{}, eventName string, c EventCallback) {
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

// AddForEventName adds a new callback for a specific event name
func (h *EventHandler) AddForEventName(eventName string, c EventCallback) {
	h.Add(eventDefaultTarget, eventName, c)
}

// AddForTarget adds a new callback for a specific target
func (h *EventHandler) AddForTarget(target interface{}, c EventCallback) {
	h.Add(target, eventDefaultEventName, c)
}

// AddForAll adds a new callback for all events
func (h *EventHandler) AddForAll(c EventCallback) {
	h.Add(eventDefaultTarget, eventDefaultEventName, c)
}

func (h *EventHandler) del(target interface{}, eventName string, idx int) {
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

type eventHandlerCallback struct {
	c         EventCallback
	eventName string
	idx       int
	target    interface{}
}

func (h *EventHandler) callbacks(target interface{}, eventName string) (cs []eventHandlerCallback) {
	// Lock
	h.m.Lock()
	defer h.m.Unlock()

	// Index callbacks
	ics := make(map[int]eventHandlerCallback)
	var idxs []int
	for _, target := range []interface{}{eventDefaultTarget, target} {
		if _, ok := h.cs[target]; ok {
			for _, eventName := range []string{eventDefaultEventName, eventName} {
				if _, ok := h.cs[target][eventName]; ok {
					for idx, c := range h.cs[target][eventName] {
						ics[idx] = eventHandlerCallback{
							c:         c,
							eventName: eventName,
							idx:       idx,
							target:    target,
						}
						idxs = append(idxs, idx)
					}
				}
			}
		}
	}

	// Sort
	sort.Ints(idxs)

	// Append
	for _, idx := range idxs {
		cs = append(cs, ics[idx])
	}
	return
}

// Emit emits an event
func (h *EventHandler) Emit(e Event) {
	for _, c := range h.callbacks(e.Target, e.Name) {
		if c.c(e) {
			h.del(c.target, c.eventName, c.idx)
		}
	}
}

// LoggerEventHandlerAdapter adapts the event handler so that it logs the events properly
func LoggerEventHandlerAdapter(h *EventHandler) {
	// Error
	h.AddForEventName(EventNameError, func(e Event) bool {
		var t string
		if v, ok := e.Target.(Node); ok {
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
		return false
	})

	// Node
	h.AddForEventName(EventNameNodeStarted, func(e Event) bool {
		astilog.Debugf("astiencoder: node %s (%s) is started", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})
	h.AddForEventName(EventNameNodeStopped, func(e Event) bool {
		astilog.Debugf("astiencoder: node %s (%s) is stopped", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})

	// Workflow
	h.AddForEventName(EventNameWorkflowStarted, func(e Event) bool {
		astilog.Debugf("astiencoder: workflow %s is started", e.Target.(*Workflow).Name())
		return false
	})
	h.AddForEventName(EventNameWorkflowStopped, func(e Event) bool {
		astilog.Debugf("astiencoder: workflow %s is stopped", e.Target.(*Workflow).Name())
		return false
	})
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
