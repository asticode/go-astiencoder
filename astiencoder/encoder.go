package main

import (
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

type encoder struct {
	c         *ConfigurationEncoder
	eh        *astiencoder.EventHandler
	m         *sync.Mutex
	w         *astikit.Worker
	wp        *astiencoder.WorkflowPool
	wsStarted map[string]bool
}

func newEncoder(c *ConfigurationEncoder, eh *astiencoder.EventHandler, wp *astiencoder.WorkflowPool, l astikit.StdLogger) (e *encoder) {
	e = &encoder{
		c:         c,
		eh:        eh,
		m:         &sync.Mutex{},
		w:         astikit.NewWorker(astikit.WorkerOptions{Logger: l}),
		wp:        wp,
		wsStarted: make(map[string]bool),
	}
	e.adaptEventHandler(e.eh)
	return
}

func (e *encoder) adaptEventHandler(h *astiencoder.EventHandler) {
	h.AddForEventName(astiencoder.EventNameWorkflowStarted, func(evt astiencoder.Event) bool {
		e.m.Lock()
		defer e.m.Unlock()
		e.wsStarted[evt.Target.(*astiencoder.Workflow).Name()] = true
		return false
	})
	h.AddForEventName(astiencoder.EventNameWorkflowStopped, func(evt astiencoder.Event) bool {
		e.m.Lock()
		defer e.m.Unlock()
		delete(e.wsStarted, evt.Target.(*astiencoder.Workflow).Name())
		if e.c.Exec.StopWhenWorkflowsAreStopped && len(e.wsStarted) == 0 {
			e.w.Stop()
		}
		return false
	})
}
