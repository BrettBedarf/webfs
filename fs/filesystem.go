package fs

import (
	"context"
	"io"
	"time"
)

type Filesystem interface {
	AddFileNode(req *FileCreateRequest) error
}

// FileAdapter defines the core operations for retrieving file data from various sources
type FileAdapter interface {
	// Opens the file and returns a Reader
	Open(ctx context.Context) (io.ReadCloser, error)

	// Reads up to len(p) bytes into p starting at offset
	// Returns number of bytes read and any error
	Read(ctx context.Context, offset int64, p []byte) (int, error)

	// Writes len(p) bytes from p to the file starting at offset
	// Returns number of bytes written and any error
	Write(ctx context.Context, offset int64, p []byte) (int, error)

	// Returns the size of the file
	Size(ctx context.Context) (int64, error)

	// Checks if the file exists
	Exists(ctx context.Context) (bool, error)
}

// NodeRequest are common fields embedded in concrete request types
type NodeRequest struct {
	Path     string
	Type     NodeCreateRequestType
	UUID     string // Optional UUID to enable linking at request time
	Size     uint64
	Atime    time.Time // Last Accessed at
	Mtime    time.Time // Last Modified at
	Ctime    time.Time // Created at (Default current time)
	Perms    uint32    // i.e. 0755
	OwnerUid uint32
	OwnerGid uint32
	// Blksize is the preferred size for file system operations.
	Blksize uint32
}

// Valid types are FileNodeType, DirNodeType
type NodeCreateRequestType string

const (
	FileNodeType NodeCreateRequestType = "file"
	DirNodeType  NodeCreateRequestType = "dir"
	// TODO: HardlinkNodeType, SymlinkNodeType
	// HardlinkNodeType NodeCreateRequestType = "hardlink"
	// SymlinkNodeType  NodeCreateRequestType = "symlink"
)

type FileCreateRequest struct {
	NodeRequest
	Sources []SourceConfig `json:"sources"`
}

type DirCreateRequest struct {
	NodeRequest
}

// Implement NodeRequestor interface for DirCreateRequest
func (r *DirCreateRequest) GetNodeRequest() *NodeRequest {
	return &r.NodeRequest
}

// AdapterProvider wraps the concrete adapter generated from request's SourceConfig
type AdapterProvider interface {
	Adapter() FileAdapter
}

type SourceConfig struct {
	AdapterProvider
	Priority int `json:"priority,omitempty"` // Lower number = higher priority
}
