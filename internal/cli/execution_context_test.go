//go:build unix

package cli

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// TestExecutionContextCancelsOnSignal proves the foreground run context is
// cancelled by SIGTERM (and therefore SIGINT), so signal handling can stop
// owned work instead of the process dying without cleanup.
func TestExecutionContextCancelsOnSignal(t *testing.T) {
	ctx, stop := executionContext()
	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("context was not cancelled by SIGTERM")
	}
}
