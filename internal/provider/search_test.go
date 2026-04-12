package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/scheiblingco/dnstui/internal/provider"
)

type mockProvider struct {
	name     string
	accounts []provider.Account
	zones    map[string][]provider.Zone
	err      error
}

func (m *mockProvider) ProviderName() string { return "mock" }
func (m *mockProvider) FriendlyName() string { return m.name }

func (m *mockProvider) ListAccounts(_ context.Context) ([]provider.Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.accounts, nil
}

func (m *mockProvider) ListZones(_ context.Context, accountID string) ([]provider.Zone, error) {
	if m.err != nil {
		return nil, m.err
	}
	// BuildSearchCache calls ListZones with an empty accountID to fetch all zones.
	if accountID == "" {
		var all []provider.Zone
		for _, zz := range m.zones {
			all = append(all, zz...)
		}
		return all, nil
	}
	return m.zones[accountID], nil
}

func (m *mockProvider) ListRecords(_ context.Context, _ string) ([]provider.Record, error) {
	return nil, nil
}
func (m *mockProvider) CreateRecord(_ context.Context, _ string, r provider.Record) (provider.Record, error) {
	return r, nil
}
func (m *mockProvider) UpdateRecord(_ context.Context, _, _ string, r provider.Record) (provider.Record, error) {
	return r, nil
}
func (m *mockProvider) DeleteRecord(_ context.Context, _, _ string) error { return nil }

func TestBuildSearchCache_Empty(t *testing.T) {
	entries, err := provider.BuildSearchCache(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for nil provider slice, got %d", len(entries))
	}
}

func TestBuildSearchCache_SingleProvider(t *testing.T) {
	p := &mockProvider{
		name: "Test",
		accounts: []provider.Account{
			{ID: "acc1", Name: "Account One"},
		},
		zones: map[string][]provider.Zone{
			"acc1": {
				{ID: "z1", Name: "example.com", AccountID: "acc1"},
				{ID: "z2", Name: "example.net", AccountID: "acc1"},
			},
		},
	}

	entries, err := provider.BuildSearchCache(context.Background(), []provider.Provider{p})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect 1 account entry + 2 zone entries = 3.
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	var accounts, domains int
	for _, e := range entries {
		switch e.Kind {
		case provider.SearchEntryKindAccount:
			accounts++
			if !contains(e.Label, "account:") {
				t.Errorf("account entry label missing prefix: %q", e.Label)
			}
		case provider.SearchEntryKindDomain:
			domains++
			if !contains(e.Label, "domain:") {
				t.Errorf("domain entry label missing prefix: %q", e.Label)
			}
		}
	}
	if accounts != 1 {
		t.Errorf("expected 1 account entry, got %d", accounts)
	}
	if domains != 2 {
		t.Errorf("expected 2 domain entries, got %d", domains)
	}
}

func TestBuildSearchCache_MultipleProviders(t *testing.T) {
	p1 := &mockProvider{
		name:     "P1",
		accounts: []provider.Account{{ID: "a1", Name: "A1"}},
		zones:    map[string][]provider.Zone{"a1": {{ID: "z1", Name: "p1.com", AccountID: "a1"}}},
	}
	p2 := &mockProvider{
		name:     "P2",
		accounts: []provider.Account{{ID: "a2", Name: "A2"}},
		zones:    map[string][]provider.Zone{"a2": {{ID: "z2", Name: "p2.com", AccountID: "a2"}}},
	}

	entries, err := provider.BuildSearchCache(context.Background(), []provider.Provider{p1, p2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 accounts + 2 zones = 4
	if len(entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(entries))
	}
}

func TestBuildSearchCache_ProviderError(t *testing.T) {
	p := &mockProvider{
		name: "Broken",
		err:  errors.New("network unreachable"),
	}
	_, err := provider.BuildSearchCache(context.Background(), []provider.Provider{p})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildSearchCache_NoZones(t *testing.T) {
	p := &mockProvider{
		name:     "Empty",
		accounts: []provider.Account{{ID: "a", Name: "Empty"}},
		zones:    map[string][]provider.Zone{},
	}
	entries, err := provider.BuildSearchCache(context.Background(), []provider.Provider{p})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 account + 0 zones = 1
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := range s {
		if i+len(sub) <= len(s) && s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
