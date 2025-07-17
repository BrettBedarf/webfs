package webfs

import "time"

// NodeRequest has common fields embedded in concrete request types
type NodeRequest struct {
	Path     string
	Type     NodeCreateRequestType
	UUID     string // Optional UUID to enable linking at request time
	Size     uint64
	Atime    time.Time // Last Accessed at
	Mtime    time.Time // Last Modified at
	Ctime    time.Time // Created at (Default current time)
	Perms    uint32    // i.e. 0755
	OwnerUID uint32
	OwnerGID uint32
	// Blksize is the preferred size for file system operations.
	Blksize uint32
}

// NodeCreateRequestType valid types are FileNodeType "file", DirNodeType "dir"
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
	Sources []FileSource `json:"sources"`
}

type DirCreateRequest struct {
	NodeRequest
}
