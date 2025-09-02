package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"syscall"
	"testing"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/internal/mocks"
	"github.com/brettbedarf/webfs/internal/util"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// fakeAdapter provides an in-memory FileAdapter implementation for stable
// read/metadata tests.
type fakeAdapter struct {
	data    []byte
	meta    *webfs.FileMetadata
	readErr error
}

func (f *fakeAdapter) Read(ctx context.Context, offset int64, size int64, buf []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	if offset >= int64(len(f.data)) {
		return 0, io.EOF
	}
	end := min(offset+size, int64(len(f.data)))
	return copy(buf, f.data[offset:end]), nil
}

func (f *fakeAdapter) Open(ctx context.Context) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.data)), nil
}

func (f *fakeAdapter) Write(context.Context, int64, []byte) (int, error) {
	return 0, fmt.Errorf("unsupported")
}

func (f *fakeAdapter) GetMeta(context.Context) (*webfs.FileMetadata, error) {
	return f.meta, nil
}

// testProvider returns a preset adapter or error for adapter construction edge tests.
type testProvider struct {
	adapter webfs.FileAdapter
	err     error
}

func (p *testProvider) Adapter(config []byte) (webfs.FileAdapter, error) { return p.adapter, p.err }

func createTestConfig() *config.Config {
	return &config.Config{
		LogLvl:       util.InfoLevel,
		ChunkSize:    1024 * 1024,
		CacheMaxSize: 100 * 1024 * 1024,
		MaxWrite:     1024 * 1024,
		AttrTimeout:  1.0,
		EntryTimeout: 1.0,
		DirectIO:     true,
	}
}

// createFileWithFakeProvider creates a file node with a fake adapter containing the given data
func createFileWithFakeProvider(t *testing.T, fs *FileSystem, path string, data []byte) *Node {
	provider := &testProvider{
		adapter: &fakeAdapter{
			data: data,
			meta: &webfs.FileMetadata{Size: uint64(len(data))},
		},
	}

	req := &webfs.FileCreateRequest{
		NodeRequest: webfs.NodeRequest{
			Path:  path,
			Perms: 0o644,
		},
		Sources: []webfs.FileSource{
			{Provider: provider, Config: []byte(`{}`), Priority: 1},
		},
	}

	node, err := fs.AddFileNode(req)
	require.NoError(t, err)
	return node
}

// createFileWithMockProvider creates a file node with a mock provider for testing provider interactions
func createFileWithMockProvider(t *testing.T, fs *FileSystem, path string, mockProvider *mocks.MockAdapterProvider) *Node {
	req := &webfs.FileCreateRequest{
		NodeRequest: webfs.NodeRequest{
			Path:  path,
			Perms: 0o644,
		},
		Sources: []webfs.FileSource{
			{Provider: mockProvider, Config: []byte(`{"url":"test://test"}`), Priority: 1},
		},
	}

	node, err := fs.AddFileNode(req)
	require.NoError(t, err)
	return node
}

// createDirNode creates a directory node with the given path and permissions
func createDirNode(t *testing.T, fs *FileSystem, path string, perms uint32) *Node {
	req := &webfs.DirCreateRequest{
		NodeRequest: webfs.NodeRequest{
			Path:  path,
			Perms: perms,
		},
	}

	node, err := fs.AddDirNode(req)
	require.NoError(t, err)
	return node
}

// verifyChildAccessible verifies that a child can be accessed via GetChildCtx and returns the context
func verifyChildAccessible(t *testing.T, fs *FileSystem, parentID uint64, childName string) *NodeContext {
	ctx := fs.GetChildCtx(parentID, childName)
	require.NotNil(t, ctx)
	return ctx
}

func TestNewFS(t *testing.T) {
	t.Parallel()

	cfg := createTestConfig()

	fs := NewFS(cfg)

	require.NotNil(t, fs)

	// Test FS can provide root context
	rootCtx := fs.RootCtx()
	require.NotNil(t, rootCtx)
	defer rootCtx.Close()
}

func TestFileSystem_RootCtx(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())

	ctx := fs.RootCtx()
	defer ctx.Close()

	require.NotNil(t, ctx)
	assert.Equal(t, fs.root, ctx.node)
}

func TestFileSystem_GetNodeCtx(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())

	t.Run("ExistingNode", func(t *testing.T) {
		t.Parallel()

		ctx := fs.GetNodeCtx(fuse.FUSE_ROOT_ID)
		defer ctx.Close()

		require.NotNil(t, ctx)
		assert.Equal(t, fs.root, ctx.node)
	})

	t.Run("NonExistentNode", func(t *testing.T) {
		t.Parallel()

		ctx := fs.GetNodeCtx(999)

		assert.Nil(t, ctx)
	})
}

