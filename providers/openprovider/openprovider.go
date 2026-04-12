package openprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
)

func init() {
	provider.Register("openprovider", New)
}

const defaultBaseURL = "https://api.openprovider.eu/v1beta"

type Settings struct {
	// Username for dynamic token login.
	Username string `mapstructure:"username"`
	// Password for dynamic token login.
	Password string `mapstructure:"password"`
	// Token is a static Bearer token.  If set, Username/Password are ignored.
	Token string `mapstructure:"token"`
	// BaseURL overrides the default API endpoint (used for testing).
	BaseURL string `mapstructure:"base_url"`
}

type opProvider struct {
	name       string
	settings   Settings
	token      string
	resellerID int
	client     *http.Client
}

func trimDomain(name, zone string) string {
	xname := strings.TrimSuffix(name, zone)
	xname = strings.TrimSuffix(xname, ".")
	if xname == "@" {
		return ""
	}
	return xname
}

func New(cfg config.ProviderConfig) (provider.Provider, error) {
	var s Settings
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &s,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
	})
	if err != nil {
		return nil, fmt.Errorf("openprovider: creating settings decoder: %w", err)
	}
	if err := dec.Decode(cfg.Settings); err != nil {
		return nil, fmt.Errorf("openprovider: decoding settings: %w", err)
	}
	if s.Token == "" && (s.Username == "" || s.Password == "") {
		return nil, fmt.Errorf("openprovider: either settings.token or settings.username+password is required")
	}
	if s.BaseURL == "" {
		s.BaseURL = defaultBaseURL
	}
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")

	p := &opProvider{
		name:     cfg.Name,
		settings: s,
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	// Obtain auth token at construction time.
	if s.Token != "" {
		p.token = s.Token
	} else {
		if err := p.login(context.Background()); err != nil {
			return nil, fmt.Errorf("openprovider: authentication failed: %w", err)
		}
	}
	return p, nil
}

func (p *opProvider) ProviderName() string { return "openprovider" }
func (p *opProvider) FriendlyName() string { return p.name }

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IP       string `json:"ip"`
}

type opEnvelope struct {
	Code int             `json:"code"`
	Desc string          `json:"desc"`
	Data json.RawMessage `json:"data"`
}

