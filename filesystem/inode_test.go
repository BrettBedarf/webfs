package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brettbedarf/webfs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Mock adapter for testing
type mockAdapter struct {
	data       []byte
	shouldFail bool
	readDelay  time.Duration
	callCount  int32
}

func (m *mockAdapter) Read(ctx context.Context, offset int64, size int64, buf []byte) (int, error) {
	atomic.AddInt32(&m.callCount, 1)

	if m.shouldFail {
		return 0, fmt.Errorf("mock adapter failure")
	}

	if m.readDelay > 0 {
		time.Sleep(m.readDelay)
	}

	if offset >= int64(len(m.data)) {
		return 0, nil
	}

	end := min(offset+size, int64(len(m.data)))

	bytesToCopy := end - offset
	copy(buf, m.data[offset:end])
	return int(bytesToCopy), nil
}

func (m *mockAdapter) Open(ctx context.Context) (io.ReadCloser, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock adapter failure")
	}
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

func (m *mockAdapter) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	return 0, fmt.Errorf("write not implemented in mock")
}

func (m *mockAdapter) GetMeta(ctx context.Context) (*webfs.FileMetadata, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock adapter failure")
	}

	lastModified := time.Now()
	return &webfs.FileMetadata{
		Size:         uint64(len(m.data)),
		LastModified: &lastModified,
	}, nil
}

func (m *mockAdapter) GetCallCount() int {
	return int(atomic.LoadInt32(&m.callCount))
}

func (m *mockAdapter) ResetCallCount() {
	atomic.StoreInt32(&m.callCount, 0)
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

func TestInode_BasicRead(t *testing.T) {
	testData := []byte("hello world")
	adapter := &mockAdapter{data: testData}
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	data, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Read should succeed: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("Read data %q doesn't match expected %q", data, testData)
	}

	if adapter.GetCallCount() != 1 {
		t.Errorf("Adapter should be called exactly once, got %d calls", adapter.GetCallCount())
	}
}

func TestInode_ReadCaching(t *testing.T) {
	testData := []byte("cached content test")
	adapter := &mockAdapter{data: testData}
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// First read should hit adapter
	data1, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("First read should succeed: %v", err)
	}

	if adapter.GetCallCount() != 1 {
		t.Error("First read should call adapter once")
	}

	// Verify data is cached
	if !inode.IsCached(0, int64(len(testData))) {
		t.Error("Data should be cached after first read")
	}

	// Second read should hit cache (no additional adapter calls)
	data2, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Second read should succeed: %v", err)
	}

	if adapter.GetCallCount() != 1 {
		t.Error("Second read should not call adapter (cache hit)")
	}

	if string(data1) != string(data2) {
		t.Error("Cached data should match original data")
	}
}

func TestInode_PartialRead(t *testing.T) {
	testData := []byte("hello world from cache")
	adapter := &mockAdapter{data: testData}
	inode := createTestInode([]webfs.FileAdapter{adapter})
	ctx := context.Background()

	// Read full data to populate cache
	_, err := inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Initial read should succeed: %v", err)
	}

	// Partial read should hit cache
	adapter.ResetCallCount()              // Reset to verify cache behavior
	partial, err := inode.Read(ctx, 6, 5) // Read "world"
	if err != nil {
		t.Fatalf("Partial read should succeed: %v", err)
	}

	expected := "world"
	if string(partial) != expected {
		t.Errorf("Partial read got %q, expected %q", partial, expected)
	}

	if adapter.GetCallCount() != 0 {
		t.Error("Partial read should hit cache, not call adapter")
	}

	// Verify cache reports the range as cached
	if !inode.IsCached(6, 5) {
		t.Error("Partial range should be reported as cached")
	}
}

func TestInode_ReadBeyondCachedRange(t *testing.T) {
	testData := []byte("short")
	adapter := &mockAdapter{data: testData}
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
	adapter.ResetCallCount()
	_, err = inode.Read(ctx, 0, int64(len(testData))) // Read full "short"
	if err != nil {
		t.Fatalf("Extended read should succeed: %v", err)
	}

	if adapter.GetCallCount() != 1 {
		t.Error("Reading beyond cache should call adapter")
	}
}

func TestInode_ClearCache(t *testing.T) {
	testData := []byte("cache clear test")
	adapter := &mockAdapter{data: testData}
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
	adapter.ResetCallCount()
	_, err = inode.Read(ctx, 0, int64(len(testData)))
	if err != nil {
		t.Fatalf("Read after cache clear should succeed: %v", err)
	}

	if adapter.GetCallCount() != 1 {
		t.Error("Read after cache clear should call adapter")
	}
}

func TestInode_AdapterFailover(t *testing.T) {
	testData := []byte("backup content")
	primaryAdapter := &mockAdapter{shouldFail: true}
	backupAdapter := &mockAdapter{data: testData}
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
	if primaryAdapter.GetCallCount() != 1 {
		t.Error("Primary adapter should be tried first")
	}

	if backupAdapter.GetCallCount() != 1 {
		t.Error("Backup adapter should be called after primary fails")
	}
}

func TestInode_AllAdaptersFail(t *testing.T) {
	primaryAdapter := &mockAdapter{shouldFail: true}
	backupAdapter := &mockAdapter{shouldFail: true}
	inode := createTestInode([]webfs.FileAdapter{primaryAdapter, backupAdapter})
	ctx := context.Background()

	// Read should fail when all adapters fail
	_, err := inode.Read(ctx, 0, 10)
	if err == nil {
		t.Error("Read should fail when all adapters fail")
	}

	// Verify both adapters were tried
	if primaryAdapter.GetCallCount() != 1 {
		t.Error("Primary adapter should be tried")
	}

	if backupAdapter.GetCallCount() != 1 {
		t.Error("Backup adapter should be tried after primary fails")
	}
}

func TestInode_NoAdapters(t *testing.T) {
	inode := createTestInode(nil) // No adapters
	ctx := context.Background()

	// Read should fail with no adapters
	_, err := inode.Read(ctx, 0, 10)
	if err == nil {
		t.Error("Read should fail when no adapters are available")
	}

	if err.Error() != "adapter pool not set" {
		t.Errorf("Expected 'adapter pool not set' error, got: %v", err)
	}
}

func TestInode_ConcurrentReads(t *testing.T) {
	testData := []byte("concurrent read test data")
	adapter := &mockAdapter{data: testData, readDelay: 10 * time.Millisecond}
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
	// Exact call count depends on timing, but should be reasonable
	if adapter.GetCallCount() > 3 {
		t.Errorf("Too many adapter calls for concurrent reads: %d", adapter.GetCallCount())
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
	adapter := &mockAdapter{data: testData}
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
	adapter := &mockAdapter{data: testData}
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
