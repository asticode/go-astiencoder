package astiencoder

import (
	"fmt"
	"sort"
	"sync"

	"github.com/asticode/go-astikit"
)

// EventHandler represents an event handler
type EventHandler struct {
	// Indexed by target then by event name then by listener idx
	// We use a map[int]Listener so that deletion is as smooth as possible
	cs  map[interface{}]map[EventName]map[int]EventCallback
	idx int
	m   *sync.Mutex
}

// EventCallback represents an event callback
type EventCallback func(e Event) (deleteListener bool)

// NewEventHandler creates a new event handler
func NewEventHandler() *EventHandler {
	return &EventHandler{
		cs: make(map[interface{}]map[EventName]map[int]EventCallback),
		m:  &sync.Mutex{},
	}
}

// Add adds a new callback for a specific target and event name
func (h *EventHandler) Add(target interface{}, eventName EventName, c EventCallback) {
	h.m.Lock()
	defer h.m.Unlock()
	if _, ok := h.cs[target]; !ok {
		h.cs[target] = make(map[EventName]map[int]EventCallback)
	}
	if _, ok := h.cs[target][eventName]; !ok {
		h.cs[target][eventName] = make(map[int]EventCallback)
	}
	h.idx++
	h.cs[target][eventName][h.idx] = c
}

// AddForEventName adds a new callback for a specific event name
func (h *EventHandler) AddForEventName(eventName EventName, c EventCallback) {
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

func (h *EventHandler) del(target interface{}, eventName EventName, idx int) {
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
	eventName EventName
	idx       int
	target    interface{}
}

func (h *EventHandler) callbacks(target interface{}, eventName EventName) (cs []eventHandlerCallback) {
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
			eventNames := []EventName{""}
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

type EventHandlerLogAdapter func(*EventHandler, *EventLogger)

type EventHandlerLogOptions struct {
	Adapters     []EventHandlerLogAdapter
	Logger       astikit.StdLogger
	LoggerLevels map[EventName]astikit.LoggerLevel
}

func (h *EventHandler) Log(o EventHandlerLogOptions) (l *EventLogger) {
	// Create event logger
	l = newEventLogger(o.Logger)

	// Loop through adapters
	for _, a := range o.Adapters {
		a(h, l)
	}

	// Get logger levels
	lls := map[EventName]astikit.LoggerLevel{
		EventNameError:           astikit.LoggerLevelError,
		EventNameNodeClosed:      astikit.LoggerLevelInfo,
		EventNameNodePaused:      astikit.LoggerLevelInfo,
		EventNameNodeStarted:     astikit.LoggerLevelInfo,
		EventNameNodeStopped:     astikit.LoggerLevelInfo,
		EventNameWorkflowStarted: astikit.LoggerLevelInfo,
		EventNameWorkflowStopped: astikit.LoggerLevelInfo,
	}
	for n, ll := range o.LoggerLevels {
		lls[n] = ll
	}

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
		l.Writef(lls[e.Name], "%s%s", e.Payload.(error), t)
		return false
	})

	// Node
	h.AddForEventName(EventNameNodeClosed, func(e Event) bool {
		l.Writef(lls[e.Name], "astiencoder: node %s (%s) is closed", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})
	h.AddForEventName(EventNameNodePaused, func(e Event) bool {
		l.Writef(lls[e.Name], "astiencoder: node %s (%s) is paused", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})
	h.AddForEventName(EventNameNodeStarted, func(e Event) bool {
		l.Writef(lls[e.Name], "astiencoder: node %s (%s) is started", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})
	h.AddForEventName(EventNameNodeStopped, func(e Event) bool {
		l.Writef(lls[e.Name], "astiencoder: node %s (%s) is stopped", e.Target.(Node).Metadata().Name, e.Target.(Node).Metadata().Label)
		return false
	})

	// Workflow
	h.AddForEventName(EventNameWorkflowStarted, func(e Event) bool {
		l.Writef(lls[e.Name], "astiencoder: workflow %s is started", e.Target.(*Workflow).Name())
		return false
	})
	h.AddForEventName(EventNameWorkflowStopped, func(e Event) bool {
		l.Writef(lls[e.Name], "astiencoder: workflow %s is stopped", e.Target.(*Workflow).Name())
		return false
	})
	return
}
