package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	// LogLevel controls verbosity: debug, info, warn, error.
	LogLevel string `mapstructure:"log_level" yaml:"log_level"`

	// LogFile is the path where log output is written. If empty, logging is
	// silenced. Useful when log_level is "debug" to avoid corrupting the TUI.
	// Example: /tmp/dnstui.log
	LogFile string `mapstructure:"log_file" yaml:"log_file"`

	// Providers is the ordered list of configured DNS provider accounts.
	Providers []ProviderConfig `mapstructure:"providers" yaml:"providers"`

	// Cache controls caching behaviour.
	Cache CacheConfig `mapstructure:"cache" yaml:"cache"`
}

type ProviderConfig struct {
	// Name is a human-readable alias shown in the TUI (e.g. "CF Personal").
	Name string `mapstructure:"name" yaml:"name"`

	// Type identifies the provider implementation: cloudflare, technitium, cloudns, openprovider.
	Type string `mapstructure:"type" yaml:"type"`

	// Settings is a free-form map of provider-specific credential/endpoint fields.
	// Each provider's New() function decodes this into its own typed struct.
	Settings map[string]any `mapstructure:"settings" yaml:"settings"`
}

type CacheConfig struct {
	// TTLSeconds is how long (in seconds) cached lists are considered fresh.
	// Default: 300 (5 minutes).
	TTLSeconds int `mapstructure:"ttl_seconds" yaml:"ttl_seconds"`

	// DiskCache enables reading/writing the cache to disk across sessions.
	DiskCache bool `mapstructure:"disk_cache" yaml:"disk_cache"`
}

func DefaultConfig() Config {
	return Config{
		LogLevel: "info",
		Cache: CacheConfig{
			TTLSeconds: 300,
			DiskCache:  true,
		},
	}
}

func Load(v *viper.Viper, cfgFile string) (*Config, error) {
	// Apply defaults.
	def := DefaultConfig()
	v.SetDefault("log_level", def.LogLevel)
	v.SetDefault("log_file", def.LogFile)
	v.SetDefault("cache.ttl_seconds", def.Cache.TTLSeconds)
	v.SetDefault("cache.disk_cache", def.Cache.DiskCache)

	// Environment variables.
	v.SetEnvPrefix("DNSTUI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file.
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("dnstui")
		v.SetConfigType("yaml")
		v.AddConfigPath("$HOME/.config/dnstui")
		v.AddConfigPath("$XDG_CONFIG_HOME/dnstui")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		// Missing config file is acceptable — defaults + env vars take over.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.LogLevel)] {
		return fmt.Errorf("log_level must be one of debug, info, warn, error; got %q", c.LogLevel)
	}

	for i, p := range c.Providers {
		if p.Name == "" {
			return fmt.Errorf("providers[%d]: name is required", i)
		}
		if p.Type == "" {
			return fmt.Errorf("providers[%d] (%s): type is required", i, p.Name)
		}
	}

	if c.Cache.TTLSeconds < 0 {
		return fmt.Errorf("cache.ttl_seconds must be >= 0")
	}

	return nil
}
