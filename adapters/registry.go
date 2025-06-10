package adapters

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/brettbedarf/webfs/fs"
)

var (
	mu        sync.RWMutex
	factories = map[string]func([]byte) (fs.AdapterProvider, error){}
)

// Register ties a JSON‐raw factory to a “type” key and should be called for each
// adapter type during app init
func Register(adapterType string, unmarshal func(raw []byte) (fs.AdapterProvider, error)) {
	mu.Lock()
	factories[adapterType] = unmarshal
	mu.Unlock()
}

// GetFactory picks the right factory based on the "type" field.
// All expected source adapter types should be registered with [Register]
// before calling this function.
func GetFactory(raw []byte) (fs.AdapterProvider, error) {
	var meta struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	mu.RLock()
	f, ok := factories[meta.Type]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no factory for %q", meta.Type)
	}
	return f(raw)
}
