package config_test

import (
	"testing"

	"github.com/spf13/viper"

	"github.com/scheiblingco/dnstui/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	def := config.DefaultConfig()
	if def.LogLevel != "info" {
		t.Errorf("expected default log_level 'info', got %q", def.LogLevel)
	}
	if def.Cache.TTLSeconds != 300 {
		t.Errorf("expected default cache.ttl_seconds 300, got %d", def.Cache.TTLSeconds)
	}
	if !def.Cache.DiskCache {
		t.Error("expected default cache.disk_cache to be true")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name: "valid minimal config",
			cfg: config.Config{
				LogLevel: "info",
				Cache:    config.CacheConfig{TTLSeconds: 60},
			},
		},
		{
			name:    "invalid log level",
			cfg:     config.Config{LogLevel: "verbose"},
			wantErr: true,
		},
		{
			name: "provider missing name",
			cfg: config.Config{
				LogLevel: "info",
				Providers: []config.ProviderConfig{
					{Type: "cloudflare"},
				},
			},
			wantErr: true,
		},
		{
			name: "provider missing type",
			cfg: config.Config{
				LogLevel: "info",
				Providers: []config.ProviderConfig{
					{Name: "CF"},
				},
			},
			wantErr: true,
		},
		{
			name: "negative TTL",
			cfg: config.Config{
				LogLevel: "warn",
				Cache:    config.CacheConfig{TTLSeconds: -1},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadNoFile(t *testing.T) {
	v := viper.New()
	// No config file — should succeed using defaults only.
	cfg, err := config.Load(v, "")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected log_level 'info', got %q", cfg.LogLevel)
	}
}
