package provider

import "time"

type RecordType string

const (
	RecordTypeA     RecordType = "A"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeCNAME RecordType = "CNAME"
	RecordTypeMX    RecordType = "MX"
	RecordTypeTXT   RecordType = "TXT"
	RecordTypeNS    RecordType = "NS"
	RecordTypeSRV   RecordType = "SRV"
	RecordTypeCAA   RecordType = "CAA"
	RecordTypePTR   RecordType = "PTR"
	RecordTypeSOA   RecordType = "SOA"
	RecordTypeTLSA  RecordType = "TLSA"
	RecordTypeSSHFP RecordType = "SSHFP"
	RecordTypeNAPTR RecordType = "NAPTR"
)

type Account struct {
	// ID is the provider-internal account identifier.
	ID string

	// Name is a human-readable label (account email, sub-account name, etc.).
	Name string
}

type Zone struct {
	// ID is the provider-internal zone identifier.
	ID string

	// Name is the zone's domain name (e.g. "example.com").
	Name string

	// AccountID links the zone back to its parent account.
	AccountID string
}

type Record struct {
	// ID is the provider-internal record identifier.
	ID string

	// ZoneID links the record to its parent zone.
	ZoneID string

	// Name is the hostname/label (relative or FQDN depending on provider).
	Name string

	// Type is the DNS record type.
	Type RecordType

	// TTL is time-to-live in seconds. 0 means "use provider default / automatic".
	TTL int

	// Value is the primary record value:
	//   A/AAAA → IP address string
	//   CNAME/NS/MX → target hostname
	//   TXT → text content
	//   SRV → "priority weight port target"
	Value string

	// Priority is used for MX and SRV records.
	Priority int

	// Extra carries provider-specific fields that don't fit the common model
	// (e.g. Cloudflare proxied status, Technitium comments).
	// Keys are lower-snake-case strings; values are provider-defined.
	Extra map[string]any

	// CreatedAt and UpdatedAt are set when available from the provider API.
	CreatedAt time.Time
	UpdatedAt time.Time
}
