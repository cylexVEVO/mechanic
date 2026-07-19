package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var (
	logsLines  int
	logsFollow bool
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Show service logs",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, "Number of lines to show")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
}

func runLogs(cmd *cobra.Command, args []string) error {
	name := args[0]

	st, err := store.NewForScope(systemScope)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	if _, err := st.Get(name); err != nil {
		return err
	}

	mgr := service.NewForScope(systemScope)
	if err := mgr.Logs(name, logsLines, logsFollow); err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	return nil
}
