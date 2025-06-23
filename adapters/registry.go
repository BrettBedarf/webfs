package adapters

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/brettbedarf/webfs/api"
	"github.com/brettbedarf/webfs/util"
)

var (
	mu        sync.RWMutex
	factories = map[string]func([]byte) (api.AdapterProvider, error){}
)

// Register ties a JSON‐raw factory to a “type” key and should be called for each
// adapter type during app init
func Register(adapterType string, unmarshal func(raw []byte) (api.AdapterProvider, error)) {
	mu.Lock()
	factories[adapterType] = unmarshal
	mu.Unlock()
}

// GetProvider picks the right factory based on the "type" field.
// All expected source adapter types should be registered with [Register]
// before calling this function.
func GetProvider(raw []byte) (api.AdapterProvider, error) {
	logger := util.GetLogger("adapters.GetFactory")
	var meta struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		logger.Error().Err(err)
		return nil, err
	}
	mu.RLock()
	f, ok := factories[meta.Type]
	mu.RUnlock()
	if !ok {
		err := fmt.Errorf("no factory for %q", meta.Type)
		logger.Error().Err(err)
		return nil, err
	}
	return f(raw)
}
