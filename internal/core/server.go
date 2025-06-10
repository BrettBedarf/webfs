package core

import (
	"github.com/brettbedarf/webfs/config"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Server wraps the underlying fuse.Server.
type Server struct {
	server *fuse.Server
}

// Mount mounts the WebFs at mountPoint according to opts.
// Returns a Server you can Serve() and Unmount().
func Mount(w *WebFs, mountPoint string, opts *config.MountOptions) (*Server, error) {
	if opts == nil {
		opts = &config.MountOptions{
			FsName: "webfs",
			Name:   "webfs",
		}
	}
	fuseOpts := &fuse.MountOptions{
		FsName: opts.FsName,
		Name:   opts.Name,
		Debug:  opts.Debug,
	}

	raw := NewFuseRaw(mountPoint, w)
	srv, err := fuse.NewServer(raw, mountPoint, fuseOpts)
	if err != nil {
		return nil, err
	}
	return &Server{server: srv}, nil
}

// Serve starts serving and waits until the filesystem is mounted.
func (s *Server) Serve() error {
	go s.server.Serve()
	return s.server.WaitMount()
}

// Unmount cleanly unmounts the filesystem.
func (s *Server) Unmount() error {
	return s.server.Unmount()
}
