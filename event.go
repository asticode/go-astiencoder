package astiencoder

import (
	"sync"

	"github.com/asticode/go-astilog"
)

// Default event names
var (
	EventNameError = "error"
)

// Event is an event coming out of the encoder
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

// HandleEventFunc returns a method that can handle events coming out of the encoder
type HandleEventFunc func() (isBlocking bool, fn func(e Event))

// LoggerHandleEventFunc returns the logger handle event func
var LoggerHandleEventFunc = func() (isBlocking bool, fn func(e Event)) {
	return true, func(e Event) {
		switch e.Name {
		case EventNameError:
			if v, ok := e.Payload.(error); ok {
				astilog.Error(v)
			}
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

func (e *eventEmitter) addHandleEventFunc(f HandleEventFunc) {
	e.m.Lock()
	defer e.m.Unlock()
	isBlocking, fn := f()
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
