package core

import (
	"sync"
)

type Node struct {
	*Inode
	Name     string // Name of the node (last part of the path)
	parent   *Node
	children map[string]*Node // map of child nodes by name
	mu       sync.RWMutex
}

// NewNode creates a new Node and adds it to the INode's hard links
// NOTE: Parent node is responsible for adding itself to the returned Node's
// Parent ref when linking as its child
func NewNode(name string, inode *Inode) *Node {
	node := &Node{
		Inode:    inode,
		Name:     name,
		parent:   nil, // parent node must add this node as child
		children: make(map[string]*Node),
	}

	// Need to add new node to inode's hard links
	inode.AddHardLink(node)
	return node
}

// AddChild adds a child node to the node's children map
// and sets the child's parent to this node
func (n *Node) AddChild(child *Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.children[child.Name] = child

	child.mu.Lock()
	defer child.mu.Unlock()
	child.parent = n
}

func (n *Node) GetChild(name string) (child *Node, ok bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	child, ok = n.children[name]
	return
}

func (n *Node) RemoveChild(name string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if child, exists := n.children[name]; exists {
		child.parent = nil
		delete(n.children, name)
		return true
	}
	return false
}
