package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all mechanic-managed services",
	Args:  cobra.NoArgs,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	st, err := store.NewForScope(systemScope)
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	services := st.List()
	if len(services) == 0 {
		fmt.Println("No services found. Use 'mechanic create' to add one.")
		return nil
	}

	// Sort by name for stable output
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	// Print header
	fmt.Printf("%-20s %-10s %-8s %-9s %-12s %s\n", "NAME", "STATE", "SCOPE", "ENABLED", "CREATED", "COMMAND")
	fmt.Printf("%-20s %-10s %-8s %-9s %-12s %s\n",
		"--------------------",
		"----------",
		"--------",
		"---------",
		"------------",
		"-------")

	for _, svc := range services {
		mgr := service.NewForScope(systemScope)
		st, err := mgr.Status(svc.Name)
		state := "unknown"
		if err == nil {
			state = st.State
		}

		enabled := "no"
		if svc.Enabled {
			enabled = "yes"
		}
		scope := "user"
		if systemScope {
			scope = "system"
		}

		created := svc.CreatedAt.Format("2006-01-02")
		command := svc.Command
		if len(command) > 40 {
			command = command[:37] + "..."
		}

		fmt.Printf("%-20s %-10s %-8s %-9s %-12s %s\n",
			svc.Name, state, scope, enabled, created, command)
	}

	return nil
}