func TestFileSystem_GetChildCtx(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())

	mockAdapter := &mocks.MockFileAdapter{}
	mockAdapter.On("GetMeta", mock.Anything).Return(&webfs.FileMetadata{Size: 1024}, nil).Maybe()

	mockProvider := &mocks.MockAdapterProvider{}
	mockProvider.On("Adapter", mock.Anything).Return(mockAdapter, nil)

	_ = createFileWithMockProvider(t, fs, "child.txt", mockProvider)

	t.Run("ExistingChild", func(t *testing.T) {
		t.Parallel()

		ctx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "child.txt")
		defer ctx.Close()
	})

	t.Run("NonExistentChild", func(t *testing.T) {
		t.Parallel()

		ctx := fs.GetChildCtx(fuse.FUSE_ROOT_ID, "nonexistent.txt")

		assert.Nil(t, ctx)
	})

	t.Run("NonExistentParent", func(t *testing.T) {
		t.Parallel()

		ctx := fs.GetChildCtx(999, "child.txt")

		assert.Nil(t, ctx)
	})
}

func TestFileSystem_AddDirNode(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())

	t.Run("SingleDirectory", func(t *testing.T) {
		_ = createDirNode(t, fs, "testdir", 0o755)

		ctx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "testdir")
		defer ctx.Close()
	})

	t.Run("NestedDirectories", func(t *testing.T) {
		_ = createDirNode(t, fs, "path/to/nested/dir", 0o755)

		pathCtx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "path")
		defer pathCtx.Close()

		toCtx := verifyChildAccessible(t, fs, pathCtx.node.NodeID(), "to")
		defer toCtx.Close()

		nestedCtx := verifyChildAccessible(t, fs, toCtx.node.NodeID(), "nested")
		defer nestedCtx.Close()

		dirCtx := verifyChildAccessible(t, fs, nestedCtx.node.NodeID(), "dir")
		defer dirCtx.Close()
	})

	t.Run("ExistingDirectory", func(t *testing.T) {
		node1 := createDirNode(t, fs, "existing", 0o755)
		node2 := createDirNode(t, fs, "existing", 0o755)

		assert.Equal(t, node1, node2)
	})
}

