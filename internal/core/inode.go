package core

import (
	"sync"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type Inode struct {
	// Low-level fuse wire protocol attributes
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

// Adds a new Node, including the initial, as hard link
func (n *Inode) AddHardLink(node *Node) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.hLinks = append(n.hLinks, node)
	n.fuseAttr.Nlink++
}

// type PersistInode struct {
// 	Ino  uint64
// 	Size uint64
//
// 	// Blocks is the number of 512-byte blocks that the file occupies on disk.
// 	Blocks    uint64
// 	Atime     uint64
// 	Mtime     uint64
// 	Ctime     uint64
// 	Atimensec uint32
// 	Mtimensec uint32
// 	Ctimensec uint32
// 	Mode      uint32
// 	Nlink     uint32
// 	OwnerUid  uint32
// 	OwnerGid  uint32
// 	// NOTE: Only non-zero for device files (see S_IFCHR and S_IFBLK) (N/A)
// 	Rdev uint32
//
// 	// Blksize is the preferred size for file system operations.
// 	Blksize   uint32
// 	Padding   uint32
// 	HLinkIDs  []int64 // IDs of hardlinked nodes TODO: might have circular ref problems persisting
// 	SLinkIDs  []int64 // IDs of symbolic linked nodes
// 	SourceDef any     // TODO: maybe a table with type/meta + json blob?
// }
