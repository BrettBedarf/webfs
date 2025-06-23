package webfs

import (
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/internal/core"
	"github.com/brettbedarf/webfs/util"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// WebFs contains the core filesystem state and operations with abstractions
// over the underlying FUSE wire protocol implementation
type WebFs struct {
	*core.FileSystem
	cfg    *config.Config
	server *fuse.Server
}

// New creates a WebFs instance given your config.
func New(cfg *config.Config) *WebFs {
	return &WebFs{
		core.NewFS(cfg),
		cfg,
		nil,
	}
}

// Serve mounts and serves the filesystem at the given mountPoint.
func (fs *WebFs) Serve(mountPoint string) error {
	raw := core.NewFuseRaw(fs.FileSystem)
	opts := fs.cfg.MountOptions
	srv, err := fuse.NewServer(raw, mountPoint, &fuse.MountOptions{
		Name:   opts.Name,
		FsName: opts.FsName,
		Debug:  opts.Debug,
		Logger: util.NewLogLogger("FuseServer"),
	})
	if err != nil {
		return err
	}
	fs.server = srv

	go srv.Serve()
	return srv.WaitMount()
}

func (fs *WebFs) ServeAsync(mountPoint string) <-chan error {
	done := make(chan error, 1)

	go func() {
		done <- fs.Serve(mountPoint)
		close(done)
	}()

	return done
}

// Unmount cleanly unmounts the filesystem.
func (fs *WebFs) Unmount() error {
	if fs.server == nil {
		return nil
	}
	return fs.server.Unmount()
}
