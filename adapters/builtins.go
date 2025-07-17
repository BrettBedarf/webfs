package adapters

// NOTE: If build bloat becomes a concern for unused adapters
// look into build tags i.e. +build !nohttp
// or nested packages with init() and main app can include just importing
// import (_ github.com/.../adapters/http)

type BuiltInAdapterType = string

const (
	HTTPAdapterType BuiltInAdapterType = "http"
)

// RegisterBuiltins registers all built-in adapters by default
// or only the specific ones if keys are provided
func RegisterBuiltins(adapters ...BuiltInAdapterType) {
	if len(adapters) == 0 {
		// Include all built-in adapters here when adding implementations
		adapters = append(adapters, HTTPAdapterType)
	}

	for _, key := range adapters {
		switch key {
		case HTTPAdapterType:
			RegisterHTTP()
		}
	}
}
