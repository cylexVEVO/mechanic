//go:build linux

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

type systemdManager struct {
	system bool
}

// New returns a user-service Manager for the active Linux init system.
func New() Manager {
	return NewForScope(false)
}

// NewForScope returns a Manager for user or system services.
func NewForScope(system bool) Manager {
	switch strings.ToLower(os.Getenv("MECHANIC_INIT")) {
	case "systemd":
		return &systemdManager{system: system}
	case "openrc":
		return &openRCManager{system: system}
	}

	if pathExists("/run/openrc") {
		return &openRCManager{system: system}
	}
	if pathExists("/run/systemd/system") {
		return &systemdManager{system: system}
	}

	if _, err := exec.LookPath("systemctl"); err == nil {
		return &systemdManager{system: system}
	}
	if openRCAvailable() {
		return &openRCManager{system: system}
	}

	return &unavailableLinuxManager{}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func unitName(name string) string {
	return fmt.Sprintf("mechanic-%s.service", name)
}

func (m *systemdManager) unitPath(name string) (string, error) {
	if m.system {
		return filepath.Join("/etc", "systemd", "system", unitName(name)), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	return filepath.Join(dir, unitName(name)), nil
}

func (m *systemdManager) systemctl(args ...string) (string, error) {
	if !m.system {
		args = append([]string{"--user"}, args...)
	}
	return runCmd("systemctl", args...)
}

func restartPolicy(r store.RestartPolicy) string {
	switch r {
	case store.RestartAlways:
		return "always"
	case store.RestartNever:
		return "no"
	default:
		return "on-failure"
	}
}

func (m *systemdManager) Install(svc *store.Service) error {
	path, err := m.unitPath(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to determine unit path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create systemd unit directory: %w", err)
	}

	desc := svc.Description
	if desc == "" {
		desc = svc.Name
	}

	var envLines strings.Builder
	for _, e := range svc.Env {
		envLines.WriteString(fmt.Sprintf("Environment=%s\n", systemdQuote(e)))
	}

	wantedBy := "default.target"
	if m.system {
		wantedBy = "multi-user.target"
	}

	unit := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
ExecStart=/bin/sh -c '%s'
WorkingDirectory=%s
Restart=%s
%s
[Install]
WantedBy=%s
`, systemdQuote("Mechanic: "+desc), systemdExecEscape(svc.Command), systemdQuote(svc.WorkingDir),
		restartPolicy(svc.Restart), envLines.String(), wantedBy)

	if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
		return fmt.Errorf("failed to write unit file: %w", err)
	}

	if out, err := m.systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %s: %w", out, err)
	}

	return nil
}

func (m *systemdManager) Uninstall(name string) error {
	path, err := m.unitPath(name)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unit file: %w", err)
	}

	m.systemctl("daemon-reload") //nolint:errcheck
	return nil
}

func (m *systemdManager) Start(name string) error {
	out, err := m.systemctl("start", unitName(name))
	if err != nil {
		return fmt.Errorf("failed to start service: %s: %w", out, err)
	}
	return nil
}

func (m *systemdManager) Stop(name string) error {
	out, err := m.systemctl("stop", unitName(name))
	if err != nil {
		return fmt.Errorf("failed to stop service: %s: %w", out, err)
	}
	return nil
}

func (m *systemdManager) Restart(name string) error {
	out, err := m.systemctl("restart", unitName(name))
	if err != nil {
		return fmt.Errorf("failed to restart service: %s: %w", out, err)
	}
	return nil
}

func (m *systemdManager) Status(name string) (*Status, error) {
	out, err := m.systemctl("show", unitName(name),
		"--property=ActiveState,SubState,MainPID,StateChangeTimestamp")
	if err != nil {
		return &Status{Name: name, State: "unknown", Message: out}, nil
	}

	props := parseProperties(out)

	state := "unknown"
	active := props["ActiveState"]
	sub := props["SubState"]

	switch active {
	case "active":
		if sub == "running" {
			state = "running"
		} else {
			state = "running"
		}
	case "failed":
		state = "failed"
	case "inactive":
		state = "stopped"
	}

	pid := 0
	if pidStr, ok := props["MainPID"]; ok {
		pid, _ = strconv.Atoi(pidStr)
	}

	since := props["StateChangeTimestamp"]

	return &Status{
		Name:  name,
		State: state,
		PID:   pid,
		Since: since,
	}, nil
}

func (m *systemdManager) Logs(name string, lines int, follow bool) error {
	args := m.journalArgs(name, lines)
	if follow {
		args = append(args, "-f")
	}
	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *systemdManager) journalArgs(name string, lines int) []string {
	args := []string{"-u", unitName(name), "-n", strconv.Itoa(lines), "--no-pager"}
	if !m.system {
		args = append([]string{"--user"}, args...)
	}
	return args
}

func (m *systemdManager) ReadLogs(name string, lines int) (string, error) {
	out, err := exec.Command("journalctl", m.journalArgs(name, lines)...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

func (m *systemdManager) Enable(name string) error {
	out, err := m.systemctl("enable", unitName(name))
	if err != nil {
		return fmt.Errorf("failed to enable service: %s: %w", out, err)
	}
	return nil
}

func (m *systemdManager) Disable(name string) error {
	out, err := m.systemctl("disable", unitName(name))
	if err != nil {
		return fmt.Errorf("failed to disable service: %s: %w", out, err)
	}
	return nil
}

// shellEscape escapes single quotes in a command string for use in /bin/sh -c '...'
func shellEscape(cmd string) string {
	return strings.ReplaceAll(cmd, "'", "'\\''")
}

func systemdExecEscape(value string) string {
	value = shellEscape(value)
	value = strings.ReplaceAll(value, "%", "%%")
	return strings.ReplaceAll(value, "$", "$$")
}

func systemdQuote(value string) string {
	value = strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"%", "%%",
	).Replace(value)
	return `"` + value + `"`
}

// runCmd runs a command and returns combined output + error
func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// parseProperties parses KEY=VALUE lines from systemctl show output
func parseProperties(output string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			key := line[:idx]
			val := line[idx+1:]
			props[key] = val
		}
	}
	return props
}

type unavailableLinuxManager struct{}

func (m *unavailableLinuxManager) err() error {
	return fmt.Errorf("no supported service manager found (install systemd or OpenRC, or set MECHANIC_INIT)")
}

func (m *unavailableLinuxManager) Install(*store.Service) error { return m.err() }
func (m *unavailableLinuxManager) Uninstall(string) error       { return m.err() }
func (m *unavailableLinuxManager) Start(string) error           { return m.err() }
func (m *unavailableLinuxManager) Stop(string) error            { return m.err() }
func (m *unavailableLinuxManager) Restart(string) error         { return m.err() }
func (m *unavailableLinuxManager) Status(name string) (*Status, error) {
	return &Status{Name: name, State: "unknown"}, m.err()
}
func (m *unavailableLinuxManager) ReadLogs(string, int) (string, error) {
	return "", m.err()
}
func (m *unavailableLinuxManager) Logs(string, int, bool) error { return m.err() }
func (m *unavailableLinuxManager) Enable(string) error          { return m.err() }
func (m *unavailableLinuxManager) Disable(string) error         { return m.err() }
