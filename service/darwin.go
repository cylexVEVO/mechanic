//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"mechanic/store"
)

type darwinManager struct {
	system bool
}

// New returns a Manager for macOS (launchd agents)
func New() Manager {
	return NewForScope(false)
}

// NewForScope returns a launchd manager for agents or system daemons.
func NewForScope(system bool) Manager {
	return &darwinManager{system: system}
}

func label(name string) string {
	return fmt.Sprintf("com.mechanic.%s", name)
}

func (m *darwinManager) plistPath(name string) (string, error) {
	if m.system {
		return filepath.Join("/Library", "LaunchDaemons", label(name)+".plist"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label(name)+".plist"), nil
}

func (m *darwinManager) launchDomain() string {
	if m.system {
		return "system"
	}
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func (m *darwinManager) launchTarget(name string) string {
	return fmt.Sprintf("%s/%s", m.launchDomain(), label(name))
}

func logPath(dataDir, name string) string {
	return filepath.Join(dataDir, "logs", name+".log")
}

func keepAlive(r store.RestartPolicy) string {
	switch r {
	case store.RestartAlways:
		return "<key>KeepAlive</key>\n\t<true/>"
	default:
		return "<key>KeepAlive</key>\n\t<false/>"
	}
}

func (m *darwinManager) Install(svc *store.Service) error {
	path, err := m.plistPath(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to determine plist path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create launchd service directory: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	dataDir := filepath.Join(home, ".mechanic")
	if m.system {
		dataDir = filepath.Join("/var", "log", "mechanic")
	}
	logsDir := filepath.Join(dataDir, "logs")
	if m.system {
		logsDir = dataDir
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	lp := logPath(dataDir, svc.Name)
	if m.system {
		lp = filepath.Join(dataDir, svc.Name+".log")
	}

	desc := svc.Description
	if desc == "" {
		desc = svc.Name
	}

	var envXML strings.Builder
	if len(svc.Env) > 0 {
		envXML.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		for _, e := range svc.Env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envXML.WriteString(fmt.Sprintf("\t\t<key>%s</key>\n\t\t<string>%s</string>\n",
					xmlEscape(parts[0]), xmlEscape(parts[1])))
			}
		}
		envXML.WriteString("\t</dict>\n")
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/sh</string>
		<string>-c</string>
		<string>%s</string>
	</array>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
	<key>RunAtLoad</key>
	<%t/>
	%s
%s</dict>
</plist>
`, label(svc.Name), xmlEscape(svc.Command), xmlEscape(svc.WorkingDir),
		xmlEscape(lp), xmlEscape(lp), svc.Enabled, keepAlive(svc.Restart), envXML.String())

	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	return nil
}

func (m *darwinManager) Uninstall(name string) error {
	// Stop first (ignore errors)
	m.bootout(name) //nolint:errcheck

	path, err := m.plistPath(name)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %w", err)
	}
	return nil
}

func (m *darwinManager) Start(name string) error {
	path, err := m.plistPath(name)
	if err != nil {
		return err
	}

	// bootstrap first (in case it's not loaded)
	bootstrapOut, bootstrapErr := runCmd("launchctl", "bootstrap", m.launchDomain(), path)
	if bootstrapErr != nil {
		// May already be bootstrapped, try kickstart
		out, err := runCmd("launchctl", "kickstart", m.launchTarget(name))
		if err != nil {
			return fmt.Errorf("failed to start service: %s: %w", out, err)
		}
		return nil
	}
	_ = bootstrapOut
	return nil
}

func (m *darwinManager) Stop(name string) error {
	out, err := runCmd("launchctl", "kill", "SIGTERM", m.launchTarget(name))
	if err != nil {
		return fmt.Errorf("failed to stop service: %s: %w", out, err)
	}
	return nil
}

func (m *darwinManager) Restart(name string) error {
	// Reload the plist so edits apply to already loaded jobs.
	path, err := m.plistPath(name)
	if err != nil {
		return err
	}
	if _, err := runCmd("launchctl", "print", m.launchTarget(name)); err == nil {
		if err := m.bootout(name); err != nil {
			return err
		}
	}
	if out, err := runCmd("launchctl", "bootstrap", m.launchDomain(), path); err != nil {
		return fmt.Errorf("failed to reload service: %s: %w", out, err)
	}
	return m.Start(name)
}

func (m *darwinManager) Status(name string) (*Status, error) {
	out, err := runCmd("launchctl", "print", m.launchTarget(name))
	if err != nil {
		return &Status{Name: name, State: "stopped", Message: "not loaded"}, nil
	}

	state := "unknown"
	pid := 0

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "state = ") {
			stateVal := strings.TrimPrefix(line, "state = ")
			switch stateVal {
			case "running":
				state = "running"
			case "waiting":
				state = "stopped"
			default:
				state = stateVal
			}
		}
		if strings.HasPrefix(line, "pid = ") {
			pidStr := strings.TrimPrefix(line, "pid = ")
			pid, _ = strconv.Atoi(pidStr)
		}
	}

	return &Status{
		Name:  name,
		State: state,
		PID:   pid,
	}, nil
}

func (m *darwinManager) Logs(name string, lines int, follow bool) error {
	lp, err := m.logFilePath(name)
	if err != nil {
		return err
	}

	var args []string
	if follow {
		args = []string{"-f", lp}
	} else {
		args = []string{"-n", strconv.Itoa(lines), lp}
	}

	cmd := exec.Command("tail", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *darwinManager) logFilePath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataDir := filepath.Join(home, ".mechanic")
	if m.system {
		dataDir = filepath.Join("/var", "log", "mechanic")
	}
	lp := logPath(dataDir, name)
	if m.system {
		lp = filepath.Join(dataDir, name+".log")
	}

	return lp, nil
}

func (m *darwinManager) ReadLogs(name string, lines int) (string, error) {
	lp, err := m.logFilePath(name)
	if err != nil {
		return "", err
	}
	out, err := exec.Command("tail", "-n", strconv.Itoa(lines), lp).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

func (m *darwinManager) Enable(name string) error {
	path, err := m.plistPath(name)
	if err != nil {
		return err
	}
	// Update plist to set RunAtLoad=true
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read plist: %w", err)
	}

	updated := strings.ReplaceAll(string(data),
		"<key>RunAtLoad</key>\n\t<false/>",
		"<key>RunAtLoad</key>\n\t<true/>")

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to update plist: %w", err)
	}

	// Reload
	m.bootout(name)                                          //nolint:errcheck
	runCmd("launchctl", "bootstrap", m.launchDomain(), path) //nolint:errcheck
	return nil
}

func (m *darwinManager) Disable(name string) error {
	path, err := m.plistPath(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read plist: %w", err)
	}

	updated := strings.ReplaceAll(string(data),
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>RunAtLoad</key>\n\t<false/>")

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to update plist: %w", err)
	}

	// Reload
	m.bootout(name)                                          //nolint:errcheck
	runCmd("launchctl", "bootstrap", m.launchDomain(), path) //nolint:errcheck
	return nil
}

func (m *darwinManager) bootout(name string) error {
	out, err := runCmd("launchctl", "bootout", m.launchTarget(name))
	if err != nil {
		_ = out
		return fmt.Errorf("failed to bootout: %w", err)
	}
	return nil
}

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
