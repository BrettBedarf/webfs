package fuse

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/brettbedarf/webfs/filesystem"
	"github.com/brettbedarf/webfs/internal/util"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/puzpuzpuz/xsync/v4"
)

// ServerBridger handles mapping between runtime NodeIDs and core Nodes.
// Context-based methods are thread-safe wrappers whose ctx.Close()
// must be called when finished to unlock all associated locks
type ServerBridger interface {
	GetNodeCtx(nodeID uint64) (ctx *filesystem.NodeContext)
	// See [webfs.FileSystem.LookupChildCtx]
	GetChildCtx(parentID uint64, name string) (ctx *filesystem.NodeContext)
	ForgetNodeID(id uint64)
	EnsureNodeID(node *filesystem.Node) uint64
	Read(ctx context.Context, nodeID uint64, offset int64, size int64) ([]byte, error)
}

// TODO: these are in cfg but need to define and pass fuse cfg
const (
	defaultAttrTimeout  = time.Duration(1) * time.Second  // 1 second
	defaultEntryTimeout = time.Duration(60) * time.Second // 60 seconds
)

// FuseRaw implements the low-level FUSE wire protocol
// It serves as protocol adapter between the FUSE and core filesystem
// See https://www.man7.org/linux//man-pages/man4/fuse.4.html
type FuseRaw struct {
	fuse.RawFileSystem
	fs         ServerBridger
	nextDirFh  atomic.Uint64                          // directory file handle counter
	openDirs   *xsync.Map[uint64, []*filesystem.Node] // map of open dir Fhs stable-order child slices TODO: rethink if we can avoid storing underlying refs
	nextFileFh atomic.Uint64                          // file handle counter
	openFiles  *xsync.Map[uint64, uint64]             // map of file Fhs to NodeIDs
	server     *fuse.Server
}

func NewFuseRaw(fs *filesystem.FileSystem) *FuseRaw {
	r := FuseRaw{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
		fs:            fs,
		openDirs:      xsync.NewMap[uint64, []*filesystem.Node](),
		openFiles:     xsync.NewMap[uint64, uint64](),
	}
	r.nextDirFh.Store(1)
	r.nextFileFh.Store(1)
	return &r
}

func (r *FuseRaw) Init(s *fuse.Server) {
	logger := util.GetLogger("Fuse.Init")
	logger.Debug().Interface("DebugData", s).Msg("FUSE initialized")
	r.server = s
}

func (r *FuseRaw) OnUnmount() {
	logger := util.GetLogger("Fuse.OnUnmount")
	logger.Info().Msg("FUSE unmounted")
}

func (r *FuseRaw) String() string {
	return "FuseRaw"
}

// Access is called when the kernel wants to know if the user has permission to access the node.
// If the 'default_permissions' mount option is given, this method is not called.
func (r *FuseRaw) Access(cancel <-chan struct{}, input *fuse.AccessIn) fuse.Status {
	logger := util.GetLogger("Fuse.Access")
	logger.Debug().
		Interface("input", input).
		Msg("Access called")

	// TODO:handle access permissions properly
	//
	// For simplicity, we allow read access to all files
	return fuse.OK
}

// Lookup is called by the kernel when the VFS wants to know
// about a file inside a directory. Many lookup calls can
// occur in parallel, but only one call happens for each (dir,
// name) pair.
func (r *FuseRaw) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	logger := util.GetLogger("Fuse.Lookup")
	logger.Debug().Interface("header", header).Str("name", name).Msg("Lookup called")

	ctx := r.fs.GetChildCtx(header.NodeId, name)
	defer ctx.Close()
	if ctx == nil {
		logger.Debug().Str("name", name).Msg("Lookup: no child found")
		return fuse.ENOENT
	}
	out.NodeId = ctx.NodeID()
	out.Attr = ctx.Attr()
	out.SetAttrTimeout(defaultAttrTimeout)
	out.SetEntryTimeout(defaultEntryTimeout)
	return fuse.OK
}

