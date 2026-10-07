package planflow

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// Reconciliation must preserve every historical lifecycle value even when it
// installs a material change to a different, never-executed task.
func TestPendingReconciliationPreservesEveryLifecycleState(t *testing.T) {
	states := []domain.TaskStatus{
		domain.PLANNED, domain.READY, domain.BRANCH_CREATED, domain.TESTS_WRITTEN,
		domain.RED_VERIFIED, domain.IMPLEMENTING, domain.LOCAL_TESTS_PASS,
		domain.REVIEW, domain.REVIEW_PASS, domain.PR_OPEN, domain.CI_RUNNING,
		domain.CI_PASS, domain.FIX_REQUIRED, domain.MERGED, domain.DONE,
		domain.LOCAL_DONE, domain.BLOCKED, domain.NOT_REQUIRED,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			dir := t.TempDir()
			st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
			historical := taskByID(t, st.tasks, "S001")
			historical.Status = state
			historical.Attempt = 2
			historical.MaxAttempts = 5
			historical.Attempts = []domain.Attempt{{
				Number: 2, Status: state, Reason: "historical disposition",
				Output: "retained evidence", Duration: time.Second,
				Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			}}
			if state == domain.BLOCKED {
				historical.BlockedReason = domain.CONTINUATION_EXHAUSTED
			}
			before := *historical
			before.Attempts = append([]domain.Attempt(nil), historical.Attempts...)
			if historical.DependencyIDs != nil {
				before.DependencyIDs = append([]string{}, historical.DependencyIDs...)
			}
			// Capture the historical execution evidence (report/audit/approval markers)
			// so a pending-task update is proven not to disturb it. The compiled plan
			// artifacts are deliberately excluded: a legitimate update refreshes them.
			markers := seedEvidenceMarkers(t, dir)
			evidence := make(map[string]string, len(markers))
			for path := range markers {
				evidence[path] = read(t, filepath.Join(dir, path))
			}
			// The completed task receives only cosmetic formatting differences;
			// the pending task gets an intentional objective change.
			changed := strings.Replace(reconcileDocTwo, "Create it.", "Create   it.", 1)
			changed = strings.Replace(changed, "Ingest it.", "Ingest it with a documented schema.", 1)
			write(t, dir, "PLAN.md", changed)
			result, err := Reconcile(context.Background(), ReconcileOptions{
				Dir: dir, PlanSource: filepath.Join(dir, "PLAN.md"), Store: st,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Updated, []string{"S002"}) ||
				len(result.ChangedExecuted) != 0 || len(result.Accepted) != 0 ||
				len(result.AutoReconciled) != 0 {
				t.Fatalf("unexpected reconciliation: %+v", result)
			}
			got := taskByID(t, st.tasks, "S001")
			if got != historical || !reflect.DeepEqual(*got, before) {
				t.Fatalf("historical task changed: got %+v, want %+v", got, before)
			}
			for path, want := range evidence {
				if got := read(t, filepath.Join(dir, path)); got != want {
					t.Errorf("historical evidence %s changed: got %q, want %q", path, got, want)
				}
			}
			pending := taskByID(t, st.tasks, "S002")
			if pending.Status != domain.PLANNED || pending.Attempt != 0 ||
				pending.Objective != "Ingest it with a documented schema." {
				t.Fatalf("pending update lost lifecycle or definition: %+v", pending)
			}
		})
	}
}
