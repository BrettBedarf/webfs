package filesystem

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/brettbedarf/webfs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper to create a test inode
func createTestInodeForNode(ino uint64, isDir bool) *Inode {
	mode := uint32(0o644)
	if isDir {
		mode = uint32(0o755) | fuse.S_IFDIR
	} else {
		mode |= fuse.S_IFREG
	}

	attr := &fuse.Attr{
		Ino:  ino,
		Size: 1024,
		Mode: mode,
	}
	return NewInode(attr, nil)
}

// Test helper to create root inode (ino=1)
func createRootInode() *Inode {
	return createTestInodeForNode(1, true)
}

func TestNewNode_NilInode(t *testing.T) {
	node, err := NewNode("test.txt", nil)

	// Should error when creating node with nil inode
	assert.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "cannot create node with nil inode")
}

func TestNode_AddChild(t *testing.T) {
	parentInode := createTestInodeForNode(1, true)
	childInode := createTestInodeForNode(2, false)

	parent, err := NewNode("parent", parentInode)
	require.NoError(t, err)
	child, err := NewNode("child.txt", childInode)
	require.NoError(t, err)

	// Add child to parent
	parent.AddChild(child)

	// Verify child was added
	retrievedChild, exists := parent.GetChild("child.txt")
	require.True(t, exists)
	assert.Equal(t, child, retrievedChild)

	// Verify parent reference was set
	child.mu.RLock()
	assert.Equal(t, parent, child.parent)
	child.mu.RUnlock()
}

func TestNode_GetChild(t *testing.T) {
	parentInode := createTestInodeForNode(1, true)
	childInode := createTestInodeForNode(2, false)

	parent, err := NewNode("parent", parentInode)
	require.NoError(t, err)
	child, err := NewNode("child.txt", childInode)
	require.NoError(t, err)
	parent.AddChild(child)

	// Test existing child
	retrievedChild, exists := parent.GetChild("child.txt")
	assert.True(t, exists)
	assert.Equal(t, child, retrievedChild)

	// Test non-existing child
	nonExistentChild, exists := parent.GetChild("nonexistent.txt")
	assert.False(t, exists)
	assert.Nil(t, nonExistentChild)
}

func TestNode_RemoveChild(t *testing.T) {
	parentInode := createTestInodeForNode(1, true)
	childInode := createTestInodeForNode(2, false)

	parent, err := NewNode("parent", parentInode)
	require.NoError(t, err)
	child, err := NewNode("child.txt", childInode)
	require.NoError(t, err)
	parent.AddChild(child)

	// Verify child exists before removal
	_, exists := parent.GetChild("child.txt")
	require.True(t, exists)

	// Remove child
	removed := parent.RemoveChild("child.txt")
	assert.True(t, removed)

	// Verify child no longer exists
	_, exists = parent.GetChild("child.txt")
	assert.False(t, exists)

	// Verify parent reference was cleared
	child.mu.RLock()
	assert.Nil(t, child.parent)
	child.mu.RUnlock()

	// Test removing non-existent child
	removedNonExistent := parent.RemoveChild("nonexistent.txt")
	assert.False(t, removedNonExistent)
}

func TestNode_Path_Root(t *testing.T) {
	rootInode := createRootInode()
	root, err := NewNode("", rootInode)
	require.NoError(t, err)

	path, err := root.Path()
	assert.NoError(t, err)
	assert.Equal(t, "", path)
	assert.True(t, root.IsRoot())
}

func TestNode_Path_Simple(t *testing.T) {
	rootInode := createRootInode()
	childInode := createTestInodeForNode(2, false)

	root, err := NewNode("", rootInode)
	require.NoError(t, err)
	child, err := NewNode("file.txt", childInode)
	require.NoError(t, err)
	root.AddChild(child)

	path, err := child.Path()
	assert.NoError(t, err)
	assert.Equal(t, "file.txt", path)
}

