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
	s         *astiencoder.Stater
	w         *astikit.Worker
	ws        *astiencoder.Server
	wsStarted map[string]bool
}

func newEncoder(c *ConfigurationEncoder, eh *astiencoder.EventHandler, ws *astiencoder.Server, l astikit.StdLogger, s *astiencoder.Stater) (e *encoder) {
	e = &encoder{
		c:         c,
		eh:        eh,
		m:         &sync.Mutex{},
		s:         s,
		w:         astikit.NewWorker(astikit.WorkerOptions{Logger: l}),
		ws:        ws,
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
