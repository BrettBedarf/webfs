// Package webfs contains core domain types and interfaces for the WebFS filesystem
package webfs

import (
	"context"
	"io"
	"time"
)

// FileAdapter defines the core operations for retrieving file data from various sources.
// Instances are 1:1 with the underlying filesystem Node and as such only responsible for
// managing data from a single file
type FileAdapter interface {
	// TODO: Adapters should define their capabilities

	// Opens the file and returns a Reader
	Open(ctx context.Context) (io.ReadCloser, error)

	// Reads up to len(buf) bytes into buf starting at offset. Returns number of bytes read and any error
	Read(ctx context.Context, offset int64, size int64, buf []byte) (int, error)

	// Writes len(p) bytes from p to the file starting at offset
	// Returns number of bytes written and any error
	Write(ctx context.Context, offset int64, p []byte) (int, error)

	GetMeta(ctx context.Context) (*FileMetadata, error)

	// TODO: Close()/Cleanup()/Down() etc to handle cleanup if applicable
	// so core fs can manage individual adapter instance lifecycles as resources
}

// AdapterProvider is a factory for concrete [FileAdapter] implementations
// generated from request's SourceConfig.
// Implementations should handle resource management (connection pooling etc) for its adapters
type AdapterProvider interface {
	Adapter(raw []byte) (FileAdapter, error)
}

// FileSource is a container for concrete adapter implementations that can be
// passed to the core filesystem
type FileSource struct {
	Provider AdapterProvider
	Config   []byte // Raw JSON config
	Priority int    `json:"priority,omitempty"` // Lower number = higher priority
}

// Adapter creates a FileAdapter from this source's configuration
func (fs *FileSource) Adapter() (FileAdapter, error) {
	return fs.Provider.Adapter(fs.Config)
}

// FileMetadata contains standardized metadata across all adapter types
type FileMetadata struct {
	Size         uint64
	LastModified *time.Time
	Version      string // Generic version identifier
	// Cache TTL: nil = default cache policy, 0 = no cache, >0 = cache for duration.
	// TTL returned from adapters should be considered "best effort"/hint
	TTL *time.Duration
}
