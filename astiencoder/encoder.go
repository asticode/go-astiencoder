package main

import (
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/worker"
)

type encoder struct {
	c         *ConfigurationEncoder
	ee        *astiencoder.EventEmitter
	m         *sync.Mutex
	w         *astiworker.Worker
	wp        *astiencoder.WorkflowPool
	wsStarted map[string]bool
}

func newEncoder(c *ConfigurationEncoder, ee *astiencoder.EventEmitter, wp *astiencoder.WorkflowPool) (e *encoder) {
	e = &encoder{
		c:         c,
		ee:        ee,
		m:         &sync.Mutex{},
		w:         astiworker.NewWorker(),
		wp:        wp,
		wsStarted: make(map[string]bool),
	}
	e.ee.AddHandler(e)
	return
}

// HandleEvent implements the EventHandler interface
func (e *encoder) HandleEvent(evt astiencoder.Event) {
	switch evt.Name {
	case astiencoder.EventNameWorkflowStarted:
		e.m.Lock()
		defer e.m.Unlock()
		e.wsStarted[evt.Payload.(*astiencoder.Workflow).Name()] = true
	case astiencoder.EventNameWorkflowStopped:
		e.m.Lock()
		defer e.m.Unlock()
		delete(e.wsStarted, evt.Payload.(*astiencoder.Workflow).Name())
		if e.c.Exec.StopWhenWorkflowsAreStopped && len(e.wsStarted) == 0 {
			e.w.Stop()
		}
	}
}
