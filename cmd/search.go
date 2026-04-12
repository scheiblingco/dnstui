package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/scheiblingco/dnstui/internal/provider"
	"github.com/scheiblingco/dnstui/internal/tui"
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Open the global search view across all providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Providers) == 0 {
			return fmt.Errorf("no providers configured")
		}
		providers, err := provider.NewAll(cfg.Providers)
		if err != nil {
			return fmt.Errorf("initialising providers: %w", err)
		}
		return tui.RunWithSearch(providers)
	},
}
