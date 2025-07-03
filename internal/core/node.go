package core

import (
	"sync"
	"sync/atomic"
)

// TODO: separate lock just for children slice

type Node struct {
	*Inode
	name     string        // Name of the node (last part of the path)
	nodeID   atomic.Uint64 // Active registry ID; 0 if not registered
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
		name:     name,
		parent:   nil, // parent node must add this node as child
		children: make(map[string]*Node),
	}

	// Need to add new node to inode's hard links
	inode.AddHardLink(node)
	return node
}

// NodeID returns the nodeID of the node (Thread-safe); 0 if not registered
func (n *Node) NodeID() uint64 {
	return n.nodeID.Load()
}

// AddChild adds a child node to the node's children map
// and sets the child's parent to this node
func (n *Node) AddChild(child *Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.children[child.name] = child

	child.mu.Lock()
	defer child.mu.Unlock()
	child.parent = n
}

// internal children accessor when Node is already locked
// such as in NodeContext
// TODO: Thread-safe way to pass around Map/List of actual child Nodes or even ditch direct access
func (n *Node) childrenLocked() []string {
	keys := make([]string, 0, len(n.children))
	for name := range n.children {
		keys = append(keys, name)
	}
	return keys
}

// ChildrenSafe returns the children of the node with its own lock.
// Not to be called when Node is already locked in NodeContext
func (n *Node) ChildrenSafe() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.childrenLocked()
}

// internal child accessor when Node is already locked
func (n *Node) getChildLocked(name string) (child *Node, ok bool) {
	child, ok = n.children[name]
	return
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

// internal name accessor when Node is already locked
func (n *Node) nameLocked() string {
	return n.name
}

// NameSafe returns the name of the node with its own lock.
// Not to be called when Node is already locked in NodeContext
func (n *Node) NameSafe() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.nameLocked()
}
