package filesystem

import (
	"sync"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type Inode struct {
	// Low-level fuse wire protocol attributes; Only access directly if
	// handling locks manually
	fuseAttr *fuse.Attr
	hLinks   []*Node // Hard links to this inode
	sLinks   []*Node // Symbolic links to this inode
	mu       sync.RWMutex
}

func NewInode(attr *fuse.Attr) *Inode {
	return &Inode{
		fuseAttr: attr,
		hLinks:   make([]*Node, 0, 1), // 1 init capacity since most inodes expected to have 1
		sLinks:   make([]*Node, 0),    // 0 init capacity since assumed low usage
	}
}

// addHardLinkLocked appends a new hard link to the inode.
// Caller must hold n.mu.Lock().
func (n *Inode) addHardLinkLocked(node *Node) {
	n.hLinks = append(n.hLinks, node)
	n.fuseAttr.Nlink++
}

// AddHardLink adds a new Node, including the initial, as hard link
func (n *Inode) AddHardLink(node *Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.addHardLinkLocked(node)
}

// CopyAttr returns a thread-safe copy of the inode's attributes
func (n *Inode) CopyAttr() fuse.Attr {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return *n.fuseAttr
}
