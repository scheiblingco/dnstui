package provider

import (
	"context"
	"fmt"
)

type SearchEntryKind string

const (
	SearchEntryKindAccount SearchEntryKind = "account"
	SearchEntryKindDomain  SearchEntryKind = "domain"
)

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
		}

		// All provider support listing zones without an accountID, so we can fetch them all with a single call.
		zones, err := p.ListZones(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("provider %s: listing zones: %w", p.FriendlyName(), err)
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

	return entries, nil
}
