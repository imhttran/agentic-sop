package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Environment a started background job's own process reads on exit. The MCP
// server sets them on the detached child, so the child records its terminal state
// and enqueues its lifecycle event itself. That is what makes a job that outlives
// the MCP server still notify: the server-side watcher only runs while the server
// is alive, so it cannot be the sole emitter.
const (
	JobEnvDir     = "SOP_JOB_DIR"
	JobEnvID      = "SOP_JOB_ID"
	JobEnvProject = "SOP_JOB_PROJECT"
)

// FinishBackgroundJob records a started job's exit and enqueues its terminal event
// from the job's own process. It is a no-op for an ordinary foreground invocation,
// is best-effort, and never changes the process exit code: notification is
// provenance, not execution.
func FinishBackgroundJob(code int) {
	jobDir := strings.TrimSpace(os.Getenv(JobEnvDir))
	if jobDir == "" {
		return
	}
	jobID := strings.TrimSpace(os.Getenv(JobEnvID))
	if jobID == "" {
		jobID = filepath.Base(jobDir)
	}
	projectDir := strings.TrimSpace(os.Getenv(JobEnvProject))
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	// Record the exit first, so a crash between the two writes still leaves the
	// truthful outcome on disk; the event is derived from it.
	_ = writeJSONFile(filepath.Join(jobDir, workExitFile), workExit{ExitCode: code, EndedAt: time.Now().UTC()})

	next := "inspect the result with sop_work_status or sop status"
	if code == 0 {
		_, _, _ = emitTransitionCompleted(projectDir, "", jobID, 1,
			fmt.Sprintf("job %s completed (exit 0)", jobID), next)
		return
	}
	_, _, _ = emitTransitionFailed(projectDir, "", jobID, 1, false,
		fmt.Sprintf("job %s exited %d before recording a task result", jobID, code),
		"inspect the job log with sop_work_status")
}
