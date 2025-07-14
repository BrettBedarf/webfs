package core

import (
	"sync"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
)

// TODO: separate lock just for children slice

type Node struct {
	name     string                    // Name of the node (last part of the path). Protected by mu
	parent   *Node                     // Protected by mu
	mu       sync.RWMutex              // Protects the fields above
	nodeID   atomic.Uint64             // Active registry ID; 0 if not registered
	children *xsync.Map[string, *Node] // thread-safe map of child nodes by name
	isDel    atomic.Bool
	*Inode
}

// NewNode creates a new Node and adds it to the INode's hard links
//
// NOTE: Parent node is responsible for adding itself to the returned Node's
// Parent ref when linking as its child
func NewNode(name string, inode *Inode) *Node {
	node := &Node{
		Inode:    inode,
		name:     name,
		parent:   nil, // parent node must add this node as child
		children: xsync.NewMap[string, *Node](),
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
	n.children.Store(child.name, child)

	child.mu.Lock()
	defer child.mu.Unlock()
	child.parent = n
}

// GetChild returns a child node.
// Safe to call when Node is already locked
func (n *Node) GetChild(name string) (child *Node, ok bool) {
	return n.children.Load(name)
}

func (n *Node) RemoveChild(name string) bool {
	// TODO: return the detached Node?
	if child, exists := n.children.LoadAndDelete(name); exists {
		child.mu.Lock()
		defer child.mu.Unlock()
		child.parent = nil
		return true
	}
	return false
}

// internal name accessor when Node is already locked
func (n *Node) nameLocked() string {
	return n.name
}

// Name returns the node's immutable Name.
func (n *Node) Name() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.nameLocked()
}

func (n *Node) IsDel() bool {
	return n.isDel.Load()
}

// Del marks the node as deleted and runs any cleanup
func (n *Node) Del() {
	n.isDel.Store(true)
}

// NameSafe returns the name of the node with its own lock.
// Not to be called when Node is already locked in NodeContext
func (n *Node) NameSafe() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.nameLocked()
}
