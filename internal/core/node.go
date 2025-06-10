package core

import (
	"fmt"
	"sync"
)

type Node struct {
	*Inode
	Name     string // Name of the node (last part of the path)
	Parent   *Node
	Children map[string]*Node // map of child nodes by name
	sync.RWMutex
}

// This function should NOT be directly called by external code, as inode
// numbering must be managed by the core filesystem
func NewNode(name string, inode *Inode) *Node {
	node := &Node{
		Inode:    inode,
		Name:     name,
		Parent:   nil, // parent node must add this node as child
		Children: make(map[string]*Node),
	}

	// Need to add new node to inode's hard links
	inode.AddHardLink(node)
	return node
}

func (n *Node) AddChild(child *Node) {
	n.Lock()
	defer n.Unlock()
	n.Children[child.Name] = child
	child.Parent = n
}

func (n *Node) RemoveChild(name string) bool {
	n.Lock()
	defer n.Unlock()
	if child, exists := n.Children[name]; exists {
		child.Parent = nil
		delete(n.Children, name)
		return true
	}
	return false
}

// Stable, persistent attributes needed to recreate the filesystem structure.
// Serves as the bridge between the core filesystem (Inode/Node) and data sources.
type PersistNode struct {
	ID          string  // Primary persistent uuid
	InodeID     string  // uuid of underlying inode
	Path        string  // Path relative to the root of the filesystem (quick lookups)
	ParentID    int64   // Foreign key to parent node
	ChildrenIDs []int64 // Foreign keys to child nodes
}

// Conversion functions
func NodeToPersist(n *Node) (*PersistNode, error) {
	return nil, fmt.Errorf("NodeToPersist not yet implemented")
}

func PersistToNode(p PersistNode) (*Node, error) {
	return nil, fmt.Errorf("PersistToNode not yet implemented")
}
