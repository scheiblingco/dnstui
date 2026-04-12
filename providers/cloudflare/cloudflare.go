package cloudflare

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
)

const defaultAPIBase = "https://api.cloudflare.com/client/v4"

type Settings struct {
	// APIToken is a scoped Cloudflare API token (preferred auth method).
	APIToken string `mapstructure:"api_token"`
	// APIKey is the legacy "Global API Key" (requires APIEmail too).
	APIKey string `mapstructure:"api_key"`
	// APIEmail is required when using APIKey auth.
	APIEmail string `mapstructure:"api_email"`
	// BaseURL overrides the default API endpoint. Intended for testing only.
	BaseURL string `mapstructure:"base_url"`
}

type cfProvider struct {
	name     string
	settings Settings
	client   *http.Client
}

func init() {
	provider.Register("cloudflare", New)
}

func New(cfg config.ProviderConfig) (provider.Provider, error) {
	var s Settings
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &s,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
	})
	if err != nil {
		return nil, fmt.Errorf("cloudflare: creating settings decoder: %w", err)
	}
	if err := dec.Decode(cfg.Settings); err != nil {
		return nil, fmt.Errorf("cloudflare: decoding settings: %w", err)
	}

	if s.APIToken == "" && (s.APIKey == "" || s.APIEmail == "") {
		return nil, fmt.Errorf("cloudflare: settings must include api_token, or both api_key and api_email")
	}
	if s.BaseURL == "" {
		s.BaseURL = defaultAPIBase
	}

	return &cfProvider{
		name:     cfg.Name,
		settings: s,
		client:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (p *cfProvider) ProviderName() string { return "cloudflare" }
func (p *cfProvider) FriendlyName() string { return p.name }
