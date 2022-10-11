package astiencoder

import (
	"context"
	"sort"
	"sync"

	"github.com/asticode/go-astikit"
)

// Node represents a node
type Node interface {
	Closer
	NodeChild
	NodeDescriptor
	NodeParent
	Starter
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
	Tags        []string
}

// Extend extends the node metadata
func (m NodeMetadata) Extend(name, label, description string, tags ...string) NodeMetadata {
	if len(m.Description) == 0 {
		m.Description = description
	}
	if len(m.Label) == 0 {
		m.Label = label
	}
	if len(m.Name) == 0 {
		m.Name = name
	}
	for _, t := range tags {
		var found bool
		for _, ft := range m.Tags {
			if ft == t {
				found = true
				break
			}
		}
		if !found {
			m.Tags = append(m.Tags, t)
		}
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
	StatusCreated = "created"
	StatusPaused  = "paused"
	StatusRunning = "running"
	StatusStopped = "stopped"
)

type Closer interface {
	AddClose(astikit.CloseFunc)
	AddCloseWithError(astikit.CloseFuncWithError)
	Close() error
	DoWhenUnclosed(func())
	IsClosed() bool
}

// Starter represents an object that can start/pause/continue/stop
type Starter interface {
	Continue()
	Pause()
	Start(ctx context.Context, t CreateTaskFunc)
	Status() string
	Stop()
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
	c               *astikit.Closer
	cancel          context.CancelFunc
	cancelPause     context.CancelFunc
	children        map[string]Node
	childrenStarted map[string]bool
	ctx             context.Context
	ctxPause        context.Context
	eh              *EventHandler
	et              EventTypeTransformer
	o               NodeOptions
	m               *sync.Mutex
	oStart          *sync.Once
	oStop           *sync.Once
	parents         map[string]Node
	parentsStarted  map[string]bool
	s               *Stater
	ss              []astikit.StatOptions
	status          string
	target          interface{}
}

// NewBaseNode creates a new base node
func NewBaseNode(o NodeOptions, c *astikit.Closer, eh *EventHandler, s *Stater, target interface{}, et EventTypeTransformer) (n *BaseNode) {
	// Create node
	n = &BaseNode{
		c:               c.NewChild(),
		children:        make(map[string]Node),
		childrenStarted: make(map[string]bool),
		m:               &sync.Mutex{},
		eh:              eh,
		et:              et,
		o:               o,
		oStart:          &sync.Once{},
		oStop:           &sync.Once{},
		parents:         make(map[string]Node),
		parentsStarted:  make(map[string]bool),
		s:               s,
		status:          StatusCreated,
		target:          target,
	}

	// Set closer callback
	n.c.OnClosed(func(err error) {
		eh.Emit(Event{
			Name:   et(EventTypeClosed),
			Target: target,
		})
	})
	return
}

func (n *BaseNode) AddClose(fn astikit.CloseFunc) {
	n.c.Add(fn)
}

func (n *BaseNode) AddCloseWithError(fn astikit.CloseFuncWithError) {
	n.c.AddWithError(fn)
}

func (n *BaseNode) Close() error {
	return n.c.Close()
}

func (n *BaseNode) IsClosed() bool {
	return n.c.IsClosed()
}

func (n *BaseNode) DoWhenUnclosed(fn func()) {
	n.c.Do(fn)
}

// Context returns the node context
func (n *BaseNode) Context() context.Context {
	return n.ctx
}

// CreateTaskFunc is a method that can create a task
type CreateTaskFunc func() *astikit.Task

// BaseNodeStartFunc represents a node start func
type BaseNodeStartFunc func()

// BaseNodeExecFunc represents a node exec func
type BaseNodeExecFunc func(t *astikit.Task)

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
		n.eh.Emit(Event{
			Name:   n.et(EventTypeStarted),
			Target: n.target,
		})

		// Execute the rest in a goroutine
		go func() {
			// Task is done
			defer t.Done()

			// Send stopped event
			defer n.eh.Emit(Event{
				Name:   n.et(EventTypeStopped),
				Target: n.target,
			})

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

			// Handle stats
			if n.s != nil {
				// Make sure to delete stats
				defer func() {
					// Lock
					n.m.Lock()
					defer n.m.Unlock()

					// Delete stats
					n.s.DelStats(n.target, n.ss...)
				}()

				// Add stats
				n.m.Lock()
				n.s.AddStats(n.target, n.ss...)
				n.m.Unlock()
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
			n.cancel = nil
		}

		// Cancel pause context as well
		// We need to cancel stop context before cancelling pause context for the following workflow:
		//   - in its loop, node handles pause before checking stop ctx
		//   - in that case, we want the stop ctx to be accurate once the pause ctx is done
		if n.cancelPause != nil {
			n.cancelPause()
			n.cancelPause = nil
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
	n.eh.Emit(Event{
		Name:   n.et(EventTypePaused),
		Target: n.target,
	})
}

// Continue implements the Starter interface
func (n *BaseNode) Continue() {
	n.continueFunc(func() {
		if n.cancelPause != nil {
			n.cancelPause()
			n.cancelPause = nil
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
	n.eh.Emit(Event{
		Name:   n.et(EventTypeContinued),
		Target: n.target,
	})
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
	// Lock
	n.m.Lock()

	// Node doesn't exist
	if _, ok := n.children[i.Metadata().Name]; ok {
		n.m.Unlock()
		return
	}

	// Add child
	n.children[i.Metadata().Name] = i

	// Unlock
	n.m.Unlock()

	// Send event
	// Mutex should be unlocked at this point
	n.eh.Emit(Event{
		Name:    n.et(EventTypeChildAdded),
		Payload: i,
		Target:  n.target,
	})
}

// DelChild implements the NodeParent interface
func (n *BaseNode) DelChild(i Node) {
	// Lock
	n.m.Lock()

	// Delete child
	delete(n.children, i.Metadata().Name)

	// Unlock
	n.m.Unlock()

	// Send event
	// Mutex should be unlocked at this point
	n.eh.Emit(Event{
		Name:    n.et(EventTypeChildRemoved),
		Payload: i,
		Target:  n.target,
	})
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

// AddStats adds stats
func (n *BaseNode) AddStats(ss ...astikit.StatOptions) {
	n.m.Lock()
	defer n.m.Unlock()
	n.ss = append(n.ss, ss...)
}
