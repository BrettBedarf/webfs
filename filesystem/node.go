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
	inode    *Inode
}

// NewNode creates a new Node and adds it to the INode's hard links
//
// NOTE: Parent node is responsible for adding itself to the returned Node's
// Parent ref when linking as its child
func NewNode(name string, inode *Inode) (*Node, error) {
	if inode == nil {
		return nil, fmt.Errorf("cannot create node with nil inode")
	}

	node := &Node{
		inode:    inode,
		name:     name,
		parent:   nil, // parent node must add this node as child
		children: xsync.NewMap[string, *Node](),
	}

	// Need to add new node to inode's hard links
	inode.AddHardLink(node)
	return node, nil
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

// See [Node.Path]. Only safe to call when Node is already locked
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
// and sets the child's parent to this node.
// If a child with the same name already exists, it will be properly deleted first.
func (n *Node) AddChild(child *Node) {
	// Check if a child with this name already exists
	if existingChild, exists := n.children.LoadAndDelete(child.name); exists {
		// Use standard deletion logic which handles all cleanup
		existingChild.Del()
	}

	// Add the new child
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

// See [Node.Name] only safe to call when Node is already locked
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

// Del marks the node as deleted and handles all cleanup.
// This includes detaching from parent, clearing cache, and preparing for
// future event notifications to FUSE layer and watchers.
func (n *Node) Del() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 1. Mark as deleted first to prevent races
	n.isDel.Store(true)

	// 2. Detach from parent (break tree relationship)
	n.parent = nil

	// 3. Clear any cached data from the inode
	n.inode.ClearCache()

	// TODO: Remove this node from inode's hard links list
	// n.RemoveHardLink(n) - needs implementation

	// TODO: Implement channel-based event system
	// Future: Send deletion event through filesystem event channel
	// This will notify:
	// - FUSE layer (invalidate cache entries, handle open file descriptors)
	// - File watchers (inotify-style notifications)
	// - Logging system
	// - Metrics collectors
	//
	// Proposed structure:
	// if n.filesystem != nil && n.filesystem.events != nil {
	//     select {
	//     case n.filesystem.events <- NodeEvent{
	//         Type: NodeDeleted,
	//         Node: n,
	//         Path: n.pathLocked(), // Capture path before cleanup
	//         Timestamp: time.Now(),
	//     }:
	//     default:
	//         // Event channel full - could increment dropped events counter
	//     }
	// }

	// TODO: Cleanup adapter resources
	// If this node has file adapters (HTTP connections, file handles, etc.)
	// they should be properly closed to prevent resource leaks
	// This might need to be async to avoid blocking deletion

	// TODO: Handle open file descriptors
	// If FUSE layer has active handles to this file, coordinate cleanup
	// This is especially important for:
	// - Active read/write operations
	// - Memory-mapped files
	// - Directory handles with readdir in progress
}

// NameSafe returns the name of the node with its own lock.
// Not to be called when Node is already locked in NodeContext
func (n *Node) NameSafe() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.nameLocked()
}

// Inode returns the underlying inode
func (n *Node) Inode() *Inode {
	return n.inode
}

func (n *Node) IsRoot() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isRootLocked()
}

// See [Node.IsRoot]. Only safe to call when Node is already locked.
func (n *Node) isRootLocked() bool {
	ret := false
	if n.parent == nil {
		// cover detached nodes
		n.inode.mu.RLock()
		defer n.inode.mu.RUnlock()
		ret = n.inode.attr.Ino == 1
	}
	return ret
}
