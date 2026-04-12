package cloudflare

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// cfResponse is the standard Cloudflare API envelope.
type cfResponse[T any] struct {
	Success    bool          `json:"success"`
	Errors     []cfAPIError  `json:"errors"`
	Result     T             `json:"result"`
	ResultInfo *cfResultInfo `json:"result_info,omitempty"`
}

// cfAPIError is a single error entry returned by the Cloudflare API.
type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// cfResultInfo contains pagination metadata.
type cfResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
}

// cfAccount represents a Cloudflare account.
type cfAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// cfZoneAccount is the embedded account object within a zone.
type cfZoneAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// cfZone represents a Cloudflare zone (domain).
type cfZone struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Account cfZoneAccount `json:"account"`
}

// cfRecord represents a Cloudflare DNS record as returned by the API.
type cfRecord struct {
	ID         string          `json:"id"`
	ZoneID     string          `json:"zone_id"`
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Content    string          `json:"content"`
	Proxiable  bool            `json:"proxiable"`
	Proxied    *bool           `json:"proxied,omitempty"`
	TTL        int             `json:"ttl"`
	Priority   *uint16         `json:"priority,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	Comment    string          `json:"comment,omitempty"`
	CreatedOn  time.Time       `json:"created_on"`
	ModifiedOn time.Time       `json:"modified_on"`
}

// cfRecordRequest is the payload used for create (POST) and replace (PUT) operations.
type cfRecordRequest struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`
	Content  string          `json:"content,omitempty"`
	TTL      int             `json:"ttl"`
	Proxied  *bool           `json:"proxied,omitempty"`
	Priority *uint16         `json:"priority,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Comment  string          `json:"comment,omitempty"`
}

// apiErrors converts a slice of cfAPIError into a single Go error.
func apiErrors(errs []cfAPIError) error {
	if len(errs) == 0 {
		return fmt.Errorf("unknown API error")
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = fmt.Sprintf("code %d: %s", e.Code, e.Message)
	}
	return fmt.Errorf("%s", strings.Join(msgs, "; "))
}
