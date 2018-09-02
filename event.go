package astiencoder

import (
	"github.com/asticode/go-astilog"
	"sync"
)

// Default event names
var (
	EventNameError = "error"
)

// Event is an event coming out of the worker
type Event struct {
	Name    string      `json:"name"`
	Payload interface{} `json:"payload"`
}

// EventError returns an error event
func EventError(err error) Event {
	return Event{
		Name:    EventNameError,
		Payload: err,
	}
}

// EventHandler is a method capable of handling events coming out of the worker
type EventHandler func(e Event)

// EventHandlerLogger is the logger event handler
var EventHandlerLogger = func(e Event) {
	switch e.Name {
	case EventNameError:
		if v, ok := e.Payload.(error); ok {
			astilog.Error(v)
		}
	}
}

// EventEmitter is a method capable of emitting events out of the worker
type EventEmitter func(e Event)

type eventEmitter struct {
	hs []EventHandler
	m  *sync.Mutex
}

func newEventEmitter() *eventEmitter {
	return &eventEmitter{
		m: &sync.Mutex{},
	}
}

func (e *eventEmitter) addEventHandler(h EventHandler) {
	e.m.Lock()
	defer e.m.Unlock()
	e.hs = append(e.hs, h)
}

func (e *eventEmitter) emit(evt Event) {
	e.m.Lock()
	defer e.m.Unlock()
	for _, h := range e.hs {
		go h(evt)
	}
}
