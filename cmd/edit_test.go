package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mechanic/service"
	"mechanic/store"
)

type editManager struct {
	service.Manager
	installs    []*store.Service
	failInstall bool
	restartErr  error
	restarted   bool
}

func (m *editManager) Install(s *store.Service) error {
	copy := *s
	m.installs = append(m.installs, &copy)
	if m.failInstall && len(m.installs) == 1 {
		return errors.New("install failed")
	}
	return nil
}
func (m *editManager) Restart(string) error { m.restarted = true; return m.restartErr }

func editFixture(t *testing.T) (*store.Store, *store.Service) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	st, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	svc := &store.Service{Name: "worker", Command: "sleep 10", Description: "original", WorkingDir: "/tmp", Restart: store.RestartNever, Env: []string{"VALUE=a,b"}, Enabled: true, CreatedAt: time.Now().UTC()}
	if err := st.Add(svc); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	return st, svc
}

func TestSaveServiceEdit(t *testing.T) {
	st, original := editFixture(t)
	before := *original
	next := *original
	next.Command, next.Description, next.Env = "sleep 20", "", nil
	mgr := &editManager{}
	if _, err := saveServiceEdit(st, mgr, &next, true); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reloaded.Get(next.Name)
	if got.Command != "sleep 20" || got.Description != "" || len(got.Env) != 0 || !got.Enabled || !got.CreatedAt.Equal(before.CreatedAt) || got.WorkingDir != before.WorkingDir || got.Restart != before.Restart {
		t.Fatalf("unexpected saved service: %+v", got)
	}
	if !mgr.restarted {
		t.Fatal("restart not requested")
	}
}

func TestSaveServiceEditFailures(t *testing.T) {
	for _, kind := range []string{"command", "env", "policy", "install", "restart", "save"} {
		t.Run(kind, func(t *testing.T) {
			st, original := editFixture(t)
			before, next := *original, *original
			next.Command = "sleep 20"
			mgr := &editManager{}
			switch kind {
			case "command":
				next.Command = " "
			case "env":
				next.Env = []string{"INVALID"}
			case "policy":
				next.Restart = "invalid"
			case "install":
				mgr.failInstall = true
			case "restart":
				mgr.restartErr = errors.New("restart failed")

			}
			if kind == "save" {
				if err := os.Remove(filepath.Join(st.DataDir(), "registry.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(st.DataDir(), "registry.json"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			result, err := saveServiceEdit(st, mgr, &next, kind == "restart")
			if err == nil {
				t.Fatal("expected failure")
			}
			if kind == "restart" {
				if result == nil || original.Command != next.Command {
					t.Fatal("saved edit lost on restart failure")
				}
			} else {
				if result != nil || !reflect.DeepEqual(*original, before) {
					t.Fatal("failed edit mutated registry")
				}
			}
			if kind == "command" || kind == "env" || kind == "policy" {
				if len(mgr.installs) != 0 {
					t.Fatal("invalid service installed")
				}
			}
			if kind == "install" || kind == "save" {
				if len(mgr.installs) != 2 || !reflect.DeepEqual(*mgr.installs[1], before) {
					t.Fatal("previous definition not restored")
				}
			}
		})
	}
}

func TestTUIEditForm(t *testing.T) {
	_, svc := editFixture(t)
	m := tuiModel{rows: []tuiRow{{service: svc}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m = updated.(tuiModel)
	if !m.creating || !m.createForm.editing || m.createForm.command != svc.Command || m.createForm.name != svc.Name || m.createForm.env != `"VALUE=a,b"` {
		t.Fatalf("incorrect edit form: %+v", m.createForm)
	}
	if !strings.Contains(m.View(), "Restart now") {
		t.Fatal("missing restart option")
	}
	updateTUICreateForm(&m.createForm, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.createForm.field != 6 {
		t.Fatal("name must not be editable")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(tuiModel).creating || svc.Command != "sleep 10" {
		t.Fatal("cancel changed service")
	}
}

func TestEditCommandRequiresChanges(t *testing.T) {
	cmd := newEditCmd()
	cmd.SetArgs([]string{"worker"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing changes error")
	}
	cmd = newEditCmd()
	cmd.SetArgs([]string{"worker", "--env", "A=B", "--clear-env"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected conflicting flags error")
	}
}
