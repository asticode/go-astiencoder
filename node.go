package astiencoder

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/worker"
)

// Node represents a node
type Node interface {
	NodeChild
	NodeDescriptor
	NodeParent
	Starter
	Stater
}

// NodeDescriptor represents an object that can describe a node
type NodeDescriptor interface {
	Metadata() NodeMetadata
}

// NodeMetadata represents node metadata
type NodeMetadata struct {
	Description string
	Label       string
	Name        string
}

// Extend extends the node metadata
func (m NodeMetadata) Extend(name, label, description string) NodeMetadata {
	if len(m.Description) == 0 {
		m.Description = description
	}
	if len(m.Label) == 0 {
		m.Label = label
	}
	if len(m.Name) == 0 {
		m.Name = name
	}
	return m
}

// NodeChild represents an object with parent nodes
type NodeChild interface {
	NodeChildMapper
	ParentIsStarted(m NodeMetadata)
	ParentIsStopped(m NodeMetadata)
}

// NodeChildMapper represents an object that can play with its parents map
type NodeChildMapper interface {
	AddParent(n Node)
	DelParent(n Node)
	Parents() []Node
}

// NodeParent represents an object with child nodes
type NodeParent interface {
	NodeParentMapper
	ChildIsStarted(m NodeMetadata)
	ChildIsStopped(m NodeMetadata)
}

// NodeParentMapper represents an object that can play with its children map
type NodeParentMapper interface {
	AddChild(n Node)
	Children() []Node
	DelChild(n Node)
}

// Statuses
const (
	StatusPaused  = "paused"
	StatusRunning = "running"
	StatusStopped = "stopped"
)

// Starter represents an object that can start/pause/continue/stop
type Starter interface {
	Continue()
	Pause()
	Start(ctx context.Context, t CreateTaskFunc)
	Status() string
	Stop()
}

// Stater represents an object that can return its stater
type Stater interface {
	Stater() *astistat.Stater
}

// ConnectNodes connects 2 nodes
func ConnectNodes(parent, child Node) {
	parent.AddChild(child)
	child.AddParent(parent)
}

// DisconnectNodes disconnects 2 nodes
func DisconnectNodes(parent, child Node) {
	parent.DelChild(child)
	child.DelParent(parent)
}

// NodeOptions represents node options
type NodeOptions struct {
	Metadata       NodeMetadata
	NoIndirectStop bool
}

// BaseNode represents a base node
type BaseNode struct {
	cancel          context.CancelFunc
	cancelPause     context.CancelFunc
	children        map[string]Node
	childrenStarted map[string]bool
	ctx             context.Context
	ctxPause        context.Context
	eh              *EventHandler
	eg              EventGenerator
	o               NodeOptions
	m               *sync.Mutex
	oStart          *sync.Once
	oStop           *sync.Once
	parents         map[string]Node
	parentsStarted  map[string]bool
	s               *astistat.Stater
	status          string
}

// NewBaseNode creates a new base node
func NewBaseNode(o NodeOptions, eg EventGenerator, eh *EventHandler) (n *BaseNode) {
	n = &BaseNode{
		children:        make(map[string]Node),
		childrenStarted: make(map[string]bool),
		m:               &sync.Mutex{},
		eh:              eh,
		eg:              eg,
		o:               o,
		oStart:          &sync.Once{},
		oStop:           &sync.Once{},
		parents:         make(map[string]Node),
		parentsStarted:  make(map[string]bool),
		status:          StatusStopped,
	}
	n.s = astistat.NewStater(2*time.Second, n.statsHandleFunc)
	return
}

// Context returns the node context
func (n *BaseNode) Context() context.Context {
	return n.ctx
}

// CreateTaskFunc is a method that can create a task
type CreateTaskFunc func() *astiworker.Task

// BaseNodeStartFunc represents a node start func
type BaseNodeStartFunc func()

// BaseNodeExecFunc represents a node exec func
type BaseNodeExecFunc func(t *astiworker.Task)

// Status implements the Starter interface
func (n *BaseNode) Status() string {
	n.m.Lock()
	defer n.m.Unlock()
	return n.status
}

// Start starts the node
func (n *BaseNode) Start(ctx context.Context, tc CreateTaskFunc, execFunc BaseNodeExecFunc) {
	// Make sure the node can only be started once
	n.oStart.Do(func() {
		// Check context
		if ctx.Err() != nil {
			return
		}

		// Create task
		t := tc()

		// Reset context
		n.ctx, n.cancel = context.WithCancel(ctx)

		// Reset once
		n.oStop = &sync.Once{}

		// Loop through children
		for _, c := range n.Children() {
			c.ParentIsStarted(n.o.Metadata)
		}

		// Loop through parents
		for _, p := range n.Parents() {
			p.ChildIsStarted(n.o.Metadata)
		}

		// Update status
		n.m.Lock()
		n.status = StatusRunning
		n.m.Unlock()

		// Send started event
		n.eh.Emit(n.eg.Event(EventTypeStarted, nil))

		// Execute the rest in a goroutine
		go func() {
			// Task is done
			defer t.Done()

			// Send stopped event
			defer n.eh.Emit(n.eg.Event(EventTypeStopped, nil))

			// Make sure the status is updated once everything is done
			defer func() {
				n.m.Lock()
				defer n.m.Unlock()
				n.status = StatusStopped
			}()

			// Let children and parents know the node is stopped
			defer func() {
				// Loop through children
				for _, c := range n.Children() {
					c.ParentIsStopped(n.o.Metadata)
				}

				// Loop through parents
				for _, p := range n.Parents() {
					p.ChildIsStopped(n.o.Metadata)
				}
			}()

			// Make sure the node is stopped properly
			defer n.Stop()

			// Handle the stater
			if n.s != nil {
				// Make sure the stater is stopped properly
				defer n.s.Stop()

				// Start stater
				n.s.Start(n.ctx)
			}

			// Exec func
			execFunc(t)
		}()
	})
}

