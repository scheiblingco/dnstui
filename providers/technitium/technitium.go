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

type Settings struct {
	// BaseURL is the root URL of the Technitium DNS server API (e.g. "http://192.168.1.1:5380").
	BaseURL string `mapstructure:"base_url"`
	// APIKey is the Technitium access token (generated in Settings → API Keys).
	APIKey string `mapstructure:"api_key"`
	// IgnoreTLS optionally disables TLS verification for self-signed certs (not recommended).
	IgnoreTLS bool `mapstructure:"ignore_tls"`
}

type tProvider struct {
	name     string
	settings Settings
	client   *http.Client
}

func init() {
	provider.Register("technitium", New)
}

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

func (p *tProvider) apiURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("token", p.settings.APIKey)
	return p.settings.BaseURL + "/api/" + path + "?" + params.Encode()
}

type tResponse[T any] struct {
	Status   string `json:"status"`  // "ok" or "error"
	Message  string `json:"message"` // error message when status == "error"
	Response T      `json:"response"`
}

func (p *tProvider) doGET(ctx context.Context, apiPath string, params url.Values) ([]byte, error) {
	return p.doRequest(ctx, http.MethodGet, apiPath, params, nil)
}

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

		if resp.StatusCode > 299 {
			return nil, fmt.Errorf("technitium: API request to %s failed with status %d", apiPath, resp.StatusCode)
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

func decodeResponse[T any](raw []byte, apiPath string) (T, error) {
	var env tResponse[T]
	if err := json.Unmarshal(raw, &env); err != nil {
		var zero T
		return zero, fmt.Errorf("technitium: decoding response from %s: %w", apiPath, err)
	}
	if env.Status != "ok" {
		var zero T
		return zero, fmt.Errorf("technitium: %s API error: %s %v", apiPath, env.Message, env)
	}
	return env.Response, nil
}

type tSessionInfo struct {
	Username string `json:"username"`
}

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

type tZonesResponse struct {
	Zones []tZone `json:"zones"`
}

type tZone struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // Primary, Secondary, Stub, Forwarder, …
	Disabled bool   `json:"disabled"`
}

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

