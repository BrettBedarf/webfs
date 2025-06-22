package core

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type FS struct {
	root     *Node             // Root of node tree
	inodeMap map[uint64]*Inode // Map of inode numbers to Inodes
	nextIno  atomic.Uint64     // Next inode number to assign
	mu       sync.RWMutex
	cfg      *config.Config
}

func NewFS(config *config.Config) *FS {
	rootAttr := newDefaultAttr()
	rootAttr.Ino = fuse.FUSE_ROOT_ID
	rootAttr.Mode = uint32(syscall.S_IFDIR | 0755) // directory with rwxr-xr-x permissions

	rootInode := NewInode(rootAttr)
	rootNode := NewNode("", rootInode)

	inodeMap := make(map[uint64]*Inode)
	inodeMap[rootAttr.Ino] = rootInode

	fs := FS{
		root:     rootNode,
		inodeMap: inodeMap,
	}
	fs.nextIno.Store(fuse.FUSE_ROOT_ID + 1)

	return &fs
}

func (fs *FS) AddFileNode(req *fs.FileCreateRequest) error {
	return nil
}

func (fs *FS) AddDirNode(req *fs.DirCreateRequest) error {
	return nil
}

// Config returns a copy of the config
func (fs *FS) Config() config.Config {
	return *fs.cfg
}

func newDefaultAttr() fuse.Attr {
	now := time.Now()
	return fuse.Attr{
		Nlink: 1,
		Owner: fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
		Atime:     uint64(now.Unix()),
		Mtime:     uint64(now.Unix()),
		Ctime:     uint64(now.Unix()),
		Atimensec: uint32(now.Nanosecond()),
		Mtimensec: uint32(now.Nanosecond()),
		Ctimensec: uint32(now.Nanosecond()),
		Blksize:   4096, // preferred size for fs ops
		// Only non-zero for device files (see S_IFCHR and S_IFBLK) (N/A)
		Rdev: 0,
		// Used for byte-to-byte compat with fuse wire protocol
		// but *should* be handled by go-fuse or not relevant and just
		// set/default to 0
		Padding: 0,
	}
}
