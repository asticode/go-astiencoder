package astiencoder

import (
	"context"
	"sync"

	"sort"

	"github.com/asticode/go-astitools/worker"
)

// Node represents a node
type Node interface {
	NodeChild
	NodeParent
	Starter
	Metadata() NodeMetadata
}

// NodeMetadata represents node metadata
type NodeMetadata struct {
	Description string
	Name        string
	Label       string
}

// NodeChild represents an object with parent nodes
type NodeChild interface {
	AddParent(n Node)
	ParentIsDone(m NodeMetadata)
}

// NodeParent represents an object with child nodes
type NodeParent interface {
	AddChild(n Node)
	Children() []Node
}

// StartOptions represents start options
type StartOptions struct {
	StopChildrenWhenDone bool
}

// Starter represents an object that can start/stop
type Starter interface {
	Start(ctx context.Context, o StartOptions, t CreateTaskFunc)
	Stop()
}

// ConnectNodes connects 2 nodes
func ConnectNodes(parent, child Node) {
	parent.AddChild(child)
	child.AddParent(parent)
}

// BaseNode represents a base node
type BaseNode struct {
	cancel      context.CancelFunc
	children    map[string]Node
	ctx         context.Context
	m           *sync.Mutex
	md          NodeMetadata
	oStart      *sync.Once
	oStop       *sync.Once
	parents     map[string]Node
	parentsDone map[string]bool
}

// NewBaseNode creates a new base node
func NewBaseNode(m NodeMetadata) *BaseNode {
	return &BaseNode{
		children:    make(map[string]Node),
		m:           &sync.Mutex{},
		md:          m,
		oStart:      &sync.Once{},
		oStop:       &sync.Once{},
		parents:     make(map[string]Node),
		parentsDone: make(map[string]bool),
	}
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

// Start starts the node
func (n *BaseNode) Start(ctx context.Context, o StartOptions, tc CreateTaskFunc, execFunc BaseNodeExecFunc) {
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

		// Execute the rest in a goroutine
		go func() {
			// Task is done
			defer t.Done()

			// Make sure the node is stopped properly
			defer n.Stop()

			// Exec func
			execFunc(t)

			// Stop children
			if o.StopChildrenWhenDone && (n.ctx.Err() == nil || n.allParentsAreDone()) {
				// Loop through children
				for _, c := range n.Children() {
					c.ParentIsDone(n.md)
				}
			}
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

// AddChild implements the NodeParent interface
func (n *BaseNode) AddChild(i Node) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.children[i.Metadata().Name]; ok {
		return
	}
	n.children[i.Metadata().Name] = i
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

// ParentIsDone implements the NodeChild interface
func (n *BaseNode) ParentIsDone(m NodeMetadata) {
	n.m.Lock()
	defer n.m.Unlock()
	if _, ok := n.parents[m.Name]; !ok {
		return
	}
	n.parentsDone[m.Name] = true
	if len(n.parentsDone) == len(n.parents) {
		n.Stop()
	}
}

func (n *BaseNode) allParentsAreDone() bool {
	n.m.Lock()
	defer n.m.Unlock()
	return len(n.parentsDone) == len(n.parents)
}

// Metadata implements the Node interface
func (n *BaseNode) Metadata() NodeMetadata {
	return n.md
}
