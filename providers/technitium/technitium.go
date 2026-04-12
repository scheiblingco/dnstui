// Package technitium implements the Technitium DNS Server provider for dnstui.
//
// Authentication uses an API token passed as a query parameter (?token=…).
// Multiple server connections are supported via separate ProviderConfig entries.
//
// Self-registers as provider type "technitium" via init().
package technitium

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
)

// Settings holds Technitium-specific credentials decoded from ProviderConfig.Settings.
type Settings struct {
	// BaseURL is the root URL of the Technitium DNS server API (e.g. "http://192.168.1.1:5380").
	BaseURL string `mapstructure:"base_url"`
	// APIKey is the Technitium access token (generated in Settings → API Keys).
	APIKey string `mapstructure:"api_key"`
	// IgnoreTLS optionally disables TLS verification for self-signed certs (not recommended).
	IgnoreTLS bool `mapstructure:"ignore_tls"`
}

// tProvider implements provider.Provider for Technitium DNS server.
type tProvider struct {
	name     string
	settings Settings
	client   *http.Client
}

func init() {
	provider.Register("technitium", New)
}

// New constructs a Technitium provider from a ProviderConfig.
func New(cfg config.ProviderConfig) (provider.Provider, error) {
	var s Settings
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &s,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
	})
	if err != nil {
		return nil, fmt.Errorf("technitium: creating settings decoder: %w", err)
	}
	if err := dec.Decode(cfg.Settings); err != nil {
		return nil, fmt.Errorf("technitium: decoding settings: %w", err)
	}
	if s.BaseURL == "" {
		return nil, fmt.Errorf("technitium: settings.base_url is required")
	}
	if s.APIKey == "" {
		return nil, fmt.Errorf("technitium: settings.api_key is required")
	}
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")

	client := &http.Client{Timeout: 30 * time.Second}
	if s.IgnoreTLS {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}

	return &tProvider{
		name:     cfg.Name,
		settings: s,
		client:   client,
	}, nil
}

func (p *tProvider) ProviderName() string { return "technitium" }
func (p *tProvider) FriendlyName() string { return p.name }

// ── HTTP helpers ─────────────────────────────────────────────────────────────

// apiURL builds the full URL for a Technitium API path with the auth token appended.
func (p *tProvider) apiURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("token", p.settings.APIKey)
	return p.settings.BaseURL + "/api/" + path + "?" + params.Encode()
}

// tResponse is the standard Technitium API response envelope.
type tResponse[T any] struct {
	Status   string `json:"status"`  // "ok" or "error"
	Message  string `json:"message"` // error message when status == "error"
	Response T      `json:"response"`
}

// doGET performs a GET against the Technitium API with retry on 5xx.
func (p *tProvider) doGET(ctx context.Context, apiPath string, params url.Values) ([]byte, error) {
	return p.doRequest(ctx, http.MethodGet, apiPath, params, nil)
}

// doPOST performs a POST with a JSON body.
func (p *tProvider) doPOST(ctx context.Context, apiPath string, params url.Values, body any) ([]byte, error) {
	return p.doRequest(ctx, http.MethodPost, apiPath, params, body)
}

func (p *tProvider) doRequest(ctx context.Context, method, apiPath string, params url.Values, body any) ([]byte, error) {
	const maxAttempts = 3
	fullURL := p.apiURL(apiPath, params)

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}

		var bodyReader io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshaling request body: %w", err)
			}
			bodyReader = bytes.NewReader(b)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error %d", resp.StatusCode)
			continue
		}
		return b, nil
	}
	return nil, lastErr
}

// decodeResponse unmarshals the Technitium envelope and returns error if status != "ok".
func decodeResponse[T any](raw []byte, apiPath string) (T, error) {
	var env tResponse[T]
	if err := json.Unmarshal(raw, &env); err != nil {
		var zero T
		return zero, fmt.Errorf("technitium: decoding response from %s: %w", apiPath, err)
	}
	if env.Status != "ok" {
		var zero T
		return zero, fmt.Errorf("technitium: %s API error: %s", apiPath, env.Message)
	}
	return env.Response, nil
}

// ── Provider interface ────────────────────────────────────────────────────────

// tAccountResponse is the outer response wrapper for user/session calls.
// Technitium doesn't have a true "accounts" concept — we synthesise a single
// account from the server's own session info (username + server URL).
type tSessionInfo struct {
	Username string `json:"username"`
}

