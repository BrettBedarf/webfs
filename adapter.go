// Package webfs contains core domain types and interfaces for the WebFS filesystem
package webfs

import (
	"context"
	"io"
)

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

	// TODO: Close()/Cleanup()/Down() etc to handle cleanup if applicable
	// so core fs can manage individual adapter instance lifecycles as resources
}

// AdapterProvider wraps the concrete adapter generated from request's SourceConfig
type AdapterProvider interface {
	Adapter() FileAdapter
}

// FileSource is a container for concrete adapter implementations that can be
// passed to the core filesystem
type FileSource struct {
	AdapterProvider
	Priority int `json:"priority,omitempty"` // Lower number = higher priority
}