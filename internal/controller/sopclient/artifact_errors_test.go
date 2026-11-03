package sopclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// damagedTask seeds one task and returns its detail view, read straight from the
// store so no SOP binary is needed.
func damagedTask(t *testing.T, root string) TaskDetail {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','BLOCKED','X',1,3,'`+now+`','`+now+`')`)
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	d, err := st.Task(context.Background(), "t1", ApprovalsListing{})
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	return d
}

// Absence stays absence: nothing is reported as damaged when no artifact exists.
func TestAbsentArtifactsReportNoError(t *testing.T) {
	d := damagedTask(t, newProject(t))

	if d.RunError != "" || d.ValidationError != "" || d.ReviewError != "" || d.ActivityError != "" {
		t.Fatalf("absent artifacts reported as damaged: run=%q validation=%q review=%q activity=%q",
			d.RunError, d.ValidationError, d.ReviewError, d.ActivityError)
	}
	if d.ValidationStatus.Present || d.ReviewStatus.Present || d.RunStatus.Present {
		t.Fatalf("absent artifacts reported present: validation=%+v review=%+v run=%+v",
			d.ValidationStatus, d.ReviewStatus, d.RunStatus)
	}
}

// A corrupt artifact is damage, not absence: the file is named and the
// corresponding status is never presented as a clean or absent result.
func TestCorruptArtifactsAreReportedAsDamaged(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		check func(TaskDetail) string
	}{
		{"validation", "validation.json", func(d TaskDetail) string { return d.ValidationError }},
		{"review", "review.json", func(d TaskDetail) string { return d.ReviewError }},
		{"state", "state.json", func(d TaskDetail) string { return d.RunError }},
		{"report", "report.json", func(d TaskDetail) string { return d.RunError }},
		{"classification", "classification.json", func(d TaskDetail) string { return d.RunError }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			writeArtifact(t, root, "t1", tc.file, `{"truncated": `)

			d := damagedTask(t, root)
			got := tc.check(d)
			if got == "" {
				t.Fatalf("corrupt %s was not reported (absence reported instead)", tc.file)
			}
			if !strings.Contains(got, tc.file) {
				t.Fatalf("error %q does not name %s", got, tc.file)
			}
			if strings.Contains(got, "\n") {
				t.Fatalf("error %q spans lines; a template renders one line", got)
			}
		})
	}
}

// A corrupt validation or review artifact must not be aggregated into a status:
// the view reports the read failure instead of "never ran" or a verdict.
func TestCorruptEvidenceDoesNotAggregate(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "validation.json", "{oops")
	writeArtifact(t, root, "t1", "review.json", "{oops")

	d := damagedTask(t, root)
	if d.ValidationStatus.Present || d.ValidationStatus.Level == ValidationPass {
		t.Fatalf("corrupt validation aggregated as %+v", d.ValidationStatus)
	}
	if d.ReviewStatus.Present || d.ReviewStatus.Level == ReviewPassed {
		t.Fatalf("corrupt review aggregated as %+v", d.ReviewStatus)
	}
}

// An artifact that exists but cannot be read is damage too, not absence.
func TestUnreadableArtifactIsReported(t *testing.T) {
	root := newProject(t)
	dir := filepath.Join(root, ".agent-sdlc", "runs", "t1", "validation.json")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	d := damagedTask(t, root)
	if !strings.Contains(d.ValidationError, "validation.json") {
		t.Fatalf("ValidationError = %q, want the unreadable artifact named", d.ValidationError)
	}
}

// Skipped activity lines are counted, and the readable history still renders.
func TestActivitySkipsAreCounted(t *testing.T) {
	root := newProject(t)
	good := `{"stage":"IMPLEMENTING","action":"mutate","detail":"edited a file","timestamp":"2026-11-02T10:00:00Z"}`
	writeArtifact(t, root, "t1", "activity.jsonl",
		good+"\n"+`{"stage":"BROKEN"`+"\n"+strings.Repeat("x", maxActivityLine+1)+"\n")

	d := damagedTask(t, root)
	if len(d.Activity) != 1 {
		t.Fatalf("Activity = %d events, want the one readable event", len(d.Activity))
	}
	if d.ActivitySkipped != 2 {
		t.Fatalf("ActivitySkipped = %d, want 2 (one malformed, one oversized)", d.ActivitySkipped)
	}
	if d.ActivityError != "" {
		t.Fatalf("ActivityError = %q, want no read error", d.ActivityError)
	}
}

// Several damaged artifacts are reported together, one line each.
func TestMultipleDamagedArtifactsAreJoined(t *testing.T) {
	root := newProject(t)
	writeArtifact(t, root, "t1", "state.json", "{oops")
	writeArtifact(t, root, "t1", "report.json", "{oops")

	d := damagedTask(t, root)
	if !strings.Contains(d.RunError, "state.json") || !strings.Contains(d.RunError, "report.json") {
		t.Fatalf("RunError = %q, want both damaged artifacts named", d.RunError)
	}
}