func TestNode_Path_Nested(t *testing.T) {
	rootInode := createRootInode()
	dirInode := createTestInodeForNode(2, true)
	fileInode := createTestInodeForNode(3, false)

	root, err := NewNode("", rootInode)
	require.NoError(t, err)
	dir, err := NewNode("dir", dirInode)
	require.NoError(t, err)
	file, err := NewNode("file.txt", fileInode)
	require.NoError(t, err)

	root.AddChild(dir)
	dir.AddChild(file)

	path, err := file.Path()
	assert.NoError(t, err)
	assert.Equal(t, "dir/file.txt", path)
}

func TestNode_Path_DetachedNode(t *testing.T) {
	childInode := createTestInodeForNode(2, false)
	child, err := NewNode("detached.txt", childInode)
	require.NoError(t, err)

	// Child has no parent (detached)
	path, err := child.Path()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "detached node")
	assert.Equal(t, "detached.txt", path)
}

func TestNode_Path_DeletedNode(t *testing.T) {
	childInode := createTestInodeForNode(2, false)
	child, err := NewNode("deleted.txt", childInode)
	require.NoError(t, err)

	// Mark node as deleted
	child.Del()
	assert.True(t, child.IsDel())

	path, err := child.Path()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deleted node")
	assert.Equal(t, "", path)
}

func TestNode_NodeID_Operations(t *testing.T) {
	inode := createTestInodeForNode(42, false)
	node, err := NewNode("test.txt", inode)
	require.NoError(t, err)

	// Initially not registered
	assert.Equal(t, uint64(0), node.NodeID())

	// Simulate registration (this would normally be done by filesystem)
	node.nodeID.Store(123)
	assert.Equal(t, uint64(123), node.NodeID())
}

func TestNode_Del(t *testing.T) {
	inode := createTestInodeForNode(42, false)
	node, err := NewNode("test.txt", inode)
	require.NoError(t, err)

	// Initially not deleted
	assert.False(t, node.IsDel())

	// Delete node
	node.Del()
	assert.True(t, node.IsDel())
}

func TestNode_Del_EnhancedCleanup(t *testing.T) {
	// Create a node with some test data and cache it
	testData := []byte("test file content")
	adapter := createSuccessfulMockAdapter(testData)
	inode := NewInode(&fuse.Attr{
		Ino:  42,
		Size: uint64(len(testData)),
		Mode: 0o644 | fuse.S_IFREG,
	}, []webfs.FileAdapter{adapter})

	node, err := NewNode("test.txt", inode)
	require.NoError(t, err)

	// Populate cache by reading
	ctx := context.Background()
	_, err = inode.Read(ctx, 0, int64(len(testData)))
	require.NoError(t, err)

	// Verify cache is populated
	assert.True(t, inode.IsCached(0, int64(len(testData))))

	// Add to parent to test parent reference clearing
	parentInode := createTestInodeForNode(1, true)
	parent, err := NewNode("parent", parentInode)
	require.NoError(t, err)
	parent.AddChild(node)

	// Verify node has parent
	node.mu.RLock()
	assert.Equal(t, parent, node.parent)
	node.mu.RUnlock()

	// Delete node
	node.Del()

	// Verify all cleanup was performed
	assert.True(t, node.IsDel())

	// Verify parent reference was cleared
	node.mu.RLock()
	assert.Nil(t, node.parent)
	node.mu.RUnlock()

	// Verify cache was cleared
	assert.False(t, inode.IsCached(0, int64(len(testData))))
}

func TestNode_Name_Methods(t *testing.T) {
	inode := createTestInodeForNode(42, false)
	node, err := NewNode("test.txt", inode)
	require.NoError(t, err)

	assert.Equal(t, "test.txt", node.Name())
	assert.Equal(t, "test.txt", node.NameSafe())

	// Both methods should return the same value
	node.mu.RLock()
	assert.Equal(t, "test.txt", node.nameLocked())
	node.mu.RUnlock()
}

