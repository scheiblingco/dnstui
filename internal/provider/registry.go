package provider

import (
	"fmt"

	"github.com/scheiblingco/dnstui/internal/config"
)

// Constructor is a function that builds a Provider from a raw ProviderConfig.
// Each provider package registers its constructor during init().
type Constructor func(cfg config.ProviderConfig) (Provider, error)

var registry = map[string]Constructor{}

// Register makes a provider constructor available by its type name.
// It panics if the same type name is registered twice (programming error).
// Call this from each provider package's init() function.
func Register(typeName string, ctor Constructor) {
	if _, exists := registry[typeName]; exists {
		panic(fmt.Sprintf("provider type %q already registered", typeName))
	}
	registry[typeName] = ctor
}

// New instantiates a Provider for the given ProviderConfig.
// Returns an error if the type is unknown or the constructor fails.
func New(cfg config.ProviderConfig) (Provider, error) {
	ctor, ok := registry[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown provider type %q (forgot to import the provider package?)", cfg.Type)
	}
	return ctor(cfg)
}

// NewAll instantiates a Provider for every entry in the supplied slice.
// Returns on the first error encountered, with the index of the failing config.
func NewAll(cfgs []config.ProviderConfig) ([]Provider, error) {
	providers := make([]Provider, 0, len(cfgs))
	for i, cfg := range cfgs {
		p, err := New(cfg)
		if err != nil {
			return nil, fmt.Errorf("providers[%d] (%s): %w", i, cfg.Name, err)
		}
		providers = append(providers, p)
	}
	return providers, nil
}

// RegisteredTypes returns the list of provider type names that have been registered.
// Useful for validation error messages.
func RegisteredTypes() []string {
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}
