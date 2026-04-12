package provider

import (
	"context"
	"fmt"
)

// SearchEntryKind identifies whether a search entry represents an account or a domain.
type SearchEntryKind string

const (
	SearchEntryKindAccount SearchEntryKind = "account"
	SearchEntryKindDomain  SearchEntryKind = "domain"
)

// SearchEntry is a single item in the global search index.
// Entries are pre-fetched at startup and used for Ctrl+K search.
type SearchEntry struct {
	// Kind is either SearchEntryKindAccount or SearchEntryKindDomain.
	Kind SearchEntryKind

	// Label is the pre-formatted display string shown in the search list,
	// e.g. "account: My Account" or "domain: example.com".
	Label string

	// Provider is the owning DNS provider.
	Provider Provider

	// Account is populated when Kind == SearchEntryKindAccount.
	Account Account

	// Zone is populated when Kind == SearchEntryKindDomain.
	Zone Zone
}

// BuildSearchCache fetches all accounts and zones from every provider and
// returns a flat slice of SearchEntry values for use in the global search.
// Each account produces an "account: " entry; each zone produces a "domain: " entry.
func BuildSearchCache(ctx context.Context, providers []Provider) ([]SearchEntry, error) {
	var entries []SearchEntry

	for _, p := range providers {
		accounts, err := p.ListAccounts(ctx)
		if err != nil {
			return nil, fmt.Errorf("provider %s: listing accounts: %w", p.FriendlyName(), err)
		}

		for _, acc := range accounts {
			entries = append(entries, SearchEntry{
				Kind:     SearchEntryKindAccount,
				Label:    "account: " + acc.Name,
				Provider: p,
				Account:  acc,
			})

			zones, err := p.ListZones(ctx, acc.ID)
			if err != nil {
				return nil, fmt.Errorf("provider %s, account %s: listing zones: %w", p.FriendlyName(), acc.Name, err)
			}

			for _, z := range zones {
				entries = append(entries, SearchEntry{
					Kind:     SearchEntryKindDomain,
					Label:    "domain: " + z.Name,
					Provider: p,
					Zone:     z,
				})
			}
		}
	}

	return entries, nil
}
