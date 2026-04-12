package cloudns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
)

func init() {
	provider.Register("cloudns", New)
}

const defaultBase = "https://api.cloudns.net"

type Settings struct {
	// AuthID is the main ClouDNS account auth-id.
	AuthID int `mapstructure:"auth_id"`
	// SubAuthID is optionally used instead of AuthID for API sub-users.
	SubAuthID int `mapstructure:"sub_auth_id"`
	// AuthPassword is the account password.
	AuthPassword string `mapstructure:"auth_password"`
	// BaseURL overrides the API base URL (used for testing).
	BaseURL string `mapstructure:"base_url"`
}

type cnsProvider struct {
	name     string
	settings Settings
	client   *http.Client
}

func New(cfg config.ProviderConfig) (provider.Provider, error) {
	var s Settings
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &s,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
	})
	if err != nil {
		return nil, fmt.Errorf("cloudns: creating settings decoder: %w", err)
	}
	if err := dec.Decode(cfg.Settings); err != nil {
		return nil, fmt.Errorf("cloudns: decoding settings: %w", err)
	}
	if s.AuthPassword == "" {
		return nil, fmt.Errorf("cloudns: settings.auth_password is required")
	}
	if s.AuthID == 0 && s.SubAuthID == 0 {
		return nil, fmt.Errorf("cloudns: settings.auth_id or settings.sub_auth_id is required")
	}
	if s.BaseURL == "" {
		s.BaseURL = defaultBase
	}
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")
	return &cnsProvider{
		name:     cfg.Name,
		settings: s,
		client:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (p *cnsProvider) ProviderName() string { return "cloudns" }
func (p *cnsProvider) FriendlyName() string { return p.name }

func (p *cnsProvider) accountID() string {
	if p.settings.SubAuthID != 0 {
		return "sub-" + strconv.Itoa(p.settings.SubAuthID)
	}
	return strconv.Itoa(p.settings.AuthID)
}

func (p *cnsProvider) authFields(extra map[string]any) map[string]any {
	m := make(map[string]any, len(extra)+2)
	for k, v := range extra {
		m[k] = v
	}
	m["auth-password"] = p.settings.AuthPassword
	if p.settings.SubAuthID != 0 {
		m["sub-auth-id"] = p.settings.SubAuthID
	} else {
		m["auth-id"] = p.settings.AuthID
	}
	return m
}

func (p *cnsProvider) post(ctx context.Context, path string, query map[string]any) ([]byte, error) {
	// b, err := json.Marshal(body)
	// if err != nil {
	// 	return nil, fmt.Errorf("cloudns: marshaling request body: %w", err)
	// }

	queryParams := url.Values{}
	for k, v := range query {
		queryParams.Set(k, fmt.Sprintf("%v", v))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.settings.BaseURL+path+"?"+queryParams.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// return nil, fmt.Errorf("Request to %s with query params: %v", p.settings.BaseURL+path+"?"+queryParams.Encode(), query) // Debug log

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("cloudns: HTTP %d from %s", resp.StatusCode, path)
	}
	return data, nil
}

func checkErr(data []byte, path string) error {
	var env struct {
		Status string `json:"status"`
		Desc   string `json:"statusDescription"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil
	}
	if env.Status == "Failed" {
		return fmt.Errorf("cloudns: %s: %s", path, env.Desc)
	}
	return nil
}

func (p *cnsProvider) ListAccounts(ctx context.Context) ([]provider.Account, error) {
	data, err := p.post(ctx, "/dns/login.json", p.authFields(nil))
	if err != nil {
		return nil, fmt.Errorf("cloudns: verifying account: %w", err)
	}
	if err := checkErr(data, "/dns/login.json"); err != nil {
		return nil, err
	}
	return []provider.Account{
		{ID: p.accountID(), Name: p.name},
	}, nil
}

type cnsZoneInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func (p *cnsProvider) ListZones(ctx context.Context, _ string) ([]provider.Zone, error) {
	const pageSize = 100
	var zones []provider.Zone

	for page := 1; ; page++ {
		body := p.authFields(map[string]any{
			"page":          page,
			"rows-per-page": pageSize,
		})
		data, err := p.post(ctx, "/dns/list-zones.json", body)
		if err != nil {
			return nil, fmt.Errorf("cloudns: listing zones page %d: %w", page, err)
		}
		if err := checkErr(data, "/dns/list-zones.json"); err != nil {
			return nil, err
		}

		// Response is a JSON array of zone objects.
		var page_zones []cnsZoneInfo
		if err := json.Unmarshal(data, &page_zones); err != nil {
			return nil, fmt.Errorf("cloudns: parsing zone list: %w", err)
		}

		for _, z := range page_zones {
			zones = append(zones, provider.Zone{
				ID:        z.Name,
				Name:      z.Name,
				AccountID: p.accountID(),
			})
		}

		// Fewer results than requested means we've reached the last page.
		if len(page_zones) < pageSize {
			break
		}
	}
	return zones, nil
}

type cnsRecord struct {
	ID       string `json:"id"`
	Host     string `json:"host"`
	Type     string `json:"type"`
	Record   string `json:"record"`
	TTL      string `json:"ttl"`
	Priority string `json:"priority,omitempty"`
	Weight   string `json:"weight,omitempty"`
	Port     string `json:"port,omitempty"`
	// CAA
	CaaFlag  string `json:"caa_flag,omitempty"`
	CaaType  string `json:"caa_type,omitempty"`
	CaaValue string `json:"caa_value,omitempty"`
	// SSHFP
	Algorithm string `json:"algorithm,omitempty"`
	FpType    int    `json:"fptype,omitempty"`
	// TLSA
	TlsaUsage        string `json:"tlsa_usage,omitempty"`
	TlsaSelector     string `json:"tlsa_selector,omitempty"`
	TlsaMatchingType string `json:"tlsa_matching_type,omitempty"`
	// NAPTR
	Order   string `json:"order,omitempty"`
	Pref    string `json:"pref,omitempty"`
	Flag    string `json:"flag,omitempty"`
	Params  string `json:"params,omitempty"`
	Regexp  string `json:"regexp,omitempty"`
	Replace string `json:"replace,omitempty"`
}

func (p *cnsProvider) ListRecords(ctx context.Context, zoneID string) ([]provider.Record, error) {
	query := p.authFields(map[string]any{"domain-name": zoneID})
	data, err := p.post(ctx, "/dns/records.json", query)
	if err != nil {
		return nil, fmt.Errorf("cloudns: listing records for %s: %w", zoneID, err)
	}
	if err := checkErr(data, "/dns/records.json"); err != nil {
		return nil, err
	}

	// An empty record list is returned as JSON array [], not an object.
	if len(data) > 0 && data[0] == '[' {
		return []provider.Record{}, nil
	}

	var raw map[string]cnsRecord
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("cloudns: parsing records for %s: %w", zoneID, err)
	}

	records := make([]provider.Record, 0, len(raw))
	for _, r := range raw {
		records = append(records, toRecord(r, zoneID))
	}
	return records, nil
}

func toRecord(r cnsRecord, zoneID string) provider.Record {
	ttl, _ := strconv.Atoi(r.TTL)
	rec := provider.Record{
		ID:     r.ID,
		ZoneID: zoneID,
		Name:   r.Host,
		Type:   provider.RecordType(strings.ToUpper(r.Type)),
		TTL:    ttl,
		Value:  r.Record,
		Extra:  make(map[string]any),
	}
	if p, _ := strconv.Atoi(r.Priority); p > 0 {
		rec.Priority = p
	}
	if w, _ := strconv.Atoi(r.Weight); w > 0 {
		rec.Extra["weight"] = w
	}
	if port, _ := strconv.Atoi(r.Port); port > 0 {
		rec.Extra["port"] = port
	}
	if r.CaaFlag != "" {
		rec.Extra["caa_flags"] = r.CaaFlag
		rec.Extra["caa_tag"] = r.CaaType
		rec.Value = r.CaaValue
	}
	if r.TlsaUsage != "" {
		rec.Extra["tlsa_usage"] = r.TlsaUsage
		rec.Extra["tlsa_selector"] = r.TlsaSelector
		rec.Extra["tlsa_matching"] = r.TlsaMatchingType
	}
	if r.Algorithm != "" {
		rec.Extra["sshfp_algorithm"] = r.Algorithm
		rec.Extra["sshfp_fp_type"] = r.FpType
	}
	if r.Order != "" {
		if order, _ := strconv.Atoi(r.Order); order > 0 {
			rec.Priority = order
		}
		if pref, _ := strconv.Atoi(r.Pref); pref > 0 {
			rec.Extra["naptr_pref"] = pref
		}
		rec.Extra["naptr_flags"] = r.Flag
		rec.Extra["naptr_service"] = r.Params
		rec.Extra["naptr_regexp"] = r.Regexp
		rec.Value = r.Replace
	}
	return rec
}

type cnsStatusResp struct {
	Status string `json:"status"`
	Desc   string `json:"statusDescription"`
	Data   struct {
		ID int `json:"id"`
	} `json:"data"`
}

func (p *cnsProvider) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	if r.Name == "@" {
		r.Name = ""
	}

	body := p.authFields(map[string]any{
		"domain-name": zoneID,
		"record-type": string(r.Type),
		"host":        r.Name,
		"record":      r.Value,
		"ttl":         r.TTL,
	})
	if r.Priority > 0 {
		body["priority"] = r.Priority
	}
	if w := extraInt(r, "weight"); w > 0 {
		body["weight"] = w
	}
	if port := extraInt(r, "port"); port > 0 {
		body["port"] = port
	}
	if r.Type == provider.RecordTypeCAA {
		body["caa_flag"] = extraStr(r, "caa_flags")
		body["caa_type"] = extraStr(r, "caa_tag")
		body["caa_value"] = r.Value
		body["record"] = ""
	}
	if r.Type == provider.RecordTypeTLSA {
		body["tlsa_usage"] = extraStr(r, "tlsa_usage")
		body["tlsa_selector"] = extraStr(r, "tlsa_selector")
		body["tlsa_matching_type"] = extraStr(r, "tlsa_matching")
	}

	data, err := p.post(ctx, "/dns/add-record.json", body)
	if err != nil {
		return provider.Record{}, fmt.Errorf("cloudns: creating record: %w", err)
	}
	var result cnsStatusResp
	if err := json.Unmarshal(data, &result); err != nil {
		return provider.Record{}, fmt.Errorf("cloudns: parsing create response: %w", err)
	}
	if result.Status != "Success" {
		return provider.Record{}, fmt.Errorf("cloudns: creating record: %s", result.Desc)
	}
	r.ID = strconv.Itoa(result.Data.ID)
	r.ZoneID = zoneID
	return r, nil
}

func (p *cnsProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	body := p.authFields(map[string]any{
		"domain-name": zoneID,
		"record-id":   recordID,
		"host":        r.Name,
		"record":      r.Value,
		"ttl":         r.TTL,
	})
	if r.Priority > 0 {
		body["priority"] = r.Priority
	}
	if w := extraInt(r, "weight"); w > 0 {
		body["weight"] = w
	}
	if port := extraInt(r, "port"); port > 0 {
		body["port"] = port
	}

	data, err := p.post(ctx, "/dns/mod-record.json", body)
	if err != nil {
		return provider.Record{}, fmt.Errorf("cloudns: updating record %s: %w", recordID, err)
	}
	if err := checkErr(data, "/dns/mod-record.json"); err != nil {
		return provider.Record{}, err
	}
	r.ID = recordID
	r.ZoneID = zoneID
	return r, nil
}

func (p *cnsProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	body := p.authFields(map[string]any{
		"domain-name": zoneID,
		"record-id":   recordID,
	})
	data, err := p.post(ctx, "/dns/delete-record.json", body)
	if err != nil {
		return fmt.Errorf("cloudns: deleting record %s: %w", recordID, err)
	}
	return checkErr(data, "/dns/delete-record.json")
}

func extraStr(r provider.Record, key string) string {
	if v, ok := r.Extra[key].(string); ok {
		return v
	}
	return ""
}

func extraInt(r provider.Record, key string) int {
	if v, ok := r.Extra[key]; ok {
		switch vv := v.(type) {
		case int:
			return vv
		case float64:
			return int(vv)
		}
	}
	return 0
}
