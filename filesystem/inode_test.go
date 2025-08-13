package filesystem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/brettbedarf/webfs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/mock"
)

// Mock adapter for testing using testify
type mockAdapter struct {
	mock.Mock
}

func (m *mockAdapter) Read(ctx context.Context, offset int64, size int64, buf []byte) (int, error) {
	args := m.Called(ctx, offset, size, buf)
	// Handle function return types from testify
	if fn, ok := args.Get(0).(func(context.Context, int64, int64, []byte) int); ok {
		return fn(ctx, offset, size, buf), args.Error(1)
	}
	if args.Get(0) == nil {
		return 0, args.Error(1)
	}
	return args.Get(0).(int), args.Error(1)
}

func (m *mockAdapter) Open(ctx context.Context) (io.ReadCloser, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *mockAdapter) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	args := m.Called(ctx, offset, p)
	if args.Get(0) == nil {
		return 0, args.Error(1)
	}
	return args.Get(0).(int), args.Error(1)
}

func (m *mockAdapter) GetMeta(ctx context.Context) (*webfs.FileMetadata, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*webfs.FileMetadata), args.Error(1)
}

// Test helper to create a test inode
func createTestInode(adapters []webfs.FileAdapter) *Inode {
	attr := &fuse.Attr{
		Ino:  1,
		Size: 1024,
		Mode: 0o644,
	}
	return NewInode(attr, adapters)
}

// Helper to create mock adapter that returns data successfully
func createSuccessfulMockAdapter(data []byte) *mockAdapter {
	mockAdapter := &mockAdapter{}
	// Use a more complex mock that calculates the correct return value
	mockAdapter.On("Read", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("int64"), mock.AnythingOfType("[]uint8")).Return(
		func(ctx context.Context, offset int64, size int64, buf []byte) int {
			if offset >= int64(len(data)) {
				return 0
			}
			end := min(offset+size, int64(len(data)))
			bytesToCopy := end - offset
			copy(buf, data[offset:end])
			return int(bytesToCopy)
		},
		nil,
	).Maybe()

	mockAdapter.On("GetMeta", mock.Anything).Return(
		&webfs.FileMetadata{
			Size:         uint64(len(data)),
			LastModified: func() *time.Time { t := time.Now(); return &t }(),
		},
		nil,
	).Maybe()
	mockAdapter.On("Open", mock.Anything).Return(
		io.NopCloser(bytes.NewReader(data)),
		nil,
	).Maybe()
	return mockAdapter
}

// Helper to create mock adapter that fails
func createFailingMockAdapter() *mockAdapter {
	mockAdapter := &mockAdapter{}
	mockAdapter.On("Read", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("int64"), mock.AnythingOfType("[]uint8")).Return(
		0, fmt.Errorf("mock adapter failure")).Maybe()
	mockAdapter.On("GetMeta", mock.Anything).Return(
		(*webfs.FileMetadata)(nil), fmt.Errorf("mock adapter failure")).Maybe()
	mockAdapter.On("Open", mock.Anything).Return(
		(io.ReadCloser)(nil), fmt.Errorf("mock adapter failure")).Maybe()
	return mockAdapter
}

func TestInode_BasicRead(t *testing.T) {
	testData := []byte("hello world")
	adapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	data, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Read should succeed: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("Read data %q doesn't match expected %q", data, testData)
	}

	adapter.AssertNumberOfCalls(t, "Read", 1)
}

func TestInode_ReadCaching(t *testing.T) {
	testData := []byte("cached content test")
	adapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// First read should hit adapter
	data1, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("First read should succeed: %v", err)
	}

	adapter.AssertNumberOfCalls(t, "Read", 1)

	// Verify data is cached
	if !inode.IsCached(0, int64(len(testData))) {
		t.Error("Data should be cached after first read")
	}

	// Second read should hit cache (no additional adapter calls)
	data2, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Second read should succeed: %v", err)
	}

	// Should still be only 1 call since second read hits cache
	adapter.AssertNumberOfCalls(t, "Read", 1)

	if string(data1) != string(data2) {
		t.Error("Cached data should match original data")
	}
}

