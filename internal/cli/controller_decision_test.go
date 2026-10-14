package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runUnified runs the dispatcher with an injectable working directory, for the
// unified controller/decision commands.
func runUnified(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, deps{getwd: func() (string, error) { return dir, nil }})
	return code, out.String(), errOut.String()
}

// TestUnifiedControllerAndDecisionCommands covers MIG-004: the unified `sop
// controller` and `sop decision` commands, their help, exit codes and the
// JSON-only stdout invariant, and that neither initializes SOP lifecycle state.
func TestUnifiedControllerAndDecisionCommands(t *testing.T) {
	dir := t.TempDir()

	// Bare `sop controller` and `sop decision` display help (exit 0).
	if code, out, _ := runUnified(t, dir, "controller"); code != exitOK || !strings.Contains(out, "sop controller") {
		t.Fatalf("controller help: code=%d out=%q", code, out)
	}
	if code, out, _ := runUnified(t, dir, "decision"); code != exitOK || !strings.Contains(out, "decision decide") {
		t.Fatalf("decision help: code=%d out=%q", code, out)
	}
	// Help and standalone decision must not initialize .agent-sdlc.
	if _, err := os.Stat(filepath.Join(dir, stateDirName)); !os.IsNotExist(err) {
		t.Errorf("help/standalone decision initialized .agent-sdlc (stat err=%v)", err)
	}

	// Unknown subcommands are usage errors.
	if code, _, _ := runUnified(t, dir, "controller", "bogus"); code != exitUsage {
		t.Errorf("controller bogus: code=%d, want %d", code, exitUsage)
	}
	if code, _, _ := runUnified(t, dir, "decision", "bogus"); code != 2 {
		t.Errorf("decision bogus: code=%d, want 2", code)
	}

	// The connector's appended decide token: argv [sop, decision] + "decide ..."
	// dispatches without recursion.
	t.Setenv("SOP_DECISION_FALLBACK", "")
	t.Setenv("CLEF_ENABLED", "")
	code, out, errOut := runUnified(t, dir, "decision", "decide",
		"-provider", "nimble", "-base-url", "http://127.0.0.1:1", "-timeout", "2s",
		"-choices", "LOW,HIGH", "-json")
	if code == 0 {
		t.Fatalf("decide against an unreachable provider must not succeed: out=%q", out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("JSON-only stdout violated: a failed decide wrote %q to stdout", out)
	}
	if strings.TrimSpace(errOut) == "" {
		t.Errorf("a failed decide must write a diagnostic to stderr")
	}

	// Missing -choices is a usage error (exit 2) from the shared implementation.
	if code, _, _ := runUnified(t, dir, "decision", "decide", "-provider", "nimble"); code != 2 {
		t.Errorf("decide without -choices: code=%d, want 2", code)
	}
}

// TestControllerServeHelpIsSideEffectFree pins the fix for `sop controller serve
// --help`/`-h`: it must print usage and exit 0 WITHOUT loading runtime configuration,
// discovering projects or opening a listener. A listener would block Serve forever,
// so the bounded wait proves no bind was attempted, and the .agent-sdlc check proves no
// lifecycle state was created.
func TestControllerServeHelpIsSideEffectFree(t *testing.T) {
	type runResult struct {
		code           int
		stdout, stderr string
	}

	for _, flag := range []string{"--help", "-h"} {
		dir := t.TempDir()
		done := make(chan runResult, 1)
		go func() {
			var out, errOut bytes.Buffer
			code := run([]string{"controller", "serve", flag}, &out, &errOut,
				deps{getwd: func() (string, error) { return dir, nil }})
			done <- runResult{code, out.String(), errOut.String()}
		}()

		var res runResult
		select {
		case res = <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("controller serve %s did not return: it opened a listener instead of printing usage", flag)
		}
		if res.code != exitOK {
			t.Errorf("controller serve %s: code=%d, want 0; stderr=%q", flag, res.code, res.stderr)
		}
		if !strings.Contains(res.stdout, "sop controller") || !strings.Contains(res.stdout, "serve") {
			t.Errorf("controller serve %s: usage not printed; stdout=%q", flag, res.stdout)
		}
		if _, err := os.Stat(filepath.Join(dir, stateDirName)); !os.IsNotExist(err) {
			t.Errorf("controller serve %s initialized .agent-sdlc (stat err=%v)", flag, err)
		}
	}
}
