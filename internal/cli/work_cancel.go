package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cancellation of adapter-owned started work (ACP-006).
//
// A started job runs as a detached child (detachedProcAttr gives it its own
// session on Unix and its own process group on Windows). The job record carries
// its pid, and the exit recorder writes exit.json when the child exits.
//
// Cancellation cannot be derived from the exit record: signalling a child makes
// it exit non-zero, which would be indistinguishable from a failure. It is
// therefore recorded as a separate cancelled.json marker, written only after the
// child is confirmed gone. listWorkJobs treats the marker as authoritative, and
// a job that finished before the stop request never receives one, so a
// completion always keeps its own outcome.

// workCancelFile is the cancellation marker inside a job directory.
const workCancelFile = "cancelled.json"

// workStateCancelled is the derived state of a job stopped by cancellation. It
// is distinct from completed, failed, and ended.
const workStateCancelled = "cancelled"

// workCancel records one confirmed cancellation in the job's own directory.
type workCancel struct {
	CancelledAt time.Time `json:"cancelled_at"`
	By          string    `json:"by,omitempty"`
}

// workStopper signals one started job's child process (force selects the
// unignorable signal). It is a seam so cancellation is testable without
// signalling the test process itself.
type workStopper func(pid int, force bool) error

// workCancelStop bounds how long cancelWorkJob waits for the child to exit after
// the first signal, and again after the forced signal. It is a variable so tests
// can shorten it.
var workCancelStop = 5 * time.Second

// workCancelPoll is the liveness poll interval while waiting out that bound.
var workCancelPoll = 25 * time.Millisecond

// cancelWorkJob stops the started job jobID in dir and records the cancellation.
//
// It returns nil only when the job's record reflects a cancellation: this call
// stopped the child, the child had already finished (the record's own outcome
// wins), the job had already been cancelled (idempotent), or no live child
// remained to stop. It returns an error when the stop could not be confirmed
// within workCancelStop, and leaves no marker in that case, so no caller can
// report "cancelled" without evidence.
func cancelWorkJob(ctx context.Context, dir, jobID string, env workEnv) error {
	if ctx == nil {
		ctx = context.Background()
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || jobID != filepath.Base(jobID) || jobID == "." || jobID == ".." {
		return fmt.Errorf("cancel: invalid job id %q", jobID)
	}
	jobDir := filepath.Join(workRoot(dir), jobID)
	var job workJob
	if !readJSONFile(filepath.Join(jobDir, workJobFile), &job) || job.ID != jobID {
		return fmt.Errorf("cancel: no started job %q in this project", jobID)
	}
	cancelPath := filepath.Join(jobDir, workCancelFile)
	if _, err := os.Stat(cancelPath); err == nil {
		return nil // already cancelled: idempotent
	}
	if _, err := os.Stat(filepath.Join(jobDir, workExitFile)); err == nil {
		return nil // the job finished first: its recorded outcome wins
	}

	alive := env.alive
	if alive == nil {
		alive = workPIDAlive
	}
	if job.PID <= 0 || !alive(job.PID) {
		// No live child remains, and no exit was recorded: record the operator's
		// cancellation, because there is no completion to preserve.
		return writeWorkCancel(cancelPath)
	}

	stop := env.stop
	if stop == nil {
		stop = signalWorkStop
	}

	// Record the intent before signalling, so the exit recorder cannot relabel a
	// child that dies from the signal. The marker is removed again if the stop is
	// not confirmed.
	if err := writeWorkCancel(cancelPath); err != nil {
		return err
	}
	if err := stop(job.PID, false); err != nil {
		_ = os.Remove(cancelPath)
		return fmt.Errorf("cancel: stop job %s: %w", jobID, err)
	}
	if waitGone(ctx, job.PID, alive) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(cancelPath)
		return fmt.Errorf("cancel: job %s: %w", jobID, err)
	}
	if err := stop(job.PID, true); err != nil {
		_ = os.Remove(cancelPath)
		return fmt.Errorf("cancel: force-stop job %s: %w", jobID, err)
	}
	if waitGone(ctx, job.PID, alive) {
		return nil
	}
	_ = os.Remove(cancelPath)
	return fmt.Errorf("cancel: job %s is still running after the forced stop; the cancellation is not confirmed", jobID)
}

// writeWorkCancel records one confirmed cancellation.
func writeWorkCancel(path string) error {
	return writeJSONFile(path, workCancel{CancelledAt: time.Now().UTC(), By: "sop"})
}

// waitGone reports whether pid stopped within workCancelStop and the caller's
// bound. The caller's context wins: an expired CancelStopBound is never extended
// by this helper.
func waitGone(ctx context.Context, pid int, alive workProcessAlive) bool {
	deadline := time.Now().Add(workCancelStop)
	for {
		if !alive(pid) {
			return true
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return false
		}
		time.Sleep(workCancelPoll)
	}
}
