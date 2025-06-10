package core

import (
	"github.com/brettbedarf/webfs/util"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FuseRaw implements the FUSE wire protocol
type FuseRaw struct {
	fuse.RawFileSystem
	fs *WebFs
}

func NewFuseRaw(mountPath string, fs *WebFs) *FuseRaw {
	return nil
}

func (fr *FuseRaw) String() string {
	return "webfs"
}

// Called when the kernel wants to know if the user has permission to access the node. See [libfuse docs].
// [libfuse docs]: https://libgithub.io/doxygen/structfuse__operations.html#a4dd366b9f74ead6927fb75afb91863bc
func (fr *FuseRaw) Access(cancel <-chan struct{}, input *fuse.AccessIn) fuse.Status {
	logger := util.GetLogger("Fuse.Access")
	logger.Debug().
		Interface("input", input).
		Msg("Access called")

	// TODO:handle access permissions properly
	//
	// For simplicity, we allow read access to all files
	return fuse.OK
}

func (r *FuseRaw) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out fuse.EntryOut) (status fuse.Status) {
	return fuse.OK
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
