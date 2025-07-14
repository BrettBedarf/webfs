package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/brettbedarf/webfs/internal/util"
	"gopkg.in/yaml.v3"
)

// MB is bytes per megabyte
const MB = 1024 * 1024

// Default configuration constants. See [Config] for field descriptions.
const (
	// Uses 31 bits (2^31 - 1 = 2,147,483,647) to ensure compatibility with libfuse
	// and avoid signed integer overflow. This provides over 2 billion unique file
	// handles while staying within safe interop limits.
	DefaultMaxFH = (1 << 31) - 1

	// DefaultChunkSize is the size of each data chunk in bytes
	DefaultChunkSize = 1 * MB

	// DefaultCacheMaxSize is the maximum total cache size in bytes
	DefaultCacheMaxSize = 200 * MB

	// DefaultMaxPrefetchAhead is the maximum bytes to prefetch ahead of current read position
	DefaultMaxPrefetchAhead = 100 * MB

	// DefaultPrefetchBatchSize is the number of chunks to fetch concurrently each batch
	DefaultPrefetchBatchSize = 3

	// DefaultMaxWrite is the maximum write size per FUSE request
	DefaultMaxWrite = 1 * MB

	// DefaultAttrTimeout is the attribute cache timeout in seconds
	DefaultAttrTimeout = 1.0

	// DefaultEntryTimeout is the directory entry cache timeout in seconds
	DefaultEntryTimeout = 1.0

	// DefaultDirectIO determines whether to bypass page cache for HTTP files
	DefaultDirectIO = true

	DefaultLogLvl = util.InfoLevel
	DefaultFsName = "webfs"
	DefaultName   = "webfs"
)

// MountOptions holds high-level settings for mounting.
type MountOptions struct {
	FsName string // mount's FsName
	Name   string // mount's Name
}

// Config contains runtime configuration values for the HTTP filesystem.
type Config struct {
	mu sync.RWMutex
	MountOptions
	ChunkSize         int           // Size of each data chunk in bytes (affects memory usage and transfer efficiency) (Default 1MB)
	CacheMaxSize      int           // Maximum total cache size in bytes (Default 200MB)
	MaxPrefetchAhead  int           // Maximum bytes to prefetch ahead of current read position (Default 100MB)
	PrefetchBatchSize int           // Number of chunks to fetch concurrently in each prefetch batch (Default 3)
	LogLvl            util.LogLevel // Verbosity level for logs

	// NOTE: Low-level FUSE config (strongly recommend defaults unless you really know what you're doing):

	MaxFH        int     // Maximum file handle value for FUSE compatibility (Default 2147483647)
	MaxWrite     int     // Maximum write size per FUSE request (Default 1MB)
	AttrTimeout  float64 // Attribute cache timeout in seconds (Default 1.0)
	EntryTimeout float64 // Directory entry (dentry) cache timeout in seconds (Default 1.0) //TODO: bad timeout default
	DirectIO     bool    // Whether to bypass kernel page cache for files (Default true)
}

// NumCacheChunks returns the number of cache chunks derived from CacheMaxSize / ChunkSize.
// Returns 0 if ChunkSize is 0 to avoid division by zero.
func (c *Config) NumCacheChunks() int {
	if c.ChunkSize == 0 {
		return 0
	}
	return c.CacheMaxSize / c.ChunkSize
}

