package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
)

// Legacy task-evidence bootstrap tests.
//
// A task implemented before changed-files.json existed has no task change
// artifact, but the run's persisted activity stream still records every repository
// mutation the task's agent performed (CHANGE events carrying the path). The
// bootstrap recovers those paths lazily when JEV is about to run, so a legacy task
// is reviewed by its accumulated implementation instead of an empty change set —
// without ever guessing from the dirty working tree.

const changedFilesArtifact = "changed-files.json"

func legacyRunDir(dir, taskID string) string {
	return filepath.Join(dir, config.DirName, "runs", taskID)
}

func writeRunFile(t *testing.T, dir, taskID, name, content string) {
	t.Helper()
	runDir := legacyRunDir(dir, taskID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedLegacyActivity writes the run's activity stream (and nothing else), modeling
// a task whose implementation predates changed-file persistence.
func seedLegacyActivity(t *testing.T, dir, taskID string, lines ...string) {
	t.Helper()
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	writeRunFile(t, dir, taskID, activityArtifactName, content)
}

func changeEvent(taskID, action, path string) string {
	return fmt.Sprintf(`{"task_id":%q,"stage":"CHANGE","action":%q,"detail":%q}`, taskID, action, path)
}

// noChangeAgent completes without touching the repository, so an invocation
// records no change of its own and the bootstrap has a chance to run.
func noChangeAgent() *evidenceAgent {
	return &evidenceAgent{implOutcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: false}}
}

// A task that already has changed-files.json is not bootstrapped: the activity
// stream is ignored and the existing behavior is unchanged.
func TestJEVLegacyBootstrapSkippedWhenEvidenceExists(t *testing.T) {
	dir := setupJEVTask(t)
	writeRunFile(t, dir, "T001", changedFilesArtifact, "[\n  \"internal/a.go\"\n]\n")
	seedLegacyActivity(t, dir, "T001", changeEvent("T001", "editing", "internal/b.go"))

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("ChangedFiles = %v, want [internal/a.go] (b.go must not be bootstrapped)", got)
	}
}

// A missing changed-files.json plus reliable activity mutation evidence recovers
// the task's paths, persists them, and gives JEV the implementation source.
func TestJEVLegacyBootstrapRecoversActivityEvidence(t *testing.T) {
	dir := setupJEVTask(t)
	writeRepoFile(t, dir, "internal/a.go", "package internal\n\nconst Marker = 1\n")
	seedLegacyActivity(t, dir, "T001",
		`{"task_id":"T001","stage":"DISCOVER","action":"reading","detail":"internal/other.go"}`,
		changeEvent("T001", "editing", "internal/a.go"),
	)

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	req := analyzer.requests[0]
	if !sameStrings(req.ChangedFiles, []string{"internal/a.go"}) {
		t.Errorf("JEV ChangedFiles = %v, want the recovered [internal/a.go]", req.ChangedFiles)
	}
	if !strings.Contains(req.RepositoryContext, "const Marker = 1") {
		t.Errorf("JEV repository context lacks the recovered implementation:\n%s", req.RepositoryContext)
	}

	// The recovered evidence becomes normal persisted provenance.
	data, err := os.ReadFile(filepath.Join(legacyRunDir(dir, "T001"), changedFilesArtifact))
	if err != nil {
		t.Fatalf("changed-files.json not persisted: %v", err)
	}
	if !strings.Contains(string(data), "internal/a.go") {
		t.Errorf("persisted evidence = %q, want internal/a.go", data)
	}
}

// Evidence from several prior invocations is unioned.
func TestJEVLegacyBootstrapUnionsInvocations(t *testing.T) {
	dir := setupJEVTask(t)
	seedLegacyActivity(t, dir, "T001",
		changeEvent("T001", "editing", "internal/a.go"),
		changeEvent("T001", "creating", "internal/b.go"),
		changeEvent("T001", "editing", "internal/sopclient/c.go"),
	)

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	want := []string{"internal/a.go", "internal/b.go", "internal/sopclient/c.go"}
	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, want) {
		t.Errorf("ChangedFiles = %v, want the union %v", got, want)
	}
}

// Repeated mutations of one path are deduplicated.
func TestJEVLegacyBootstrapDeduplicates(t *testing.T) {
	dir := setupJEVTask(t)
	seedLegacyActivity(t, dir, "T001",
		changeEvent("T001", "editing", "internal/a.go"),
		changeEvent("T001", "editing", "internal/a.go"),
	)

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("ChangedFiles = %v, want [internal/a.go]", got)
	}
}

