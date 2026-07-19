package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var (
	createDescription string
	createRestart     string
	createEnv         []string
	createDir         string
	createStart       bool
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)
var envRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*=`)

var createCmd = &cobra.Command{
	Use:   "create <name> <command>",
	Short: "Create a new managed service",
	Args:  cobra.ExactArgs(2),
	RunE:  runCreate,
}

func init() {
	createCmd.Flags().StringVarP(&createDescription, "description", "d", "", "Service description")
	createCmd.Flags().StringVarP(&createRestart, "restart", "r", "on-failure", "Restart policy: always|on-failure|never")
	createCmd.Flags().StringArrayVar(&createEnv, "env", nil, "Environment variable KEY=VAL (repeatable)")
	createCmd.Flags().StringVar(&createDir, "dir", "", "Working directory (defaults to home dir)")
	createCmd.Flags().BoolVar(&createStart, "start", false, "Start service immediately after creating")
}

func runCreate(cmd *cobra.Command, args []string) error {
	options := createOptions{
		name:        args[0],
		command:     args[1],
		description: createDescription,
		workingDir:  createDir,
		restart:     createRestart,
		env:         createEnv,
		start:       createStart,
		system:      systemScope,
	}
	if _, err := createManagedService(options); err != nil {
		return err
	}

	fmt.Printf("Service '%s' created successfully.\n", options.name)
	if options.start {
		fmt.Printf("Service '%s' started.\n", options.name)
	}
	return nil
}

type createOptions struct {
	name        string
	command     string
	description string
	workingDir  string
	restart     string
	env         []string
	start       bool
	system      bool
}

func createManagedService(options createOptions) (*store.Service, error) {
	name := options.name

	// Validate name
	if name == "" {
		return nil, fmt.Errorf("service name is required")
	}
	if len(name) > 63 {
		return nil, fmt.Errorf("service name must be 63 characters or fewer")
	}
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("service name must contain only alphanumeric characters and hyphens, and must start with a letter or digit")
	}
	if strings.TrimSpace(options.command) == "" {
		return nil, fmt.Errorf("service command is required")
	}
	for _, entry := range options.env {
		if !envRe.MatchString(entry) {
			return nil, fmt.Errorf("invalid environment variable %q: expected KEY=VALUE", entry)
		}
	}

	// Validate restart policy
	var restartPolicy store.RestartPolicy
	switch options.restart {
	case "always":
		restartPolicy = store.RestartAlways
	case "on-failure":
		restartPolicy = store.RestartOnFailure
	case "never":
		restartPolicy = store.RestartNever
	default:
		return nil, fmt.Errorf("invalid restart policy '%s': must be always, on-failure, or never", options.restart)
	}

	// Default working dir to home
	workDir := options.workingDir
	if workDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not determine home directory: %w", err)
		}
		workDir = home
	}

	// Load store
	st, err := store.NewForScope(options.system)
	if err != nil {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	// Check not already exists
	if _, err := st.Get(name); err == nil {
		return nil, fmt.Errorf("service '%s' already exists. Remove it first or use the other scope", name)
	}

	svc := &store.Service{
		Name:        name,
		Command:     options.command,
		Description: options.description,
		WorkingDir:  workDir,
		Restart:     restartPolicy,
		Env:         options.env,
		Enabled:     false,
		System:      options.system,
		CreatedAt:   time.Now(),
	}

	mgr := service.NewForScope(options.system)
	if err := mgr.Install(svc); err != nil {
		return nil, fmt.Errorf("failed to install service: %w", err)
	}

	if err := st.Add(svc); err != nil {
		return nil, fmt.Errorf("failed to save service: %w", err)
	}
	if err := st.Save(); err != nil {
		return nil, fmt.Errorf("failed to save store: %w", err)
	}

	if options.start {
		if err := mgr.Start(name); err != nil {
			return svc, fmt.Errorf("service created but failed to start: %w", err)
		}
	}

	return svc, nil
}
