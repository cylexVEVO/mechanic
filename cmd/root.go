package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var systemScope bool

var rootCmd = &cobra.Command{
	Use:   "mechanic",
	Short: "Manage services for ad-hoc commands",
	Long: `mechanic lets you create and manage services for ad-hoc commands.
It uses native service managers (systemd or OpenRC on Linux, launchd on macOS)
and manages user services by default. Use --system for system-wide services.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&systemScope, "system", false, "Manage system-wide services (requires root)")
	rootCmd.AddCommand(
		createCmd,
		listCmd,
		startCmd,
		stopCmd,
		restartCmd,
		removeCmd,
		statusCmd,
		logsCmd,
		enableCmd,
		disableCmd,
		tuiCmd,
	)
}
