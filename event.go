package astiencoder

import (
	"fmt"
	"sort"
	"sync"

	"github.com/asticode/go-astikit"
)

// Default event names
var (
	EventNameError             = "astiencoder.error"
	EventNameNodeContinued     = "astiencoder.node.continued"
	EventNameNodePaused        = "astiencoder.node.paused"
	EventNameNodeStarted       = "astiencoder.node.started"
	EventNameNodeStopped       = "astiencoder.node.stopped"
	EventNameStats             = "astiencoder.stats"
	EventNameWorkflowContinued = "astiencoder.workflow.continued"
	EventNameWorkflowPaused    = "astiencoder.workflow.paused"
	EventNameWorkflowStarted   = "astiencoder.workflow.started"
	EventNameWorkflowStopped   = "astiencoder.workflow.stopped"
	EventTypeContinued         = "continued"
	EventTypePaused            = "paused"
	EventTypeStarted           = "started"
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
	h.Add(nil, eventName, c)
}

// AddForTarget adds a new callback for a specific target
func (h *EventHandler) AddForTarget(target interface{}, c EventCallback) {
	h.Add(target, "", c)
}

// AddForAll adds a new callback for all events
func (h *EventHandler) AddForAll(c EventCallback) {
	h.Add(nil, "", c)
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
	targets := []interface{}{nil}
	if target != nil {
		targets = append(targets, target)
	}
	for _, target := range targets {
		if _, ok := h.cs[target]; ok {
			eventNames := []string{""}
			if eventName != "" {
				eventNames = append(eventNames, eventName)
			}
			for _, eventName := range eventNames {
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
func LoggerEventHandlerAdapter(i astikit.StdLogger, h *EventHandler) {
	// Create logger
	l := astikit.AdaptStdLogger(i)

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
		l.Errorf("%s%s", e.Payload.(error), t)
		return false
	})

	// Node
	h.AddForEventName(EventNameNodePaused, func(e Event) bool {
		l.Debugf("astiencoder: node %s (%s) is paused", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})
	h.AddForEventName(EventNameNodeStarted, func(e Event) bool {
		l.Debugf("astiencoder: node %s (%s) is started", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})
	h.AddForEventName(EventNameNodeStopped, func(e Event) bool {
		l.Debugf("astiencoder: node %s (%s) is stopped", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})

	// Workflow
	h.AddForEventName(EventNameWorkflowStarted, func(e Event) bool {
		l.Debugf("astiencoder: workflow %s is started", e.Target.(*Workflow).Name())
		return false
	})
	h.AddForEventName(EventNameWorkflowStopped, func(e Event) bool {
		l.Debugf("astiencoder: workflow %s is stopped", e.Target.(*Workflow).Name())
		return false
	})
}

// EventTypeTransformer represents a function capable of transforming an event type to an event name
type EventTypeTransformer func(eventType string) string

// EventTypeToNodeEventName is the node EventTypeTransformer
func EventTypeToNodeEventName(eventType string) string {
	switch eventType {
	case EventTypeContinued:
		return EventNameNodeContinued
	case EventTypePaused:
		return EventNameNodePaused
	case EventTypeStarted:
		return EventNameNodeStarted
	case EventTypeStopped:
		return EventNameNodeStopped
	default:
		return ""
	}
}

// EventTypeToWorkflowEventName is the workflow EventTypeTransformer
func EventTypeToWorkflowEventName(eventType string) string {
	switch eventType {
	case EventTypeContinued:
		return EventNameWorkflowContinued
	case EventTypePaused:
		return EventNameWorkflowPaused
	case EventTypeStarted:
		return EventNameWorkflowStarted
	case EventTypeStopped:
		return EventNameWorkflowStopped
	default:
		return ""
	}
}
