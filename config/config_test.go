package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/brettbedarf/webfs/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestNewConfig_WithNilOverride tests that NewConfig creates a config with all default values
// when no override is provided.
func TestNewConfig_WithNilOverride(t *testing.T) {
	t.Parallel()

	cfg := NewConfig(nil)

	require.NotNil(t, cfg)
	assert.Equal(t, createDefaultCfg(), cfg, "must use default values when no config provided")
}

// TestNewConfig_WithOverride tests that NewConfig properly applies overrides while
// preserving defaults for unset fields.
func TestNewConfig_WithAllOverride(t *testing.T) {
	t.Parallel()

	override := createOverride()
	// hack for bad log verbosity vs internal log level pattern
	override.LogLvl = util.Pointer(TraceVerbose)
	cfg := NewConfig(override)

	expCfg := &Config{
		MountOptions: MountOptions{
			FsName: "test_fs",
			Name:   "test_name",
		},
		LogLvl:            util.TraceLevel,
		ChunkSize:         *override.ChunkSize,
		CacheMaxSize:      *override.CacheMaxSize,
		MaxPrefetchAhead:  *override.MaxPrefetchAhead,
		PrefetchBatchSize: *override.PrefetchBatchSize,
		MaxFH:             *override.MaxFH,
		MaxWrite:          *override.MaxWrite,
		AttrTimeout:       *override.AttrTimeout,
		EntryTimeout:      *override.EntryTimeout,
		DirectIO:          *override.DirectIO,
	}
	require.NotNil(t, cfg)
	assert.Equal(t, expCfg, cfg, "must override all provided fields")
}

func TestConfig_Merge_LogLvlConversion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		verboseValue  int
		expectedLevel util.LogLevel
	}{
		{"verbose_1_error", 1, util.ErrorLevel},
		{"verbose_2_warn", 2, util.WarnLevel},
		{"verbose_3_info", 3, util.InfoLevel},
		{"verbose_4_debug", 4, util.DebugLevel},
		{"verbose_5_trace", 5, util.TraceLevel},
		{"verbose_0_clamped_to_1", 0, util.ErrorLevel},     // clamped to 1
		{"verbose_100_clamped_to_5", 100, util.TraceLevel}, // clamped to 5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			override := &ConfigOverride{
				LogLvl: &tt.verboseValue,
			}

			cfg := NewConfig(override)

			assert.Equal(t, tt.expectedLevel, cfg.LogLvl,
				"CLI verbose %d should map to util.LogLevel %v", tt.verboseValue, tt.expectedLevel)
		})
	}
}

func TestConfig_Merge_NilOverrideVals(t *testing.T) {
	t.Parallel()

	override := &ConfigOverride{}

	cfg := NewConfig(override)

	require.NotNil(t, cfg)
	assert.Equal(t, createDefaultCfg(), cfg, "must use default values for nil override fields")
}

func TestConfig_Merge_PartialOverride(t *testing.T) {
	t.Parallel()

	override := &ConfigOverride{
		FsName:    util.Pointer("test_fs"),
		ChunkSize: util.Pointer(DefaultChunkSize + 1),
	}
	cfg := NewConfig(override)

	expCfg := createDefaultCfg()
	expCfg.FsName = "test_fs"
	expCfg.ChunkSize = DefaultChunkSize + 1

	require.NotNil(t, cfg)
	assert.Equal(t, expCfg, cfg, "must override all provided fields and leave rest default")
}

func TestConfig_NumCacheChunks(t *testing.T) {
	t.Parallel()

	t.Run("Zero ChunkSize", func(t *testing.T) {
		t.Parallel()
		cfg := Config{
			ChunkSize:    0,
			CacheMaxSize: 200,
		}
		assert.Equal(t, 0, cfg.NumCacheChunks(),
			"must return 0 when ChunkSize is 0")
	})
	t.Run("Divides evenly", func(t *testing.T) {
		t.Parallel()
		cfg := Config{
			ChunkSize:    100,
			CacheMaxSize: 200,
		}
		assert.Equal(t, 2, cfg.NumCacheChunks(),
			"must return 2 when CacheMaxSize / ChunkSize evenly divides")
	})
	t.Run("Divides with remainder", func(t *testing.T) {
		t.Parallel()
		cfg := Config{
			ChunkSize:    100,
			CacheMaxSize: 299,
		}
		assert.Equal(t, 2, cfg.NumCacheChunks(),
			"must return the quotient without rounding up")
	})
}

