package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/notify"
)

// TestFinishBackgroundJobEmitsFromTheJobProcess proves a detached job records its
// own exit and enqueues its terminal event, so it notifies even when the process
// that started it is gone.
func TestFinishBackgroundJobEmitsFromTheJobProcess(t *testing.T) {
	project := t.TempDir()
	jobDir := filepath.Join(project, "mcp-work", "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv(JobEnvDir, jobDir)
	t.Setenv(JobEnvID, "job-1")
	t.Setenv(JobEnvProject, project)

	sink := NewMemoryEventSink()
	prev := setLifecycleSink(sink)
	defer setLifecycleSink(prev)

	FinishBackgroundJob(0)
	var exit workExit
	data, err := os.ReadFile(filepath.Join(jobDir, workExitFile))
	if err != nil {
		t.Fatalf("exit record: %v", err)
	}
	if err := json.Unmarshal(data, &exit); err != nil {
		t.Fatalf("decode exit: %v", err)
	}
	if exit.ExitCode != 0 || exit.EndedAt.IsZero() {
		t.Fatalf("exit = %+v", exit)
	}
	if sink.Len() != 1 {
		t.Fatalf("events = %d, want 1 completed event", sink.Len())
	}
	if got := sink.Events()[0]; got.Kind != notify.KindCompleted || got.JobID != "job-1" {
		t.Fatalf("event = %+v", got)
	}

	FinishBackgroundJob(2)
	if sink.Len() != 2 {
		t.Fatalf("events = %d, want a failed event for the second exit", sink.Len())
	}
	if got := sink.Events()[1]; got.Kind != notify.KindFailed || got.JobID != "job-1" {
		t.Fatalf("second event = %+v", got)
	}
}

// TestFinishBackgroundJobIsNoOpInForeground proves an ordinary invocation with no
// job environment records nothing and emits nothing.
func TestFinishBackgroundJobIsNoOpInForeground(t *testing.T) {
	t.Setenv(JobEnvDir, "")
	sink := NewMemoryEventSink()
	prev := setLifecycleSink(sink)
	defer setLifecycleSink(prev)

	FinishBackgroundJob(0)
	if sink.Len() != 0 {
		t.Fatalf("foreground invocation emitted %d events", sink.Len())
	}
}
