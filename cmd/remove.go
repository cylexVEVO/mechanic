package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a service (stops it first)",
	Args:  cobra.ExactArgs(1),
	RunE:  runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	st, err := store.NewForScope(systemScope)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	if _, err := st.Get(name); err != nil {
		return err
	}

	mgr := service.NewForScope(systemScope)

	// Stop first, ignore errors
	mgr.Stop(name) //nolint:errcheck

	if err := mgr.Uninstall(name); err != nil {
		return fmt.Errorf("failed to uninstall service: %w", err)
	}

	if err := st.Remove(name); err != nil {
		return fmt.Errorf("failed to remove from store: %w", err)
	}
	if err := st.Save(); err != nil {
		return fmt.Errorf("failed to save store: %w", err)
	}

	fmt.Printf("Service '%s' removed.\n", name)
	return nil
}
