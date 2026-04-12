package cache

// Package cache provides an in-memory TTL cache for provider account and zone
// lists, with optional JSON persistence to disk between sessions.
//
// Usage:
//
//	c, _ := cache.New(cfg.Cache)
//	defer c.Save()
//	providers = cache.WrapAll(providers, c)

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

// ── Entry types ──────────────────────────────────────────────────────────────

type accountEntry struct {
	Data     []provider.Account `json:"data"`
	CachedAt time.Time          `json:"cached_at"`
}

type zoneEntry struct {
	Data     []provider.Zone `json:"data"`
	CachedAt time.Time       `json:"cached_at"`
}

// diskData is the top-level structure serialised to the on-disk cache file.
type diskData struct {
	Accounts map[string]accountEntry `json:"accounts"`
	Zones    map[string]zoneEntry    `json:"zones"`
}

// ── Cache ────────────────────────────────────────────────────────────────────

// Cache is a thread-safe in-memory TTL cache for provider list results.
// When DiskCache is enabled, it is serialised to disk on Save() and restored
// on startup via New().
type Cache struct {
	mu       sync.Mutex
	ttl      time.Duration
	diskPath string // empty if disk caching is disabled
	accounts map[string]accountEntry
	zones    map[string]zoneEntry
}

// New creates a Cache from the given CacheConfig.  If DiskCache is true and a
// persisted cache file exists, any entries that are still within the TTL window
// are preloaded into memory.
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

// ── Read / write ─────────────────────────────────────────────────────────────

// GetAccounts returns cached accounts for key if the entry exists and is within
// the TTL.  A zero TTL means entries never expire.
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

// SetAccounts stores accounts under key with the current timestamp.
func (c *Cache) SetAccounts(key string, accounts []provider.Account) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accounts[key] = accountEntry{Data: accounts, CachedAt: time.Now()}
}

// GetZones returns cached zones for key if the entry exists and is within the TTL.
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

// SetZones stores zones under key with the current timestamp.
func (c *Cache) SetZones(key string, zones []provider.Zone) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.zones[key] = zoneEntry{Data: zones, CachedAt: time.Now()}
}

// Invalidate clears all in-memory cache entries.  Call Save afterwards to
// persist the cleared state to disk.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accounts = make(map[string]accountEntry)
	c.zones = make(map[string]zoneEntry)
}

// ── Persistence ──────────────────────────────────────────────────────────────

// Save writes the current cache state to disk.  It is a no-op when disk
// caching is disabled.
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

// load reads the on-disk cache file and imports entries that are still within
// the TTL window.  Errors (missing file, JSON errors) are silently ignored.
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

// ── Helpers ───────────────────────────────────────────────────────────────────

// Key returns a canonical cache key for the given provider/account combination.
func Key(providerName, friendlyName, suffix string) string {
	return providerName + ":" + friendlyName + ":" + suffix
}
