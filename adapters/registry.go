package adapters

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/brettbedarf/webfs"
	"github.com/brettbedarf/webfs/internal/util"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]webfs.AdapterProvider
	log       util.Logger
}

func NewRegistry() *Registry {
	return &Registry{
		providers: map[string]webfs.AdapterProvider{},
		log:       util.GetLogger("Registry"),
	}
}

// Register ties a JSON‐raw factory to a “type” key and should be called for each
// adapter type during app init
func (r *Registry) Register(adapterType string, provider webfs.AdapterProvider) {
	r.mu.Lock()
	if _, ok := r.providers[adapterType]; ok {
		r.log.Warn().Str("type", adapterType).Msg("Adapter type already registered")
		return
	}
	r.providers[adapterType] = provider
	r.mu.Unlock()
}

// GetProvider returns the provider for the given adapter type
func (r *Registry) GetProvider(adapterType string) (webfs.AdapterProvider, error) {
	r.mu.RLock()
	provider, ok := r.providers[adapterType]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no provider for %q", adapterType)
	}
	return provider, nil
}

// NewAdapter picks the right provider based on the "type" field and creates an adapter.
// All expected source adapter types should be registered with [Register]
// before calling this function.
func (r *Registry) NewAdapter(rawCfg []byte) (webfs.FileAdapter, error) {
	var meta struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rawCfg, &meta); err != nil {
		r.log.Error().Err(err).Str("config", string(rawCfg)).Msg("Failed to get adapter type")
		return nil, err
	}
	provider, err := r.GetProvider(meta.Type)
	if err != nil {
		r.log.Error().Err(err)
		return nil, err
	}
	return provider.Adapter(rawCfg)
}
