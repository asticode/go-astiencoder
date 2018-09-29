package astiencoder

import (
	"sync"

	"github.com/asticode/go-astilog"
)

// Default event names
var (
	EventNameError           = "error"
	EventNameNodeStarted     = "node.started"
	EventNameNodeStopped     = "node.stopped"
	EventNameWorkflowDone    = "workflow.done"
	EventNameWorkflowStarted = "workflow.started"
	EventNameWorkflowStopped = "workflow.stopped"
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

// EventHandler returns a method that can handle events coming out of the encoder
type EventHandler func() (isBlocking bool, fn func(e Event))

// LoggerHandleEventFunc returns the logger handle event func
var LoggerHandleEventFunc = func() (isBlocking bool, fn func(e Event)) {
	return true, func(e Event) {
		switch e.Name {
		case EventNameError:
			astilog.Error(e.Payload.(error))
		case EventNameNodeStarted:
			astilog.Debugf("astiencoder: node %s is started", e.Payload.(string))
		case EventNameNodeStopped:
			astilog.Debugf("astiencoder: node %s is stopped", e.Payload.(string))
		case EventNameWorkflowDone:
			astilog.Debugf("astiencoder: workflow %s is done", e.Payload.(string))
		case EventNameWorkflowStarted:
			astilog.Debugf("astiencoder: workflow %s is started", e.Payload.(string))
		case EventNameWorkflowStopped:
			astilog.Debugf("astiencoder: workflow %s is stopped", e.Payload.(string))
		}
	}
}

// EmitEventFunc is a method that can emit events out of the encoder
type EmitEventFunc func(e Event)

type eventHandler struct {
	fn         func(e Event)
	isBlocking bool
}

type eventEmitter struct {
	hs []eventHandler
	m  *sync.Mutex
}

func newEventEmitter() *eventEmitter {
	return &eventEmitter{
		m: &sync.Mutex{},
	}
}

func (e *eventEmitter) addHandler(h EventHandler) {
	e.m.Lock()
	defer e.m.Unlock()
	isBlocking, fn := h()
	e.hs = append(e.hs, eventHandler{
		isBlocking: isBlocking,
		fn:         fn,
	})
}

func (e *eventEmitter) emit(evt Event) {
	e.m.Lock()
	defer e.m.Unlock()
	for _, h := range e.hs {
		if h.isBlocking {
			h.fn(evt)
		} else {
			go h.fn(evt)
		}
	}
}