// ConfigOverride uses pointer fields to distinguish between unset and zero values
// when loading partial configuration. See [Config] for field descriptions.
type ConfigOverride struct {
	ChunkSize         *int           `yaml:"chunk_size,omitempty" json:"chunk_size,omitempty"`
	CacheMaxSize      *int           `yaml:"cache_max_size,omitempty" json:"cache_max_size,omitempty"`
	MaxPrefetchAhead  *int           `yaml:"max_prefetch_ahead,omitempty" json:"max_prefetch_ahead,omitempty"`
	PrefetchBatchSize *int           `yaml:"prefetch_batch_size,omitempty" json:"prefetch_batch_size,omitempty"`
	MaxFH             *int           `yaml:"max_fh,omitempty" json:"max_fh,omitempty"`
	MaxWrite          *int           `yaml:"max_write,omitempty" json:"max_write,omitempty"`
	AttrTimeout       *float64       `yaml:"attr_timeout,omitempty" json:"attr_timeout,omitempty"`
	EntryTimeout      *float64       `yaml:"entry_timeout,omitempty" json:"entry_timeout,omitempty"`
	DirectIO          *bool          `yaml:"direct_io,omitempty" json:"direct_io,omitempty"`
	LogLvl            *util.LogLevel `yaml:"verbose,omitempty" json:"verbose,omitempty"`
	FsName            *string        `yaml:"fs_name,omitempty" json:"fs_name,omitempty"`
	Name              *string        `yaml:"name,omitempty" json:"name,omitempty"`
}

// NewConfig creates a new Config with all default values.
func NewConfig(override *ConfigOverride) *Config {
	cfg := &Config{
		MountOptions: MountOptions{
			FsName: DefaultFsName,
			Name:   DefaultName,
		},
		LogLvl:            DefaultLogLvl,
		ChunkSize:         DefaultChunkSize,
		CacheMaxSize:      DefaultCacheMaxSize,
		MaxPrefetchAhead:  DefaultMaxPrefetchAhead,
		PrefetchBatchSize: DefaultPrefetchBatchSize,
		MaxFH:             DefaultMaxFH,
		MaxWrite:          DefaultMaxWrite,
		AttrTimeout:       DefaultAttrTimeout,
		EntryTimeout:      DefaultEntryTimeout,
		DirectIO:          DefaultDirectIO,
	}

	cfg.Merge(override)

	return cfg
}

// Merge applies non-nil values from override onto this Config.
// This allows partial configuration updates while preserving existing values.
func (c *Config) Merge(override *ConfigOverride) {
	if override == nil {
		return
	}
	if override.ChunkSize != nil {
		c.ChunkSize = *override.ChunkSize
	}
	if override.CacheMaxSize != nil {
		c.CacheMaxSize = *override.CacheMaxSize
	}
	if override.MaxPrefetchAhead != nil {
		c.MaxPrefetchAhead = *override.MaxPrefetchAhead
	}
	if override.PrefetchBatchSize != nil {
		c.PrefetchBatchSize = *override.PrefetchBatchSize
	}
	if override.MaxFH != nil {
		c.MaxFH = *override.MaxFH
	}
	if override.MaxWrite != nil {
		c.MaxWrite = *override.MaxWrite
	}
	if override.AttrTimeout != nil {
		c.AttrTimeout = *override.AttrTimeout
	}
	if override.EntryTimeout != nil {
		c.EntryTimeout = *override.EntryTimeout
	}
	if override.DirectIO != nil {
		c.DirectIO = *override.DirectIO
	}
	if override.LogLvl != nil {
		c.LogLvl = *override.LogLvl
	}
	if override.FsName != nil {
		c.FsName = *override.FsName
	}
	if override.Name != nil {
		c.Name = *override.Name
	}
}

// LoadConfigOverrideFile loads configuration overrides from a file without merging.
// Supports both YAML (.yaml, .yml) and JSON (.json) formats.
func LoadConfigOverrideFile(path string) (*ConfigOverride, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var override ConfigOverride

	// Determine format by file extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &override); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &override); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown config file extension: %s", path)
	}

	return &override, nil
}

// NewConfigFromFile creates a new Config by merging file overrides with defaults.
// This is a convenience function that combines NewDefaultConfig, LoadConfigOverrideFile, and Merge.
func NewConfigFromFile(path string) (*Config, error) {
	override, err := LoadConfigOverrideFile(path)
	if err != nil {
		return nil, err
	}
	cfg := NewConfig(override)
	return cfg, nil
}
