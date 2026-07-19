package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// RestartPolicy defines restart behavior
type RestartPolicy string

const (
	RestartAlways    RestartPolicy = "always"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartNever     RestartPolicy = "never"
)

// Service represents a mechanic-managed service
type Service struct {
	Name        string        `json:"name"`
	Command     string        `json:"command"`
	Description string        `json:"description"`
	WorkingDir  string        `json:"working_dir"`
	Restart     RestartPolicy `json:"restart"`
	Env         []string      `json:"env"`
	Enabled     bool          `json:"enabled"`
	System      bool          `json:"system,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

type registry struct {
	Services map[string]*Service `json:"services"`
}

// Store manages persistent service records
type Store struct {
	reg      registry
	dataDir  string
	filePath string
}

// New creates or loads the user service store from ~/.mechanic/registry.json.
func New() (*Store, error) {
	return NewForScope(false)
}

// NewForScope creates or loads the user or system service store.
func NewForScope(system bool) (*Store, error) {
	var dataDir string
	if system {
		switch runtime.GOOS {
		case "darwin":
			dataDir = filepath.Join("/Library", "Application Support", "Mechanic")
		case "windows":
			dataDir = filepath.Join(os.Getenv("ProgramData"), "Mechanic")
		default:
			dataDir = filepath.Join("/var", "lib", "mechanic")
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not determine home directory: %w", err)
		}
		dataDir = filepath.Join(home, ".mechanic")
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("could not create %s service data directory: %w", scopeName(system), err)
	}

	filePath := filepath.Join(dataDir, "registry.json")
	s := &Store{
		reg:      registry{Services: make(map[string]*Service)},
		dataDir:  dataDir,
		filePath: filePath,
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("could not read registry: %w", err)
	}

	if err := json.Unmarshal(data, &s.reg); err != nil {
		return nil, fmt.Errorf("could not parse registry: %w", err)
	}
	if s.reg.Services == nil {
		s.reg.Services = make(map[string]*Service)
	}

	return s, nil
}

func scopeName(system bool) string {
	if system {
		return "system"
	}
	return "user"
}

// Save writes the registry to disk
func (s *Store) Save() error {
	data, err := json.MarshalIndent(s.reg, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal registry: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("could not write registry: %w", err)
	}
	return nil
}

// Add inserts a new service into the store (does not save)
func (s *Store) Add(svc *Service) error {
	if _, exists := s.reg.Services[svc.Name]; exists {
		return fmt.Errorf("service '%s' already exists", svc.Name)
	}
	s.reg.Services[svc.Name] = svc
	return nil
}

// Remove deletes a service from the store (does not save)
func (s *Store) Remove(name string) error {
	if _, exists := s.reg.Services[name]; !exists {
		return fmt.Errorf("service '%s' not found. Use 'mechanic list' to see all services.", name)
	}
	delete(s.reg.Services, name)
	return nil
}

// Get retrieves a service by name
func (s *Store) Get(name string) (*Service, error) {
	svc, exists := s.reg.Services[name]
	if !exists {
		return nil, fmt.Errorf("service '%s' not found. Use 'mechanic list' to see all services.", name)
	}
	return svc, nil
}

// List returns all services
func (s *Store) List() []*Service {
	services := make([]*Service, 0, len(s.reg.Services))
	for _, svc := range s.reg.Services {
		services = append(services, svc)
	}
	return services
}

// DataDir returns the mechanic data directory (~/.mechanic)
func (s *Store) DataDir() string {
	return s.dataDir
}
