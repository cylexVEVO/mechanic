package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var disableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable service from starting on boot",
	Args:  cobra.ExactArgs(1),
	RunE:  runDisable,
}

func runDisable(cmd *cobra.Command, args []string) error {
	name := args[0]

	st, err := store.NewForScope(systemScope)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	svc, err := st.Get(name)
	if err != nil {
		return err
	}

	mgr := service.NewForScope(systemScope)
	if err := mgr.Disable(name); err != nil {
		return fmt.Errorf("failed to disable service: %w", err)
	}

	svc.Enabled = false
	if err := st.Save(); err != nil {
		return fmt.Errorf("failed to save store: %w", err)
	}

	fmt.Printf("Service '%s' disabled (will not start on boot).\n", name)
	return nil
}