func (p *opProvider) login(ctx context.Context) error {
	body := loginRequest{
		Username: p.settings.Username,
		Password: p.settings.Password,
		IP:       "0.0.0.0",
	}
	data, err := p.postJSON(ctx, "/auth/login", body, false)
	if err != nil {
		return err
	}
	var result struct {
		Token      string `json:"token"`
		ResellerID int    `json:"reseller_id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parsing login response: %w", err)
	}
	if result.Token == "" {
		return fmt.Errorf("login response contained no token")
	}
	p.token = result.Token
	p.resellerID = result.ResellerID
	return nil
}

func (p *opProvider) apiRequest(ctx context.Context, method, path string, reqBody any, authed bool) ([]byte, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("openprovider: marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.settings.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authed {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openprovider: HTTP %d from %s %s: %s", resp.StatusCode, method, path, raw)
	}

	var env opEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("openprovider: parsing envelope from %s: %w", path, err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("openprovider: API error %d from %s: %s", env.Code, path, env.Desc)
	}
	return env.Data, nil
}

func (p *opProvider) getJSON(ctx context.Context, path string) ([]byte, error) {
	return p.apiRequest(ctx, http.MethodGet, path, nil, true)
}

func (p *opProvider) postJSON(ctx context.Context, path string, body any, authed bool) ([]byte, error) {
	return p.apiRequest(ctx, http.MethodPost, path, body, authed)
}

func (p *opProvider) putJSON(ctx context.Context, path string, body any) ([]byte, error) {
	return p.apiRequest(ctx, http.MethodPut, path, body, true)
}

func (p *opProvider) ListAccounts(ctx context.Context) ([]provider.Account, error) {
	accountName := p.settings.Username
	if accountName == "" {
		accountName = p.name
	}
	id := p.name
	if p.resellerID != 0 {
		id = fmt.Sprintf("%d", p.resellerID)
	}
	return []provider.Account{
		{ID: id, Name: accountName},
	}, nil
}

type opZone struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type opZoneListData struct {
	Results []opZone `json:"results"`
	Total   int      `json:"total"`
}

func (p *opProvider) ListZones(ctx context.Context, _ string) ([]provider.Zone, error) {
	const limit = 500
	var zones []provider.Zone

	for offset := 0; ; offset += limit {
		path := fmt.Sprintf("/dns/zones?limit=%d&offset=%d", limit, offset)
		data, err := p.getJSON(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("openprovider: listing zones at offset %d: %w", offset, err)
		}
		var listData opZoneListData
		if err := json.Unmarshal(data, &listData); err != nil {
			return nil, fmt.Errorf("openprovider: parsing zone list: %w", err)
		}
		for _, z := range listData.Results {
			zones = append(zones, provider.Zone{
				ID:        z.Name,
				Name:      z.Name,
				AccountID: p.name,
			})
		}
		if len(zones) >= listData.Total || len(listData.Results) < limit {
			break
		}
	}
	return zones, nil
}

type opRecord struct {
	Name  string `json:"name"`
	TTL   int    `json:"ttl"`
	Type  string `json:"type"`
	Value string `json:"value"`
	Prio  int    `json:"prio,omitempty"`
}

type opRecordListData struct {
	Results []opRecord `json:"results"`
	Total   int        `json:"total"`
}

func (p *opProvider) ListRecords(ctx context.Context, zoneID string) ([]provider.Record, error) {
	const limit = 500
	var records []provider.Record

	for offset := 0; ; offset += limit {
		path := fmt.Sprintf("/dns/zones/%s/records?limit=%d&offset=%d", zoneID, limit, offset)
		data, err := p.getJSON(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("openprovider: listing records for %s at offset %d: %w", zoneID, offset, err)
		}
		var listData opRecordListData
		if err := json.Unmarshal(data, &listData); err != nil {
			return nil, fmt.Errorf("openprovider: parsing records for %s: %w", zoneID, err)
		}
		for _, r := range listData.Results {
			records = append(records, opRecordToProvider(r, zoneID))
		}
		if len(records) >= listData.Total || len(listData.Results) < limit {
			break
		}
	}
	return records, nil
}

func opRecordToProvider(r opRecord, zoneID string) provider.Record {
	id, _ := json.Marshal(r)
	return provider.Record{
		ID:       string(id),
		ZoneID:   zoneID,
		Name:     r.Name,
		Type:     provider.RecordType(strings.ToUpper(r.Type)),
		TTL:      r.TTL,
		Value:    r.Value,
		Priority: r.Prio,
		Extra:    make(map[string]any),
	}
}

func providerToOpRecord(r provider.Record, zoneID string) opRecord {
	nname := r.Name
	if nname != zoneID {
		nname = strings.TrimSuffix(r.Name, "."+zoneID)
	}
	return opRecord{
		Name:  nname,
		TTL:   r.TTL,
		Type:  strings.ToLower(string(r.Type)),
		Value: r.Value,
		Prio:  r.Priority,
	}
}

func (p *opProvider) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	newRec := providerToOpRecord(r, zoneID)

	newRec.Name = trimDomain(newRec.Name, zoneID)

	if newRec.TTL < 600 {
		newRec.TTL = 600
	}

	body := map[string]any{
		"name":    zoneID,
		"records": map[string]any{"add": []opRecord{newRec}},
	}
	if _, err := p.putJSON(ctx, "/dns/zones/"+zoneID, body); err != nil {
		return provider.Record{}, fmt.Errorf("openprovider: creating record: %w", err)
	}
	// Encode the new record as the ID so subsequent updates/deletes work.
	id, _ := json.Marshal(newRec)
	r.ID = string(id)
	r.ZoneID = zoneID
	return r, nil
}

func (p *opProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	var original opRecord
	if err := json.Unmarshal([]byte(recordID), &original); err != nil {
		return provider.Record{}, fmt.Errorf("openprovider: decoding original record from ID: %w", err)
	}

	updated := providerToOpRecord(r, zoneID)

	original.Name = trimDomain(original.Name, zoneID)
	updated.Name = trimDomain(updated.Name, zoneID)

	// oRecName := strings.Clone(original.Name)

	// original.Name = strings.TrimSuffix(original.Name, "."+zoneID)
	// updated.Name = strings.TrimSuffix(r.Name, "."+zoneID)

	// originalOriginalName := original.Name

	// if original.Name != zoneID {
	// 	original.Name = strings.TrimSuffix(original.Name, "."+zoneID)
	// }

	// return provider.Record{}, fmt.Errorf("Old value: %s A %s, old value before format: %s A %s, new value %s A %s", string(original.Name), original.Value, string(originalOriginalName), original.Value, string(updated.Name), updated.Value)

	body := map[string]any{
		"name": zoneID,
		"records": map[string]any{
			"update": []map[string]any{
				{
					"original_record": original,
					"record":          updated,
				},
			},
		},
	}
	if _, err := p.putJSON(ctx, "/dns/zones/"+zoneID, body); err != nil {
		return provider.Record{}, fmt.Errorf("openprovider: updating record: %w", err)
	}
	newID, _ := json.Marshal(updated)
	r.ID = string(newID)
	r.ZoneID = zoneID
	return r, nil
}

func (p *opProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	var rec opRecord
	if err := json.Unmarshal([]byte(recordID), &rec); err != nil {
		return fmt.Errorf("openprovider: decoding record from ID: %w", err)
	}

	rec.Name = trimDomain(rec.Name, zoneID)

	body := map[string]any{
		"name":    zoneID,
		"records": map[string]any{"remove": []opRecord{rec}},
	}
	if _, err := p.putJSON(ctx, "/dns/zones/"+zoneID, body); err != nil {
		return fmt.Errorf("openprovider: deleting record: %w", err)
	}
	return nil
}
