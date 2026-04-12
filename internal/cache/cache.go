package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
)

type accountEntry struct {
	Data     []provider.Account `json:"data"`
	CachedAt time.Time          `json:"cached_at"`
}

type zoneEntry struct {
	Data     []provider.Zone `json:"data"`
	CachedAt time.Time       `json:"cached_at"`
}

type diskData struct {
	Accounts map[string]accountEntry `json:"accounts"`
	Zones    map[string]zoneEntry    `json:"zones"`
}

type Cache struct {
	mu       sync.Mutex
	ttl      time.Duration
	diskPath string // empty if disk caching is disabled
	accounts map[string]accountEntry
	zones    map[string]zoneEntry
}

func New(cfg config.CacheConfig) (*Cache, error) {
	c := &Cache{
		ttl:      time.Duration(cfg.TTLSeconds) * time.Second,
		accounts: make(map[string]accountEntry),
		zones:    make(map[string]zoneEntry),
	}
	if cfg.DiskCache {
		path, err := diskCachePath()
		if err != nil {
			return nil, err
		}
		c.diskPath = path
		_ = c.load() // missing / corrupt file is not fatal
	}
	return c, nil
}

func diskCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache: resolving user cache directory: %w", err)
	}
	return filepath.Join(dir, "dnstui", "cache.json"), nil
}

func (c *Cache) GetAccounts(key string) ([]provider.Account, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.accounts[key]
	if !ok {
		return nil, false
	}
	if c.ttl > 0 && time.Since(e.CachedAt) > c.ttl {
		return nil, false
	}
	return e.Data, true
}

func (c *Cache) SetAccounts(key string, accounts []provider.Account) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accounts[key] = accountEntry{Data: accounts, CachedAt: time.Now()}
}

func (c *Cache) GetZones(key string) ([]provider.Zone, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.zones[key]
	if !ok {
		return nil, false
	}
	if c.ttl > 0 && time.Since(e.CachedAt) > c.ttl {
		return nil, false
	}
	return e.Data, true
}

func (c *Cache) SetZones(key string, zones []provider.Zone) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.zones[key] = zoneEntry{Data: zones, CachedAt: time.Now()}
}

func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accounts = make(map[string]accountEntry)
	c.zones = make(map[string]zoneEntry)
}

func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.diskPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.diskPath), 0o700); err != nil {
		return fmt.Errorf("cache: creating cache directory: %w", err)
	}
	data := diskData{
		Accounts: c.accounts,
		Zones:    c.zones,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("cache: marshaling cache: %w", err)
	}
	if err := os.WriteFile(c.diskPath, b, 0o600); err != nil {
		return fmt.Errorf("cache: writing cache file: %w", err)
	}
	return nil
}

func (c *Cache) load() error {
	b, err := os.ReadFile(c.diskPath)
	if err != nil {
		return nil
	}
	var data diskData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil
	}
	if data.Accounts != nil {
		for k, v := range data.Accounts {
			if c.ttl == 0 || time.Since(v.CachedAt) <= c.ttl {
				c.accounts[k] = v
			}
		}
	}
	if data.Zones != nil {
		for k, v := range data.Zones {
			if c.ttl == 0 || time.Since(v.CachedAt) <= c.ttl {
				c.zones[k] = v
			}
		}
	}
	return nil
}

func Key(providerName, friendlyName, suffix string) string {
	return providerName + ":" + friendlyName + ":" + suffix
}
