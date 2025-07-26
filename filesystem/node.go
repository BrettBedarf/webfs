package filesystem

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
)

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

// Path returns the path of the node relative from root.
// If the node is the root, returns ""
//
// Returns an error if the node or ancestor is detached or deleted with the path
// up to the first detached or non-deleted node
func (n *Node) Path() (string, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.pathLocked()
}

// See [Node.Path]
func (n *Node) pathLocked() (string, error) {
	if n.isRootLocked() {
		return "", nil
	}
	if n.isDel.Load() {
		return "", fmt.Errorf("deleted node: %s", n.name)
	}
	p := n.parent
	// handle detached node
	if p == nil {
		return n.name, fmt.Errorf("detached node: %s", n.name)
	}

	pPath, err := p.Path()
	if pPath == "" {
		// relative from root
		return pPath + n.name, err
	}
	return pPath + "/" + n.name, err
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

func (n *Node) IsRoot() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isRootLocked()
}

func (n *Node) isRootLocked() bool {
	ret := false
	if n.parent == nil {
		// cover detached nodes
		n.Inode.mu.RLock()
		defer n.Inode.mu.RUnlock()
		ret = n.attr.Ino == 1
	}
	return ret
}
