package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"mechanic/service"
	"mechanic/store"
)

func newEditCmd() *cobra.Command {
	var command, description, dir, restart string
	var env []string
	var clearEnv, restartNow bool
	cmd := &cobra.Command{
		Use: "edit <name>", Short: "Edit an existing managed service",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clearEnv && cmd.Flags().Changed("env") {
				return fmt.Errorf("--env and --clear-env cannot be used together")
			}
			changed := false
			for _, flag := range []string{"command", "description", "dir", "restart", "env", "clear-env"} {
				changed = changed || cmd.Flags().Changed(flag)
			}
			if !changed {
				return fmt.Errorf("specify at least one field to edit; see 'mechanic edit --help'")
			}
			st, err := store.NewForScope(systemScope)
			if err != nil {
				return err
			}
			existing, err := st.Get(args[0])
			if err != nil {
				return err
			}
			updated := *existing
			if cmd.Flags().Changed("command") {
				updated.Command = command
			}
			if cmd.Flags().Changed("description") {
				updated.Description = description
			}
			if cmd.Flags().Changed("dir") {
				updated.WorkingDir = dir
			}
			if cmd.Flags().Changed("restart") {
				updated.Restart = store.RestartPolicy(restart)
			}
			if cmd.Flags().Changed("env") {
				updated.Env = env
			}
			if clearEnv {
				updated.Env = nil
			}
			if _, err := saveServiceEdit(st, service.NewForScope(systemScope), &updated, restartNow); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Service '%s' updated successfully.\n", updated.Name)
			if restartNow {
				fmt.Fprintln(cmd.OutOrStdout(), "Service restarted.")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Restart the service to apply changes to a running process.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&command, "command", "", "Service command")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Service description (empty clears it)")
	cmd.Flags().StringVar(&dir, "dir", "", "Working directory (empty resets to home)")
	cmd.Flags().StringVarP(&restart, "restart", "r", "", "Restart policy: always|on-failure|never")
	cmd.Flags().StringArrayVar(&env, "env", nil, "Replace environment with KEY=VAL entries (repeatable)")
	cmd.Flags().BoolVar(&clearEnv, "clear-env", false, "Remove all environment variables")
	cmd.Flags().BoolVar(&restartNow, "restart-now", false, "Restart service after saving")
	return cmd
}

// The result is non-nil when saved, even if the subsequent restart fails.
func saveServiceEdit(st *store.Store, mgr service.Manager, updated *store.Service, restartNow bool) (*store.Service, error) {
	existing, err := st.Get(updated.Name)
	if err != nil {
		return nil, err
	}
	next := *existing
	next.Command, next.Description, next.WorkingDir, next.Restart = updated.Command, updated.Description, updated.WorkingDir, updated.Restart
	next.Env = append([]string(nil), updated.Env...)
	dir, policy, err := validateServiceOptions(createOptions{name: next.Name, command: next.Command, description: next.Description, workingDir: next.WorkingDir, restart: string(next.Restart), env: next.Env})
	if err != nil {
		return nil, err
	}
	next.WorkingDir, next.Restart = dir, policy
	if err := mgr.Install(&next); err != nil {
		rollbackErr := mgr.Install(existing)
		if rollbackErr != nil {
			return nil, fmt.Errorf("failed to update service: %v; restoring previous definition also failed: %w", err, rollbackErr)
		}
		return nil, fmt.Errorf("failed to update service: %w", err)
	}
	previous := *existing
	*existing = next
	if err := st.Save(); err != nil {
		*existing = previous
		if rollbackErr := mgr.Install(&previous); rollbackErr != nil {
			return nil, fmt.Errorf("failed to save edits: %v; restoring previous definition also failed: %w", err, rollbackErr)
		}
		return nil, fmt.Errorf("failed to save edits: %w", err)
	}
	if restartNow {
		if err := mgr.Restart(next.Name); err != nil {
			return existing, fmt.Errorf("service updated but failed to restart: %w", err)
		}
	}
	return existing, nil
}