// Forget is called when the kernel discards entries from its
// dentry cache. This happens on unmount, and when the kernel
// is short on memory. Since it is not guaranteed to occur at
// any moment, and since there is no return value, Forget
// should not do I/O, as there is no channel to report back
// I/O errors.
func (r *FuseRaw) Forget(nodeid, nlookup uint64) {
	logger := util.GetLogger("Fuse.Forget")
	logger.Trace().Uint64("nodeid", nodeid).Uint64("nlookup", nlookup).Msg("Forget called")

	r.fs.ForgetNodeID(nodeid)
	// TODO: May/may not need to clean up open Fhs (unlikely kernel would forget on any open handles)
}

func (r *FuseRaw) GetAttr(cancel <-chan struct{}, header *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	logger := util.GetLogger("Fuse.GetAttr")
	logger.Debug().Interface("header", header).Msg("GetAttr called")

	ctx := r.fs.GetNodeCtx(header.NodeId)
	defer ctx.Close()
	if ctx == nil {
		logger.Debug().Uint64("nodeid", header.NodeId).Msg("No node found")
		return fuse.ENOENT
	}
	out.Attr = ctx.Attr()
	out.SetTimeout(defaultAttrTimeout)
	return fuse.OK
}

func (r *FuseRaw) SetAttr(cancel <-chan struct{}, header *fuse.SetAttrIn, out *fuse.AttrOut) fuse.Status {
	logger := util.GetLogger("Fuse.SetAttr")
	logger.Debug().Interface("header", header).Msg("SetAttr called")
	return fuse.ENOSYS
}

// Directory Handlers

func (r *FuseRaw) OpenDir(cancel <-chan struct{}, in *fuse.OpenIn, out *fuse.OpenOut) (status fuse.Status) {
	logger := util.GetLogger("Fuse.OpenDir")
	logger.Trace().Interface("in", in).Msg("OpenDir called")

	node := r.fs.GetNodeCtx(in.NodeId)
	defer node.Close()
	if node == nil {
		logger.Debug().Uint64("nodeid", in.NodeId).Msg("No node found")
		return fuse.ENOENT
	}

	// Build a snapshot of directory entries
	children := node.UnsafeChildren()

	// Get a new file handle and store the snapshot
	fh := r.nextDirFh.Add(1)
	r.openDirs.Store(fh, children)

	out.Fh = fh
	return fuse.OK
}

func (r *FuseRaw) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	logger := util.GetLogger("Fuse.ReadDir")
	logger.Trace().Interface("input", input).Msg("ReadDirPlus called")

	parent := r.fs.GetNodeCtx(input.NodeId)
	defer parent.Close()
	if parent == nil {
		logger.Debug().Uint64("nodeid", input.NodeId).Msg("No parent found")
		return fuse.ENOENT
	}

	children, ok := r.openDirs.Load(input.Fh)
	if !ok {
		logger.Debug().Uint64("nodeid", input.NodeId).Uint64("fh", input.Fh).Msg("No open FileHandle found")
		return fuse.ENOENT
	}
	if input.Offset >= uint64(len(children)) {
		return fuse.OK
	}
	children = children[input.Offset:] // handle split up reads
	for i, ch := range children {
		if ch == nil {
			continue
		}
		// TODO: Can make whether to return deleted nodes/stale reads configurable
		if ch.IsDel() {
			children[i] = nil
			continue
		}
		attr := ch.CopyAttr()

		ok := out.AddDirEntry(fuse.DirEntry{
			Name: ch.Name(),
			Mode: attr.Mode,
			Ino:  attr.Ino,
			// Offset auto-incremented
		})
		if !ok {
			// buffer full; kernel will call again with offset
			break
		}

	}
	logger.Trace().Uint64("fh", input.Fh).Uint64("nodeid", input.NodeId).Interface("out", out).Msg("ReadDirPlus returned")
	return fuse.OK
}

