//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
	"mechanic/store"
)

type windowsManager struct {
	dataDir string
}

// New returns a Manager for Windows (PID-based process manager)
func New() Manager {
	return NewForScope(false)
}

// NewForScope returns the Windows user manager. Native system services are not yet supported.
func NewForScope(system bool) Manager {
	if system {
		return &unsupportedManager{reason: "system services are not supported on Windows"}
	}
	home, _ := os.UserHomeDir()
	return &windowsManager{
		dataDir: filepath.Join(home, ".mechanic"),
	}
}

func (m *windowsManager) pidFile(name string) string {
	return filepath.Join(m.dataDir, "pids", name+".pid")
}

func (m *windowsManager) logFile(name string) string {
	return filepath.Join(m.dataDir, "logs", name+".log")
}

func (m *windowsManager) Install(svc *store.Service) error {
	if err := os.MkdirAll(filepath.Join(m.dataDir, "pids"), 0755); err != nil {
		return fmt.Errorf("failed to create pids directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(m.dataDir, "logs"), 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}
	return nil
}

func (m *windowsManager) Uninstall(name string) error {
	m.Stop(name) //nolint:errcheck
	pf := m.pidFile(name)
	if err := os.Remove(pf); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove pid file: %w", err)
	}
	return nil
}

func (m *windowsManager) Start(name string) error {
	// We need the service record to get the command; caller must pass it via Install
	// For Start we read from the store - but Manager doesn't have store access.
	// We use a wrapper approach: read a launch script from dataDir
	return fmt.Errorf("use 'mechanic start' which resolves command from store")
}

// StartWithService starts the process using service details
func (m *windowsManager) startProcess(svc *store.Service) error {
	lf := m.logFile(svc.Name)
	logFile, err := os.OpenFile(lf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command("cmd", "/C", svc.Command)
	cmd.Dir = svc.WorkingDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}

	for _, e := range svc.Env {
		cmd.Env = append(os.Environ(), e)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	pf := m.pidFile(svc.Name)
	if err := os.WriteFile(pf, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to write pid file: %w", err)
	}

	return nil
}

func (m *windowsManager) Stop(name string) error {
	pid, err := m.readPID(name)
	if err != nil {
		return nil // Already stopped
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}

	if err := p.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}

	os.Remove(m.pidFile(name)) //nolint:errcheck
	return nil
}

func (m *windowsManager) Restart(name string) error {
	return fmt.Errorf("restart on Windows requires the service command; use stop then start")
}

func (m *windowsManager) Status(name string) (*Status, error) {
	pid, err := m.readPID(name)
	if err != nil {
		return &Status{Name: name, State: "stopped"}, nil
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		return &Status{Name: name, State: "stopped"}, nil
	}

	// On Windows, FindProcess always succeeds; check if process is actually running
	// by sending signal 0 equivalent
	if err := checkProcess(p); err != nil {
		os.Remove(m.pidFile(name)) //nolint:errcheck
		return &Status{Name: name, State: "stopped"}, nil
	}

	return &Status{Name: name, State: "running", PID: pid}, nil
}

func (m *windowsManager) Logs(name string, lines int, follow bool) error {
	lf := m.logFile(name)
	if follow {
		// Simple follow: use PowerShell
		cmd := exec.Command("powershell", "-Command",
			fmt.Sprintf("Get-Content -Path '%s' -Wait -Tail %d", lf, lines))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	out, err := m.ReadLogs(name, lines)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func (m *windowsManager) ReadLogs(name string, lines int) (string, error) {
	data, err := os.ReadFile(m.logFile(name))
	if err != nil {
		if os.IsNotExist(err) {
			return "No logs found.", nil
		}
		return "", err
	}

	lineSlice := strings.Split(string(data), "\n")
	start := len(lineSlice) - lines
	if start < 0 {
		start = 0
	}
	return strings.Join(lineSlice[start:], "\n"), nil
}

func (m *windowsManager) Enable(name string) error {
	// Store the command to run at startup in the registry
	// We'll write a batch command to HKCU Run key
	// The actual command needs to come from the store - we store a marker here
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer k.Close()

	// Store a placeholder - the actual start script
	val := fmt.Sprintf(`mechanic start %s`, name)
	if err := k.SetStringValue("mechanic-"+name, val); err != nil {
		return fmt.Errorf("failed to set registry value: %w", err)
	}
	return nil
}

func (m *windowsManager) Disable(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer k.Close()

	if err := k.DeleteValue("mechanic-" + name); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("failed to delete registry value: %w", err)
	}
	return nil
}

func (m *windowsManager) readPID(name string) (int, error) {
	data, err := os.ReadFile(m.pidFile(name))
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file: %w", err)
	}
	return pid, nil
}

func checkProcess(p *os.Process) error {
	// On Windows, use OpenProcess with PROCESS_QUERY_LIMITED_INFORMATION
	handle, err := syscall.OpenProcess(0x1000, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	syscall.CloseHandle(handle)
	return nil
}
