package technitium_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
	tpkg "github.com/scheiblingco/dnstui/providers/technitium"
)

func newTestProvider(t *testing.T, serverURL string) provider.Provider {
	t.Helper()
	p, err := tpkg.New(config.ProviderConfig{
		Name:     "test",
		Type:     "technitium",
		Settings: map[string]any{},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return p
}

func okJSON(w http.ResponseWriter, response any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"response": response,
	})
}

func errJSON(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "error",
		"message": msg,
	})
}

func TestFullConnection(t *testing.T) {
	x, err := tpkg.New(config.ProviderConfig{
		Name: "test",
		Type: "technitium",
		Settings: map[string]any{

			"ignore_tls": true,
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	zone, err := x.ListRecords(context.Background(), "larsec.de")
	if err != nil {
		t.Fatalf("ListRecords() error: %v", err)
	}

	for _, rec := range zone {
		fmt.Printf("%s %s %s\n", rec.Name, rec.Type, rec.Value)
	}

	mk, err := x.CreateRecord(context.Background(), "larsec.de", provider.Record{
		Name:  "test",
		Type:  provider.RecordTypeA,
		Value: "1.2.3.4",
		TTL:   300,
	})

	if err != nil {
		t.Fatalf("CreateRecord() error: %v", err)
	}

	fmt.Println(mk)
}

func TestNew_MissingBaseURL(t *testing.T) {
	_, err := tpkg.New(config.ProviderConfig{
		Name: "bad",
		Type: "technitium",
	})
	if err == nil {

	}
}

func TestNew_MissingAPIKey(t *testing.T) {
	_, err := tpkg.New(config.ProviderConfig{
		Name: "bad",
		Type: "technitium",
	})
	if err == nil {

	}
}

func TestListAccounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/session/get" {
			http.NotFound(w, r)
			return
		}
		okJSON(w, map[string]any{"username": "admin"})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	accounts, err := p.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts() error: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].ID != srv.URL {
		t.Errorf("expected account ID %q, got %q", srv.URL, accounts[0].ID)
	}
}

func TestListAccounts_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errJSON(w, "invalid token")
	}))
	defer srv.Close()

	_, err := newTestProvider(t, srv.URL).ListAccounts(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestListZones(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/zones/list" {
			http.NotFound(w, r)
			return
		}
		okJSON(w, map[string]any{
			"zones": []map[string]any{
				{"name": "example.com", "type": "Primary", "disabled": false},
				{"name": "internal.lan", "type": "Primary", "disabled": false},
			},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	zones, err := p.ListZones(context.Background(), "")
	if err != nil {
		t.Fatalf("ListZones() error: %v", err)
	}
	if len(zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(zones))
	}
	if zones[0].Name != "example.com" {
		t.Errorf("unexpected zone name: %s", zones[0].Name)
	}
}

func TestListRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/zones/records/get" {
			http.NotFound(w, r)
			return
		}
		if zone := r.URL.Query().Get("zone"); zone != "example.com" {
			http.NotFound(w, r)
			return
		}
		okJSON(w, map[string]any{
			"zone": map[string]any{"name": "example.com"},
			"records": []map[string]any{
				{
					"name": "www.example.com", "type": "A", "ttl": 300,
					"disabled": false, "comments": "",
					"rData": map[string]any{"ipAddress": "1.2.3.4"},
				},
				{
					"name": "example.com", "type": "MX", "ttl": 300,
					"disabled": false, "comments": "mail server",
					"rData": map[string]any{"exchange": "mail.example.com", "preference": 10},
				},
				{
					"name": "example.com", "type": "TXT", "ttl": 60,
					"disabled": false, "comments": "",
					"rData": map[string]any{"text": "v=spf1 include:example.com ~all"},
				},
			},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	records, err := p.ListRecords(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("ListRecords() error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	a := records[0]
	if a.Type != provider.RecordTypeA || a.Value != "1.2.3.4" {
		t.Errorf("unexpected A record: %+v", a)
	}

	mx := records[1]
	if mx.Type != provider.RecordTypeMX || mx.Priority != 10 || mx.Value != "mail.example.com" {
		t.Errorf("unexpected MX record: %+v", mx)
	}
	if comment, _ := mx.Extra["comment"].(string); comment != "mail server" {
		t.Errorf("expected comment 'mail server', got %q", comment)
	}

	txt := records[2]
	if txt.Type != provider.RecordTypeTXT || txt.Value != "v=spf1 include:example.com ~all" {
		t.Errorf("unexpected TXT record: %+v", txt)
	}
}

func TestCreateRecord(t *testing.T) {
	var calledPath string
	var calledQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		calledQuery = r.URL.Query()
		okJSON(w, map[string]any{
			"zone": map[string]any{"name": "example.com"},
			"records": []map[string]any{
				{
					"name": "new.example.com", "type": "A", "ttl": 120,
					"disabled": false, "comments": "",
					"rData": map[string]any{"ipAddress": "9.9.9.9"},
				},
			},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	rec, err := p.CreateRecord(context.Background(), "example.com", provider.Record{
		Name:  "new.example.com",
		Type:  provider.RecordTypeA,
		Value: "9.9.9.9",
		TTL:   120,
	})
	if err != nil {
		t.Fatalf("CreateRecord() error: %v", err)
	}
	if calledPath != "/api/zones/records/add" {
		t.Errorf("expected path /api/zones/records/add, got %s", calledPath)
	}
	if calledQuery.Get("ipAddress") != "9.9.9.9" {
		t.Errorf("expected ipAddress=9.9.9.9, got %s", calledQuery.Get("ipAddress"))
	}
	if rec.Value != "9.9.9.9" {
		t.Errorf("unexpected record value: %s", rec.Value)
	}
}

func TestDeleteRecord(t *testing.T) {
	var calledPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		okJSON(w, map[string]any{})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	// recordID is "name\x00type\x00value"
	err := p.DeleteRecord(context.Background(), "example.com", "www.example.com\x00A\x001.2.3.4")
	if err != nil {
		t.Fatalf("DeleteRecord() error: %v", err)
	}
	if calledPath != "/api/zones/records/delete" {
		t.Errorf("expected path /api/zones/records/delete, got %s", calledPath)
	}
}

func TestDeleteRecord_InvalidID(t *testing.T) {
	p := newTestProvider(t, "http://localhost")
	err := p.DeleteRecord(context.Background(), "example.com", "bad-id")
	if err == nil {
		t.Error("expected error for invalid recordID")
	}
}

func TestProviderIdentity(t *testing.T) {
	p := newTestProvider(t, "http://localhost")
	if p.ProviderName() != "technitium" {
		t.Errorf("ProviderName() = %q, want 'technitium'", p.ProviderName())
	}
	if p.FriendlyName() != "test" {
		t.Errorf("FriendlyName() = %q, want 'test'", p.FriendlyName())
	}
}