func (r *FuseRaw) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	logger := util.GetLogger("Fuse.ReadDir")
	logger.Trace().Interface("input", input).Msg("ReadDirPlus called")

	parent := r.fs.GetNodeCtx(input.NodeId)
	defer parent.Close()
	if parent == nil {
		logger.Debug().Uint64("nodeid", input.NodeId).Msg("No parent found")
		return fuse.ENOENT
	}

	children, ok := r.openDirs.Load(input.Fh)
	if !ok {
		logger.Debug().Uint64("nodeid", input.NodeId).Uint64("fh", input.Fh).Msg("No open FileHandle found")
		return fuse.ENOENT
	}
	if input.Offset >= uint64(len(children)) {
		return fuse.OK
	}
	children = children[input.Offset:] // handle split up reads
	for i, ch := range children {
		if ch == nil {
			continue
		}
		// TODO: Can make whether to return deleted nodes/stale reads or not configurable
		if ch.IsDel() {
			children[i] = nil
			continue
		}
		attr := ch.CopyAttr()

		entOut := out.AddDirLookupEntry(fuse.DirEntry{
			Name: ch.Name(),
			Mode: attr.Mode,
			Ino:  attr.Ino,
			// Offset auto-incremented
		})
		if entOut == nil {
			// buffer full; kernel will call again with offset
			break
		}

		entOut.NodeId = r.fs.EnsureNodeID(ch)
		entOut.Generation = 0 // We aren't recycling NodeIDs with 64-bit counter
		entOut.Attr = attr
		entOut.SetEntryTimeout(defaultEntryTimeout)
		entOut.SetAttrTimeout(defaultAttrTimeout)

	}
	logger.Trace().Uint64("fh", input.Fh).Uint64("nodeid", input.NodeId).Interface("out", out).Msg("ReadDirPlus returned")
	return fuse.OK
}

// ReleaseDir is called when the dir Fh is closed
func (r *FuseRaw) ReleaseDir(input *fuse.ReleaseIn) {
	logger := util.GetLogger("Fuse.ReleaseDir")
	logger.Trace().Interface("input", input).Msg("ReleaseDir called")
	r.openDirs.Delete(input.Fh) // no panic if key doesn't exist
}

// File Handlers

// Open is called when a file is opened
func (r *FuseRaw) Open(cancel <-chan struct{}, in *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	logger := util.GetLogger("Fuse.Open")
	logger.Trace().Interface("in", in).Msg("Open called")

	// Verify node exists
	nodeCtx := r.fs.GetNodeCtx(in.NodeId)
	if nodeCtx == nil {
		logger.Debug().Uint64("nodeid", in.NodeId).Msg("No node found")
		return fuse.ENOENT
	}
	defer nodeCtx.Close()

	// TODO: Check if node is a file (not directory)
	// For now, assume it's a file

	// Allocate a new file handle
	fh := r.nextFileFh.Add(1)

	// Store the mapping from file handle to NodeID
	r.openFiles.Store(fh, in.NodeId)

	logger.Debug().Uint64("nodeid", in.NodeId).Uint64("fh", fh).Msg("File opened")

	out.Fh = fh
	// TODO: Set appropriate flags based on open mode
	// out.OpenFlags = fuse.FOPEN_KEEP_CACHE // Enable caching

	return fuse.OK
}

// Read is called when data is read from a file
func (r *FuseRaw) Read(cancel <-chan struct{}, in *fuse.ReadIn, buf []byte) (fuse.ReadResult, fuse.Status) {
	logger := util.GetLogger("Fuse.Read")
	logger.Trace().Interface("in", in).Int("buflen", len(buf)).Msg("Read called")

	// Get NodeID from file handle
	nodeID, ok := r.openFiles.Load(in.Fh)
	if !ok {
		logger.Debug().Uint64("fh", in.Fh).Msg("No file handle found")
		return nil, fuse.EBADF
	}

	// Create context for the read operation
	ctx := context.Background()

	// Read data from filesystem
	data, err := r.fs.Read(ctx, nodeID, int64(in.Offset), int64(in.Size))
	if err != nil {
		logger.Error().Err(err).Uint64("nodeID", nodeID).Uint64("offset", in.Offset).Uint32("size", in.Size).Msg("Read failed")
		return nil, fuse.EIO
	}

	logger.Trace().Uint64("nodeID", nodeID).Uint64("offset", in.Offset).Int("bytesRead", len(data)).Msg("Read completed")

	return fuse.ReadResultData(data), fuse.OK
}

// Release is called when a file handle is closed
func (r *FuseRaw) Release(cancel <-chan struct{}, in *fuse.ReleaseIn) {
	logger := util.GetLogger("Fuse.Release")
	logger.Trace().Interface("in", in).Msg("Release called")

	// Remove the file handle mapping
	r.openFiles.Delete(in.Fh)

	logger.Debug().Uint64("fh", in.Fh).Msg("File handle released")
}