func (p *tProvider) ListRecords(ctx context.Context, zoneID string) ([]provider.Record, error) {
	params := url.Values{"zone": {zoneID}, "domain": {zoneID}, "listZone": {"true"}}
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

func (p *tProvider) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	params := sharedToTechParams(zoneID, r, nil)
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

func (p *tProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	// recordID encodes "name/type/oldValue" — see tRecordToShared.
	parts := strings.SplitN(recordID, "\x00", 3)
	if len(parts) != 3 {
		return provider.Record{}, fmt.Errorf("technitium: invalid recordID format")
	}

	searchParams := url.Values{
		"zone":   {zoneID},
		"domain": {parts[0]},
	}

	oldRecord, err := p.doGET(ctx, "zones/records/get", searchParams)
	if err != nil {
		return provider.Record{}, fmt.Errorf("technitium: fetching old record in zone %s: %w", zoneID, err)
	}

	zResp, err := decodeResponse[tRecordsResponse](oldRecord, "zones/records/get")
	if err != nil {
		return provider.Record{}, err
	}

	var oldRec provider.Record
	for _, record := range zResp.Records {
		if record.Name == parts[0] && string(record.Type) == parts[1] {
			oldRec = tRecordToShared(record, zoneID)
			if oldRec.ID == recordID {
				break
			}
		}
	}

	if oldRec.ID == "" {
		return provider.Record{}, fmt.Errorf("technitium: old record not found for update in zone %s", zoneID)
	}

	params := sharedToTechParams(zoneID, r, &oldRec)
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

func sharedToTechParams(zoneID string, r provider.Record, oldRecord *provider.Record) url.Values {
	domainx := r.Name
	if domainx == "@" {
		domainx = zoneID
	}

	if !strings.HasSuffix(domainx, "."+zoneID) && domainx != zoneID {
		domainx = domainx + "." + zoneID
	}

	params := url.Values{
		"zone":   {zoneID},
		"domain": {domainx},

		"type": {string(r.Type)},
		"ttl":  {fmt.Sprintf("%d", r.TTL)},
	}

	rval := r
	if oldRecord != nil {
		rval = *oldRecord
	}

	switch r.Type {
	case provider.RecordTypeA, provider.RecordTypeAAAA:
		params.Set("ipAddress", rval.Value)
		if oldRecord != nil {
			params.Set("newIpAddress", r.Value)
		}
	case provider.RecordTypeCNAME:
		params.Set("cname", r.Value)
	case provider.RecordTypeNS:
		params.Set("nameServer", rval.Value)
		if oldRecord != nil {
			params.Set("newNameServer", r.Value)
		}
	case provider.RecordTypePTR:
		params.Set("ptrName", rval.Value)
		if oldRecord != nil {
			params.Set("newPtrName", r.Value)
		}
	case provider.RecordTypeMX:
		params.Set("exchange", rval.Value)
		params.Set("preference", fmt.Sprintf("%d", rval.Priority))
		if oldRecord != nil {
			params.Set("newExchange", r.Value)
			params.Set("newPreference", fmt.Sprintf("%d", r.Priority))
		}
	case provider.RecordTypeTXT:
		params.Set("text", rval.Value)
		if oldRecord != nil {
			params.Set("newText", r.Value)
		}
	case provider.RecordTypeSRV:
		params.Set("target", rval.Value)
		params.Set("priority", fmt.Sprintf("%d", rval.Priority))
		if w, ok := rval.Extra["weight"].(int); ok {
			params.Set("weight", fmt.Sprintf("%d", w))
		}
		if port, ok := rval.Extra["port"].(int); ok {
			params.Set("port", fmt.Sprintf("%d", port))
		}
		if oldRecord != nil {
			params.Set("newTarget", r.Value)
			params.Set("newPriority", fmt.Sprintf("%d", r.Priority))
			if w, ok := r.Extra["weight"].(int); ok {
				params.Set("newWeight", fmt.Sprintf("%d", w))
			}
			if port, ok := r.Extra["port"].(int); ok {
				params.Set("newPort", fmt.Sprintf("%d", port))
			}
		}
	case provider.RecordTypeCAA:
		params.Set("value", rval.Value)
		if flags, ok := toInt(rval.Extra["caa_flags"]); ok {
			params.Set("flags", fmt.Sprintf("%d", flags))
		}
		if tag, ok := rval.Extra["caa_tag"].(string); ok {
			params.Set("tag", tag)
		}
		if oldRecord != nil {
			params.Set("newValue", r.Value)
			if flags, ok := toInt(r.Extra["caa_flags"]); ok {
				params.Set("newFlags", fmt.Sprintf("%d", flags))
			}
			if tag, ok := r.Extra["caa_tag"].(string); ok {
				params.Set("newTag", tag)
			}
		}
	case provider.RecordTypeTLSA:
		params.Set("certificateAssociationData", rval.Value)
		if v, ok := toInt(rval.Extra["tlsa_usage"]); ok {
			params.Set("certificateUsage", fmt.Sprintf("%d", v))
		}
		if v, ok := toInt(rval.Extra["tlsa_selector"]); ok {
			params.Set("selector", fmt.Sprintf("%d", v))
		}
		if v, ok := toInt(rval.Extra["tlsa_matching"]); ok {
			params.Set("matchingType", fmt.Sprintf("%d", v))
		}
		if oldRecord != nil {
			params.Set("newCertificateAssociationData", r.Value)
			if v, ok := toInt(r.Extra["tlsa_usage"]); ok {
				params.Set("newCertificateUsage", fmt.Sprintf("%d", v))
			}
			if v, ok := toInt(r.Extra["tlsa_selector"]); ok {
				params.Set("newSelector", fmt.Sprintf("%d", v))
			}
			if v, ok := toInt(r.Extra["tlsa_matching"]); ok {
				params.Set("newMatchingType", fmt.Sprintf("%d", v))
			}
		}
	case provider.RecordTypeSSHFP:
		params.Set("fingerprint", rval.Value)
		if v, ok := toInt(rval.Extra["sshfp_algorithm"]); ok {
			params.Set("algorithm", fmt.Sprintf("%d", v))
		}
		if v, ok := toInt(rval.Extra["sshfp_fp_type"]); ok {
			params.Set("fingerprintType", fmt.Sprintf("%d", v))
		}
		if oldRecord != nil {
			params.Set("newFingerprint", r.Value)
			if v, ok := toInt(r.Extra["sshfp_algorithm"]); ok {
				params.Set("newAlgorithm", fmt.Sprintf("%d", v))
			}
			if v, ok := toInt(r.Extra["sshfp_fp_type"]); ok {
				params.Set("newFingerprintType", fmt.Sprintf("%d", v))
			}
		}
	case provider.RecordTypeNAPTR:
		params.Set("replacement", rval.Value)
		params.Set("order", fmt.Sprintf("%d", rval.Priority))
		if v, ok := toInt(rval.Extra["naptr_pref"]); ok {
			params.Set("preference", fmt.Sprintf("%d", v))
		}
		if v, ok := rval.Extra["naptr_flags"].(string); ok {
			params.Set("flags", v)
		}
		if v, ok := rval.Extra["naptr_service"].(string); ok {
			params.Set("services", v)
		}
		if v, ok := rval.Extra["naptr_regexp"].(string); ok {
			params.Set("regexp", v)
		}
		if oldRecord != nil {
			params.Set("newReplacement", r.Value)
			params.Set("newOrder", fmt.Sprintf("%d", r.Priority))
			if v, ok := toInt(r.Extra["naptr_pref"]); ok {
				params.Set("newPreference", fmt.Sprintf("%d", v))
			}
			if v, ok := r.Extra["naptr_flags"].(string); ok {
				params.Set("newFlags", v)
			}
			if v, ok := r.Extra["naptr_service"].(string); ok {
				params.Set("newServices", v)
			}
			if v, ok := r.Extra["naptr_regexp"].(string); ok {
				params.Set("newRegexp", v)
			}
		}
	default:
		params.Set("value", rval.Value)
		if oldRecord != nil {
			params.Set("newValue", r.Value)
		}
	}

	if comment, ok := r.Extra["comment"].(string); ok && comment != "" {
		params.Set("comments", comment)
	}
	return params
}

func toInt(v any) (int, bool) {
	switch vv := v.(type) {
	case int:
		return vv, true
	case float64:
		return int(vv), true
	}
	return 0, false
}

func findOrFallback(records []tRecord, zoneID string, wanted provider.Record) provider.Record {
	for _, r := range records {
		if r.Name == wanted.Name && r.Type == string(wanted.Type) {
			return tRecordToShared(r, zoneID)
		}
	}
	// No exact match — return the record as supplied (no server-assigned ID etc.)
	return wanted
}
