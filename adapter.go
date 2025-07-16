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

	GetMeta(ctx context.Context) (*FileMetadata, error)

	// TODO: Close()/Cleanup()/Down() etc to handle cleanup if applicable
	// so core fs can manage individual adapter instance lifecycles as resources
}

// AdapterProvider is a factory for concrete [FileAdapter] implementations
// generated from request's SourceConfig.
// Implementations should handle resource management (connection pooling etc) for its adapters
type AdapterProvider interface {
	Adapter() FileAdapter
}

// FileSource is a container for concrete adapter implementations that can be
// passed to the core filesystem
type FileSource struct {
	AdapterProvider
	Priority int `json:"priority,omitempty"` // Lower number = higher priority
}

// FileMetadata contains standardized metadata across all adapter types
type FileMetadata struct {
	Size         int64
	LastModified *time.Time
	Version      string         // Generic version identifier
	TTL          *time.Duration // Cache TTL: nil = default cache policy, 0 = no cache, >0 = cache for duration
}
