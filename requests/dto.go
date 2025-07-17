package requests

import (
	"time"

	"github.com/brettbedarf/webfs"
)

// NodeRequestDTO is the JSON representation of [webfs.NodeRequest]
type NodeRequestDTO struct {
	Path     string                      `json:"path"`
	Type     webfs.NodeCreateRequestType `json:"type"`
	UUID     *string                     `json:"uuid,omitempty"`  // Optional UUID to enable linking at request time
	Size     *uint64                     `json:"size,omitempty"`  // Optional size in bytes if known
	Atime    *time.Time                  `json:"atime,omitempty"` // Last Accessed at (Default current time)
	Mtime    *time.Time                  `json:"mtime,omitempty"` // Last Modified at (Default current time)
	Ctime    *time.Time                  `json:"ctime,omitempty"` // Created at (Default current time)
	Perms    *uint32                     `json:"perms,omitempty"` // i.e. 0755
	OwnerUID *uint32                     `json:"owner_uid,omitempty"`
	OwnerGID *uint32                     `json:"owner_gid,omitempty"`
	// Blksize is the preferred size for file system operations.
	Blksize *uint32 `json:"blksize,omitempty"`
}

// FileRequestDTO is the JSON representation of [api.FileCreateRequest]
type FileRequestDTO struct {
	NodeRequestDTO
	Sources []SourceConfigDTO `json:"sources"`
}

type DirRequestDTO struct {
	NodeRequestDTO
}

// SourceConfigDTO is the JSON representation of static [api.SourceConfig] fields
//
// Additional fields depend on the "type" value:
//
// Ex. For type="http" (see [adapters.HTTPSourceConfig]):
//
//	URL          string            `json:"url"`
//	Headers      map\[string\]string `json:"headers,omitempty"`
//	Timeout      *int              `json:"timeout,omitempty"`
//	MaxRedirects *int              `json:"maxRedirects,omitempty"`
//	...
//
// See adapters package for built-ins complete field specifications.
type SourceConfigDTO struct {
	Type     string `json:"type"`
	Priority *int   `json:"priority,omitempty"` // Lower number = higher priority, defaults to array index
}
