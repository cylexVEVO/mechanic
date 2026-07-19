package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var restartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Restart a service",
	Args:  cobra.ExactArgs(1),
	RunE:  runRestart,
}

func runRestart(cmd *cobra.Command, args []string) error {
	name := args[0]

	st, err := store.NewForScope(systemScope)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	if _, err := st.Get(name); err != nil {
		return err
	}

	mgr := service.NewForScope(systemScope)
	if err := mgr.Restart(name); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	fmt.Printf("Service '%s' restarted.\n", name)
	return nil
}
