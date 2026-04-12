// Package cmd wires the Cobra CLI and is the entry point for all subcommands.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/scheiblingco/dnstui/internal/cache"
	"github.com/scheiblingco/dnstui/internal/config"
	"github.com/scheiblingco/dnstui/internal/provider"
	"github.com/scheiblingco/dnstui/internal/tui"

	// Provider packages self-register via init(). Add new providers here.
	_ "github.com/scheiblingco/dnstui/providers/cloudflare"
	_ "github.com/scheiblingco/dnstui/providers/cloudns"
	_ "github.com/scheiblingco/dnstui/providers/openprovider"
	_ "github.com/scheiblingco/dnstui/providers/technitium"
)

var (
	cfgFile string
	cfg     *config.Config
	v       = viper.New()
)

// rootCmd is the base command; running dnstui with no subcommand launches the TUI.
var rootCmd = &cobra.Command{
	Use:   "dnstui",
	Short: "A terminal UI for managing DNS records across multiple providers",
	Long: `dnstui is a terminal-based DNS management tool.
It supports Cloudflare, Technitium, ClouDNS, and Openprovider out of the box.

Configuration can be provided via a YAML file, environment variables (DNSTUI_ prefix), or CLI flags.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		loaded, err := config.Load(v, cfgFile)
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}
		cfg = loaded
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(cfg.Providers) == 0 {
			return fmt.Errorf("no providers configured — add at least one provider to your config file")
		}
		providers, err := provider.NewAll(cfg.Providers)
		if err != nil {
			return fmt.Errorf("initialising providers: %w", err)
		}
		c, err := cache.New(cfg.Cache)
		if err != nil {
			return fmt.Errorf("initialising cache: %w", err)
		}
		defer func() { _ = c.Save() }()
		providers = cache.WrapAll(providers, c)
		return tui.Run(providers)
	},
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "path to config file (default: $HOME/.config/dnstui/dnstui.yaml)")
	rootCmd.PersistentFlags().StringP("log-level", "l", "", "log level: debug, info, warn, error (overrides config)")

	// Bind the log-level flag to Viper so it participates in the precedence chain.
	_ = v.BindPFlag("log_level", rootCmd.PersistentFlags().Lookup("log-level"))

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(providersCmd)
	rootCmd.AddCommand(searchCmd)
}
