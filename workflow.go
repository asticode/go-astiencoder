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
		BaseNode: NewBaseNode(NodeMetadata{}),
		c:        c,
		e:        e,
		j:        j,
		m:        &sync.Mutex{},
		name:     name,
		rootCtx:  rootCtx,
		t:        t,
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

// WorkflowStartOptions represents workflow start options
type WorkflowStartOptions struct {
	StopWhenNodesAreDone bool
}

// Start starts the workflow
func (w *Workflow) Start(o WorkflowStartOptions) {
	w.BaseNode.Start(w.rootCtx, WorkflowStartOptions{}, w.t, func(t *astiworker.Task) {
		// Log
		astilog.Debugf("astiencoder: starting workflow %s", w.name)

		// Start nodes
		w.startNodes(w.Children(), o, t.NewSubTask)

		// Wait for task to be done
		t.Wait()

		// Workflow is done
		if w.done() {
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

func (w *Workflow) startNodes(ns []Node, o WorkflowStartOptions, t CreateTaskFunc) {
	// Loop through nodes
	for _, n := range ns {
		// Start node
		n.Start(w.Context(), o, t)

		// Start children nodes
		w.startNodes(n.Children(), o, t)
	}
}

// Stop stops the workflow
func (w *Workflow) Stop() {
	astilog.Debugf("astiencoder: stopping workflow %s", w.name)
	w.BaseNode.Stop()
}

// Workflow is done only when:
//  - root ctx has been cancelled
//  - ctx has not been cancelled
func (w *Workflow) done() bool {
	return (w.rootCtx != nil && w.rootCtx.Err() != nil) || (w.Context() != nil && w.Context().Err() == nil)
}

func (w *Workflow) status() string {
	if w.done() {
		return "done"
	} else if w.Context() == nil || w.Context().Err() != nil {
		return "stopped"
	} else {
		return "started"
	}
}