func TestFileSystem_AddFileNode(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())

	t.Run("FileInRoot", func(t *testing.T) {
		// RefreshMeta may call GetMeta in background - make it optional
		mockAdapter := &mocks.MockFileAdapter{}
		mockAdapter.On("GetMeta", mock.Anything).Return(&webfs.FileMetadata{Size: 1024}, nil).Maybe()

		mockProvider := &mocks.MockAdapterProvider{}
		mockProvider.On("Adapter", mock.Anything).Return(mockAdapter, nil)

		_ = createFileWithMockProvider(t, fs, "test.txt", mockProvider)

		// Verify we can access the file via public API
		ctx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "test.txt")
		defer ctx.Close()

		// Verify adapter was created
		mockProvider.AssertExpectations(t)
	})

	t.Run("FileInNestedPath", func(t *testing.T) {
		// RefreshMeta may call GetMeta in background - make it optional
		mockAdapter := &mocks.MockFileAdapter{}
		mockAdapter.On("GetMeta", mock.Anything).Return(&webfs.FileMetadata{Size: 2048}, nil).Maybe()

		mockProvider := &mocks.MockAdapterProvider{}
		mockProvider.On("Adapter", mock.Anything).Return(mockAdapter, nil)

		_ = createFileWithMockProvider(t, fs, "nested/path/file.txt", mockProvider)

		// Verify nested path accessible via public API
		nestedCtx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "nested")
		defer nestedCtx.Close()

		pathCtx := verifyChildAccessible(t, fs, nestedCtx.node.NodeID(), "path")
		defer pathCtx.Close()

		fileCtx := verifyChildAccessible(t, fs, pathCtx.node.NodeID(), "file.txt")
		defer fileCtx.Close()

		// Verify adapter was created
		mockProvider.AssertExpectations(t)
	})

	t.Run("DuplicateFile", func(t *testing.T) {
		// RefreshMeta may call GetMeta in background - make it optional
		mockAdapter := &mocks.MockFileAdapter{}
		mockAdapter.On("GetMeta", mock.Anything).Return(&webfs.FileMetadata{Size: 512}, nil).Maybe()

		mockProvider := &mocks.MockAdapterProvider{}
		mockProvider.On("Adapter", mock.Anything).Return(mockAdapter, nil)

		// Create first time
		_ = createFileWithMockProvider(t, fs, "duplicate.txt", mockProvider)

		// Try to create again - should error
		req := &webfs.FileCreateRequest{
			NodeRequest: webfs.NodeRequest{
				Path:  "duplicate.txt",
				Perms: 0o644,
			},
			Sources: []webfs.FileSource{
				{Provider: mockProvider, Config: []byte(`{"url":"test://duplicate"}`), Priority: 1},
			},
		}
		_, err := fs.AddFileNode(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")

		// Verify adapter was created (only called once for first creation)
		mockProvider.AssertExpectations(t)
	})

	t.Run("NoValidAdapters", func(t *testing.T) {
		req := &webfs.FileCreateRequest{
			NodeRequest: webfs.NodeRequest{
				Path:  "invalid.txt",
				Perms: 0o644,
			},
			Sources: []webfs.FileSource{},
		}

		_, err := fs.AddFileNode(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no valid adapters")
	})
}

func TestFileSystem_Read(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())

	t.Run("NonExistentNode", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		_, err := fs.Read(ctx, 999, 0, 1024)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("ReadSuccess", func(t *testing.T) {
		t.Parallel()

		content := []byte("hello world")
		node := createFileWithFakeProvider(t, fs, "read.txt", content)
		id := fs.EnsureNodeID(node)

		buf, err := fs.Read(context.Background(), id, 0, int64(len(content)))

		require.NoError(t, err)
		assert.Equal(t, content, buf)
	})

	t.Run("MixedAdapterCreation", func(t *testing.T) {
		t.Parallel()
		goodData := []byte("good data")
		badProv := &testProvider{err: fmt.Errorf("constructor failure")}
		goodProv := &testProvider{adapter: &fakeAdapter{data: goodData, meta: &webfs.FileMetadata{Size: uint64(len(goodData))}}}
		req := &webfs.FileCreateRequest{
			NodeRequest: webfs.NodeRequest{Path: "mixed.txt", Perms: 0o644},
			Sources: []webfs.FileSource{
				{Provider: badProv, Config: []byte(`{}`), Priority: 1},
				{Provider: goodProv, Config: []byte(`{}`), Priority: 2},
			},
		}
		node, err := fs.AddFileNode(req)
		require.NoError(t, err)
		id := fs.EnsureNodeID(node)
		buf, err := fs.Read(context.Background(), id, 0, int64(len(goodData)))
		require.NoError(t, err)
		assert.Equal(t, goodData, buf)
	})

	t.Run("ReadFailoverSecondAdapterSucceeds", func(t *testing.T) {
		t.Parallel()
		// First adapter constructed fine but returns readErr; second returns data
		failAdapter := &fakeAdapter{data: []byte("bad"), meta: &webfs.FileMetadata{Size: 3}, readErr: fmt.Errorf("forced read error")}
		successData := []byte("fallback")
		successAdapter := &fakeAdapter{data: successData, meta: &webfs.FileMetadata{Size: uint64(len(successData))}}
		prov1 := &testProvider{adapter: failAdapter}
		prov2 := &testProvider{adapter: successAdapter}
		req := &webfs.FileCreateRequest{
			NodeRequest: webfs.NodeRequest{Path: "failover.txt", Perms: 0o644},
			Sources: []webfs.FileSource{
				{Provider: prov1, Config: []byte(`{}`), Priority: 1},
				{Provider: prov2, Config: []byte(`{}`), Priority: 2},
			},
		}
		node, err := fs.AddFileNode(req)
		require.NoError(t, err)
		id := fs.EnsureNodeID(node)
		buf, err := fs.Read(context.Background(), id, 0, int64(len(successData)))
		require.NoError(t, err)
		assert.Equal(t, successData, buf)
	})
}

func TestFileSystem_ModeAndConflicts(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())

	// Directory mode & perms
	_ = createDirNode(t, fs, "permtest", 0o750)
	dirCtx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "permtest")
	attr := dirCtx.node.Inode().CopyAttr()
	dirCtx.Close()

	assert.Equal(t, uint32(0o750), attr.Mode&0o777)
	assert.Equal(t, uint32(syscall.S_IFDIR), attr.Mode&uint32(syscall.S_IFDIR))

	// File mode & perms
	req := &webfs.FileCreateRequest{
		NodeRequest: webfs.NodeRequest{Path: "fileperm.txt", Perms: 0o640},
		Sources:     []webfs.FileSource{{Provider: &testProvider{adapter: &fakeAdapter{data: []byte("x"), meta: &webfs.FileMetadata{Size: 1}}}, Config: []byte(`{}`), Priority: 1}},
	}
	_, err := fs.AddFileNode(req)
	require.NoError(t, err)

	fileCtx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "fileperm.txt")
	fAttr := fileCtx.node.Inode().CopyAttr()
	fileCtx.Close()

	assert.Equal(t, uint32(0o640), fAttr.Mode&0o777)
	assert.Equal(t, uint32(syscall.S_IFREG), fAttr.Mode&uint32(syscall.S_IFREG))

	// Conflict: directory exists then file path
	_ = createDirNode(t, fs, "conflict", 0o755)
	conflictProv := &testProvider{adapter: &fakeAdapter{data: []byte("y"), meta: &webfs.FileMetadata{Size: 1}}}
	_, err = fs.AddFileNode(&webfs.FileCreateRequest{NodeRequest: webfs.NodeRequest{Path: "conflict", Perms: 0o644}, Sources: []webfs.FileSource{{Provider: conflictProv, Config: []byte(`{}`), Priority: 1}}})

	assert.Error(t, err)
	if err != nil {
		assert.Contains(t, err.Error(), "already exists")
	}
}

