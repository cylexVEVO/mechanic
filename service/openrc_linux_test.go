//go:build linux

package service

import (
	"strings"
	"testing"

	"mechanic/store"
)

func TestRenderOpenRCWrapperRestartPolicyAndQuoting(t *testing.T) {
	svc := &store.Service{
		Name:       "worker",
		Command:    "printf '%s' \"$TOKEN\"",
		WorkingDir: "/srv/worker's dir",
		Restart:    store.RestartOnFailure,
		Env:        []string{"TOKEN=a b'c"},
	}
	wrapper := renderOpenRCWrapper(svc)
	for _, want := range []string{
		"on-failure) [ \"$status\" -ne 0 ] || exit 0",
		"cd '/srv/worker'\\''s dir'",
		"export TOKEN='a b'\\''c'",
	} {
		if !strings.Contains(wrapper, want) {
			t.Errorf("wrapper missing %q:\n%s", want, wrapper)
		}
	}
}

func TestRenderOpenRCServiceUsesProvidedPIDPath(t *testing.T) {
	svc := &store.Service{Name: "worker", WorkingDir: "/srv/worker"}
	script := renderOpenRCService(svc, "/wrapper", "/var/log/worker.log", `"${XDG_RUNTIME_DIR}/worker.pid"`)
	if !strings.Contains(script, `pidfile="${XDG_RUNTIME_DIR}/worker.pid"`) {
		t.Fatalf("service does not preserve runtime pid path:\n%s", script)
	}
}
