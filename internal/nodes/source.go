package nodes

import (
	"fmt"
)

// Defines a data source for a node
type SourceDef struct {
	Type     SourceType    // Type of source (HTTP, Local, etc.)
	Location string        // Primary location (URL, path, etc.)
	Mirrors  []string      // Alternative locations
	Options  SourceOptions // Source-specific options
}

// Identifies the type of data source
type SourceType string

const (
	SourceTypeHttp SourceType = "http"
	// Backed by an underlying local file system
	SourceTypeLocal SourceType = "local"
	// Dynamic/Generated nodes i.e. user node defs, metadata
	// that makes sense to be served as files, etc
	SourceTypeVirtual SourceType = "virtual"
)

// Configurable options for sources
type SourceOptions struct {
	// Common options
	CachePolicy string
	Timeout     int64

	// Source-specific options stored as map
	// These get serialized to/from JSON
	Specific map[string]interface{}
}

// SourceFactory creates appropriate Source implementations
func SourceFactory(def SourceDef) (Source, error) {
	switch def.Type {
	case SourceTypeHttp:
		return NewHttpSource(def)
	case SourceTypeLocal:
		return NewLocalSource(def)
	// Add cases for other source types
	default:
		return nil, fmt.Errorf("unknown source type: %s", def.Type)
	}
}

// Rereates a new HTTP source
func NewHttpSource(def SourceDef) (Source, error) {
	return nil, fmt.Errorf("HTTP source not yet implemented")
}

// Creates a new local file source
func NewLocalSource(def SourceDef) (Source, error) {
	return nil, fmt.Errorf("Local source not yet implemented")
}

// The database representation of a source
type PersistSourceDef struct {
	ID       string
	NodeID   string
	Type     string
	Location string
	Mirrors  []string // Could be stored as JSON in DB
	Options  []byte   // JSON serialized options
}
