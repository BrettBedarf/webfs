package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var webfsBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "webfs-bin")
	if err != nil {
		panic(err)
	}
	webfsBin = filepath.Join(tmp, "webfs")
	// Build the webfs binary once for all tests
	cmd := exec.Command("go", "build", "-o", webfsBin, "cmd")
	if err := cmd.Run(); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func TestE2EBasic(t *testing.T) {
	// Create temporary mount directory
	tmpDir, err := os.MkdirTemp("", "webfs-e2e")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nodes definition file
	nodes := `[
		{"type":"directory","path":"/foo"},
		{"type":"file","path":"/foo/bar.txt","content":"hello"}
	]`
	nodesFile := filepath.Join(tmpDir, "nodes.json")
	if err := os.WriteFile(nodesFile, []byte(nodes), 0644); err != nil {
		t.Fatalf("failed to write nodes file: %v", err)
	}

	// Start the filesystem serve in background
	cmd := exec.Command(webfsBin, "--nodes", nodesFile, tmpDir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start webfs: %v", err)
	}
	defer func() {
		// Unmount and clean up
		exec.Command("fusermount", "-u", tmpDir).Run()
		cmd.Process.Signal(os.Interrupt)
		cmd.Wait()
	}()

	// Wait for mount to initialize
	time.Sleep(500 * time.Millisecond)

	// Verify file content
	data, err := os.ReadFile(filepath.Join(tmpDir, "foo", "bar.txt"))
	if err != nil {
		t.Fatalf("failed to read file from mount: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file content: got %q, want %q", string(data), "hello")
	}
}