func TestInode_PartialRead(t *testing.T) {
	testData := []byte("hello world from cache")
	adapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// Read full data to populate cache
	_, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Initial read should succeed: %v", err)
	}

	// Partial read should hit cache
	partial, err := inode.Read(ctx, 6, 5) // Read "world"
	if err != nil {
		t.Fatalf("Partial read should succeed: %v", err)
	}

	expected := "world"
	if string(partial) != expected {
		t.Errorf("Partial read got %q, expected %q", partial, expected)
	}

	// Should still be only 1 call since partial read hits cache
	adapter.AssertNumberOfCalls(t, "Read", 1)

	// Verify cache reports the range as cached
	if !inode.IsCached(6, 5) {
		t.Error("Partial range should be reported as cached")
	}
}

func TestInode_ReadBeyondCachedRange(t *testing.T) {
	testData := []byte("short")
	adapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// Read and cache small portion
	_, err := inode.Read(ctx, 0, 3) // Cache "sho"
	if err != nil {
		t.Fatalf("Initial read should succeed: %v", err)
	}

	// Verify partial data is cached
	if !inode.IsCached(0, 3) {
		t.Error("Initial range should be cached")
	}

	// Read beyond cached range should hit adapter again
	_, err = inode.Read(ctx, 0, int64(len(testData))) // Read full "short"
	if err != nil {
		t.Fatalf("Extended read should succeed: %v", err)
	}

	// Should now have 2 calls - initial partial read and extended read
	adapter.AssertNumberOfCalls(t, "Read", 2)
}

func TestInode_ClearCache(t *testing.T) {
	testData := []byte("cache clear test")
	adapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// Read to populate cache
	_, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Read should succeed: %v", err)
	}

	// Verify cache is populated
	if !inode.IsCached(0, int64(len(testData))) {
		t.Error("Data should be cached after read")
	}

	// Clear cache
	inode.ClearCache()

	// Verify cache is empty
	if inode.IsCached(0, int64(len(testData))) {
		t.Error("Data should not be cached after clear")
	}

	// Next read should hit adapter again
	_, err = inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Read after cache clear should succeed: %v", err)
	}

	// Should now have 2 calls - initial read and read after cache clear
	adapter.AssertNumberOfCalls(t, "Read", 2)
}

func TestInode_AdapterFailover(t *testing.T) {
	testData := []byte("backup content")
	primaryAdapter := createFailingMockAdapter()
	backupAdapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{primaryAdapter, backupAdapter})
	ctx := context.Background()

	// Read should succeed using backup adapter
	data, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Read should succeed with backup adapter: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("Read data %q doesn't match expected %q", data, testData)
	}

	// Verify both adapters were tried
	primaryAdapter.AssertNumberOfCalls(t, "Read", 1)
	backupAdapter.AssertNumberOfCalls(t, "Read", 1)
}

func TestInode_AllAdaptersFail(t *testing.T) {
	primaryAdapter := createFailingMockAdapter()
	backupAdapter := createFailingMockAdapter()
	inode := createTestInode([]webfs.FileAdapter{primaryAdapter, backupAdapter})
	ctx := context.Background()

	// Read should fail when all adapters fail
	_, err := inode.Read(ctx, 0, 10)
	if err == nil {
		t.Error("Read should fail when all adapters fail")
	}

	// Verify both adapters were tried
	primaryAdapter.AssertNumberOfCalls(t, "Read", 1)
	backupAdapter.AssertNumberOfCalls(t, "Read", 1)
}

func TestInode_NoAdapters(t *testing.T) {
	inode := createTestInode(nil) // No adapters
	ctx := context.Background()

	// Read should fail with no adapters
	_, err := inode.Read(ctx, 0, 10)
	if err == nil {
		t.Error("Read should fail when no adapters are available")
	}

	if !errors.Is(err, ErrNoAdapters) {
		t.Errorf("expected ErrNoAdapters, got %v", err)
	}
}

