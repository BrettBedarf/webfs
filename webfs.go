package webfs

import (
	"github.com/brettbedarf/webfs/config"
	"github.com/brettbedarf/webfs/internal/core"
)

// New creates a WebFs instance given your config.
func New(cfg *config.Config) *core.WebFs {
	return core.NewWebFs(cfg)
}
