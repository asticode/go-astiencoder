package astiencoder

import (
	"context"
	"fmt"

	"github.com/asticode/go-astikit"
)

// Workflow represents a workflow
type Workflow struct {
	bn   *BaseNode
	c    *astikit.Closer
	ctx  context.Context
	eh   *EventHandler
	name string
	t    *astikit.Task
	tf   CreateTaskFunc
}

// NewWorkflow creates a new workflow
func NewWorkflow(ctx context.Context, name string, eh *EventHandler, tf CreateTaskFunc, c *astikit.Closer, s *Stater) (w *Workflow) {
	// Create workflow
	w = &Workflow{
		c:    c,
		ctx:  ctx,
		eh:   eh,
		name: name,
		tf:   tf,
	}

	// Create base node
	w.bn = NewBaseNode(NodeOptions{Metadata: NodeMetadata{
		Description: "root",
		Label:       "root",
		Name:        "root",
	}}, eh, s, w, EventTypeToWorkflowEventName)

	// Add stats
	w.addStats()
	return
}

func (w *Workflow) addStats() {
	w.bn.AddStats([]astikit.StatOptions{{
		Handler: newStatPSUtil(),
		Metadata: &astikit.StatMetadata{
			Name: StatNamePSUtil,
		},
	}}...)
}

// Name returns the workflow name
func (w *Workflow) Name() string {
	return w.name
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
func (w *Workflow) StartNodesInSubTask(ns ...Node) (t *astikit.Task) {
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
	Callback func(t *astikit.Task)
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
	fn func(t *astikit.Task)
	ns []Node
	t  *astikit.Task
}

func (w *Workflow) start(ns []Node, o WorkflowStartOptions) {
	w.bn.Start(w.ctx, w.tf, func(t *astikit.Task) {
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
			w.eh.Emit(EventError(w, fmt.Errorf("astiencoder: closing workflow %s failed: %w", w.name, err)))
		}
	})
}

// Stop stops the workflow
func (w *Workflow) Stop() {
	w.bn.Stop()
}

// Pause pauses the workflow
func (w *Workflow) Pause() {
	w.bn.pauseFunc(func() {
		for _, n := range w.nodes() {
			n.Pause()
		}
	})
}

// Continue continues the workflow
func (w *Workflow) Continue() {
	w.bn.continueFunc(func() {
		for _, n := range w.nodes() {
			n.Continue()
		}
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
