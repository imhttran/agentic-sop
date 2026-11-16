package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolationMarkerFixture writes ../ran.txt when it runs, so the Windows test can
// prove the command never executed rather than only that an error came back.
const isolationMarkerFixture = `package probe

import (
	"os"
	"testing"
)

func TestWritesMarker(t *testing.T) {
	if err := os.WriteFile("../ran.txt", []byte("ran"), 0o644); err != nil {
		t.Fatal(err)
	}
}
`

// writeMarkerModule prepares a module whose `go test ./probe` leaves ran.txt in
// the repository root.
func writeMarkerModule(t *testing.T, root string) {
	t.Helper()
	writeRepoFile(t, root, "go.mod", "module probe\n\ngo 1.27\n")
	writeRepoFile(t, root, "probe/probe_test.go", isolationMarkerFixture)
}

// markerExists reports whether the command side effect landed.
func markerExists(t *testing.T, root string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, "ran.txt"))
	if err == nil {
		return true
	}
	if !os.IsNotExist(err) {
		t.Fatalf("stat marker: %v", err)
	}
	return false
}

// Isolation is unconditional. There is no platform, configuration value, or
// environment value that makes policy-only execution legal; the refusal for a
// platform with no backend comes from sandboxArgv, not from a configurable
// decision. This replaces the old pure `isolationDecision` table.
func TestIsolationIsUnconditional(t *testing.T) {
	if !sandboxRequired() {
		t.Fatal("sandboxRequired() = false; isolation must be mandatory")
	}
	for _, goos := range []string{"windows", "plan9", "aix"} {
		if hasBackend(goos) {
			t.Errorf("goos=%s: hasBackend = true, want false", goos)
		}
	}
	for _, goos := range []string{"darwin", "linux"} {
		if !hasBackend(goos) {
			t.Errorf("goos=%s: hasBackend = false, want true", goos)
		}
	}
}

// The retired D4-5 override and acknowledgement are inert. Every spelling, with
// and without the acknowledgement, leaves isolation required.
func TestRetiredOverrideVariablesHaveNoEffect(t *testing.T) {
	for _, override := range []string{"", "on", "1", "true", "yes", "off", "0", "false", "no", "OFF", " off ", "bogus", "2"} {
		for _, ack := range []string{"", "1", "true", "yes", "bogus"} {
			t.Setenv(retiredIsolationEnv, override)
			t.Setenv(retiredAckEnv, ack)
			if !sandboxRequired() {
				t.Fatalf("override=%q ack=%q disabled isolation", override, ack)
			}
		}
	}
}

// Windows runtime enforcement, exercised by the Windows CI job. Every override
// spelling and both Config values must refuse without running the command.
func TestRunCommandIsolationOnWindowsRefuses(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows B1 enforcement")
	}
	cases := []struct {
		name     string
		override string
		isolate  bool
	}{
		{"unset", "", false},
		{"on", "on", false},
		{"off", "off", false},
		{"zero", "0", false},
		{"false", "false", false},
		{"no", "no", false},
		{"unknown", "bogus", false},
		{"config-true", "off", true},
		{"config-false", "off", false},
		{"ack-only", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(retiredIsolationEnv, c.override)
			t.Setenv(retiredAckEnv, "1")
			root := t.TempDir()
			writeMarkerModule(t, root)
			cfg := DefaultConfig()
			cfg.IsolateCommands = c.isolate
			log := NewAuditLog(0)
			h := New(root, cfg, log)
			_, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go test ./probe"})
			if !errors.Is(err, errIsolationUnavailable) {
				t.Fatalf("err = %v, want errIsolationUnavailable", err)
			}
			if markerExists(t, root) {
				t.Fatal("the command executed before the refusal")
			}
			rec := log.Records()[0]
			if rec.Action != ActionDeny {
				t.Fatalf("audit action = %s, want deny", rec.Action)
			}
			if !strings.Contains(rec.Detail, "no sandbox backend") {
				t.Fatalf("audit detail = %q, want the backend refusal", rec.Detail)
			}
		})
	}
}
