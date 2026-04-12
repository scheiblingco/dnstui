package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the dnstui version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dnstui %s\n", Version)
	},
}