func TestFileSystem_EnsureNodeIDMonotonic(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())
	ids := make([]uint64, 0, 5)

	for i := range 5 {
		name := fmt.Sprintf("d%d", i)
		_ = createDirNode(t, fs, name, 0o755)
		ctx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, name)
		id := fs.EnsureNodeID(ctx.node)
		ctx.Close()

		require.NotZero(t, id)
		ids = append(ids, id)
	}

	// ensure strictly increasing order (allocation monotonicity)
	for i := 1; i < len(ids); i++ {
		assert.Greater(t, ids[i], ids[i-1])
	}

	// ensure uniqueness via map
	seen := make(map[uint64]struct{})
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			assert.Fail(t, "duplicate NodeID", "id=%d", id)
		}
		seen[id] = struct{}{}
	}
}

func TestFileSystem_ForgetNodeID(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())
	// create a directory node
	_ = createDirNode(t, fs, "gone", 0o755)
	ctx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "gone")
	id := fs.EnsureNodeID(ctx.node)
	ctx.Close()

	// ensure present
	ctx2 := fs.GetNodeCtx(id)
	require.NotNil(t, ctx2)
	ctx2.Close()

	// forget
	fs.ForgetNodeID(id)

	ctx3 := fs.GetNodeCtx(id)
	assert.Nil(t, ctx3)

	// create another directory and ensure new ID is greater and not reused
	_ = createDirNode(t, fs, "newdir", 0o755)
	ctxNew := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "newdir")
	newID := fs.EnsureNodeID(ctxNew.node)
	ctxNew.Close()

	assert.Greater(t, newID, id)

	// forgetting non-existent should be no-op
	fs.ForgetNodeID(9999999)
}

func TestFileSystem_ConcurrentAddDirNode(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())
	path := "a/b/c"
	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for range 20 {
		wg.Go(func() {
			_, err := fs.AddDirNode(&webfs.DirCreateRequest{NodeRequest: webfs.NodeRequest{Path: path, Perms: 0o755}})
			errs <- err
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	aCtx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "a")
	bCtx := verifyChildAccessible(t, fs, aCtx.node.NodeID(), "b")
	cCtx := verifyChildAccessible(t, fs, bCtx.node.NodeID(), "c")
	cCtx.Close()
	bCtx.Close()
	aCtx.Close()
}

func TestFileSystem_ReadCaching(t *testing.T) {
	t.Parallel()

	fs := NewFS(createTestConfig())
	content := []byte("cache-data-example")
	node := createFileWithFakeProvider(t, fs, "cache.txt", content)
	id := fs.EnsureNodeID(node)
	ctx := verifyChildAccessible(t, fs, fuse.FUSE_ROOT_ID, "cache.txt")
	inode := ctx.node.Inode()
	ctx.Close()

	assert.False(t, inode.IsCached(0, int64(len(content))))

	data1, err := fs.Read(context.Background(), id, 0, int64(len(content)))

	require.NoError(t, err)
	assert.Equal(t, content, data1)
	assert.True(t, inode.IsCached(0, int64(len(content))))

	data2, err := fs.Read(context.Background(), id, 0, int64(len(content)))

	require.NoError(t, err)
	assert.Equal(t, content, data2)
}
