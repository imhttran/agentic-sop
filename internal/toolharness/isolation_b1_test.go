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

// Windows B1: a platform with no supported backend must refuse, whatever the
// override or the programmatic opt-in says. This is the regression for the
// override bypass, and it runs on every host because the decision is pure.
func TestIsolationDecisionUnsupportedPlatforms(t *testing.T) {
	overrides := []string{"", "on", "1", "true", "off", "0", "false", "no", "OFF", " off ", "bogus", "2"}
	acks := []string{"", "1", "true", "yes", "bogus"}
	for _, goos := range []string{"windows", "plan9", "aix"} {
		for _, override := range overrides {
			for _, ack := range acks {
				for _, isolate := range []bool{false, true} {
					if !isolationDecision(goos, override, ack, isolate) {
						t.Errorf("goos=%s override=%q ack=%q IsolateCommands=%v: policy-only execution allowed, want refusal",
							goos, override, ack, isolate)
					}
				}
			}
		}
	}
}

// The authorized behavior on a platform with a backend is unchanged.
func TestIsolationDecisionSupportedPlatforms(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		if !isolationDecision(goos, "", "", false) {
			t.Errorf("goos=%s default: isolation not requested", goos)
		}
		for _, override := range []string{"off", "0", "false", "no", "OFF", " off "} {
			for _, ack := range []string{"", "bogus", "0", "false", "no"} {
				if !isolationDecision(goos, override, ack, false) {
					t.Errorf("goos=%s override=%q ack=%q: unacknowledged override took effect", goos, override, ack)
				}
			}
			for _, ack := range []string{"1", "true", "yes", " YES "} {
				if isolationDecision(goos, override, ack, false) {
					t.Errorf("goos=%s override=%q ack=%q: acknowledged override ignored", goos, override, ack)
				}
			}
			if !isolationDecision(goos, override, "1", true) {
				t.Errorf("goos=%s: Config.IsolateCommands must win over an acknowledged override", goos)
			}
		}
		for _, override := range []string{"on", "1", "true", "yes", "bogus", "2"} {
			if !isolationDecision(goos, override, "1", false) {
				t.Errorf("goos=%s override=%q: the default was not kept", goos, override)
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
			t.Setenv(IsolationEnv, c.override)
			t.Setenv(UnsandboxedAckEnv, "1")
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
