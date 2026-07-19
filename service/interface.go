package service

import (
	"fmt"

	"mechanic/store"
)

// Status holds runtime status of a service
type Status struct {
	Name    string
	State   string // "running", "stopped", "failed", "unknown"
	PID     int
	Since   string
	Message string
}

// Manager is the platform-agnostic interface for service management
type Manager interface {
	Install(svc *store.Service) error
	Uninstall(name string) error
	Start(name string) error
	Stop(name string) error
	Restart(name string) error
	Status(name string) (*Status, error)
	ReadLogs(name string, lines int) (string, error)
	Logs(name string, lines int, follow bool) error
	Enable(name string) error
	Disable(name string) error
}

type unsupportedManager struct {
	reason string
}

func (m *unsupportedManager) err() error                   { return fmt.Errorf("%s", m.reason) }
func (m *unsupportedManager) Install(*store.Service) error { return m.err() }
func (m *unsupportedManager) Uninstall(string) error       { return m.err() }
func (m *unsupportedManager) Start(string) error           { return m.err() }
func (m *unsupportedManager) Stop(string) error            { return m.err() }
func (m *unsupportedManager) Restart(string) error         { return m.err() }
func (m *unsupportedManager) Status(name string) (*Status, error) {
	return &Status{Name: name, State: "unknown"}, m.err()
}
func (m *unsupportedManager) ReadLogs(string, int) (string, error) { return "", m.err() }
func (m *unsupportedManager) Logs(string, int, bool) error         { return m.err() }
func (m *unsupportedManager) Enable(string) error                  { return m.err() }
func (m *unsupportedManager) Disable(string) error                 { return m.err() }
