package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/brettbedarf/httpfs/internal/filesystem"
	"github.com/brettbedarf/httpfs/util"
	. "github.com/hanwen/go-fuse/v2/fuse"
)

// HttpFS implements the FUSE filesystem
type HttpFS struct {
	RawFileSystem
	store *filesystem.
}

func NewHttpFS() *HttpFS {
	store := filesystem.NewFileTree()

	return &HttpFS{
		NewDefaultRawFileSystem(),
		store,
	}
}

// TODO: see if the higher-lvl v2/fs lib has default method implementations for boilerplate
// methods

func (fs *HttpFS) String() string {
	return "httpfs"
}

// Called when the kernel wants to know if the user has permission to access the node. See [libfuse docs].
// [libfuse docs]: https://libgithub.io/doxygen/structfuse__operations.html#a4dd366b9f74ead6927fb75afb91863bc
func (fs *HttpFS) Access(cancel <-chan struct{}, input *AccessIn) Status {
	logger := util.GetLogger("access")
	logger.Debug().
		Interface("input", input).
		Msg("Access called")

	// TODO:handle access permissions properly
	//
	// For simplicity, we allow read access to all files
	return OK
}

func Lookup(cancel <-chan struct{}, header *InHeader, name string, out *EntryOut) (status Status) {
	return OK
}

//	func (fs *HttpFS) ReadDir(cancel <-chan struct{}, input *ReadIn, out *DirEntryList) Status {
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
func main() {
	// Parse command line arguments
	// configPath := flag.String("config", "", "Path to config file")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()
	mountPoint := flag.Arg(0)

	// Initialize logger
	logLevel := util.InfoLevel
	if *debug {
		logLevel = util.DebugLevel
	}
	util.InitializeLogger(logLevel)
	logger := util.GetLogger("main")

	// Check if mount point is provided
	if mountPoint == "" {
		logger.Fatal().Msg("Mount point not specified; it must be passed as the argument")
	}

	// Create filesystem
	fs := NewHttpFS()
	opts := &MountOptions{
		FsName: "httpfs",
		Name:   "httpfs",
		Debug:  *debug,
		Logger: log.Default(),
	}

	// Create server
	server, err := NewServer(fs, mountPoint, opts)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create FUSE server")
	}

	// Setup signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in the background
	go server.Serve()

	// Wait until the filesystem is mounted
	if err := server.WaitMount(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to mount filesystem")
	}

	logger.Info().Str("mountpoint", mountPoint).Msg("Filesystem mounted successfully")

	// Wait for termination signal
	sig := <-signalChan
	logger.Info().Str("signal", sig.String()).Msg("Received signal, unmounting filesystem")

	// Unmount the filesystem
	if err := server.Unmount(); err != nil {
		logger.Error().Err(err).Msg("Failed to unmount filesystem")
	}

	logger.Info().Msg("Filesystem unmounted successfully")
}