// Stop stops the node
func (n *BaseNode) Stop() {
	// Make sure the node can only be stopped once
	n.oStop.Do(func() {
		// Cancel context
		if n.cancel != nil {
			n.cancel()
		}

		// Reset once
		n.oStart = &sync.Once{}
	})
}

// Pause implements the Starter interface
func (n *BaseNode) Pause() {
	n.pauseFunc(func() {
		n.ctxPause, n.cancelPause = context.WithCancel(n.ctx)
	})
}

// Pause implements the Starter interface
func (n *BaseNode) pauseFunc(fn func()) {
	// Status is not running
	if n.Status() != StatusRunning {
		return
	}

	// Callback
	fn()

	// Update status
	n.m.Lock()
	n.status = StatusPaused
	n.m.Unlock()

	// Send paused event
	n.eh.Emit(n.eg.Event(EventTypePaused, nil))
}

// Continue implements the Starter interface
func (n *BaseNode) Continue() {
	n.continueFunc(func() {
		if n.cancelPause != nil {
			n.cancelPause()
		}
	})
}

func (n *BaseNode) continueFunc(fn func()) {
	// Status is not paused
	if n.Status() != StatusPaused {
		return
	}

	// Callback
	fn()

	// Update status
	n.m.Lock()
	n.status = StatusRunning
	n.m.Unlock()

	// Send continued event
	n.eh.Emit(n.eg.Event(EventTypeContinued, nil))
}

// HandlePause handles the pause
func (n *BaseNode) HandlePause() {
	// Status is not paused
	if n.Status() != StatusPaused {
		return
	}

	// Wait for ctx to be done
	<-n.ctxPause.Done()
}

// AddChild implements the NodeParent interface
func (n *BaseNode) AddChild(i Node) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.children[i.Metadata().Name]; ok {
		return
	}
	n.children[i.Metadata().Name] = i
}

// DelChild implements the NodeParent interface
func (n *BaseNode) DelChild(i Node) {
	n.m.Lock()
	defer n.m.Unlock()
	delete(n.children, i.Metadata().Name)
}

// ChildIsStarted implements the NodeParent interface
func (n *BaseNode) ChildIsStarted(m NodeMetadata) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.children[m.Name]; !ok {
		return
	}
	n.childrenStarted[m.Name] = true
}

// ChildIsStopped implements the NodeParent interface
func (n *BaseNode) ChildIsStopped(m NodeMetadata) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.children[m.Name]; !ok {
		return
	}
	delete(n.childrenStarted, m.Name)
	if len(n.childrenStarted) == 0 && !n.o.NoIndirectStop {
		n.Stop()
	}
}

// Children implements the NodeParent interface
func (n *BaseNode) Children() (ns []Node) {
	n.m.Lock()
	defer n.m.Unlock()
	var ks []string
	for k := range n.children {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		ns = append(ns, n.children[k])
	}
	return
}

// AddParent implements the NodeChild interface
func (n *BaseNode) AddParent(i Node) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.parents[i.Metadata().Name]; ok {
		return
	}
	n.parents[i.Metadata().Name] = i
}

// DelParent implements the NodeChild interface
func (n *BaseNode) DelParent(i Node) {
	n.m.Lock()
	defer n.m.Unlock()
	delete(n.parents, i.Metadata().Name)
}

// ParentIsStarted implements the NodeChild interface
func (n *BaseNode) ParentIsStarted(m NodeMetadata) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.parents[m.Name]; !ok {
		return
	}
	n.parentsStarted[m.Name] = true
}

// ParentIsStopped implements the NodeChild interface
func (n *BaseNode) ParentIsStopped(m NodeMetadata) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.parents[m.Name]; !ok {
		return
	}
	delete(n.parentsStarted, m.Name)
	if len(n.parentsStarted) == 0 && !n.o.NoIndirectStop {
		n.Stop()
	}
}

// Parents implements the NodeChild interface
func (n *BaseNode) Parents() (ns []Node) {
	n.m.Lock()
	defer n.m.Unlock()
	var ks []string
	for k := range n.parents {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		ns = append(ns, n.parents[k])
	}
	return
}

// Metadata implements the Node interface
func (n *BaseNode) Metadata() NodeMetadata {
	return n.o.Metadata
}

// Stater returns the node stater
func (n *BaseNode) Stater() *astistat.Stater {
	return n.s
}

// EventStat represents a stat event
type EventStat struct {
	Description string
	Label       string
	Unit        string
	Value       interface{}
}

func (n *BaseNode) statsHandleFunc(stats []astistat.Stat) {
	// No stats
	if len(stats) == 0 {
		return
	}

	// Loop through stats
	ss := []EventStat{}
	for _, s := range stats {
		ss = append(ss, EventStat{
			Description: s.Description,
			Label:       s.Label,
			Unit:        s.Unit,
			Value:       s.Value,
		})
	}

	// Send event
	n.eh.Emit(n.eg.Event(EventTypeStats, ss))
}