func TestInode_ConcurrentReads(t *testing.T) {
	testData := []byte("concurrent read test data")
	adapter := &mockAdapter{}

	// Set up mock with delay to simulate real network latency
	adapter.On("Read", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("int64"), mock.AnythingOfType("[]uint8")).Return(
		func(ctx context.Context, offset int64, size int64, buf []byte) int {
			// Add slight delay to simulate network
			time.Sleep(10 * time.Millisecond)
			if offset >= int64(len(testData)) {
				return 0
			}
			end := min(offset+size, int64(len(testData)))
			bytesToCopy := end - offset
			copy(buf, testData[offset:end])
			return int(bytesToCopy)
		},
		nil,
	).Maybe()
	adapter.On("GetMeta", mock.Anything).Return(
		&webfs.FileMetadata{
			Size:         uint64(len(testData)),
			LastModified: func() *time.Time { t := time.Now(); return &t }(),
		},
		nil,
	).Maybe()

	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// Perform concurrent reads
	done := make(chan bool, 3)
	for range 3 {
		go func() {
			defer func() { done <- true }()
			data, err := inode.Read(ctx, 0, int64(len(testData)))
			if err != nil {
				t.Errorf("Concurrent read should succeed: %v", err)
				return
			}
			if string(data) != string(testData) {
				t.Errorf("Concurrent read data mismatch")
			}
		}()
	}

	// Wait for all goroutines
	for range 3 {
		<-done
	}

	// After first read caches, subsequent reads should be fast
	// Exact call count depends on timing, but should be reasonable (at most 3)
	readCalls := 0
	for _, call := range adapter.Calls {
		if call.Method == "Read" {
			readCalls++
		}
	}
	if readCalls > 3 {
		t.Errorf("Too many adapter calls for concurrent reads: %d", readCalls)
	}
}

func TestInode_HardLinkManagement(t *testing.T) {
	attr := &fuse.Attr{
		Ino:   1,
		Size:  100,
		Mode:  0o644,
		Nlink: 1,
	}
	inode := NewInode(attr, nil)

	// Check initial state through public interface
	initialAttr := inode.CopyAttr()
	if initialAttr.Nlink != 1 {
		t.Error("New inode should have Nlink=1")
	}

	// Add hard link through public interface
	dummyNode := &Node{name: "test"}
	inode.AddHardLink(dummyNode)

	// Verify change through public interface
	updatedAttr := inode.CopyAttr()
	if updatedAttr.Nlink != 2 {
		t.Errorf("After adding hard link, Nlink should be 2, got %d", updatedAttr.Nlink)
	}
}

func TestInode_AttrCopy(t *testing.T) {
	originalAttr := &fuse.Attr{
		Ino:  123,
		Size: 456,
		Mode: 0o755,
	}
	inode := NewInode(originalAttr, nil)

	// Test that CopyAttr returns a copy, not the original
	copied := inode.CopyAttr()

	if copied.Ino != originalAttr.Ino {
		t.Error("Copied attr should have same Ino")
	}

	if copied.Size != originalAttr.Size {
		t.Error("Copied attr should have same Size")
	}

	// Modify the copy and ensure original is unchanged
	copied.Size = 999

	if inode.CopyAttr().Size == 999 {
		t.Error("Modifying copied attr should not affect original")
	}
}

func TestInode_ReadAtEOF(t *testing.T) {
	testData := []byte("short")
	adapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// Read beyond end of file
	data, err := inode.Read(ctx, int64(len(testData)), 10)
	if err != nil {
		t.Fatalf("Read at EOF should not error: %v", err)
	}

	if len(data) != 0 {
		t.Errorf("Read at EOF should return empty data, got %d bytes", len(data))
	}
}

func TestInode_ReadPartialAtEnd(t *testing.T) {
	testData := []byte("hello")
	adapter := createSuccessfulMockAdapter(testData)
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// Read that extends beyond file end
	data, err := inode.Read(ctx, 3, 10) // Should get "lo" only
	if err != nil {
		t.Fatalf("Partial read at end should succeed: %v", err)
	}

	expected := "lo"
	if string(data) != expected {
		t.Errorf("Partial read got %q, expected %q", data, expected)
	}
}