func TestNode_IsRoot(t *testing.T) {
	// Test root node (ino=1, no parent)
	rootInode := createRootInode()
	root, err := NewNode("", rootInode)
	require.NoError(t, err)
	assert.True(t, root.IsRoot())

	// Test non-root node with parent
	childInode := createTestInodeForNode(2, false)
	child, err := NewNode("child.txt", childInode)
	require.NoError(t, err)
	root.AddChild(child)
	assert.False(t, child.IsRoot())

	// Test detached non-root node (no parent, but ino != 1)
	detachedInode := createTestInodeForNode(3, false)
	detached, err := NewNode("detached.txt", detachedInode)
	require.NoError(t, err)
	assert.False(t, detached.IsRoot())
}

func TestNode_ConcurrentChildOperations(t *testing.T) {
	parentInode := createTestInodeForNode(1, true)
	parent, err := NewNode("parent", parentInode)
	require.NoError(t, err)

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent add/remove operations
	for i := range numGoroutines {
		go func(goroutineID int) {
			defer wg.Done()
			for j := range numOperations {
				childName := fmt.Sprintf("child_%d_%d", goroutineID, j)
				childInode := createTestInodeForNode(uint64(goroutineID*1000+j), false)
				child, err := NewNode(childName, childInode)
				require.NoError(t, err)
				// Add child
				parent.AddChild(child)

				// Verify it was added
				_, exists := parent.GetChild(childName)
				assert.True(t, exists)

				// Remove child
				removed := parent.RemoveChild(childName)
				assert.True(t, removed)

				// Verify it was removed
				_, exists = parent.GetChild(childName)
				assert.False(t, exists)
			}
		}(i)
	}

	wg.Wait()
}

func TestNode_ConcurrentPathAccess(t *testing.T) {
	rootInode := createRootInode()
	dirInode := createTestInodeForNode(2, true)
	fileInode := createTestInodeForNode(3, false)

	root, err := NewNode("", rootInode)
	require.NoError(t, err)
	dir, err := NewNode("dir", dirInode)
	require.NoError(t, err)
	file, err := NewNode("file.txt", fileInode)
	require.NoError(t, err)

	root.AddChild(dir)
	dir.AddChild(file)

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent path access
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				path, err := file.Path()
				assert.NoError(t, err)
				assert.Equal(t, "dir/file.txt", path)

				// Small delay to increase chance of race conditions
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

func TestNode_ConcurrentNameAccess(t *testing.T) {
	inode := createTestInodeForNode(42, false)
	node, err := NewNode("test.txt", inode)
	require.NoError(t, err)

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent name access using different methods
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				name1 := node.Name()
				name2 := node.NameSafe()
				assert.Equal(t, "test.txt", name1)
				assert.Equal(t, "test.txt", name2)

				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

// Test edge case: Adding child with same name should overwrite and detach previous child
func TestNode_AddChild_Overwrite(t *testing.T) {
	parentInode := createTestInodeForNode(1, true)
	child1Inode := createTestInodeForNode(2, false)
	child2Inode := createTestInodeForNode(3, false)

	parent, err := NewNode("parent", parentInode)
	require.NoError(t, err)
	child1, err := NewNode("samename.txt", child1Inode)
	require.NoError(t, err)
	child2, err := NewNode("samename.txt", child2Inode)
	require.NoError(t, err)

	// Add first child
	parent.AddChild(child1)
	retrieved, exists := parent.GetChild("samename.txt")
	require.True(t, exists)
	assert.Equal(t, child1, retrieved)

	// Verify first child has parent reference
	child1.mu.RLock()
	assert.Equal(t, parent, child1.parent)
	child1.mu.RUnlock()

	// Add second child with same name (should overwrite and detach first child)
	parent.AddChild(child2)
	retrieved, exists = parent.GetChild("samename.txt")
	require.True(t, exists)
	assert.Equal(t, child2, retrieved)

	// First child should now be detached (parent reference cleared)
	child1.mu.RLock()
	assert.Nil(t, child1.parent)
	child1.mu.RUnlock()

	// Second child should have parent set
	child2.mu.RLock()
	assert.Equal(t, parent, child2.parent)
	child2.mu.RUnlock()
}
