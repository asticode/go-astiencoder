package astiencoder

import (
	"context"
	"sync"

	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Errors
var (
	ErrNodeNotFound = errors.New("node.not.found")
)

// Workflow represents a workflow
type Workflow struct {
	bn      *BaseNode
	c       *Closer
	e       EmitEventFunc
	j       Job
	m       *sync.Mutex
	name    string
	ns      map[string]Node
	rootCtx context.Context
	t       *astiworker.Task
	tf      CreateTaskFunc
}

func newWorkflow(name string, j Job, rootCtx context.Context, e EmitEventFunc, tf CreateTaskFunc, c *Closer) *Workflow {
	return &Workflow{
		bn: NewBaseNode(nil, NodeMetadata{
			Description: "root",
			Label:       "root",
			Name:        "root",
		}),
		c:       c,
		e:       e,
		j:       j,
		m:       &sync.Mutex{},
		name:    name,
		ns:      make(map[string]Node),
		rootCtx: rootCtx,
		tf:      tf,
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

func (w *Workflow) indexNodes() {
	// Lock
	w.m.Lock()
	defer w.m.Unlock()

	// Index
	w.indexNodesFunc(w.bn.Children())
}

func (w *Workflow) indexNodesFunc(ns []Node) {
	// Loop through nodes
	for _, n := range ns {
		// Add node
		w.ns[n.Metadata().Name] = n

		// Index children nodes
		w.indexNodesFunc(n.Children())
	}
}

// Start starts the workflow
func (w *Workflow) Start() {
	w.start(w.nodes()...)
}

func (w *Workflow) start(ns ...Node) {
	w.bn.Start(w.rootCtx, w.tf, func(t *astiworker.Task) {
		// Log
		astilog.Debugf("astiencoder: starting workflow %s", w.name)

		// Store task
		w.t = t

		// Loop through nodes
		for _, n := range ns {
			w.startNode(n)
		}

		// Send event
		w.e(Event{
			Name:    EventNameWorkflowStarted,
			Payload: w.name,
		})

		// Wait for task to be done
		t.Wait()

		// Close
		if err := w.c.Close(); err != nil {
			w.e(EventError(errors.Wrapf(err, "astiencoder: closing workflow %s failed", w.name)))
		}

		// Send event
		w.e(Event{
			Name:    EventNameWorkflowStopped,
			Payload: w.name,
		})
	})
}

func (w *Workflow) startNode(n Node) {
	n.Start(w.bn.Context(), w.t.NewSubTask)
}

// Stop stops the workflow
func (w *Workflow) Stop() {
	astilog.Debugf("astiencoder: stopping workflow %s", w.name)
	w.bn.Stop()
}

// Pause pauses the workflow
func (w *Workflow) Pause() {
	// Workflow is not running
	if w.bn.Status() != StatusRunning {
		return
	}

	// Pause
	astilog.Debugf("astiencoder: pausing workflow %s", w.name)
	for _, n := range w.nodes() {
		n.Pause()
	}

	// Update status
	w.bn.m.Lock()
	w.bn.status = StatusPaused
	w.bn.m.Unlock()

	// Send event
	w.e(Event{
		Name:    EventNameWorkflowPaused,
		Payload: w.name,
	})
}

// Continue continues the workflow
func (w *Workflow) Continue() {
	// Workflow is not paused
	if w.bn.Status() != StatusPaused {
		return
	}

	// Continue
	astilog.Debugf("astiencoder: continuing workflow %s", w.name)
	for _, n := range w.nodes() {
		n.Continue()
	}

	// Update status
	w.bn.m.Lock()
	w.bn.status = StatusRunning
	w.bn.m.Unlock()

	// Send event
	w.e(Event{
		Name:    EventNameWorkflowStarted,
		Payload: w.name,
	})
}

// AddChild adds a child to the workflow
func (w *Workflow) AddChild(n Node) {
	w.bn.AddChild(n)
}

// Children returns the workflow children
func (w *Workflow) Children() []Node {
	return w.bn.Children()
}

// Status returns the workflow status
func (w *Workflow) Status() string {
	return w.bn.Status()
}

// Node retrieves a node from the workflow
func (w *Workflow) Node(name string) (n Node, err error) {
	w.m.Lock()
	defer w.m.Unlock()
	var ok bool
	if n, ok = w.ns[name]; !ok {
		err = ErrNodeNotFound
		return
	}
	return
}

func (w *Workflow) nodes() (ns []Node) {
	w.m.Lock()
	defer w.m.Unlock()
	for _, n := range w.ns {
		ns = append(ns, n)
	}
	return
}
