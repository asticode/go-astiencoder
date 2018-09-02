package astiencoder

import "sync"

// Event is an event coming out of the worker
type Event struct {
	Name    string      `json:"name"`
	Payload interface{} `json:"payload"`
}

// EventHandler is an object capable of handling events coming out of the worker
type EventHandler interface {
	HandleEvent(e Event)
}

// EventEmitter is an object capable of emitting events out of the worker
type EventEmitter interface {
	Emit(e Event)
}

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

// Emit implements the EventEmitter interface
func (e *eventEmitter) Emit(evt Event) {
	// TODO
}