func TestLoadConfigOverrideFile_Valid(t *testing.T) {
	t.Parallel()

	type tc struct {
		ext   string
		build func() (*ConfigOverride, []byte)
	}

	cases := []tc{
		{
			ext: ".yaml",
			build: func() (*ConfigOverride, []byte) {
				o := createOverride()
				b, err := yaml.Marshal(o)
				require.NoError(t, err)
				return o, b
			},
		},
		{
			ext: ".yml",
			build: func() (*ConfigOverride, []byte) {
				o := createOverride()
				b, err := yaml.Marshal(o)
				require.NoError(t, err)
				return o, b
			},
		},
		{
			ext: ".json",
			build: func() (*ConfigOverride, []byte) {
				o := createOverride()
				b, err := json.Marshal(o)
				require.NoError(t, err)
				return o, b
			},
		},
	}

	for _, c := range cases {
		name := "valid" + c.ext
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			override, data := c.build()
			dir := t.TempDir()
			path := filepath.Join(dir, "override"+c.ext)
			require.NoError(t, os.WriteFile(path, data, 0o600))

			loaded, err := LoadConfigOverrideFile(path)

			require.NoError(t, err)
			require.NotNil(t, loaded)
			assert.Equal(t, *override, *loaded)
		})
	}
}

// TestLoadConfigOverrideFile_NonExistentFile tests error handling
// when trying to load a file that doesn't exist.
func TestLoadConfigOverrideFile_NonExistentFile(t *testing.T) {
	t.Parallel()

	// Setup: use path to non-existent file
	path := filepath.Join(t.TempDir(), "does_not_exist.yaml")

	// Execute
	_, err := LoadConfigOverrideFile(path)
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err), "expected not exist error, got %v", err)
}

// TestLoadConfigOverrideFile_UnsupportedExtension tests error handling
// for file extensions that aren't supported (.txt, .xml, etc).
func TestLoadConfigOverrideFile_UnsupportedExtension(t *testing.T) {
	t.Parallel()

	// Setup: create file with unsupported extension
	path := filepath.Join(t.TempDir(), "override.txt")
	require.NoError(t, os.WriteFile(path, []byte("chunk_size: 1"), 0o600))

	// Execute
	_, err := LoadConfigOverrideFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config file extension")
}

// TestNewConfigFromFile_FileError tests that file loading errors
// are properly propagated by the convenience function.
func TestNewConfigFromFile_FileError(t *testing.T) {
	t.Parallel()

	// Setup: use non-existent file path
	path := filepath.Join(t.TempDir(), "missing.json")

	// Execute
	_, err := NewConfigFromFile(path)
	require.Error(t, err)
}

func createDefaultCfg() *Config {
	return &Config{
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
}

// createOverride makes a ConfigOverride with all non-default values
func createOverride() *ConfigOverride {
	testLogVerbose := TraceVerbose
	if DefaultLogLvl == util.TraceLevel {
		testLogVerbose = DebugVerbose
	}
	return &ConfigOverride{
		ChunkSize:         util.Pointer(DefaultChunkSize + 1),
		CacheMaxSize:      util.Pointer(DefaultCacheMaxSize + 1),
		MaxPrefetchAhead:  util.Pointer(DefaultMaxPrefetchAhead + 1),
		PrefetchBatchSize: util.Pointer(DefaultPrefetchBatchSize + 1),
		MaxFH:             util.Pointer(1),
		MaxWrite:          util.Pointer(DefaultMaxWrite + 1),
		AttrTimeout:       util.Pointer(float64(DefaultAttrTimeout + 1)),
		EntryTimeout:      util.Pointer(float64(DefaultEntryTimeout + 1)),
		DirectIO:          util.Pointer(!DefaultDirectIO),
		LogLvl:            util.Pointer(testLogVerbose),
		FsName:            util.Pointer("test_fs"),
		Name:              util.Pointer("test_name"),
	}
}
