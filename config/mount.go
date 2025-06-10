package config

// MountOptions holds high-level settings for mounting.
// No go-fuse types are exposed here.
type MountOptions struct {
	Debug  bool   // fuse debug logs
	FsName string // mount's FsName
	Name   string // mount's Name
}
