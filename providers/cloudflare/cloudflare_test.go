package cloudflare_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
	cfpkg "github.com/scheiblingco/dnstui/providers/cloudflare"
)

func newTestProvider(t *testing.T, serverURL string) provider.Provider {
	t.Helper()
	p, err := cfpkg.New(config.ProviderConfig{
		Name: "test",
		Type: "cloudflare",
		Settings: map[string]any{
			"api_token": "test-token",
			"base_url":  serverURL,
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return p
}

func jsonResponse(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"errors":  []any{},
		"result":  result,
		"result_info": map[string]any{
			"page": 1, "per_page": 100, "total_pages": 1, "count": 1, "total_count": 1,
		},
	})
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"errors":  []map[string]any{{"code": 1003, "message": msg}},
		"result":  nil,
	})
}

func TestNew_MissingCredentials(t *testing.T) {
	_, err := cfpkg.New(config.ProviderConfig{
		Name:     "bad",
		Type:     "cloudflare",
		Settings: map[string]any{},
	})
	if err == nil {
		t.Error("expected error for missing credentials, got nil")
	}
}

func TestNew_APIKeyRequiresBothFields(t *testing.T) {
	_, err := cfpkg.New(config.ProviderConfig{
		Name: "bad",
		Type: "cloudflare",
		Settings: map[string]any{
			"api_key": "key-only-no-email",
		},
	})
	if err == nil {
		t.Error("expected error when api_email is missing alongside api_key")
	}
}

func TestNew_ValidToken(t *testing.T) {
	_, err := cfpkg.New(config.ProviderConfig{
		Name: "ok",
		Type: "cloudflare",
		Settings: map[string]any{
			"api_token": "tok",
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNew_ValidKeyPair(t *testing.T) {
	_, err := cfpkg.New(config.ProviderConfig{
		Name: "ok",
		Type: "cloudflare",
		Settings: map[string]any{
			"api_key":   "key",
			"api_email": "user@example.com",
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListAccounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts" {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, []map[string]any{
			{"id": "acc1", "name": "Personal"},
			{"id": "acc2", "name": "Work"},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	accounts, err := p.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts() error: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}
	if accounts[0].ID != "acc1" || accounts[0].Name != "Personal" {
		t.Errorf("unexpected account[0]: %+v", accounts[0])
	}
}

func TestListAccounts_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errorResponse(w, http.StatusForbidden, "insufficient permissions")
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	_, err := p.ListAccounts(context.Background())
	if err == nil {
		t.Error("expected error on API failure")
	}
}

func TestListZones(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones" {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, []map[string]any{
			{"id": "zone1", "name": "example.com", "account": map[string]any{"id": "acc1", "name": "Personal"}},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	zones, err := p.ListZones(context.Background(), "acc1")
	if err != nil {
		t.Fatalf("ListZones() error: %v", err)
	}
	if len(zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(zones))
	}
	if zones[0].ID != "zone1" || zones[0].Name != "example.com" || zones[0].AccountID != "acc1" {
		t.Errorf("unexpected zone: %+v", zones[0])
	}
}

func TestListRecords(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/zone1/dns_records" {
			http.NotFound(w, r)
			return
		}
		proxied := true
		jsonResponse(w, []map[string]any{
			{
				"id":          "rec1",
				"zone_id":     "zone1",
				"name":        "www.example.com",
				"type":        "A",
				"content":     "1.2.3.4",
				"proxiable":   true,
				"proxied":     proxied,
				"ttl":         1,
				"created_on":  now.Format(time.RFC3339),
				"modified_on": now.Format(time.RFC3339),
			},
			{
				"id":          "rec2",
				"zone_id":     "zone1",
				"name":        "example.com",
				"type":        "MX",
				"content":     "mail.example.com",
				"proxiable":   false,
				"ttl":         300,
				"priority":    10,
				"created_on":  now.Format(time.RFC3339),
				"modified_on": now.Format(time.RFC3339),
			},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	records, err := p.ListRecords(context.Background(), "zone1")
	if err != nil {
		t.Fatalf("ListRecords() error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	a := records[0]
	if a.Type != provider.RecordTypeA || a.Value != "1.2.3.4" {
		t.Errorf("unexpected A record: %+v", a)
	}
	if a.TTL != 0 {
		t.Errorf("CF ttl=1 should map to provider TTL=0 (auto), got %d", a.TTL)
	}
	if proxied, _ := a.Extra["proxied"].(bool); !proxied {
		t.Errorf("expected proxied=true in Extra")
	}

	mx := records[1]
	if mx.Type != provider.RecordTypeMX || mx.Priority != 10 {
		t.Errorf("unexpected MX record: %+v", mx)
	}
}

func TestCreateRecord(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/zones/zone1/dns_records" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		jsonResponse(w, map[string]any{
			"id":          "new-rec",
			"zone_id":     "zone1",
			"name":        received["name"],
			"type":        received["type"],
			"content":     received["content"],
			"ttl":         received["ttl"],
			"proxiable":   true,
			"created_on":  time.Now().Format(time.RFC3339),
			"modified_on": time.Now().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	created, err := p.CreateRecord(context.Background(), "zone1", provider.Record{
		Name:  "new.example.com",
		Type:  provider.RecordTypeA,
		Value: "5.6.7.8",
		TTL:   120,
	})
	if err != nil {
		t.Fatalf("CreateRecord() error: %v", err)
	}
	if created.ID != "new-rec" {
		t.Errorf("expected ID 'new-rec', got %q", created.ID)
	}
	if received["name"] != "new.example.com" {
		t.Errorf("request had wrong name: %v", received["name"])
	}
}

func TestUpdateRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/zones/zone1/dns_records/rec1" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		jsonResponse(w, map[string]any{
			"id":          "rec1",
			"zone_id":     "zone1",
			"name":        body["name"],
			"type":        body["type"],
			"content":     body["content"],
			"ttl":         body["ttl"],
			"proxiable":   false,
			"created_on":  time.Now().Format(time.RFC3339),
			"modified_on": time.Now().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	updated, err := p.UpdateRecord(context.Background(), "zone1", "rec1", provider.Record{
		Name:  "www.example.com",
		Type:  provider.RecordTypeA,
		Value: "9.9.9.9",
		TTL:   60,
	})
	if err != nil {
		t.Fatalf("UpdateRecord() error: %v", err)
	}
	if updated.ID != "rec1" {
		t.Errorf("expected ID 'rec1', got %q", updated.ID)
	}
}

func TestDeleteRecord(t *testing.T) {
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/zones/zone1/dns_records/rec1" {
			http.NotFound(w, r)
			return
		}
		deleted = true
		jsonResponse(w, map[string]any{"id": "rec1"})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	if err := p.DeleteRecord(context.Background(), "zone1", "rec1"); err != nil {
		t.Fatalf("DeleteRecord() error: %v", err)
	}
	if !deleted {
		t.Error("DELETE endpoint was never called")
	}
}

func TestDeleteRecord_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errorResponse(w, http.StatusNotFound, "record not found")
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	err := p.DeleteRecord(context.Background(), "zone1", "rec-missing")
	if err == nil {
		t.Error("expected error for missing record")
	}
}

func TestProviderIdentity(t *testing.T) {
	p := newTestProvider(t, "http://localhost")
	if p.ProviderName() != "cloudflare" {
		t.Errorf("ProviderName() = %q, want 'cloudflare'", p.ProviderName())
	}
	if p.FriendlyName() != "test" {
		t.Errorf("FriendlyName() = %q, want 'test'", p.FriendlyName())
	}
}
