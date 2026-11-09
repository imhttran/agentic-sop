//go:build unix

package testrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A check that outlives Runner.Timeout is Canceled, and the children its shell
// started die with it instead of writing after the check has returned.
func TestCheckTimeoutKillsProcessGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := &Runner{Dir: dir, Timeout: 50 * time.Millisecond}

	start := time.Now()
	res := r.Run(context.Background(), Check{Category: UnitTest, Command: "(sleep 0.3; touch late) & wait"})
	if res.Status != Canceled {
		t.Fatalf("status = %s, want %s", res.Status, Canceled)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("check returned after %v, want it bounded by the timeout", d)
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "late")); err == nil {
		t.Fatal("a child of the timed-out check survived and wrote after the timeout")
	}
}