// The recovered set is bounded by the same limit as live evidence.
func TestJEVLegacyBootstrapBounded(t *testing.T) {
	dir := setupJEVTask(t)
	lines := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		lines = append(lines, changeEvent("T001", "editing", fmt.Sprintf("internal/f%03d.go", i)))
	}
	seedLegacyActivity(t, dir, "T001", lines...)

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	got := analyzer.requests[0].ChangedFiles
	if len(got) == 0 || len(got) > 64 {
		t.Errorf("ChangedFiles = %d entries, want a non-empty set bounded at 64", len(got))
	}
}

// An unrelated dirty file is never attributed: attribution comes from the task's
// own mutation events, never from the working tree.
func TestJEVLegacyBootstrapIgnoresUnrelatedDirtyFile(t *testing.T) {
	dir := setupJEVTask(t)
	writeRepoFile(t, dir, "notes.txt", "user scratch\n") // dirty, not the task's
	seedLegacyActivity(t, dir, "T001", changeEvent("T001", "editing", "internal/a.go"))

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("ChangedFiles = %v, want only [internal/a.go] (notes.txt is unrelated)", got)
	}
}

// SOP-owned paths are filtered even when they appear in the legacy stream.
func TestJEVLegacyBootstrapFiltersSOPOwnedPaths(t *testing.T) {
	dir := setupJEVTask(t)
	seedLegacyActivity(t, dir, "T001",
		changeEvent("T001", "editing", ".agent-sdlc/config.yaml"),
		changeEvent("T001", "editing", "docs/reports/report.md"),
		changeEvent("T001", "editing", "internal/a.go"),
	)

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("ChangedFiles = %v, want only [internal/a.go]", got)
	}
}

// With only ambiguous dirty-tree state (no reliable mutation evidence), nothing is
// attributed, no artifact is manufactured, and JEV stays evidence-insufficient.
func TestJEVLegacyBootstrapDoesNotGuessWithoutEvidence(t *testing.T) {
	dir := setupJEVTask(t)
	writeRepoFile(t, dir, "dirty.txt", "unrelated user work\n")
	seedLegacyActivity(t, dir, "T001",
		`{"task_id":"T001","stage":"DISCOVER","action":"reading","detail":"internal/a.go"}`,
		`{"task_id":"T001","stage":"VALIDATE","action":"go test ./..."}`,
	)

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusIncomplete, Summary: "no evidence"}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	code, stdout, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("an evidence-insufficient JEV must not pass; stdout=%s stderr=%s", stdout, stderr)
	}

	if got := analyzer.requests[0].ChangedFiles; got != nil {
		t.Errorf("ChangedFiles = %v, want none (no reliable attribution)", got)
	}
	if _, err := os.Stat(filepath.Join(legacyRunDir(dir, "T001"), changedFilesArtifact)); err == nil {
		t.Error("changed-files.json was manufactured despite ambiguous evidence")
	}
}

// The bootstrap is idempotent, and the persisted evidence stands on its own.
func TestJEVLegacyBootstrapIsIdempotent(t *testing.T) {
	dir := setupJEVTask(t)
	seedLegacyActivity(t, dir, "T001", changeEvent("T001", "editing", "internal/a.go"))

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("first run: code=%d stderr=%s", code, stderr)
	}
	// Clear the legacy stream: the persisted artifact must now stand alone.
	seedLegacyActivity(t, dir, "T001")
	analyzer.calls = 0
	analyzer.requests = nil

	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("second run: code=%d stderr=%s", code, stderr)
	}
	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("ChangedFiles = %v, want the persisted [internal/a.go] without re-bootstrapping", got)
	}
}

// The bootstrap only reads the activity stream and writes run evidence; it never
// touches repository source.
func TestJEVLegacyBootstrapDoesNotModifyRepository(t *testing.T) {
	dir := setupJEVTask(t)
	const before = "package internal\n\nconst Marker = 1\n"
	writeRepoFile(t, dir, "internal/a.go", before)
	seedLegacyActivity(t, dir, "T001", changeEvent("T001", "editing", "internal/a.go"))

	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), noChangeAgent(), factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	after, err := os.ReadFile(filepath.Join(dir, "internal/a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before {
		t.Errorf("repository source was modified by the bootstrap:\n%s", after)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err == nil {
		t.Error("the bootstrap created a repository file")
	}
}
