package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var enableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable service to start on boot",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnable,
}

func runEnable(cmd *cobra.Command, args []string) error {
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
	if err := mgr.Enable(name); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	svc.Enabled = true
	if err := st.Save(); err != nil {
		return fmt.Errorf("failed to save store: %w", err)
	}

	fmt.Printf("Service '%s' enabled (will start on boot).\n", name)
	return nil
}