// ListAccounts returns a single synthetic account representing this Technitium server.
func (p *tProvider) ListAccounts(ctx context.Context) ([]provider.Account, error) {
	raw, err := p.doGET(ctx, "user/session/get", nil)
	if err != nil {
		return nil, fmt.Errorf("technitium: listing accounts: %w", err)
	}
	info, err := decodeResponse[tSessionInfo](raw, "user/session/get")
	if err != nil {
		return nil, err
	}
	return []provider.Account{
		{ID: p.settings.BaseURL, Name: info.Username + " @ " + p.settings.BaseURL},
	}, nil
}

// tZonesResponse wraps the zone list from Technitium.
type tZonesResponse struct {
	Zones []tZone `json:"zones"`
}

type tZone struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // Primary, Secondary, Stub, Forwarder, …
	Disabled bool   `json:"disabled"`
}

// ListZones returns all primary/secondary zones from the Technitium server.
// accountID is ignored (Technitium has no sub-account concept).
func (p *tProvider) ListZones(ctx context.Context, _ string) ([]provider.Zone, error) {
	raw, err := p.doGET(ctx, "zones/list", nil)
	if err != nil {
		return nil, fmt.Errorf("technitium: listing zones: %w", err)
	}
	resp, err := decodeResponse[tZonesResponse](raw, "zones/list")
	if err != nil {
		return nil, err
	}

	zones := make([]provider.Zone, 0, len(resp.Zones))
	for _, z := range resp.Zones {
		zones = append(zones, provider.Zone{
			ID:        z.Name,
			Name:      z.Name,
			AccountID: p.settings.BaseURL,
		})
	}
	return zones, nil
}

// tRecordsResponse wraps the record list from Technitium.
type tRecordsResponse struct {
	Zone    tZoneInfo `json:"zone"`
	Records []tRecord `json:"records"`
}

type tZoneInfo struct {
	Name string `json:"name"`
}

type tRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      int    `json:"ttl"`
	Disabled bool   `json:"disabled"`
	Comments string `json:"comments"`
	RDATA    tRDATA `json:"rData"`
}

