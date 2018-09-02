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

// HandleEventFunc is a method that can handle events coming out of the worker
type HandleEventFunc func(e Event)

// LoggerHandleEventFunc is the logger handle event func
var LoggerHandleEventFunc = func(e Event) {
	switch e.Name {
	case EventNameError:
		if v, ok := e.Payload.(error); ok {
			astilog.Error(v)
		}
	}
}

// EmitEventFunc is a method that can emit events out of the worker
type EmitEventFunc func(e Event)

type eventEmitter struct {
	fs []HandleEventFunc
	m  *sync.Mutex
}

func newEventEmitter() *eventEmitter {
	return &eventEmitter{
		m: &sync.Mutex{},
	}
}

func (e *eventEmitter) addHandleEventFunc(f HandleEventFunc) {
	e.m.Lock()
	defer e.m.Unlock()
	e.fs = append(e.fs, f)
}

func (e *eventEmitter) emit(evt Event) {
	e.m.Lock()
	defer e.m.Unlock()
	for _, f := range e.fs {
		go f(evt)
	}
}
