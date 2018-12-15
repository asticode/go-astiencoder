package astiencoder

import (
	"context"

	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Workflow represents a workflow
type Workflow struct {
	bn   *BaseNode
	c    *Closer
	ctx  context.Context
	e    *EventEmitter
	name string
	t    *astiworker.Task
	tf   CreateTaskFunc
}

// NewWorkflow creates a new workflow
func NewWorkflow(ctx context.Context, name string, e *EventEmitter, tf CreateTaskFunc, c *Closer) *Workflow {
	return &Workflow{
		bn: NewBaseNode(nil, NodeMetadata{
			Description: "root",
			Label:       "root",
			Name:        "root",
		}),
		c:    c,
		ctx:  ctx,
		e:    e,
		name: name,
		tf:   tf,
	}
}

func (w *Workflow) nodes() (ns []Node) {
	for _, n := range w.indexedNodes() {
		ns = append(ns, n)
	}
	return
}

func (w *Workflow) indexedNodes() (ns map[string]Node) {
	ns = make(map[string]Node)
	w.indexedNodesFunc(ns, w.bn.Children())
	return
}

func (w *Workflow) indexedNodesFunc(ns map[string]Node, children []Node) {
	for _, n := range children {
		ns[n.Metadata().Name] = n
		w.indexedNodesFunc(ns, n.Children())
	}
}

// StartNodes starts nodes
func (w *Workflow) StartNodes(ns ...Node) {
	for _, n := range ns {
		n.Start(w.bn.Context(), w.t.NewSubTask)
	}
}

// StartNodesInSubTask starts nodes in a new sub task
func (w *Workflow) StartNodesInSubTask(ns ...Node) (t *astiworker.Task) {
	t = w.t.NewSubTask()
	for _, n := range ns {
		n.Start(w.bn.Context(), t.NewSubTask)
	}
	return
}

// WorkflowStartOptions represents workflow start options
type WorkflowStartOptions struct {
	Groups []WorkflowStartGroup
}

// WorkflowStartGroup represents a workflow start group
type WorkflowStartGroup struct {
	Callback func(t *astiworker.Task)
	Nodes    []Node
}

// Start starts the workflow
func (w *Workflow) Start() {
	w.start(w.nodes(), WorkflowStartOptions{})
}

// StartWithOptions starts the workflow with options
func (w *Workflow) StartWithOptions(o WorkflowStartOptions) {
	w.start(w.nodes(), o)
}

type workflowStartGroup struct {
	fn func(t *astiworker.Task)
	ns []Node
	t  *astiworker.Task
}

func (w *Workflow) start(ns []Node, o WorkflowStartOptions) {
	w.bn.Start(w.ctx, w.tf, func(t *astiworker.Task) {
		// Log
		astilog.Debugf("astiencoder: starting workflow %s", w.name)

		// Store task
		w.t = t

		// Index groups
		var gs []*workflowStartGroup
		ngs := make(map[Node]*workflowStartGroup)
		for _, og := range o.Groups {
			g := &workflowStartGroup{fn: og.Callback}
			for _, n := range og.Nodes {
				ngs[n] = g
			}
			gs = append(gs, g)
		}

		// Loop through nodes
		for _, n := range ns {
			if g, ok := ngs[n]; ok {
				g.ns = append(g.ns, n)
			} else {
				w.StartNodes(n)
			}
		}

		// Loop through groups
		for _, g := range gs {
			g.t = w.StartNodesInSubTask(g.ns...)
		}

		// Send event
		w.e.Emit(Event{
			Name:    EventNameWorkflowStarted,
			Payload: w.name,
		})

		// Execute groups callbacks
		for _, g := range gs {
			if g.fn != nil {
				g.fn(g.t)
			}
		}

		// Wait for task to be done
		t.Wait()

		// Close
		if err := w.c.Close(); err != nil {
			w.e.Emit(EventError(errors.Wrapf(err, "astiencoder: closing workflow %s failed", w.name)))
		}

		// Send event
		w.e.Emit(Event{
			Name:    EventNameWorkflowStopped,
			Payload: w.name,
		})
	})
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
	w.e.Emit(Event{
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
	w.e.Emit(Event{
		Name:    EventNameWorkflowStarted,
		Payload: w.name,
	})
}

// AddChild adds a child to the workflow
func (w *Workflow) AddChild(n Node) {
	w.bn.AddChild(n)
}

// DelChild deletes a child from the workflow
func (w *Workflow) DelChild(n Node) {
	w.bn.DelChild(n)
}

// Children returns the workflow children
func (w *Workflow) Children() []Node {
	return w.bn.Children()
}

// Status returns the workflow status
func (w *Workflow) Status() string {
	return w.bn.Status()
}
