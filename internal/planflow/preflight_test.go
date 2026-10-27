package planflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestPreflightPlanMatchesReconcileGates pins PreflightPlan to the gates Prepare
// applies, without mutating anything.
func TestPreflightPlanMatchesReconcileGates(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) string {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	active := write("docs/plans/ACTIVE.md", "# Active\n")
	next := write("docs/plans/NEXT.md", "# Next\n")
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	writeMeta := func(m Metadata) {
		t.Helper()
		data, _ := json.Marshal(m)
		if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metaPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	open := []*domain.Task{{ID: "A1", Status: domain.PLANNED}}
	done := []*domain.Task{{ID: "A1", Status: domain.LOCAL_DONE}}

	// No tasks: Prepare initializes, nothing to gate.
	if err := PreflightPlan(dir, next, nil); err != nil {
		t.Fatalf("no tasks: %v", err)
	}
	// No recorded provenance: reuse.
	if err := PreflightPlan(dir, next, open); err != nil {
		t.Fatalf("no metadata: %v", err)
	}

	writeMeta(Metadata{Source: "docs/plans/ACTIVE.md", SourceSHA256: SourceFingerprint([]byte("# Active\n")), PlanID: "plan-active"})
	if err := PreflightPlan(dir, active, open); err != nil {
		t.Fatalf("same plan, same content: %v", err)
	}
	if err := PreflightPlan(dir, next, open); err == nil || !strings.Contains(err.Error(), "different plan is already active") {
		t.Fatalf("different plan with open work: %v", err)
	}
	if err := PreflightPlan(dir, next, done); err != nil {
		t.Fatalf("different plan after completion should hand off: %v", err)
	}
	write("docs/plans/ACTIVE.md", "# Active, edited\n")
	if err := PreflightPlan(dir, active, done); err == nil || !strings.Contains(err.Error(), "plan changed") {
		t.Fatalf("edited active plan: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DirName, planFileName)); !os.IsNotExist(err) {
		t.Fatalf("preflight must not write plan.json: %v", err)
	}

	src, sha, ok := ActivePlanSource(dir)
	if !ok || src != "docs/plans/ACTIVE.md" || sha != SourceFingerprint([]byte("# Active\n")) {
		t.Fatalf("ActivePlanSource = %q %q %v", src, sha, ok)
	}
	if got := SourceRel(dir, active); got != "docs/plans/ACTIVE.md" {
		t.Fatalf("SourceRel = %q", got)
	}
}
