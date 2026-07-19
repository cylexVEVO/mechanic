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

type openRCManager struct {
	system bool
}

func openRCAvailable() bool {
	for _, command := range []string{"openrc-run", "rc-service", "rc-update"} {
		if _, err := exec.LookPath(command); err != nil {
			return false
		}
	}
	return true
}

func openRCName(name string) string {
	return "mechanic-" + name
}

func openRCConfigDir() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "rc"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rc"), nil
}

func (m *openRCManager) servicePath(name string) (string, error) {
	if m.system {
		return filepath.Join("/etc", "init.d", openRCName(name)), nil
	}
	dir, err := openRCConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "init.d", openRCName(name)), nil
}

func (m *openRCManager) wrapperPath(name string) (string, error) {
	if m.system {
		return filepath.Join("/var", "lib", "mechanic", "openrc", name+".sh"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mechanic", "openrc", name+".sh"), nil
}

func (m *openRCManager) logPath(name string) (string, error) {
	if m.system {
		return filepath.Join("/var", "log", "mechanic", name+".log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mechanic", "logs", name+".log"), nil
}

func openRCRuntimeDir() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return "", fmt.Errorf("XDG_RUNTIME_DIR is required for OpenRC user services")
	}
	return dir, nil
}

func (m *openRCManager) pidPath(name string) (string, error) {
	if m.system {
		return filepath.Join("/run", openRCName(name)+".pid"), nil
	}
	dir, err := openRCRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, openRCName(name)+".pid"), nil
}

func shQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func renderOpenRCWrapper(svc *store.Service) string {
	var env strings.Builder
	for _, entry := range svc.Env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		env.WriteString("export ")
		env.WriteString(parts[0])
		env.WriteString("=")
		env.WriteString(shQuote(parts[1]))
		env.WriteString("\n")
	}

	return fmt.Sprintf(`#!/bin/sh
cd %s || exit 1
%s
child=
stop_child() {
	if [ -n "$child" ]; then
		kill -TERM "$child" 2>/dev/null
		wait "$child" 2>/dev/null
	fi
	exit 0
}
trap stop_child TERM INT

while :; do
	/bin/sh -c %s &
	child=$!
	wait "$child"
	status=$?
	child=

	case %s in
		always) ;;
		on-failure) [ "$status" -ne 0 ] || exit 0 ;;
		never) exit "$status" ;;
	esac
	sleep 1
done
`, shQuote(svc.WorkingDir), env.String(), shQuote(svc.Command), shQuote(string(svc.Restart)))
}

func renderOpenRCService(svc *store.Service, wrapperPath, logPath, pidPath string) string {
	desc := svc.Description
	if desc == "" {
		desc = svc.Name
	}

	return fmt.Sprintf(`#!/sbin/openrc-run
name=%s
description=%s
command=%s
command_background=true
directory=%s
pidfile=%s
output_log=%s
error_log=%s
retry="TERM/10/KILL/5"
`, shQuote("Mechanic: "+svc.Name), shQuote(desc), shQuote(wrapperPath), shQuote(svc.WorkingDir),
		pidPath, shQuote(logPath), shQuote(logPath))
}

func (m *openRCManager) Install(svc *store.Service) error {
	servicePath, err := m.servicePath(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to determine OpenRC service path: %w", err)
	}
	wrapperPath, err := m.wrapperPath(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to determine OpenRC wrapper path: %w", err)
	}
	logPath, err := m.logPath(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to determine OpenRC log path: %w", err)
	}

	pidPath := fmt.Sprintf(`"${XDG_RUNTIME_DIR}/%s.pid"`, openRCName(svc.Name))
	if m.system {
		pidPath = shQuote(filepath.Join("/run", openRCName(svc.Name)+".pid"))
	}

	for _, dir := range []string{filepath.Dir(servicePath), filepath.Dir(wrapperPath), filepath.Dir(logPath)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create OpenRC directory: %w", err)
		}
	}
	if err := os.WriteFile(wrapperPath, []byte(renderOpenRCWrapper(svc)), 0755); err != nil {
		return fmt.Errorf("failed to write OpenRC command wrapper: %w", err)
	}
	if err := os.WriteFile(servicePath, []byte(renderOpenRCService(svc, wrapperPath, logPath, pidPath)), 0755); err != nil {
		return fmt.Errorf("failed to write OpenRC service: %w", err)
	}

	return nil
}

