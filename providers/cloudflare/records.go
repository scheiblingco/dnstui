package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/scheiblingco/dnstui/internal/provider"
)

func (p *cfProvider) ListAccounts(ctx context.Context) ([]provider.Account, error) {
	cfAccounts, err := getAllPages[cfAccount](ctx, p, "/accounts")
	if err != nil {
		return nil, fmt.Errorf("cloudflare: listing accounts: %w", err)
	}

	accounts := make([]provider.Account, 0, len(cfAccounts))
	for _, a := range cfAccounts {
		accounts = append(accounts, provider.Account{
			ID:   a.ID,
			Name: a.Name,
		})
	}
	return accounts, nil
}

func (p *cfProvider) ListZones(ctx context.Context, accountID string) ([]provider.Zone, error) {
	path := "/zones"
	if accountID != "" {
		path += "?account.id=" + url.QueryEscape(accountID)
	}

	cfZones, err := getAllPages[cfZone](ctx, p, path)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: listing zones: %w", err)
	}

	zones := make([]provider.Zone, 0, len(cfZones))
	for _, z := range cfZones {
		zones = append(zones, provider.Zone{
			ID:        z.ID,
			Name:      z.Name,
			AccountID: z.Account.ID,
		})
	}
	return zones, nil
}

func (p *cfProvider) ListRecords(ctx context.Context, zoneID string) ([]provider.Record, error) {
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records"

	cfRecords, err := getAllPages[cfRecord](ctx, p, path)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: listing records for zone %s: %w", zoneID, err)
	}

	records := make([]provider.Record, 0, len(cfRecords))
	for _, r := range cfRecords {
		records = append(records, toRecord(r))
	}
	return records, nil
}

func (p *cfProvider) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	req := toRequest(r)
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records"

	created, err := doJSON[cfRecord](ctx, p, http.MethodPost, path, req)
	if err != nil {
		return provider.Record{}, fmt.Errorf("cloudflare: creating record in zone %s: %w", zoneID, err)
	}
	return toRecord(created), nil
}

func (p *cfProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	req := toRequest(r)
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)

	updated, err := doJSON[cfRecord](ctx, p, http.MethodPut, path, req)
	if err != nil {
		return provider.Record{}, fmt.Errorf("cloudflare: updating record %s in zone %s: %w", recordID, zoneID, err)
	}
	return toRecord(updated), nil
}

func (p *cfProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)

	b, status, err := p.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("cloudflare: deleting record %s in zone %s: %w", recordID, zoneID, err)
	}
	if status >= 400 {
		var resp cfResponse[json.RawMessage]
		if jsonErr := json.Unmarshal(b, &resp); jsonErr == nil && !resp.Success {
			return fmt.Errorf("cloudflare: deleting record %s: %w", recordID, apiErrors(resp.Errors))
		}
		return fmt.Errorf("cloudflare: deleting record %s: HTTP %d", recordID, status)
	}
	return nil
}
