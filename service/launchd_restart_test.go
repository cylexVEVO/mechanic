package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRestartLaunchdStartsExactlyOnce(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, keepAlive := range []bool{false, true} {
			for _, loaded := range []bool{false, true} {
				t.Run(fmt.Sprintf("enabled=%t/keepAlive=%t/loaded=%t", enabled, keepAlive, loaded), func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "worker.plist")
					data := fmt.Sprintf(`<plist><dict><key>RunAtLoad</key><%t/><key>KeepAlive</key><%t/></dict></plist>`, enabled, keepAlive)
					if err := os.WriteFile(path, []byte(data), 0644); err != nil {
						t.Fatal(err)
					}
					var calls []string
					starts := 0
					run := func(command string, args ...string) (string, error) {
						if command != "launchctl" {
							t.Fatalf("unexpected command %s", command)
						}
						calls = append(calls, args[0])
						switch args[0] {
						case "print":
							if !loaded {
								return "", errors.New("not loaded")
							}
						case "bootstrap":
							if !reflect.DeepEqual(args, []string{"bootstrap", "gui/501", path}) {
								t.Fatalf("bootstrap args: %v", args)
							}
							if enabled || keepAlive {
								starts++
							} // A short command can finish before bootstrap returns.
						case "kickstart":
							if !reflect.DeepEqual(args, []string{"kickstart", "gui/501/com.mechanic.worker"}) {
								t.Fatalf("kickstart args: %v", args)
							}
							starts++
						}
						return "", nil
					}
					if err := restartLaunchd(path, "gui/501", "gui/501/com.mechanic.worker", run); err != nil {
						t.Fatal(err)
					}
					if starts != 1 {
						t.Fatalf("command ran %d times, want exactly once", starts)
					}
					want := []string{"print"}
					if loaded {
						want = append(want, "bootout")
					}
					want = append(want, "bootstrap")
					if !enabled && !keepAlive {
						want = append(want, "kickstart")
					}
					if !reflect.DeepEqual(calls, want) {
						t.Fatalf("calls = %v, want %v", calls, want)
					}
				})
			}
		}
	}
}

func TestRestartLaunchdStopsOnFailure(t *testing.T) {
	for _, fail := range []string{"read", "parse", "bootout", "bootstrap", "kickstart"} {
		t.Run(fail, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "worker.plist")
			data := `<plist><dict><key>RunAtLoad</key><false/></dict></plist>`
			if fail == "parse" {
				data = `<plist>`
			}
			if fail != "read" {
				if err := os.WriteFile(path, []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			run := func(_ string, args ...string) (string, error) {
				calls = append(calls, args[0])
				if args[0] == fail {
					return "failure", errors.New("failed")
				}
				return "", nil
			}
			if err := restartLaunchd(path, "system", "system/com.mechanic.worker", run); err == nil {
				t.Fatal("expected failure")
			}
			if fail == "read" || fail == "parse" {
				if len(calls) != 0 {
					t.Fatalf("commands ran before validation: %v", calls)
				}
			} else if calls[len(calls)-1] != fail {
				t.Fatalf("continued after failure: %v", calls)
			}
		})
	}
}
