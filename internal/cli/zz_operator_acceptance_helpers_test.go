//go:build acceptance

// Operator-authored acceptance helpers for SOP-RUN-ACCEPT-001 (ACC-008..ACC-010).
// These files are the deterministic acceptance checks for the corrective tasks; they
// are built only with -tags acceptance so a red check never blocks ordinary
// validation, and every identifier is prefixed accOp to avoid collisions.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// accOpWrite writes content to dir/rel, creating parent directories.
func accOpWrite(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// accOpModule creates a Go module in a fresh temp dir and writes the given files.
func accOpModule(t *testing.T, files map[string]string) string {
	t.Helper()
	// Isolate the fixture from inherited Go flags so results are environment-independent.
	t.Setenv("GOFLAGS", "")
	dir := t.TempDir()
	accOpWrite(t, dir, "go.mod", "module example.com/accop\n\ngo 1.22\n")
	for rel, content := range files {
		accOpWrite(t, dir, rel, content)
	}
	return dir
}

// accOpPassGate is a passing quality gate.
func accOpPassGate() quality.Result {
	return quality.Result{Decision: quality.Pass}
}

// accOpOutcome derives the outcome for a spec with the given checks and one
// free-text criterion, through the production deriveOutcome seam.
func accOpOutcome(t *testing.T, dir string, checks ...string) *outcomeArtifact {
	t.Helper()
	spec := &taskfile.Spec{
		ID:                 "ACCOP",
		Title:              "operator acceptance",
		AcceptanceCriteria: []string{"free-text criterion"},
		AcceptanceChecks:   checks,
	}
	res := deriveOutcome(spec, dir, accOpPassGate())
	if res == nil {
		t.Fatal("deriveOutcome returned nil for a non-nil spec")
	}
	return res
}

// accOpCheck returns the criterion result whose Command equals command.
func accOpCheck(t *testing.T, res *outcomeArtifact, command string) outcome.CriterionResult {
	t.Helper()
	for _, c := range res.Criteria {
		if c.Command == command {
			return c
		}
	}
	t.Fatalf("no criterion for command %q in %+v", command, res.Criteria)
	return outcome.CriterionResult{}
}

// accOpExists reports whether path exists.
func accOpExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// accOpScript writes an executable shell script that touches marker and then
// execs its arguments, so any invocation of it is observable.
func accOpScript(t *testing.T, dir, name, marker string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\ntouch '" + marker + "'\nexec \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// accOpCount returns the number of non-empty lines in path (0 when absent).
func accOpCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
