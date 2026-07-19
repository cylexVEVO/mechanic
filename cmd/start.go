package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a service",
	Args:  cobra.ExactArgs(1),
	RunE:  runStart,
}

func runStart(cmd *cobra.Command, args []string) error {
	name := args[0]

	st, err := store.NewForScope(systemScope)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	if _, err := st.Get(name); err != nil {
		return err
	}

	mgr := service.NewForScope(systemScope)
	if err := mgr.Start(name); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Printf("Service '%s' started.\n", name)
	return nil
}