// tRDATA contains all possible record-type specific fields.
// Technitium uses a flat rData object with type-dependent fields present.
// Note: "flags" is an int for CAA and a string for NAPTR; we use `any` to
// accept both without a JSON decode error.
type tRDATA struct {
	// A / AAAA
	IPAddress string `json:"ipAddress"`
	// CNAME / NS / PTR
	CName   string `json:"cname"`
	NSDName string `json:"nsdname"`
	PtrName string `json:"ptrName"`
	// MX / NAPTR reuse the same "preference" (int) field
	Exchange   string `json:"exchange"`
	Preference int    `json:"preference"`
	// TXT
	Text string `json:"text"`
	// SRV
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	Port     int    `json:"port"`
	Target   string `json:"target"`
	// CAA/NAPTR: flags is int for CAA, string for NAPTR; use any
	Flags any    `json:"flags"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
	// TLSA
	CertificateUsage           int    `json:"certificateUsage"`
	Selector                   int    `json:"selector"`
	MatchingType               int    `json:"matchingType"`
	CertificateAssociationData string `json:"certificateAssociationData"`
	// SSHFP
	Algorithm       int    `json:"algorithm"`
	FingerprintType int    `json:"fingerprintType"`
	Fingerprint     string `json:"fingerprint"`
	// NAPTR
	Order       int    `json:"order"`
	Services    string `json:"services"`
	Regexp      string `json:"regexp"`
	Replacement string `json:"replacement"`
}

// ListRecords returns all records in the given zone (by zone name).
func (p *tProvider) ListRecords(ctx context.Context, zoneID string) ([]provider.Record, error) {
	params := url.Values{"zone": {zoneID}}
	raw, err := p.doGET(ctx, "zones/records/get", params)
	if err != nil {
		return nil, fmt.Errorf("technitium: listing records for zone %s: %w", zoneID, err)
	}
	resp, err := decodeResponse[tRecordsResponse](raw, "zones/records/get")
	if err != nil {
		return nil, err
	}

	records := make([]provider.Record, 0, len(resp.Records))
	for _, r := range resp.Records {
		records = append(records, tRecordToShared(r, zoneID))
	}
	return records, nil
}

// CreateRecord adds a new record via Technitium's zones/records/add endpoint.
func (p *tProvider) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	params := sharedToTechParams(zoneID, r)
	raw, err := p.doGET(ctx, "zones/records/add", params)
	if err != nil {
		return provider.Record{}, fmt.Errorf("technitium: creating record in zone %s: %w", zoneID, err)
	}
	// Technitium returns the full updated record set; find the one we just added.
	resp, err := decodeResponse[tRecordsResponse](raw, "zones/records/add")
	if err != nil {
		return provider.Record{}, err
	}
	return findOrFallback(resp.Records, zoneID, r), nil
}

// UpdateRecord modifies an existing record.
// Technitium identifies the "old" record by its current fields and replaces it.
func (p *tProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	// recordID encodes "name/type/oldValue" — see tRecordToShared.
	parts := strings.SplitN(recordID, "\x00", 3)
	if len(parts) != 3 {
		return provider.Record{}, fmt.Errorf("technitium: invalid recordID format")
	}
	params := sharedToTechParams(zoneID, r)
	// Add old-value params for the overwrite call.
	params.Set("type", parts[1])
	params.Set("domain", parts[0])
	params.Set("oldValue", parts[2])

	raw, err := p.doGET(ctx, "zones/records/update", params)
	if err != nil {
		return provider.Record{}, fmt.Errorf("technitium: updating record in zone %s: %w", zoneID, err)
	}
	resp, err := decodeResponse[tRecordsResponse](raw, "zones/records/update")
	if err != nil {
		return provider.Record{}, err
	}
	return findOrFallback(resp.Records, zoneID, r), nil
}

// DeleteRecord removes a record. recordID encodes "name\x00type\x00value".
func (p *tProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	parts := strings.SplitN(recordID, "\x00", 3)
	if len(parts) != 3 {
		return fmt.Errorf("technitium: invalid recordID format")
	}
	params := url.Values{
		"zone":   {zoneID},
		"domain": {parts[0]},
		"type":   {parts[1]},
		"value":  {parts[2]},
	}
	raw, err := p.doGET(ctx, "zones/records/delete", params)
	if err != nil {
		return fmt.Errorf("technitium: deleting record from zone %s: %w", zoneID, err)
	}
	type emptyResp struct{}
	_, err = decodeResponse[emptyResp](raw, "zones/records/delete")
	return err
}

// ── Type mapping ─────────────────────────────────────────────────────────────

// tRecordToShared converts a Technitium record to the canonical provider.Record.
// The ID is a composite "name\x00type\x00primaryValue" that allows round-trip updates.
func tRecordToShared(r tRecord, zoneID string) provider.Record {
	value, priority := tExtractValue(r)
	rec := provider.Record{
		ID:       r.Name + "\x00" + r.Type + "\x00" + value,
		ZoneID:   zoneID,
		Name:     r.Name,
		Type:     provider.RecordType(r.Type),
		TTL:      r.TTL,
		Value:    value,
		Priority: priority,
		Extra:    map[string]any{},
	}
	if r.Comments != "" {
		rec.Extra["comment"] = r.Comments
	}
	if r.Disabled {
		rec.Extra["disabled"] = true
	}
	// SRV weight/port in Extra.
	if r.Type == "SRV" {
		rec.Extra["weight"] = r.RDATA.Weight
		rec.Extra["port"] = r.RDATA.Port
	}
	// CAA tag/flags in Extra — Flags is any (number from JSON → float64).
	if r.Type == "CAA" {
		switch v := r.RDATA.Flags.(type) {
		case float64:
			rec.Extra["caa_flags"] = int(v)
		case int:
			rec.Extra["caa_flags"] = v
		}
		rec.Extra["caa_tag"] = r.RDATA.Tag
	}
	// TLSA fields in Extra.
	if r.Type == "TLSA" {
		rec.Extra["tlsa_usage"] = r.RDATA.CertificateUsage
		rec.Extra["tlsa_selector"] = r.RDATA.Selector
		rec.Extra["tlsa_matching"] = r.RDATA.MatchingType
	}
	// SSHFP fields in Extra.
	if r.Type == "SSHFP" {
		rec.Extra["sshfp_algorithm"] = r.RDATA.Algorithm
		rec.Extra["sshfp_fp_type"] = r.RDATA.FingerprintType
	}
	// NAPTR fields in Extra — flags is a string for NAPTR.
	if r.Type == "NAPTR" {
		rec.Extra["naptr_pref"] = r.RDATA.Preference
		if s, ok := r.RDATA.Flags.(string); ok {
			rec.Extra["naptr_flags"] = s
		}
		rec.Extra["naptr_service"] = r.RDATA.Services
		rec.Extra["naptr_regexp"] = r.RDATA.Regexp
	}
	return rec
}

// tExtractValue returns the primary value string and priority for a Technitium record.
func tExtractValue(r tRecord) (string, int) {
	switch r.Type {
	case "A", "AAAA":
		return r.RDATA.IPAddress, 0
	case "CNAME":
		return r.RDATA.CName, 0
	case "NS":
		return r.RDATA.NSDName, 0
	case "PTR":
		return r.RDATA.PtrName, 0
	case "MX":
		return r.RDATA.Exchange, r.RDATA.Preference
	case "TXT":
		return r.RDATA.Text, 0
	case "SRV":
		return r.RDATA.Target, r.RDATA.Priority
	case "CAA":
		return r.RDATA.Value, 0
	case "TLSA":
		return r.RDATA.CertificateAssociationData, 0
	case "SSHFP":
		return r.RDATA.Fingerprint, 0
	case "NAPTR":
		return r.RDATA.Replacement, r.RDATA.Order
	default:
		return r.RDATA.Value, 0
	}
}

// sharedToTechParams builds the query parameters for add/update calls.
func sharedToTechParams(zoneID string, r provider.Record) url.Values {
	params := url.Values{
		"zone":   {zoneID},
		"domain": {r.Name},
		"type":   {string(r.Type)},
		"ttl":    {fmt.Sprintf("%d", r.TTL)},
	}

	switch r.Type {
	case provider.RecordTypeA, provider.RecordTypeAAAA:
		params.Set("ipAddress", r.Value)
	case provider.RecordTypeCNAME:
		params.Set("cname", r.Value)
	case provider.RecordTypeNS:
		params.Set("nameServer", r.Value)
	case provider.RecordTypePTR:
		params.Set("ptrName", r.Value)
	case provider.RecordTypeMX:
		params.Set("exchange", r.Value)
		params.Set("preference", fmt.Sprintf("%d", r.Priority))
	case provider.RecordTypeTXT:
		params.Set("text", r.Value)
	case provider.RecordTypeSRV:
		params.Set("target", r.Value)
		params.Set("priority", fmt.Sprintf("%d", r.Priority))
		if w, ok := r.Extra["weight"].(int); ok {
			params.Set("weight", fmt.Sprintf("%d", w))
		}
		if port, ok := r.Extra["port"].(int); ok {
			params.Set("port", fmt.Sprintf("%d", port))
		}
	case provider.RecordTypeCAA:
		params.Set("value", r.Value)
		if flags, ok := toInt(r.Extra["caa_flags"]); ok {
			params.Set("flags", fmt.Sprintf("%d", flags))
		}
		if tag, ok := r.Extra["caa_tag"].(string); ok {
			params.Set("tag", tag)
		}
	case provider.RecordTypeTLSA:
		params.Set("certificateAssociationData", r.Value)
		if v, ok := toInt(r.Extra["tlsa_usage"]); ok {
			params.Set("certificateUsage", fmt.Sprintf("%d", v))
		}
		if v, ok := toInt(r.Extra["tlsa_selector"]); ok {
			params.Set("selector", fmt.Sprintf("%d", v))
		}
		if v, ok := toInt(r.Extra["tlsa_matching"]); ok {
			params.Set("matchingType", fmt.Sprintf("%d", v))
		}
	case provider.RecordTypeSSHFP:
		params.Set("fingerprint", r.Value)
		if v, ok := toInt(r.Extra["sshfp_algorithm"]); ok {
			params.Set("algorithm", fmt.Sprintf("%d", v))
		}
		if v, ok := toInt(r.Extra["sshfp_fp_type"]); ok {
			params.Set("fingerprintType", fmt.Sprintf("%d", v))
		}
	case provider.RecordTypeNAPTR:
		params.Set("replacement", r.Value)
		params.Set("order", fmt.Sprintf("%d", r.Priority))
		if v, ok := toInt(r.Extra["naptr_pref"]); ok {
			params.Set("preference", fmt.Sprintf("%d", v))
		}
		if v, ok := r.Extra["naptr_flags"].(string); ok {
			params.Set("flags", v)
		}
		if v, ok := r.Extra["naptr_service"].(string); ok {
			params.Set("services", v)
		}
		if v, ok := r.Extra["naptr_regexp"].(string); ok {
			params.Set("regexp", v)
		}
	default:
		params.Set("value", r.Value)
	}

	if comment, ok := r.Extra["comment"].(string); ok && comment != "" {
		params.Set("comments", comment)
	}
	return params
}

// toInt coerces a numeric any value (int or float64) to int.
func toInt(v any) (int, bool) {
	switch vv := v.(type) {
	case int:
		return vv, true
	case float64:
		return int(vv), true
	}
	return 0, false
}

// findOrFallback looks for a matching record in the returned list or returns a
// best-effort reconstruction of what was just written.
func findOrFallback(records []tRecord, zoneID string, wanted provider.Record) provider.Record {
	for _, r := range records {
		if r.Name == wanted.Name && r.Type == string(wanted.Type) {
			return tRecordToShared(r, zoneID)
		}
	}
	// No exact match — return the record as supplied (no server-assigned ID etc.)
	return wanted
}
