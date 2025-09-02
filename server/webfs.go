package server

import (
	"github.com/brettbedarf/webfs/adapters"
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/filesystem"
	wfuse "github.com/brettbedarf/webfs/fuse"
	"github.com/brettbedarf/webfs/internal/util"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// WebFS contains the core filesystem state and operations with abstractions
// over the underlying FUSE wire protocol implementation
type WebFS struct {
	*filesystem.FileSystem
	cfg      *config.Config
	server   *fuse.Server
	registry *adapters.Registry
}

// New creates a WebFs instance given your config.
func New(cfg *config.Config, registry *adapters.Registry) *WebFS {
	return &WebFS{
		filesystem.NewFS(cfg),
		cfg,
		nil,
		registry,
	}
}

// Serve mounts and serves the filesystem at the given mountPoint.
func (fs *WebFS) Serve(mountPoint string) error {
	raw := wfuse.NewFuseRaw(fs.FileSystem)
	opts := fs.cfg.MountOptions
	slogger := util.NewLogLogger("FuseServer", util.TraceLevel)
	slogger.Println("SLOGGER INITIALIZED")
	srv, err := fuse.NewServer(raw, mountPoint, &fuse.MountOptions{
		Name:   opts.Name,
		FsName: opts.FsName,
		Debug:  fs.cfg.LogLvl == util.TraceLevel,
		Logger: slogger,
	})
	if err != nil {
		return err
	}
	fs.server = srv

	go srv.Serve()
	return srv.WaitMount()
}

func (fs *WebFS) ServeAsync(mountPoint string) <-chan error {
	done := make(chan error, 1)

	go func() {
		done <- fs.Serve(mountPoint)
		close(done)
	}()

	return done
}

// Unmount cleanly unmounts the filesystem.
func (fs *WebFS) Unmount() error {
	if fs.server == nil {
		return nil
	}
	return fs.server.Unmount()
}
