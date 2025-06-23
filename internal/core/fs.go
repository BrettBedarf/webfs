package core

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brettbedarf/webfs/api"
	"github.com/brettbedarf/webfs/config"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type FileSystem struct {
	cfg      *config.Config
	root     *Node             // Root of node tree
	inodeMap map[uint64]*Inode // Map of inode numbers to Inodes
	nextIno  atomic.Uint64     // Next inode number to assign
	mu       sync.RWMutex
}

func NewFS(cfg *config.Config) *FileSystem {
	rootAttr := newDefaultAttr()
	rootAttr.Ino = fuse.FUSE_ROOT_ID
	rootAttr.Mode = uint32(syscall.S_IFDIR | 0755) // directory with rwxr-xr-x permissions

	rootInode := NewInode(rootAttr)
	rootNode := NewNode("", rootInode)

	inodeMap := make(map[uint64]*Inode)
	inodeMap[rootAttr.Ino] = rootInode

	fs := FileSystem{
		cfg:      cfg,
		root:     rootNode,
		inodeMap: inodeMap,
	}
	fs.nextIno.Store(fuse.FUSE_ROOT_ID + 1)

	return &fs
}

func (fs *FileSystem) AddFileNode(req *api.FileCreateRequest) error {
	return nil
}

func (fs *FileSystem) AddDirNode(req *api.DirCreateRequest) error {
	return nil
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
