package domain

import "testing"

// These tests pin the single, authoritative definition of "executed": execution
// began when the task recorded an attempt, or left the pre-execution states
// (PLANNED, READY). Readiness is a scheduling fact, not execution evidence.

func TestExecutedNeverForPreExecutionStates(t *testing.T) {
	for _, s := range []TaskStatus{PLANNED, READY} {
		if (&Task{ID: "T", Status: s}).Executed() {
			t.Errorf("status %s with no attempts must NOT be executed", s)
		}
	}
}

func TestExecutedTrueWhenAttemptRecorded(t *testing.T) {
	if !(&Task{ID: "T", Status: READY, Attempt: 1}).Executed() {
		t.Error("a task that recorded an attempt is executed")
	}
	if !(&Task{ID: "T", Status: PLANNED, Attempts: []Attempt{{Number: 1}}}).Executed() {
		t.Error("a task with attempt history is executed")
	}
}

func TestExecutedTrueOnceExecutionBegan(t *testing.T) {
	for _, s := range []TaskStatus{
		BRANCH_CREATED, TESTS_WRITTEN, RED_VERIFIED, IMPLEMENTING, LOCAL_TESTS_PASS,
		REVIEW, REVIEW_PASS, PR_OPEN, CI_RUNNING, CI_PASS, FIX_REQUIRED,
		MERGED, DONE, LOCAL_DONE, BLOCKED,
	} {
		if !(&Task{ID: "T", Status: s}).Executed() {
			t.Errorf("status %s must be executed", s)
		}
	}
}
