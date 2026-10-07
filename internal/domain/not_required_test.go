package domain

import "testing"

// TestMarkNotRequiredFromPlanned proves PLANNED -> NOT_REQUIRED records the reason in
// history, is terminal, and is not an implementation completion.
func TestMarkNotRequiredFromPlanned(t *testing.T) {
	task := &Task{ID: "T004", Status: PLANNED}
	if err := task.MarkNotRequired("AS-CLEF-003 found no leak"); err != nil {
		t.Fatalf("MarkNotRequired: %v", err)
	}
	if task.Status != NOT_REQUIRED {
		t.Fatalf("status = %s, want NOT_REQUIRED", task.Status)
	}
	if n := len(task.Attempts); n != 1 || task.Attempts[0].Status != NOT_REQUIRED || task.Attempts[0].Reason != "AS-CLEF-003 found no leak" {
		t.Errorf("history = %+v, want one NOT_REQUIRED entry with the reason", task.Attempts)
	}
	if !task.IsSatisfied() {
		t.Error("NOT_REQUIRED must satisfy dependants")
	}
	if task.IsCompleted() || task.IsDone() || task.IsBlocked() {
		t.Error("NOT_REQUIRED must not read as completed, done, or blocked")
	}
	if task.IsRunnable() {
		t.Error("NOT_REQUIRED must not be runnable")
	}
	if err := task.Requeue(); err == nil {
		t.Error("NOT_REQUIRED must not be requeued")
	}
	if err := task.Block(NO_PROGRESS); err == nil {
		t.Error("NOT_REQUIRED must not be blocked")
	}
	if err := task.CompleteExternally(); err == nil {
		t.Error("NOT_REQUIRED must not be converted into an external completion")
	}
}

// TestMarkNotRequiredFromBlockedKeepsHistory proves BLOCKED -> NOT_REQUIRED keeps the
// earlier attempts, clears the blocked reason, and appends the disposition.
func TestMarkNotRequiredFromBlockedKeepsHistory(t *testing.T) {
	task := &Task{ID: "T004", Status: PLANNED}
	_ = task.AddAttempt(BLOCKED, "IMPLEMENT_NO_PROGRESS")
	if err := task.Block(NO_PROGRESS); err != nil {
		t.Fatal(err)
	}
	if err := task.MarkNotRequired("gate evidence"); err != nil {
		t.Fatalf("MarkNotRequired: %v", err)
	}
	if task.Status != NOT_REQUIRED || task.BlockedReason != NO_REASON {
		t.Errorf("status=%s reason=%s, want NOT_REQUIRED with no blocked reason", task.Status, task.BlockedReason)
	}
	if len(task.Attempts) != 2 || task.Attempts[0].Reason != "IMPLEMENT_NO_PROGRESS" || task.Attempts[1].Status != NOT_REQUIRED {
		t.Errorf("history = %+v, want the prior attempt then the disposition", task.Attempts)
	}
}

func TestMarkNotRequiredRejects(t *testing.T) {
	if err := (&Task{ID: "T", Status: PLANNED}).MarkNotRequired("  "); err == nil {
		t.Error("an empty reason must be rejected")
	}
	for _, s := range []TaskStatus{IMPLEMENTING, REVIEW, FIX_REQUIRED, LOCAL_DONE, DONE, MERGED, NOT_REQUIRED} {
		task := &Task{ID: "T", Status: s}
		if err := task.MarkNotRequired("reason"); err == nil {
			t.Errorf("status %s: want an error, got none", s)
		}
		if task.Status != s || len(task.Attempts) != 0 {
			t.Errorf("status %s: task changed on a rejected disposition: %+v", s, task)
		}
	}
}

// TestNotRequiredSatisfiesDependencyProgression proves a dependant becomes runnable and
// the plan can complete once its dependency is NOT_REQUIRED.
func TestNotRequiredSatisfiesDependencyProgression(t *testing.T) {
	dep := &Task{ID: "T004", Status: BLOCKED, BlockedReason: NO_PROGRESS}
	next := &Task{ID: "T005", Status: PLANNED, DependencyIDs: []string{"T004"}}
	byID := map[string]*Task{"T004": dep, "T005": next}
	if _, ok := next.ResolveDependencies(byID); ok {
		t.Fatal("a BLOCKED dependency must not be satisfied")
	}
	if err := dep.MarkNotRequired("not needed"); err != nil {
		t.Fatal(err)
	}
	if unmet, ok := next.ResolveDependencies(byID); !ok {
		t.Errorf("NOT_REQUIRED dependency must be satisfied; unmet = %+v", unmet)
	}
	next.Status = LOCAL_DONE
	if !AllSatisfied([]*Task{dep, next}) {
		t.Error("a plan of LOCAL_DONE and NOT_REQUIRED tasks must be satisfied")
	}
}
