package webfs

import (
	"context"
	"io"
)

// NodeRequestor is an interface implemented by all node request types
type NodeRequestor interface {
	GetType() NodeCreateRequestType // TODO: shouldn't need this since can just reflect
	GetPath() string
	GetAttr() *AttrCreateRequest
}

// FileSourcer is an interface implemented by all file source request types
type FileSourcer interface {
	GetSourceRequest() *SourceCreateRequest
	GetType() SourceType
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
