package mocks

import (
	"context"
	"io"

	"github.com/brettbedarf/webfs"
	"github.com/stretchr/testify/mock"
)

// MockFileAdapter implements webfs.FileAdapter for testing across packages
type MockFileAdapter struct {
	mock.Mock
}

func (m *MockFileAdapter) Open(ctx context.Context) (io.ReadCloser, error) {
	args := m.Called(ctx)

	// Handle function return types (for complex tests)
	if fn, ok := args.Get(0).(func(context.Context) io.ReadCloser); ok {
		return fn(ctx), args.Error(1)
	}

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockFileAdapter) Read(ctx context.Context, offset int64, size int64, buf []byte) (int, error) {
	args := m.Called(ctx, offset, size, buf)

	// Handle function return types (for complex inode tests)
	if fn, ok := args.Get(0).(func(context.Context, int64, int64, []byte) int); ok {
		return fn(ctx, offset, size, buf), args.Error(1)
	}

	// Handle nil returns
	if args.Get(0) == nil {
		return 0, args.Error(1)
	}

	// Handle simple int returns (for basic tests)
	return args.Get(0).(int), args.Error(1)
}

func (m *MockFileAdapter) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	args := m.Called(ctx, offset, p)

	// Handle function return types (for complex tests)
	if fn, ok := args.Get(0).(func(context.Context, int64, []byte) int); ok {
		return fn(ctx, offset, p), args.Error(1)
	}

	if args.Get(0) == nil {
		return 0, args.Error(1)
	}
	return args.Get(0).(int), args.Error(1)
}

func (m *MockFileAdapter) GetMeta(ctx context.Context) (*webfs.FileMetadata, error) {
	args := m.Called(ctx)

	// Handle function return types (for complex tests)
	if fn, ok := args.Get(0).(func(context.Context) *webfs.FileMetadata); ok {
		return fn(ctx), args.Error(1)
	}

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*webfs.FileMetadata), args.Error(1)
}

var _ webfs.FileAdapter = (*MockFileAdapter)(nil)

// MockAdapterProvider implements webfs.AdapterProvider for testing across packages
type MockAdapterProvider struct {
	mock.Mock
}

func (m *MockAdapterProvider) NewAdapter(raw []byte) (webfs.FileAdapter, error) {
	args := m.Called(raw)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(webfs.FileAdapter), args.Error(1)
}

var _ webfs.AdapterProvider = (*MockAdapterProvider)(nil)
