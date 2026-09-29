package run

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Legacy activity-evidence reader tests. The activity stream records a task's
// repository mutations (CHANGE events) with the changed path, so it is the
// authoritative legacy source for a task that predates changed-files.json.

// writeActivity seeds the run's activity artifact with the given JSON lines.
func writeActivity(t *testing.T, r *Run, lines ...string) {
	t.Helper()
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	if err := os.WriteFile(filepath.Join(r.Dir(), ActivityArtifactName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func changeLine(taskID, action, path string) string {
	return fmt.Sprintf(`{"task_id":%q,"stage":"CHANGE","action":%q,"detail":%q}`, taskID, action, path)
}

func TestActivityChangePathsRecoversMutations(t *testing.T) {
	r, _ := New(t.TempDir(), "T001")
	writeActivity(t, r,
		`{"task_id":"T001","stage":"START","action":"Add widget"}`,
		changeLine("T001", "editing", "internal/sopclient/activity_window.go"),
		`{"task_id":"T001","stage":"DISCOVER","action":"reading","detail":"internal/web/server.go"}`,
		changeLine("T001", "creating", "internal/web/activity_stream.go"),
		`{"task_id":"T001","stage":"VALIDATE","action":"go test ./..."}`,
		changeLine("T001", "editing", "static/activity-live.js"),
	)

	got := r.ActivityChangePaths()
	want := []string{
		"internal/sopclient/activity_window.go",
		"internal/web/activity_stream.go",
		"static/activity-live.js",
	}
	if len(got) != len(want) {
		t.Fatalf("ActivityChangePaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ActivityChangePaths = %v, want %v", got, want)
		}
	}
}

func TestActivityChangePathsIgnoresForeignAndPathlessEvents(t *testing.T) {
	r, _ := New(t.TempDir(), "T001")
	writeActivity(t, r,
		`{"stage":"CHANGE","action":"editing","detail":"no-task-id.go"}`, // untagged: allowed (run dir is task-scoped)
		changeLine("T999", "editing", "someone-elses.go"),                // another task
		`{"task_id":"T001","stage":"CHANGE","action":"editing"}`,         // CHANGE with no path
		`{"task_id":"T001","stage":"CHANGE","action":"editing","detail":"  "}`,
		`{not json`, // malformed line
		changeLine("T001", "editing", "kept.go"),
	)

	got := r.ActivityChangePaths()
	want := []string{"no-task-id.go", "kept.go"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ActivityChangePaths = %v, want %v", got, want)
	}
}

func TestActivityChangePathsDeduplicates(t *testing.T) {
	r, _ := New(t.TempDir(), "T001")
	writeActivity(t, r,
		changeLine("T001", "editing", "a.go"),
		changeLine("T001", "editing", "a.go"),
		changeLine("T001", "editing", "b.go"),
	)
	got := r.ActivityChangePaths()
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("ActivityChangePaths = %v, want [a.go b.go]", got)
	}
}

func TestActivityChangePathsIsBounded(t *testing.T) {
	r, _ := New(t.TempDir(), "T001")
	lines := make([]string, 0, maxActivityChangePaths*2)
	for i := 0; i < maxActivityChangePaths*2; i++ {
		lines = append(lines, changeLine("T001", "editing", fmt.Sprintf("p%03d.go", i)))
	}
	writeActivity(t, r, lines...)
	if got := r.ActivityChangePaths(); len(got) != maxActivityChangePaths {
		t.Errorf("ActivityChangePaths = %d paths, want the bound %d", len(got), maxActivityChangePaths)
	}
}

func TestActivityChangePathsMissingArtifact(t *testing.T) {
	r, _ := New(t.TempDir(), "T001")
	if got := r.ActivityChangePaths(); got != nil {
		t.Errorf("ActivityChangePaths with no artifact = %v, want nil", got)
	}
}
