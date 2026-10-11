package testrunner

import (
	"context"
	"testing"
	"time"
)

// TestRunAllFailFastSkipsRemainingChecks: a failing check stops the suite, so a
// configured later check is skipped. The skipped check is never counted as passed,
// and the suite reports the failing status.
func TestRunAllFailFastSkipsRemainingChecks(t *testing.T) {
	suite := New(t.TempDir()).RunAll(context.Background(), []Check{
		{Category: Build, Command: "false"},
		{Category: UnitTest, Command: "true"},
		{Category: Lint, Command: "true"},
	})
	if suite.Status != Fail {
		t.Fatalf("status = %s, want FAIL", suite.Status)
	}
	if suite.Passed() {
		t.Fatal("a suite with a failed check must not report passed")
	}
	if len(suite.Results) != 1 {
		t.Fatalf("fail-fast ran %d checks, want 1 (the later checks are skipped)", len(suite.Results))
	}
}

// TestRunAllSuccessPasses: an all-green suite reports PASS.
func TestRunAllSuccessPasses(t *testing.T) {
	suite := New(t.TempDir()).RunAll(context.Background(), []Check{
		{Category: Build, Command: "true"},
		{Category: UnitTest, Command: "true"},
	})
	if !suite.Passed() {
		t.Fatalf("status = %s, want PASS", suite.Status)
	}
}

// TestRunAllCanceledContextIsNotPass: a canceled context is CANCELED, never a pass.
func TestRunAllCanceledContextIsNotPass(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	suite := New(t.TempDir()).RunAll(ctx, []Check{{Category: Build, Command: "sleep 5"}})
	if suite.Status != Canceled {
		t.Fatalf("status = %s, want CANCELED", suite.Status)
	}
	if suite.Passed() {
		t.Fatal("a canceled check must not report passed")
	}
}

// TestRunAllDeadlineExceededIsNotPass: a check that runs out of time is CANCELED
// (not failed, not passed), so a timeout can never be mistaken for verification.
func TestRunAllDeadlineExceededIsNotPass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	suite := New(t.TempDir()).RunAll(ctx, []Check{{Category: UnitTest, Command: "sleep 5"}})
	if suite.Status != Canceled {
		t.Fatalf("status = %s, want CANCELED for a timed-out check", suite.Status)
	}
	if suite.Passed() {
		t.Fatal("a timed-out check must not report passed")
	}
}
