package adapters

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/internal/util"
)

var (
	mu        sync.RWMutex
	providers = map[string]webfs.AdapterProvider{}
)

// Register ties a JSON‐raw factory to a “type” key and should be called for each
// adapter type during app init
func Register(adapterType string, provider webfs.AdapterProvider) {
	mu.Lock()
	providers[adapterType] = provider
	mu.Unlock()
}

// GetProvider returns the provider for the given adapter type
func GetProvider(adapterType string) (webfs.AdapterProvider, error) {
	mu.RLock()
	provider, ok := providers[adapterType]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no provider for %q", adapterType)
	}
	return provider, nil
}

// GetAdapter picks the right provider based on the "type" field and creates an adapter.
// All expected source adapter types should be registered with [Register]
// before calling this function.
func GetAdapter(rawCfg []byte) (webfs.FileAdapter, error) {
	logger := util.GetLogger("GetAdapter")
	var meta struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rawCfg, &meta); err != nil {
		logger.Debug().Err(err).Str("config", string(rawCfg)).Msg("Failed to get adapter type")
		return nil, err
	}
	provider, err := GetProvider(meta.Type)
	if err != nil {
		logger.Error().Err(err)
		return nil, err
	}
	return provider.Adapter(rawCfg)
}
