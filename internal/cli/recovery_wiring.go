package cli

// SOP-004 recovery wiring. The pure recovery package owns classification and the
// bounded decision; this file is the production seam that records its evidence and
// applies the no-progress refusal. It never edits state.db directly and never
// resets a budget.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/recovery"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

const (
	recoveryJournalName = "recovery.jsonl"
	blockerReportName   = "blocker.json"
)

// recoveryEvidence derives the typed evidence from SOP's own observations. It
// never parses agent prose; the classification, the recorded change set, and the
// outcome signature are the inputs.
func recoveryEvidence(rn *runpkg.Run, c failure.Classification, signature string) recovery.AttemptEvidence {
	changed := rn != nil && len(rn.ChangedFiles()) > 0
	// Discovery-only is the harness no-progress signal (or an explicit no-change
	// verdict) with no repository change. A productive CONTINUE (unfinished but
	// useful work) is deliberately NOT discovery-only: the existing continuation
	// budget owns that case.
	discoveryOnly := !changed && c.Kind == failure.NoProgress
	return recovery.AttemptEvidence{
		Kind:          c.Kind,
		Disposition:   c.Disposition,
		RepoChanged:   changed,
		DiscoveryOnly: discoveryOnly,
		Signature:     signature,
	}
}

// recordRecovery appends one recovery record to the run's durable journal so the
// history and the single blocker report survive a process restart.
func recordRecovery(rn *runpkg.Run, rec recovery.Recovery) error {
	if rn == nil {
		return nil
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(rn.Dir(), recoveryJournalName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// recoveryHistory reads the run's recovery journal in order. A missing journal is
// an empty history; a malformed line is ignored rather than inventing a recovery.
func recoveryHistory(rn *runpkg.Run) []recovery.Recovery {
	if rn == nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(rn.Dir(), recoveryJournalName))
	if err != nil {
		return nil
	}
	var out []recovery.Recovery
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec recovery.Recovery
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Class.Valid() {
			out = append(out, rec)
		}
	}
	return out
}

// writeBlockerReport persists the single actionable blocker report. A report that
// already exists is preserved: repeated exhaustion events for the same task do
// not overwrite the first diagnosis.
func writeBlockerReport(rn *runpkg.Run, rep recovery.BlockerReport) (bool, error) {
	if rn == nil {
		return false, nil
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return false, err
	}
	path := filepath.Join(rn.Dir(), blockerReportName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return false, err
	}
	return true, nil
}

// recoverRefusesRetry reports whether the recovery policy refuses to retry this
// attempt because it is unchanged or made no material change (a discovery-only
// attempt). In that case the correct terminal state is a bounded no-progress
// block, not another requeue.
func recoverRefusesRetry(dec recovery.Decision) bool {
	if dec.Action != recovery.ActionHuman {
		return false
	}
	return dec.Reason == recovery.ReasonUnchangedFailure || dec.Reason == recovery.ReasonNoMaterialChange
}

// applyNoProgressBlock records the recovery history and the single blocker report,
// then terminally blocks the task. It does not spend a retry or continuation.
func applyNoProgressBlock(saver taskSaver, task *domain.Task, rn *runpkg.Run, rec recovery.Recovery, budgets recovery.Budgets, stdout io.Writer) error {
	if err := recordRecovery(rn, rec); err != nil {
		return err
	}
	history := recoveryHistory(rn)
	rep := recovery.Exhausted(task.ID, history, rec.Class, budgets)
	if _, err := writeBlockerReport(rn, rep); err != nil {
		return err
	}
	if err := blockTask(saver, task, domain.NO_PROGRESS); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s BLOCKED (NO_PROGRESS): %s\n", task.ID, rep.LastReason)
	fmt.Fprintln(stdout, "  next: "+rep.NextAction)
	return nil
}
