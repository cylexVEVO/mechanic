package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a service",
	Args:  cobra.ExactArgs(1),
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	name := args[0]

	st, err := store.NewForScope(systemScope)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	if _, err := st.Get(name); err != nil {
		return err
	}

	mgr := service.NewForScope(systemScope)
	if err := mgr.Stop(name); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	fmt.Printf("Service '%s' stopped.\n", name)
	return nil
}
