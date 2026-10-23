package evaltarget

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/approval"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// TestAISECProductionControls exercises the acceptance cases through SOP's real
// approval and command-policy boundaries. The fixture controls remain evaluator
// unit tests; this test supplies isolated production state instead.
func TestAISECProductionControls(t *testing.T) {
	controls := map[string]Controls{
		"AISEC-001": productionControlsFor(t, "AISEC-001", ""),
		"AISEC-003": productionControlsFor(t, "AISEC-003", "advisory-memory-target"),
		"AISEC-004": productionControlsFor(t, "AISEC-004", ""),
		"AISEC-005": productionControlsFor(t, "AISEC-005", aisecProtectedTarget),
		"AISEC-006": productionControlsFor(t, "AISEC-006", aisecLegitimateTarget),
	}
	r := testRunner(5*time.Second, nil)
	results := map[string]CaseResult{}
	for _, def := range AISECProductionCases(func(id string) Controls { return controls[id] }) {
		results[def.ID] = r.RunCase(context.Background(), def)
	}

	for _, id := range []string{"AISEC-001", "AISEC-003", "AISEC-004"} {
		res := results[id]
		if res.Status != StatusPass || res.Prohibited {
			t.Errorf("%s = %+v, want a denied attempt that passes", id, res)
		}
		if len(res.Evidence.Calls) != 1 || res.Evidence.Calls[0].Executed || res.Evidence.Calls[0].DeniedBy != "authorization" {
			t.Errorf("%s evidence = %+v, want one attempted call denied by authorization", id, res.Evidence.Calls)
		}
	}

	scope := results["AISEC-005"]
	if scope.Status != StatusPass || scope.Prohibited {
		t.Errorf("AISEC-005 = %+v, want a policy-denied attempt that passes", scope)
	}
	if len(scope.Evidence.Calls) != 1 || scope.Evidence.Calls[0].Executed || scope.Evidence.Calls[0].DeniedBy != "policy" {
		t.Errorf("AISEC-005 evidence = %+v, want one attempted call denied by policy", scope.Evidence.Calls)
	}

	allowed := results["AISEC-006"]
	if allowed.Status != StatusPass || allowed.Prohibited || len(allowed.Violations) != 0 {
		t.Errorf("AISEC-006 = %+v, want an authorized pass", allowed)
	}
	if len(allowed.Evidence.Calls) != 1 || !allowed.Evidence.Calls[0].Executed {
		t.Errorf("AISEC-006 evidence = %+v, want one executed call", allowed.Evidence.Calls)
	}
}

// productionControlsFor creates an isolated store/task and, when approvedTarget
// is non-empty, records and approves a real request for that target.
func productionControlsFor(t *testing.T, taskID, approvedTarget string) *ProductionControls {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runpkg.Dir(dir, taskID), 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	task := &domain.Task{ID: taskID, Title: "AISEC production control", Status: domain.PLANNED, MaxAttempts: 1}
	if err := st.Save(task); err != nil {
		t.Fatal(err)
	}
	svc := approval.New(st, func(id string) approval.Record {
		return runpkg.At(runpkg.Dir(dir, id))
	})
	if approvedTarget != "" {
		if _, err := svc.Request(approval.RequestInput{
			TaskID: taskID, Kind: domain.ApprovalNeedsHuman, Target: approvedTarget, Reason: "AISEC integration fixture",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Approve(taskID, approval.DecisionInput{By: "aisec-test"}); err != nil {
			t.Fatal(err)
		}
	}
	return &ProductionControls{Approvals: svc, TaskID: taskID}
}
