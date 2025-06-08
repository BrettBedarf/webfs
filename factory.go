package httpfs

import (
	"encoding/json"
	"fmt"
	"sync"
)

var (
	sourceUnmarshalers = make(map[SourceType]SourceTypeUnmarshaler)
	adapterCreators    = make(map[SourceType]AdapterCreator)
	registryMutex      sync.RWMutex
)

// Function types
type (
	SourceTypeUnmarshaler func(data []byte) (FileSourcer, error)
	AdapterCreator        func(source FileSourcer) (FileAdapter, error)
)

func RegisterSourceType(sourceType SourceType, unmarshaler SourceTypeUnmarshaler, creator AdapterCreator) error {
	registryMutex.Lock()
	defer registryMutex.Unlock()
	sourceUnmarshalers[sourceType] = unmarshaler
	adapterCreators[sourceType] = creator
	return nil
}

// Internal function called from requests.go
func unmarshalRegisteredSource(sourceType SourceType, rawSource json.RawMessage) (FileSourcer, error) {
	registryMutex.RLock()
	unmarshaler, exists := sourceUnmarshalers[sourceType]
	registryMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown source type: %s", sourceType)
	}

	return unmarshaler(rawSource)
}

// Creates an adapter instance for a file source
func CreateFileAdapter(source FileSourcer) (FileAdapter, error) {
	registryMutex.RLock()
	creator, exists := adapterCreators[source.GetType()]
	registryMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported source type: %s", source.GetType())
	}
	return creator(source)
}
