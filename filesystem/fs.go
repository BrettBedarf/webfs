package filesystem

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/internal/util"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/puzpuzpuz/xsync/v4"
)

// TODO: split up interfaces needed for FUSE bridge and core pub interface

// type FileSystemOperator interface {
// 	Root() *Node
// 	AddFileNode(req *webfs.FileCreateRequest) (*Node, error)
// 	AddDirNode(req *webfs.DirCreateRequest) (*Node, error)
// }

type FileSystem struct {
	cfg          *config.Config
	root         *Node                     // Root of node tree
	lastIno      atomic.Uint64             // Last fuse Attr.Ino assigned; incremented when new nodes are created
	lastNodeID   atomic.Uint64             // Last registry NodeID assigned; assigned on-demand for session only
	nodeRegistry *xsync.Map[uint64, *Node] // maps registry NodeIDs to core Nodes
}

func NewFS(cfg *config.Config) *FileSystem {
	rootAttr := newDefaultAttr(fuse.FUSE_ROOT_ID)
	rootAttr.Mode = uint32(syscall.S_IFDIR | 0o555) // directory with r-xr-xr-x permissions

	rootInode := NewInode(rootAttr, nil)
	rootNode := NewNode("", rootInode) // TODO: should be "." ??
	rootNode.nodeID.Store(fuse.FUSE_ROOT_ID)

	// inodeMap := make(map[uint64]*Inode)
	inodeMap := xsync.NewMap[uint64, *Inode]()
	inodeMap.Store(fuse.FUSE_ROOT_ID, rootInode)

	fs := FileSystem{cfg: cfg, root: rootNode}
	// initialize attribute inode and registry counters
	fs.lastIno.Store(fuse.FUSE_ROOT_ID)
	fs.lastNodeID.Store(fuse.FUSE_ROOT_ID)
	// setup registry map
	fs.nodeRegistry = xsync.NewMap[uint64, *Node]()
	fs.nodeRegistry.Store(fuse.FUSE_ROOT_ID, rootNode)
	return &fs
}

func (fs *FileSystem) RootCtx() *NodeContext {
	return NewNodeContext(fs.root)
}

// AddFileNode adds a new file node to the filesystem. It will add any missing
// directories in the path and return the newly created leaf node
// If a node already exists at the requested path, it will return an error
func (fs *FileSystem) AddFileNode(req *webfs.FileCreateRequest) (*Node, error) {
	logger := util.GetLogger("AddFileNode")

	parent := fs.root
	// TODO: Path edge cases
	dirPath, name := path.Split(req.Path) // dir is full path to dir, file is just the filename
	if dirPath != "" {
		// Implicit dir requests are just the same embedded Node values with a different path
		dirReq := webfs.DirCreateRequest{NodeRequest: req.NodeRequest}
		dirReq.Path = dirPath
		dNode, err := fs.AddDirNode(&dirReq)
		if err != nil {
			logger.Error().Err(err).Str("path", dirReq.Path).Msg("Failed to create file's ancestor directory(s)")
			return nil, err
		}
		parent = dNode
	}

	if _, ok := parent.GetChild(name); ok {
		err := fmt.Errorf("file already exists at path %s", req.Path)
		logger.Error().Err(err).Str("path", req.Path).Msg("Failed to create file")
		return nil, err
	}

	attr := newDefaultAttr(fs.lastIno.Add(1))
	attr.Mode = uint32(FileAttr) | req.Perms

	adapters := make([]webfs.FileAdapter, 0, len(req.Sources))
	for _, source := range req.Sources {
		if a, err := source.Adapter(); err != nil {
			logger.Error().Err(err).Interface("source", source).Interface("path", req.Path).Msg("Failed to create adapter")
			continue
		} else {
			adapters = append(adapters, a)
		}
	}
	if len(adapters) == 0 {
		logger.Error().Str("path", req.Path).Msg("No valid adapters")
		return nil, fmt.Errorf("no valid adapters")
	}

	inode := NewInode(attr, adapters)
	inode.RefreshMeta()
	node := NewNode(name, inode)
	parent.AddChild(node)
	logger.Debug().Str("path", req.Path).Msg("Added new file node")
	// Refresh metadata
	return node, nil
}

