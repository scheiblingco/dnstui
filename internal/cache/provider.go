package cache

import (
	"context"

	"github.com/scheiblingco/dnstui/internal/provider"
)

type cachedProvider struct {
	inner provider.Provider
	cache *Cache
}

func Wrap(inner provider.Provider, c *Cache) provider.Provider {
	if c == nil {
		return inner
	}
	return &cachedProvider{inner: inner, cache: c}
}

func WrapAll(providers []provider.Provider, c *Cache) []provider.Provider {
	if c == nil {
		return providers
	}
	out := make([]provider.Provider, len(providers))
	for i, p := range providers {
		out[i] = Wrap(p, c)
	}
	return out
}

func (p *cachedProvider) ProviderName() string { return p.inner.ProviderName() }
func (p *cachedProvider) FriendlyName() string { return p.inner.FriendlyName() }

func (p *cachedProvider) ListAccounts(ctx context.Context) ([]provider.Account, error) {
	key := Key(p.inner.ProviderName(), p.inner.FriendlyName(), "accounts")
	if accounts, ok := p.cache.GetAccounts(key); ok {
		return accounts, nil
	}
	accounts, err := p.inner.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	p.cache.SetAccounts(key, accounts)
	return accounts, nil
}

func (p *cachedProvider) ListZones(ctx context.Context, accountID string) ([]provider.Zone, error) {
	key := Key(p.inner.ProviderName(), p.inner.FriendlyName(), "zones:"+accountID)
	if zones, ok := p.cache.GetZones(key); ok {
		return zones, nil
	}
	zones, err := p.inner.ListZones(ctx, accountID)
	if err != nil {
		return nil, err
	}
	p.cache.SetZones(key, zones)
	return zones, nil
}

func (p *cachedProvider) ListRecords(ctx context.Context, zoneID string) ([]provider.Record, error) {
	return p.inner.ListRecords(ctx, zoneID)
}

func (p *cachedProvider) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	return p.inner.CreateRecord(ctx, zoneID, r)
}

func (p *cachedProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	return p.inner.UpdateRecord(ctx, zoneID, recordID, r)
}

func (p *cachedProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	return p.inner.DeleteRecord(ctx, zoneID, recordID)
}
