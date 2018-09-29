package astiencoder

import (
	"context"
	"sync"

	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Workflow represents a workflow
// TODO Allow visualising workflow => terminal + jpeg + server
type Workflow struct {
	*BaseNode
	c       *Closer
	e       EmitEventFunc
	j       Job
	m       *sync.Mutex
	name    string
	rootCtx context.Context
	t       CreateTaskFunc
}

func newWorkflow(name string, j Job, rootCtx context.Context, e EmitEventFunc, t CreateTaskFunc, c *Closer) *Workflow {
	return &Workflow{
		BaseNode: NewBaseNode(nil, NodeMetadata{
			Description: "root",
			Label:       "root",
			Name:        "root",
		}),
		c:       c,
		e:       e,
		j:       j,
		m:       &sync.Mutex{},
		name:    name,
		rootCtx: rootCtx,
		t:       t,
	}
}

// Closer returns the closer
func (w *Workflow) Closer() *Closer {
	return w.c
}

// EmitEventFunc returns the emit event func
func (w *Workflow) EmitEventFunc() EmitEventFunc {
	return w.e
}

// Start starts the workflow
func (w *Workflow) Start() {
	w.BaseNode.Start(w.rootCtx, w.t, func(t *astiworker.Task) {
		// Log
		astilog.Debugf("astiencoder: starting workflow %s", w.name)

		// Start nodes
		w.startNodes(w.Children(), t.NewSubTask)

		// Send event
		w.e(Event{
			Name:    EventNameWorkflowStarted,
			Payload: w.name,
		})

		// Wait for task to be done
		t.Wait()

		// Send event
		w.e(Event{
			Name:    EventNameWorkflowStopped,
			Payload: w.name,
		})

		// Workflow is done only when:
		//  - root ctx has been cancelled
		//  - ctx has not been cancelled
		if w.rootCtx.Err() != nil || w.Context().Err() == nil {
			// Close
			astilog.Debugf("astiencoder: closing workflow %s", w.name)
			if err := w.c.Close(); err != nil {
				w.e(EventError(errors.Wrapf(err, "astiencoder: closing workflow %s failed", w.name)))
			}

			// Send event
			if w.rootCtx.Err() == nil {
				w.e(Event{
					Name:    EventNameWorkflowDone,
					Payload: w.name,
				})
			}
		}
	})
}

func (w *Workflow) startNodes(ns []Node, t CreateTaskFunc) {
	// Loop through nodes
	for _, n := range ns {
		// Start node
		n.Start(w.Context(), t)

		// Start children nodes
		w.startNodes(n.Children(), t)
	}
}

// Stop stops the workflow
func (w *Workflow) Stop() {
	astilog.Debugf("astiencoder: stopping workflow %s", w.name)
	w.BaseNode.Stop()
}