// AddDirNode recursively adds all missing directories starting at [root]
// in the request's path and returns the leaf.
// It is equivalent to calling `mkdir -p` from a shell and similarly will only create
// directories that do not already exist and will not error if the leaf already exists.
func (fs *FileSystem) AddDirNode(req *webfs.DirCreateRequest) (*Node, error) {
	logger := util.GetLogger("AddDirNode")

	dSplit := strings.Split(req.Path, "/")
	cur := fs.root
	newCnt := 0
	// Traverse the path until we get to existing dir and make
	// any missing along the way
	for _, name := range dSplit {
		if child, ok := cur.children.Load(name); ok {
			cur = child
		} else {
			// Make new dir
			attr := newDefaultAttr(fs.lastIno.Add(1))
			attr.Mode = uint32(DirAttr) | req.Perms
			inode := NewInode(attr, nil)
			node := NewNode(name, inode)

			cur.AddChild(node)
			newCnt++
			cur = node
		}
	}
	if newCnt > 0 {
		logger.Info().Str("path", req.Path).Msg(fmt.Sprintf("Created %d new dir(s)", newCnt))
	}

	return cur, nil
}

/* [NodeIDManager] interface implementations */

// GetNodeCtx returns a locked NodeContext with its Close() wired up
// If the node does not exist, returns nil
func (fs *FileSystem) GetNodeCtx(nodeID uint64) (ctx *NodeContext) {
	logger := util.GetLogger("GetNodeCtx")
	logger.Trace().Uint64("nodeID", nodeID).Msg("GetNodeCtx called")

	if node, ok := fs.nodeRegistry.Load(nodeID); ok {
		node.mu.RLock()
		ctx = &NodeContext{node: node}
		ctx.AddClose(node.mu.RUnlock)
		return ctx
	}
	logger.Debug().Uint64("nodeID", nodeID).Msg("No node found")
	return
}

// ForgetNodeID removes the registry NodeID entry
func (fs *FileSystem) ForgetNodeID(id uint64) {
	logger := util.GetLogger("FS.ForgetNodeID")
	logger.Trace().Uint64("id", id).Msg("ForgetNodeID called")

	node := fs.GetNodeCtx(id)
	defer node.Close()
	if node == nil {
		logger.Debug().Uint64("id", id).Msg("No node found")
		return
	}
	fs.nodeRegistry.Delete(id)
}

// GetChildCtx finds a child by name and returns a locked NodeContext
// with its Close() wired up.
// A NodeID will be allocated for the child if it does not already exist.
// If the parent or child do not exist, returns nil
//
// Caller is responsible for closing the context when done `defer ctx.Close()`.
func (fs *FileSystem) GetChildCtx(parentID uint64, name string) (ctx *NodeContext) {
	logger := util.GetLogger("FS.GetChildCtx")
	logger.Trace().Uint64("parentID", parentID).Str("name", name).Msg("GetChildCtx called")

	parent, ok := fs.nodeRegistry.Load(parentID)
	if !ok {
		logger.Debug().Uint64("parentID", parentID).Str("name", name).Msg("No parent found")
		return
	}
	if child, ok := parent.GetChild(name); ok { // parent lock immediately releases
		fs.EnsureNodeID(child)
		return NewNodeContext(child)
	}
	return
}

// EnsureNodeID retrieves or allocates & sets NodeID; safe with or without held locks.
// returns NodeID
func (fs *FileSystem) EnsureNodeID(n *Node) uint64 {
	// fast path
	if id := n.nodeID.Load(); id != 0 {
		return id
	}
	// allocate a new one
	newID := fs.lastNodeID.Add(1)
	// only one CAS will succeed
	if n.nodeID.CompareAndSwap(0, newID) {
		fs.nodeRegistry.Store(newID, n)
		return newID
	}
	// someone else won the race, load the real value
	return n.nodeID.Load()
}

// newDefaultAttr returns the default attributes for a new node
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
