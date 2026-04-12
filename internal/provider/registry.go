package provider

import (
	"fmt"

	"github.com/scheiblingco/dnstui/internal/config"
)

type Constructor func(cfg config.ProviderConfig) (Provider, error)

var registry = map[string]Constructor{}

func Register(typeName string, ctor Constructor) {
	if _, exists := registry[typeName]; exists {
		panic(fmt.Sprintf("provider type %q already registered", typeName))
	}
	registry[typeName] = ctor
}

func New(cfg config.ProviderConfig) (Provider, error) {
	ctor, ok := registry[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown provider type %q (forgot to import the provider package?)", cfg.Type)
	}
	return ctor(cfg)
}

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

func RegisteredTypes() []string {
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}
