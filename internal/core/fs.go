package core

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brettbedarf/webfs/api"
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/util"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/puzpuzpuz/xsync/v4"
)

type FileSystemOperator interface {
	Root() *Node
	AddFileNode(req *api.FileCreateRequest) (*Node, error)
	AddDirNode(req *api.DirCreateRequest) (*Node, error)
	InodeFromIno(ino uint64) (inode *Inode, ok bool)
	AttrFromIno(ino uint64) (attr *fuse.Attr, ok bool)
}

type FileSystem struct {
	cfg     *config.Config
	root    *Node         // Root of node tree
	lastIno atomic.Uint64 // Last inode number assigned; incremented when new nodes are created
	mu      sync.RWMutex
}

func NewFS(cfg *config.Config) *FileSystem {
	rootAttr := newDefaultAttr(fuse.FUSE_ROOT_ID)
	rootAttr.Mode = uint32(syscall.S_IFDIR | 0755) // directory with rwxr-xr-x permissions

	rootInode := NewInode(rootAttr)
	rootNode := NewNode("", rootInode) // TODO: should be "." ??

	// inodeMap := make(map[uint64]*Inode)
	inodeMap := xsync.NewMap[uint64, *Inode]()
	inodeMap.Store(fuse.FUSE_ROOT_ID, rootInode)

	fs := FileSystem{
		cfg:  cfg,
		root: rootNode,
	}
	fs.lastIno.Store(fuse.FUSE_ROOT_ID)

	return &fs
}

func (fs *FileSystem) Root() *Node {
	return fs.root
}

// AddFileNode adds a new file node to the filesystem. It will add any missing
// directories in the path and return the newly created leaf node
// If a node already exists at the requested path, it will return an error
func (fs *FileSystem) AddFileNode(req *api.FileCreateRequest) (*Node, error) {
	logger := util.GetLogger("AddFileNode")

	parent := fs.root
	// TODO: Path edge cases
	dirPath, name := path.Split(req.Path) // dir is full path to dir, file is just the filename
	if dirPath != "" {
		// Implicit dir requests are just the same embedded Node values with a different path
		dirReq := api.DirCreateRequest{NodeRequest: req.NodeRequest}
		dirReq.Path = dirPath
		dNode, err := fs.AddDirNode(&dirReq)
		if err != nil {
			logger.Error().Err(err).Str("path", dirReq.Path).Msg("Failed to create file's ancestor directory(s)")
			return nil, err
		}
		parent = dNode
	}
	// Return error if file already exists
	if _, ok := parent.GetChild(name); ok {
		err := fmt.Errorf("file already exists at path %s", req.Path)
		logger.Error().Err(err).Str("path", req.Path).Msg("Failed to create file")
		return nil, err
	}

	attr := newDefaultAttr(fs.lastIno.Add(1))
	attr.Mode = uint32(FileAttr) | req.Perms

	inode := NewInode(attr)
	node := NewNode(name, inode)
	parent.AddChild(node)
	logger.Info().Str("path", req.Path).Msg("Added new file node")
	return node, nil
}

// AddDirNode recursively adds all missing directories starting at [root]
// in the request's path and returns the leaf.
// It is equivalent to calling `mkdir -p` from a shell and similarly will only create
// directories that do not already exist and will not error if the leaf already exists.
func (fs *FileSystem) AddDirNode(req *api.DirCreateRequest) (*Node, error) {
	logger := util.GetLogger("AddDirNode")

	dSplit := strings.Split(req.Path, "/")
	cur := fs.root
	newCnt := 0
	// Traverse the path until we get to existing dir and make
	// any missing along the way
	for _, name := range dSplit {
		cur.mu.RLock()
		prev := cur // tmp so we can unlock after re-assigning cur

		if child, ok := cur.children[name]; ok {
			cur = child
		} else {
			// Make new dir
			attr := newDefaultAttr(fs.lastIno.Add(1))
			attr.Mode = uint32(DirAttr) | req.Perms
			inode := NewInode(attr)
			node := NewNode(name, inode)

			cur.AddChild(node) // cur node will lock & unlock itself
			newCnt++
			cur = node
		}
		prev.mu.RUnlock()
	}
	if newCnt > 0 {
		logger.Info().Str("path", req.Path).Msg(fmt.Sprintf("Created %d new dir(s)", newCnt))
	}

	return cur, nil
}

// newDefaultAttr returns the default attr
// for a new node
// NOTE: Make sure to set the Mode field appropriately
func newDefaultAttr(ino uint64) *fuse.Attr {
	// TODO: Use config defaults
	now := time.Now()
	return &fuse.Attr{
		Ino:   ino,
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