func (m *openRCManager) Uninstall(name string) error {
	servicePath, err := m.servicePath(name)
	if err != nil {
		return err
	}
	wrapperPath, err := m.wrapperPath(name)
	if err != nil {
		return err
	}
	for _, path := range []string{servicePath, wrapperPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
	}
	return nil
}

func (m *openRCManager) run(args ...string) (string, error) {
	if !m.system {
		if _, err := openRCRuntimeDir(); err != nil {
			return "", err
		}
		args = append([]string{"--user"}, args...)
	}
	return runCmd("rc-service", args...)
}

func (m *openRCManager) Start(name string) error {
	out, err := m.run(openRCName(name), "start")
	if err != nil {
		return fmt.Errorf("failed to start service: %s: %w", out, err)
	}
	return nil
}

func (m *openRCManager) Stop(name string) error {
	out, err := m.run(openRCName(name), "stop")
	if err != nil {
		return fmt.Errorf("failed to stop service: %s: %w", out, err)
	}
	return nil
}

func (m *openRCManager) Restart(name string) error {
	out, err := m.run(openRCName(name), "restart")
	if err != nil {
		return fmt.Errorf("failed to restart service: %s: %w", out, err)
	}
	return nil
}

func (m *openRCManager) Status(name string) (*Status, error) {
	out, err := m.run(openRCName(name), "status")
	lower := strings.ToLower(out)
	state := "unknown"
	switch {
	case strings.Contains(lower, "crashed"):
		state = "failed"
	case strings.Contains(lower, "started") || strings.Contains(lower, "status: running"):
		state = "running"
	case strings.Contains(lower, "stopped") || strings.Contains(lower, "status: stopped"):
		state = "stopped"
	case err != nil:
		return &Status{Name: name, State: state, Message: out}, nil
	}

	status := &Status{Name: name, State: state}
	if state == "unknown" && out != "" {
		status.Message = out
	}
	if pidPath, pidErr := m.pidPath(name); pidErr == nil {
		if data, readErr := os.ReadFile(pidPath); readErr == nil {
			status.PID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		if info, statErr := os.Stat(pidPath); statErr == nil {
			status.Since = info.ModTime().Format("2006-01-02 15:04:05")
		}
	}
	return status, nil
}

func (m *openRCManager) Logs(name string, lines int, follow bool) error {
	path, err := m.logPath(name)
	if err != nil {
		return err
	}
	args := []string{"-n", strconv.Itoa(lines)}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, path)
	cmd := exec.Command("tail", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *openRCManager) ReadLogs(name string, lines int) (string, error) {
	path, err := m.logPath(name)
	if err != nil {
		return "", err
	}
	out, err := exec.Command("tail", "-n", strconv.Itoa(lines), path).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

func (m *openRCManager) Enable(name string) error {
	args := []string{"add", openRCName(name), "default"}
	if !m.system {
		if _, err := openRCRuntimeDir(); err != nil {
			return err
		}
		args = append([]string{"--user"}, args...)
	}
	out, err := runCmd("rc-update", args...)
	if err != nil {
		return fmt.Errorf("failed to enable service: %s: %w", out, err)
	}
	return nil
}

func (m *openRCManager) Disable(name string) error {
	args := []string{"del", openRCName(name), "default"}
	if !m.system {
		if _, err := openRCRuntimeDir(); err != nil {
			return err
		}
		args = append([]string{"--user"}, args...)
	}
	out, err := runCmd("rc-update", args...)
	if err != nil {
		return fmt.Errorf("failed to disable service: %s: %w", out, err)
	}
	return nil
}
