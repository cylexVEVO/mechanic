package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var statusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show detailed service status",
	Args:  cobra.ExactArgs(1),
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
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
	status, err := mgr.Status(name)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	enabled := "no"
	if svc.Enabled {
		enabled = "yes"
	}

	pidStr := ""
	if status.PID > 0 {
		pidStr = fmt.Sprintf("%d", status.PID)
	} else {
		pidStr = "-"
	}

	since := status.Since
	if since == "" {
		since = "-"
	}

	fmt.Printf("● %s\n", name)
	fmt.Printf("   State:    %s\n", status.State)
	fmt.Printf("   PID:      %s\n", pidStr)
	fmt.Printf("   Since:    %s\n", since)
	fmt.Printf("   Restart:  %s\n", string(svc.Restart))
	fmt.Printf("   Enabled:  %s\n", enabled)
	scope := "user"
	if systemScope {
		scope = "system"
	}
	fmt.Printf("   Scope:    %s\n", scope)
	fmt.Printf("   Command:  %s\n", svc.Command)
	fmt.Printf("   Dir:      %s\n", svc.WorkingDir)

	if status.Message != "" {
		fmt.Printf("   Message:  %s\n", status.Message)
	}

	return nil
}
