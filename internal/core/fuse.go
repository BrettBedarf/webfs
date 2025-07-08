package core

import (
	"time"

	"github.com/brettbedarf/webfs/util"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// NodeIDManager handles mapping between Fuse NodeIDs and core Nodes.
// Context-based methods are thread-safe wrappers whose ctx.Close()
// must be called when finished to unlock all associated locks
type NodeIDManager interface {
	// AllocateNodeID(node *Node) uint64
	GetNodeCtx(nodeID uint64) (ctx *NodeContext, ok bool)
	// See [FileSystem.LookupChildCtx]
	GetChildCtx(parentID uint64, name string) (ctx *NodeContext, ok bool)
	ForgetNodeID(id uint64)
}

// FileHandleManager is responsible for mapping between Fuse FileHandles and core Nodes
// TODO:
type FileHandleManager interface {
	// OpenHandle is called on Open(); Associates a new file handle
	// with a node
	OpenHandle(nodeID uint64) uint64
	LookupHandle(fh uint64) (*Node, error)
	// CloseHandle is called on release & cleanup
	CloseHandle(fh uint64)
}

const (
	defaultAttrTimeout  = time.Duration(1) * time.Second  // 1 second
	defaultEntryTimeout = time.Duration(60) * time.Second // 60 seconds
)

// FuseRaw implements the low-level FUSE wire protocol
// It serves as protocol adapter between the FUSE and core filesystem
// See https://www.man7.org/linux//man-pages/man4/fuse.4.html
type FuseRaw struct {
	fuse.RawFileSystem
	fs     NodeIDManager
	server *fuse.Server
}

func NewFuseRaw(fs FileSystemOperator) *FuseRaw {
	r := FuseRaw{
		RawFileSystem: fuse.NewDefaultRawFileSystem(),
		fs:            fs,
	}
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

	ctx, ok := r.fs.GetChildCtx(header.NodeId, name)
	defer ctx.Close()
	if !ok {
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
	logger.Debug().Uint64("nodeid", nodeid).Uint64("nlookup", nlookup).Msg("Forget called")

	r.fs.ForgetNodeID(nlookup)
}

func (r *FuseRaw) GetAttr(cancel <-chan struct{}, header *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	logger := util.GetLogger("Fuse.GetAttr")
	logger.Debug().Interface("header", header).Msg("GetAttr called")

	ctx, ok := r.fs.GetNodeCtx(header.NodeId)
	defer ctx.Close()
	if !ok {
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

func (r *FuseRaw) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	logger := util.GetLogger("Fuse.ReadDir")
	logger.Debug().Interface("input", input).Msg("ReadDir called")

	return fuse.ENOSYS // Not implemented yet
}

//	func (fr *FuseRaw) ReadDir(cancel <-chan struct{}, input *ReadIn, out *DirEntryList) Status {
//		logger := util.GetLogger("readdir")
//		logger.Debug().
//			Uint64("fh", input.Fh).
//			Int("offset", int(input.Offset)).
//			Msg("ReadDir called")
//
//		// Only allow reading the root directory for simplicity TODO:
//		if input.NodeId != fs.root.Ino {
//			logger.Error().
//				Uint64("inode", input.NodeId).
//				Msg("ReadDir: inode is not root")
//			return ENOTDIR
//		}
//
//		// Start at the provided offset
//		startIdx := int(input.Offset)
//
//		// First, add "." and ".." entries if we're at the beginning
//		if startIdx == 0 {
//			if !out.AddDirEntry(DirEntry{
//				Name: ".",
//				Mode: uint32(syscall.S_IFDIR | 0755),
//				Ino:  FUSE_ROOT_ID,
//			}) {
//				// Buffer is full
//				return OK
//			}
//			startIdx++
//		}
//
//		if startIdx == 1 {
//			if !out.AddDirEntry(DirEntry{
//				Name: "..",
//				Mode: uint32(syscall.S_IFDIR | 0755),
//				// TODO: can we link to the underlying fs parent?
//				Ino: FUSE_ROOT_ID, // Parent of root is root
//			}) {
//				// Buffer is full
//				return OK
//			}
//			startIdx++
//		}
//
//		// Now add file entries
//		filesLock.RLock()
//		defer filesLock.RUnlock()
//
//		idx := 2 // We've already processed . and ..
//		for filename := range sourceFiles {
//			// Skip entries before the requested offset
//			if idx < startIdx {
//				idx++
//				continue
//			}
//
//			// Get inode for this file
//			inode, exists := inodeMap[filename]
//			if !exists {
//				logger.Warn().
//					Str("filename", filename).
//					Msg("ReadDir: no inode found for file")
//				idx++
//				continue
//			}
//
//			// Get file attributes
//			// attr := fs.store.GetFileAttr(filename)
//
//			// Add this entry to the result
//			if !out.AddDirEntry(DirEntry{
//				Name: filename,
//				Mode: uint32(syscall.S_IFREG | 0444), // Regular file, read-only
//				Ino:  inode,
//			}) {
//				// The buffer is full, but we were able to add some entries.
//				// We'll return OK and let the kernel call us again with a new offset.
//				return OK
//			}
//
//			logger.Debug().
//				Str("filename", filename).
//				Uint64("inode", inode).
//				Msg("ReadDir: added entry")
//
//			idx++
//		}
//
//			return OK
//		}
//
//				return OK
//			}
