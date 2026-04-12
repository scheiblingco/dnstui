package provider

import "context"

// Provider is the interface that every DNS provider implementation must satisfy.
// All methods that return lists must return an empty slice (not nil) on success with no items.
type Provider interface {
	// ProviderName returns a stable lowercase identifier for the implementation
	// (e.g. "cloudflare", "technitium").  This matches the "type" field in config.
	ProviderName() string

	// FriendlyName returns the human-readable label configured by the user.
	FriendlyName() string

	// ListAccounts returns all accounts/sub-accounts accessible with the configured credentials.
	ListAccounts(ctx context.Context) ([]Account, error)

	// ListZones returns all zones for the given accountID.
	// Pass an empty accountID on providers that do not have a sub-account concept.
	ListZones(ctx context.Context, accountID string) ([]Zone, error)

	// ListRecords returns all records for the given zoneID.
	ListRecords(ctx context.Context, zoneID string) ([]Record, error)

	// CreateRecord creates a new record in the given zone and returns the
	// created record (with provider-assigned ID and timestamps filled in).
	CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error)

	// UpdateRecord replaces the record identified by recordID with the provided data.
	// Returns the updated record.
	UpdateRecord(ctx context.Context, zoneID, recordID string, r Record) (Record, error)

	// DeleteRecord removes the record identified by recordID from the given zone.
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
}
