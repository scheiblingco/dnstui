package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scheiblingco/dnstui/internal/provider"
)

var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "List configured and registered DNS providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		registered := provider.RegisteredTypes()
		fmt.Printf("Registered provider types: %s\n\n", strings.Join(registered, ", "))

		if cfg == nil || len(cfg.Providers) == 0 {
			fmt.Println("No providers configured.")
			return nil
		}

		fmt.Printf("%-20s %-15s\n", "NAME", "TYPE")
		fmt.Println(strings.Repeat("-", 36))
		for _, p := range cfg.Providers {
			fmt.Printf("%-20s %-15s\n", p.Name, p.Type)
		}
		return nil
	},
}
