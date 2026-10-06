package domain

import "testing"

// TestCompleteExternallyMovesToLocalDone proves an explicit external completion records
// the task in the existing local terminal state, without an attempt or an approval.
func TestCompleteExternallyMovesToLocalDone(t *testing.T) {
	task := &Task{ID: "T001", Status: PLANNED}
	if err := task.CompleteExternally(); err != nil {
		t.Fatalf("CompleteExternally: %v", err)
	}
	if task.Status != LOCAL_DONE {
		t.Errorf("status = %s, want LOCAL_DONE", task.Status)
	}
	if task.Attempt != 0 || len(task.Attempts) != 0 {
		t.Errorf("external completion fabricated an attempt: %+v", task)
	}
	if !task.IsSatisfied() {
		t.Error("an externally completed task must satisfy its dependants")
	}
}

// TestCompleteExternallyRejectsAlreadyComplete proves the transition fails closed on a
// task that is already complete.
func TestCompleteExternallyRejectsAlreadyComplete(t *testing.T) {
	for _, s := range []TaskStatus{LOCAL_DONE, DONE, MERGED} {
		task := &Task{ID: "T001", Status: s}
		if err := task.CompleteExternally(); err == nil {
			t.Errorf("status %s: want an error, got none", s)
		}
		if task.Status != s {
			t.Errorf("status %s changed to %s", s, task.Status)
		}
	}
}
