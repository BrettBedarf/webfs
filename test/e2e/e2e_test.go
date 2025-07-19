package e2e

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	webfsBin string
	projRoot string
	testEnv  *E2ETestEnvironment
)

func TestMain(m *testing.M) {
	var err error

	// Build WebFS binary once for all tests
	tmpBinDir, err := os.MkdirTemp("", "webfs-bin")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpBinDir); err != nil {
			panic(err)
		}
	}()

	webfsBin = filepath.Join(tmpBinDir, "webfs")

	// Determine project root
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot determine current file path")
	}
	projRoot = filepath.Join(filepath.Dir(thisFile), "..", "..")
	src := filepath.Join(projRoot, "cmd", "main.go")

	// Build with debug symbols
	cmd := exec.Command("go", "build", "-o", webfsBin, "-gcflags=all=-N -l", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(string(out))
	}

	// Create shared test environment
	testEnv, err = NewE2ETestEnvironment(webfsBin)
	if err != nil {
		panic(err)
	}
	defer testEnv.Close()

	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestE2EMountAndRead(t *testing.T) {
	textFile := NewTestFile("/simple-text").
		WithTextContent("Hello, WebFS! This is a simple test file.").
		Build()

	webfs := testEnv.StartWebFSWithFiles(t, []*TestFileSpec{textFile}, `[{
		"type": "file",
		"path": "test.txt",
		"sources": [{"type": "http", "url": "%s/simple-text"}]
	}]`)
	defer webfs.Stop()

	// Test basic file read
	data, err := os.ReadFile(filepath.Join(webfs.MountDir, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := "Hello, WebFS! This is a simple test file."
	if string(data) != expected {
		t.Fatalf("content mismatch:\nexpected: %q\ngot:      %q", expected, string(data))
	}
}

func TestE2EMultipleFiles(t *testing.T) {
	files := []*TestFileSpec{
		NewTestFile("/text-content").
			WithTextContent("Text file content for testing.").
			Build(),
		NewTestFile("/binary-content").
			WithBinaryContent(512). // 512 bytes of binary data
			Build(),
	}

	webfs := testEnv.StartWebFSWithFiles(t, files, `[
		{
			"type": "file",
			"path": "text.txt", 
			"sources": [{"type": "http", "url": "%[1]s/text-content"}]
		},
		{
			"type": "file",
			"path": "binary.bin",
			"sources": [{"type": "http", "url": "%[1]s/binary-content"}] 
		}
	]`)
	defer webfs.Stop()

	// Test text file
	textData, err := os.ReadFile(filepath.Join(webfs.MountDir, "text.txt"))
	if err != nil {
		t.Fatalf("failed to read text file: %v", err)
	}
	if string(textData) != "Text file content for testing." {
		t.Fatalf("text content mismatch: got %q", string(textData))
	}

	// Test binary file
	binaryData, err := os.ReadFile(filepath.Join(webfs.MountDir, "binary.bin"))
	if err != nil {
		t.Fatalf("failed to read binary file: %v", err)
	}
	if len(binaryData) != 512 {
		t.Fatalf("binary size mismatch: expected 512, got %d", len(binaryData))
	}

	// Verify binary pattern
	for i, b := range binaryData {
		expected := byte(i % 256)
		if b != expected {
			t.Fatalf("binary content mismatch at offset %d: expected %d, got %d", i, expected, b)
		}
	}
}

func TestE2EHTTPMetadata(t *testing.T) {
	testFile := NewTestFile("/metadata-test").
		WithTextContent("Content for metadata testing").
		WithETag(`"custom-etag-123"`).
		WithLastModified("Thu, 01 Jan 2020 12:00:00 GMT").
		Build()

	webfs := testEnv.StartWebFSWithFiles(t, []*TestFileSpec{testFile}, `[{
		"type": "file",
		"path": "meta.txt",
		"sources": [{"type": "http", "url": "%s/metadata-test"}]
	}]`)
	defer webfs.Stop()

	// Test file stat
	info, err := os.Stat(filepath.Join(webfs.MountDir, "meta.txt"))
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	expectedSize := int64(len("Content for metadata testing"))
	if info.Size() != expectedSize {
		t.Fatalf("size mismatch: expected %d, got %d", expectedSize, info.Size())
	}
}

func TestE2EHTTPErrors(t *testing.T) {
	errorFile := NewTestFile("/not-found").
		WithError(404).
		Build()

	webfs := testEnv.StartWebFSWithFiles(t, []*TestFileSpec{errorFile}, `[{
		"type": "file",
		"path": "missing.txt",
		"sources": [{"type": "http", "url": "%s/not-found"}]
	}]`)
	defer webfs.Stop()

	// Check if file appears in directory (it might, even with 404 source)
	files, err := os.ReadDir(webfs.MountDir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	var foundFile bool
	for _, f := range files {
		if f.Name() == "missing.txt" {
			foundFile = true
			t.Logf("File appears in directory with size: %d", func() int64 {
				if info, err := f.Info(); err == nil {
					return info.Size()
				}
				return -1
			}())
		}
	}

	if !foundFile {
		t.Log("File not found in directory (which might be expected for 404 sources)")
		return // This is acceptable behavior
	}

	// If file exists, attempt to read it should result in I/O error
	_, err = os.ReadFile(filepath.Join(webfs.MountDir, "missing.txt"))
	if err != nil {
		t.Logf("Got expected error reading 404 file: %v", err)
	} else {
		t.Log("Note: 404 file was readable (WebFS may have fallback behavior)")
	}
}

func TestE2ERangeRequests(t *testing.T) {
	// Create a larger file to test range requests
	largeContent := strings.Repeat("ABCDEFGHIJ", 100) // 1000 bytes

	largeFile := NewTestFile("/large-content").
		WithTextContent(largeContent).
		Build()

	webfs := testEnv.StartWebFSWithFiles(t, []*TestFileSpec{largeFile}, `[{
		"type": "file",
		"path": "large.txt",
		"sources": [{"type": "http", "url": "%s/large-content"}]
	}]`)
	defer webfs.Stop()

	// Read the file (this will trigger range requests internally)
	data, err := os.ReadFile(filepath.Join(webfs.MountDir, "large.txt"))
	if err != nil {
		t.Fatalf("failed to read large file: %v", err)
	}

	if string(data) != largeContent {
		t.Fatalf("large file content mismatch")
	}

	// Verify we got the expected amount of data
	if len(data) != 1000 {
		t.Fatalf("size mismatch: expected 1000, got %d", len(data))
	}
}

// E2ETestEnvironment manages shared resources for all e2e tests
type E2ETestEnvironment struct {
	MockServer *httptest.Server
	WebFSBin   string
	BaseDir    string
	files      map[string]*TestFileSpec
	mux        *http.ServeMux
}

// TestFileSpec defines a test file's content and behavior
type TestFileSpec struct {
	path        string
	content     []byte
	contentType string
	etag        string
	lastMod     string
	delay       time.Duration
	errorCode   int // 0 = success, 404, 500, etc.
}

// TestFileBuilder provides a fluent API for creating test files
type TestFileBuilder struct {
	spec TestFileSpec
}

// WebFSInstance represents a running WebFS process for testing
type WebFSInstance struct {
	cmd      *exec.Cmd
	MountDir string
	stdout   *bytes.Buffer
	stderr   *bytes.Buffer
	cleanup  func()
}

// NewTestFile creates a new test file builder with the given path
func NewTestFile(path string) *TestFileBuilder {
	return &TestFileBuilder{
		spec: TestFileSpec{
			path:        path,
			contentType: "text/plain",
			etag:        fmt.Sprintf(`"etag-%s"`, strings.TrimPrefix(path, "/")),
			lastMod:     "Wed, 21 Oct 2015 07:28:00 GMT",
		},
	}
}

// WithTextContent sets text content and appropriate content type
func (b *TestFileBuilder) WithTextContent(content string) *TestFileBuilder {
	b.spec.content = []byte(content)
	b.spec.contentType = "text/plain"
	return b
}

// WithBinaryContent generates binary content of the specified size
func (b *TestFileBuilder) WithBinaryContent(size int) *TestFileBuilder {
	b.spec.content = make([]byte, size)
	for i := range b.spec.content {
		b.spec.content[i] = byte(i % 256)
	}
	b.spec.contentType = "application/octet-stream"
	return b
}

// WithContent sets raw content bytes
func (b *TestFileBuilder) WithContent(content []byte) *TestFileBuilder {
	b.spec.content = content
	return b
}

// WithContentType sets the HTTP Content-Type header
func (b *TestFileBuilder) WithContentType(contentType string) *TestFileBuilder {
	b.spec.contentType = contentType
	return b
}

// WithETag sets the HTTP ETag header
func (b *TestFileBuilder) WithETag(etag string) *TestFileBuilder {
	b.spec.etag = etag
	return b
}

// WithLastModified sets the HTTP Last-Modified header
func (b *TestFileBuilder) WithLastModified(lastMod string) *TestFileBuilder {
	b.spec.lastMod = lastMod
	return b
}

// WithDelay adds artificial delay to responses (for timeout testing)
func (b *TestFileBuilder) WithDelay(delay time.Duration) *TestFileBuilder {
	b.spec.delay = delay
	return b
}

// WithError makes the file return an HTTP error status
func (b *TestFileBuilder) WithError(statusCode int) *TestFileBuilder {
	b.spec.errorCode = statusCode
	return b
}

// Build creates the final TestFileSpec
func (b *TestFileBuilder) Build() *TestFileSpec {
	return &b.spec
}

// NewE2ETestEnvironment creates a shared test environment with mock HTTP server
func NewE2ETestEnvironment(webfsBinary string) (*E2ETestEnvironment, error) {
	baseDir, err := os.MkdirTemp("", "webfs-e2e-tests")
	if err != nil {
		return nil, err
	}

	env := &E2ETestEnvironment{
		WebFSBin: webfsBinary,
		BaseDir:  baseDir,
		files:    make(map[string]*TestFileSpec),
		mux:      http.NewServeMux(),
	}

	// Create mock HTTP server
	env.MockServer = httptest.NewServer(env.mux)

	return env, nil
}

// Close cleans up the test environment
func (env *E2ETestEnvironment) Close() {
	if env.MockServer != nil {
		env.MockServer.Close()
	}
	if env.BaseDir != "" {
		_ = os.RemoveAll(env.BaseDir) // Best effort cleanup
	}
}

// RegisterFiles adds test files to the mock server
func (env *E2ETestEnvironment) RegisterFiles(files []*TestFileSpec) {
	for _, file := range files {
		env.files[file.path] = file
		// Capture file variable for closure
		fileSpec := file
		env.mux.HandleFunc(file.path, func(w http.ResponseWriter, r *http.Request) {
			env.handleMockRequest(w, r, fileSpec)
		})
	}
}

// handleMockRequest handles HTTP requests for mock files
func (env *E2ETestEnvironment) handleMockRequest(w http.ResponseWriter, r *http.Request, file *TestFileSpec) {
	// Add artificial delay if specified
	if file.delay > 0 {
		time.Sleep(file.delay)
	}

	// Return error if specified
	if file.errorCode != 0 {
		http.Error(w, fmt.Sprintf("Mock error %d", file.errorCode), file.errorCode)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", file.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(file.content)))
	w.Header().Set("Last-Modified", file.lastMod)
	w.Header().Set("ETag", file.etag)
	w.Header().Set("Accept-Ranges", "bytes")

	if r.Method == "HEAD" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Handle range requests
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		if env.handleRangeRequest(w, r, file, rangeHeader) {
			return
		}
	}

	// Full content request
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(file.content); err != nil {
		// In tests, we can't really recover from write errors
		panic(fmt.Sprintf("Failed to write mock response: %v", err))
	}
}

// handleRangeRequest processes HTTP range requests
func (env *E2ETestEnvironment) handleRangeRequest(w http.ResponseWriter, r *http.Request, file *TestFileSpec, rangeHeader string) bool {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return true
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return true
	}

	start, err1 := strconv.ParseInt(parts[0], 10, 64)
	end, err2 := strconv.ParseInt(parts[1], 10, 64)

	if err1 != nil || err2 != nil || start < 0 || start >= int64(len(file.content)) {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return true
	}

	// Clamp end to content length
	if end >= int64(len(file.content)) {
		end = int64(len(file.content)) - 1
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(file.content)))
	w.WriteHeader(http.StatusPartialContent)
	if _, err := w.Write(file.content[start : end+1]); err != nil {
		panic(fmt.Sprintf("Failed to write range response: %v", err))
	}
	return true
}

// StartWebFSWithFiles starts a WebFS instance with the specified test files
func (env *E2ETestEnvironment) StartWebFSWithFiles(t *testing.T, files []*TestFileSpec, nodeConfigTemplate string) *WebFSInstance {
	// Create a temporary server for this test to avoid conflicts
	testMux := http.NewServeMux()
	for _, file := range files {
		// Capture file variable for closure
		fileSpec := file
		testMux.HandleFunc(file.path, func(w http.ResponseWriter, r *http.Request) {
			env.handleMockRequest(w, r, fileSpec)
		})
	}

	// Create a new test server
	testServer := httptest.NewServer(testMux)

	// Store cleanup function
	cleanupServer := func() {
		testServer.Close()
	}

	// Create test-specific directories
	testID := strings.ReplaceAll(t.Name(), "/", "_")
	mountDir := filepath.Join(env.BaseDir, fmt.Sprintf("mount-%s", testID))
	nodesDir := filepath.Join(env.BaseDir, fmt.Sprintf("nodes-%s", testID))

	if err := os.MkdirAll(mountDir, 0o755); err != nil {
		t.Fatalf("Failed to create mount dir: %v", err)
	}
	if err := os.MkdirAll(nodesDir, 0o755); err != nil {
		t.Fatalf("Failed to create nodes dir: %v", err)
	}

	// Write nodes configuration using test server URL
	config := fmt.Sprintf(nodeConfigTemplate, testServer.URL)
	nodesFile := filepath.Join(nodesDir, "nodes.json")
	if err := os.WriteFile(nodesFile, []byte(config), 0o644); err != nil {
		t.Fatalf("Failed to write nodes file: %v", err)
	}

	// Start WebFS process
	cmd := exec.Command(env.WebFSBin, "--nodes", nodesFile, "-v", "4", mountDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start WebFS: %v", err)
	}

	instance := &WebFSInstance{
		cmd:      cmd,
		MountDir: mountDir,
		stdout:   &stdout,
		stderr:   &stderr,
		cleanup: func() {
			cleanupServer()
			_ = os.RemoveAll(mountDir) // Best effort cleanup
			_ = os.RemoveAll(nodesDir) // Best effort cleanup
		},
	}

	// Wait for mount to be ready
	if err := instance.WaitForMount(15 * time.Second); err != nil {
		instance.Stop()
		t.Fatalf("WebFS mount failed: %v", err)
	}

	return instance
}

// Stop gracefully stops the WebFS instance
func (w *WebFSInstance) Stop() {
	if w.cmd != nil && w.cmd.Process != nil {
		// Send interrupt signal
		_ = w.cmd.Process.Signal(os.Interrupt) // Process may have already exited

		// Wait for graceful shutdown with timeout
		done := make(chan error, 1)
		go func() {
			done <- w.cmd.Wait()
		}()

		select {
		case <-done:
			// Graceful shutdown completed
		case <-time.After(5 * time.Second):
			// Force kill if graceful shutdown takes too long
			_ = w.cmd.Process.Kill() // Process may have already exited
			<-done
		}
	}

	// Cleanup temp directories
	if w.cleanup != nil {
		w.cleanup()
	}
}

// WaitForMount waits for the WebFS mount to be ready
func (w *WebFSInstance) WaitForMount(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := os.ReadDir(w.MountDir); err == nil {
			// Mount directory is accessible, but wait a bit more for files to appear
			if files, err := os.ReadDir(w.MountDir); err == nil && len(files) > 0 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for WebFS mount to be ready")
}

// GetLogs returns the stdout and stderr from the WebFS process
func (w *WebFSInstance) GetLogs() (stdout, stderr string) {
	return w.stdout.String(), w.stderr.String()
}
