package provider_test

import (
	"testing"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
)

func TestRegisterAndNew(t *testing.T) {
	const testType = "test_provider"

	// Register a no-op provider for the test.
	provider.Register(testType, func(cfg config.ProviderConfig) (provider.Provider, error) {
		return nil, nil // sufficient for registration smoke test
	})

	registered := provider.RegisteredTypes()
	found := false
	for _, rt := range registered {
		if rt == testType {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RegisteredTypes() did not include %q", testType)
	}
}

func TestNewUnknownType(t *testing.T) {
	_, err := provider.New(config.ProviderConfig{
		Name: "x",
		Type: "unknown_xyz",
	})
	if err == nil {
		t.Error("expected error for unknown provider type, got nil")
	}
}
