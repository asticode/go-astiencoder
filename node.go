package astiencoder

import (
	"context"
	"sync"
)

// Node represents a node
type Node interface {
	Children() []Node
	Metadata() NodeMetadata
	Start(ctx context.Context, t CreateTaskFunc)
	Stop()
}

// NodeMetadata represents node metadata
type NodeMetadata struct {
	Description string
	Name        string
	Label       string
}

// BaseNode factorizes part of the implementation of the Node interface
type BaseNode struct {
	children        []Node
	childrenIndexed map[string]Node
	m               *sync.Mutex
	md              NodeMetadata
}

// NewBaseNode creates a new base node
func NewBaseNode(md NodeMetadata) *BaseNode {
	return &BaseNode{
		childrenIndexed: make(map[string]Node),
		m:               &sync.Mutex{},
		md:              md,
	}
}

// AddChildren adds children
func (bn *BaseNode) AddChildren(ns ...Node) {
	bn.m.Lock()
	defer bn.m.Unlock()
	for _, n := range ns {
		if _, ok := bn.childrenIndexed[n.Metadata().Name]; ok {
			continue
		}
		bn.childrenIndexed[n.Metadata().Name] = n
		bn.children = append(bn.children, n)
	}
}

// Children implements the Node interface
func (bn *BaseNode) Children() []Node {
	bn.m.Lock()
	defer bn.m.Unlock()
	return append([]Node{}, bn.children...)
}

// Metadata implements the Node interface
func (bn *BaseNode) Metadata() NodeMetadata {
	return bn.md
}
