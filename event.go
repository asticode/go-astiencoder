package astiencoder

import "sync"

// Event is an event coming out of the worker
type Event struct {}

// EventHandler is an object capable of handling events coming out of the worker
type EventHandler interface {
	HandleEvent(e Event)
}

type eventEmitter struct {
	hs []EventHandler
	m *sync.Mutex
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
