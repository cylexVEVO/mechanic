package store

import (
	"path/filepath"
	"testing"
)

func TestUserStoreRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	st, err := NewForScope(false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := st.DataDir(), filepath.Join(home, ".mechanic"); got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}

	svc := &Service{Name: "worker", Command: "sleep 10", Restart: RestartOnFailure}
	if err := st.Add(svc); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewForScope(false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.Get("worker")
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != svc.Command || got.Restart != svc.Restart {
		t.Fatalf("reloaded service = %#v, want %#v", got, svc)
	}
}
