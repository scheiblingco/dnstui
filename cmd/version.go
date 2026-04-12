package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/scheiblingco/dnstui/cmd.Version=x.y.z".
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the dnstui version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dnstui %s\n", Version)
	},
}
