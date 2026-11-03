package service

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

// restartLaunchd reloads a job's definition and starts it exactly once. Keep
// this command sequence platform-independent so it can be tested without macOS.
func restartLaunchd(path, domain, target string, run func(string, ...string) (string, error)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read plist: %w", err)
	}
	// A plist dictionary alternates key and value elements, rather than naming
	// elements after keys. Only top-level booleans control automatic startup.
	var document struct {
		Dict struct {
			Entries []struct {
				XMLName xml.Name
				Text    string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"dict"`
	}
	if err := xml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("failed to parse plist: %w", err)
	}
	autoStart := false
	entries := document.Dict.Entries
	for i := 0; i+1 < len(entries); i++ {
		if entries[i].XMLName.Local != "key" {
			continue
		}
		key := strings.TrimSpace(entries[i].Text)
		if (key == "RunAtLoad" || key == "KeepAlive") && entries[i+1].XMLName.Local == "true" {
			autoStart = true
		}
	}
	if _, err := run("launchctl", "print", target); err == nil {
		if out, err := run("launchctl", "bootout", target); err != nil {
			return fmt.Errorf("failed to unload service: %s: %w", out, err)
		}
	}
	if out, err := run("launchctl", "bootstrap", domain, path); err != nil {
		return fmt.Errorf("failed to reload service: %s: %w", out, err)
	}
	if autoStart {
		return nil
	}
	if out, err := run("launchctl", "kickstart", target); err != nil {
		return fmt.Errorf("failed to start service: %s: %w", out, err)
	}
	return nil
}
