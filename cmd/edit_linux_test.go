//go:build linux

package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mechanic/store"
)

func TestEditCLIUpdatesOnlySuppliedFields(t *testing.T) {
	_, before := editFixture(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MECHANIC_INIT", "systemd")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	cmd := newEditCmd()
	cmd.SetArgs([]string{"worker", "--description=", "--env", "NEW=a,b", "--env", "PORT=8080"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	st, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("worker")
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != before.Command || got.Restart != before.Restart || got.WorkingDir != before.WorkingDir || got.Enabled != before.Enabled || got.Description != "" || strings.Join(got.Env, "|") != "NEW=a,b|PORT=8080" {
		t.Fatalf("unexpected edit: %+v", got)
	}
	cmd = newEditCmd()
	cmd.SetArgs([]string{"worker", "--clear-env"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	st, _ = store.New()
	got, _ = st.Get("worker")
	if len(got.Env) != 0 {
		t.Fatalf("environment not cleared: %v", got.Env)
	}
}

func TestTUIDescriptionSavePreservesCRLFEnvironment(t *testing.T) {
	st, svc := editFixture(t)
	svc.Env = []string{"VALUE=first\r\nsecond\r\n", "OTHER=a,b"}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MECHANIC_INIT", "systemd")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	m := tuiModel{rows: []tuiRow{{service: svc}}}
	m.openEdit()
	m.createForm.description = "updated description"
	msg := runTUICreate(m.createForm, false)().(tuiCreateMsg)
	if msg.err != nil || !msg.created || !msg.edited {
		t.Fatalf("save failed: %+v", msg)
	}
	saved, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := saved.Get(svc.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "updated description" || !reflect.DeepEqual(got.Env, svc.Env) {
		t.Fatalf("unexpected saved service: %+v", got)
	}
}
