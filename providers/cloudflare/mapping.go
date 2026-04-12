package cloudflare

import (
	"encoding/json"

	"github.com/scheiblingco/dnstui/internal/provider"
)

func parseCFDataIntoRecord(rec *provider.Record, recType string, data json.RawMessage) {
	var blob map[string]any
	if err := json.Unmarshal(data, &blob); err != nil {
		return
	}
	intv := func(key string) int {
		if v, ok := blob[key]; ok {
			switch vv := v.(type) {
			case float64:
				return int(vv)
			case int:
				return vv
			}
		}
		return 0
	}
	strv := func(key string) string {
		if v, ok := blob[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	switch recType {
	case "SRV":
		rec.Value = strv("target")
		if p := intv("priority"); p > 0 {
			rec.Priority = p
		}
		rec.Extra["weight"] = intv("weight")
		rec.Extra["port"] = intv("port")
	case "CAA":
		rec.Value = strv("value")
		rec.Extra["caa_flags"] = intv("flags")
		rec.Extra["caa_tag"] = strv("tag")
	case "TLSA":
		rec.Value = strv("certificate")
		rec.Extra["tlsa_usage"] = intv("usage")
		rec.Extra["tlsa_selector"] = intv("selector")
		rec.Extra["tlsa_matching"] = intv("matching_type")
	case "SSHFP":
		rec.Value = strv("fingerprint")
		rec.Extra["sshfp_algorithm"] = intv("algorithm")
		rec.Extra["sshfp_fp_type"] = intv("type")
	case "NAPTR":
		rec.Value = strv("replacement")
		rec.Priority = intv("order")
		rec.Extra["naptr_pref"] = intv("preference")
		rec.Extra["naptr_flags"] = strv("flags")
		rec.Extra["naptr_service"] = strv("service")
		rec.Extra["naptr_regexp"] = strv("regex")
	}
}

func buildCFDataFromExtra(r provider.Record) json.RawMessage {
	var blob map[string]any
	switch r.Type {
	case provider.RecordTypeSRV:
		blob = map[string]any{
			"target":   r.Value,
			"priority": r.Priority,
			"weight":   extraInt(r, "weight"),
			"port":     extraInt(r, "port"),
		}
	case provider.RecordTypeCAA:
		blob = map[string]any{
			"flags": extraInt(r, "caa_flags"),
			"tag":   extraStr(r, "caa_tag"),
			"value": r.Value,
		}
	case provider.RecordTypeTLSA:
		blob = map[string]any{
			"usage":         extraInt(r, "tlsa_usage"),
			"selector":      extraInt(r, "tlsa_selector"),
			"matching_type": extraInt(r, "tlsa_matching"),
			"certificate":   r.Value,
		}
	case provider.RecordTypeSSHFP:
		blob = map[string]any{
			"algorithm":   extraInt(r, "sshfp_algorithm"),
			"type":        extraInt(r, "sshfp_fp_type"),
			"fingerprint": r.Value,
		}
	case provider.RecordTypeNAPTR:
		blob = map[string]any{
			"order":       r.Priority,
			"preference":  extraInt(r, "naptr_pref"),
			"flags":       extraStr(r, "naptr_flags"),
			"service":     extraStr(r, "naptr_service"),
			"regex":       extraStr(r, "naptr_regexp"),
			"replacement": r.Value,
		}
	default:
		return nil
	}
	b, _ := json.Marshal(blob)
	return b
}

func extraInt(r provider.Record, key string) int {
	if v, ok := r.Extra[key]; ok {
		switch vv := v.(type) {
		case int:
			return vv
		case float64:
			return int(vv)
		}
	}
	return 0
}

func extraStr(r provider.Record, key string) string {
	if v, ok := r.Extra[key].(string); ok {
		return v
	}
	return ""
}

var recordTypesWithData = map[string]bool{
	"SRV":    true,
	"CAA":    true,
	"TLSA":   true,
	"SSHFP":  true,
	"NAPTR":  true,
	"LOC":    true,
	"DNSKEY": true,
	"DS":     true,
	"URI":    true,
}

var recordTypesWithPriority = map[string]bool{
	"MX":    true,
	"SRV":   true,
	"NAPTR": true,
	"URI":   true,
}

func toRecord(r cfRecord) provider.Record {
	rec := provider.Record{
		ID:        r.ID,
		ZoneID:    r.ZoneID,
		Name:      r.Name,
		Type:      provider.RecordType(r.Type),
		TTL:       r.TTL,
		Value:     r.Content,
		Extra:     make(map[string]any),
		CreatedAt: r.CreatedOn,
		UpdatedAt: r.ModifiedOn,
	}

	// Cloudflare represents "automatic TTL" as 1. Map it to 0 (provider convention).
	if r.TTL == 1 {
		rec.TTL = 0
	}

	// Priority (MX, SRV, …).
	if r.Priority != nil {
		rec.Priority = int(*r.Priority)
	}

	// Cloudflare-specific extra fields.
	if r.Proxied != nil {
		rec.Extra["proxied"] = *r.Proxied
	}
	rec.Extra["proxiable"] = r.Proxiable

	if r.Comment != "" {
		rec.Extra["comment"] = r.Comment
	}

	// For record types that use a structured "data" object (SRV, CAA, TLSA, etc.),
	// the API returns content as a human-readable string but the canonical source
	// of truth is "data".  We parse individual fields into Extra so the form
	// sub-field system can populate the right inputs.  The raw blob is kept in
	// Extra["data"] as a fallback for round-tripping.
	if len(r.Data) > 0 && string(r.Data) != "null" {
		rec.Extra["data"] = string(r.Data)
		parseCFDataIntoRecord(&rec, r.Type, r.Data)
	}

	return rec
}

func toRequest(r provider.Record) cfRecordRequest {
	req := cfRecordRequest{
		Type:    string(r.Type),
		Name:    r.Name,
		Content: r.Value,
		TTL:     r.TTL,
	}

	// Map provider convention (0 = automatic) back to CF's representation (1).
	if req.TTL == 0 {
		req.TTL = 1
	}

	// Priority for record types that require it.
	if recordTypesWithPriority[string(r.Type)] {
		prio := uint16(r.Priority) //nolint:gosec // priority fits uint16
		req.Priority = &prio
	}

	// Cloudflare-specific fields from Extra.
	if proxied, ok := r.Extra["proxied"].(bool); ok {
		req.Proxied = &proxied
	}
	if comment, ok := r.Extra["comment"].(string); ok {
		req.Comment = comment
	}

	// For structured record types, prefer the stored blob for exact round-trips
	// (e.g. when editing an existing record fetched from CF).  For newly created
	// records from the form, build the data JSON from individual Extra fields.
	if recordTypesWithData[string(r.Type)] {
		if dataStr, ok := r.Extra["data"].(string); ok && dataStr != "" {
			req.Data = json.RawMessage(dataStr)
		} else if built := buildCFDataFromExtra(r); built != nil {
			req.Data = built
		}
		req.Content = "" // CF ignores content when data is set
	}

	return req
}
