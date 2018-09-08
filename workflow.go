package astiencoder

import (
	"context"
	"sync"

	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Workflow represents a workflow
type Workflow struct {
	c                *Closer
	e                EmitEventFunc
	m                *sync.Mutex
	parentCtx        context.Context
	rootNodes        []Node
	rootNodesIndexed map[string]Node
	t                CreateTaskFunc
	w                *Worker
}

// NewWorkflow creates a new workflow
func NewWorkflow(parentCtx context.Context, e EmitEventFunc, t CreateTaskFunc, c *Closer) *Workflow {
	return &Workflow{
		c:                c,
		e:                e,
		m:                &sync.Mutex{},
		parentCtx:        parentCtx,
		rootNodesIndexed: make(map[string]Node),
		t:                t,
		w:                NewWorker(),
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

// AddRoot adds a root node
func (w *Workflow) AddRoot(n Node) {
	w.m.Lock()
	defer w.m.Unlock()
	if _, ok := w.rootNodesIndexed[n.Metadata().Name]; ok {
		return
	}
	w.rootNodes = append(w.rootNodes, n)
	w.rootNodesIndexed[n.Metadata().Name] = n
}

// Start starts the workflow
func (w *Workflow) Start() {
	w.w.Start(w.parentCtx, w.t, nil, func(t *astiworker.Task) {
		// Get root nodes
		w.m.Lock()
		ns := append([]Node{}, w.rootNodes...)
		w.m.Unlock()

		// Start nodes
		w.startNodes(ns, t.NewSubTask)

		// Wait for task to be done
		t.Wait()

		// Workflow is done only when:
		//  - parent ctx has been cancelled
		//  - ctx has not been cancelled
		if w.parentCtx.Err() != nil || w.w.ctx.Err() == nil {
			if err := w.c.Close(); err != nil {
				w.e(EventError(errors.Wrap(err, "astiencoder: closing workflow failed")))
			}
		}
	})
}

func (w *Workflow) startNodes(ns []Node, t CreateTaskFunc) {
	// Loop through nodes
	for _, n := range ns {
		// Start node
		n.Start(w.w.ctx, t)

		// Start children nodes
		w.startNodes(n.Children(), t)
	}
}

// Stop stops the workflow
func (w *Workflow) Stop() {
	w.w.Stop()
}
