package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/store"
)

// Manual acceptance recovery (SOP-003).
//
// A manual acceptance decision is bound to the attempt whose outcome the operator
// reviewed. A supported recovery (the operator accepting the reviewed outcome and
// approving the gate) requeues the task and advances its attempt counter, so the
// next verification would otherwise see a stale attempt binding and reject a
// decision that is still valid for exactly the same criterion and evidence.
//
// The recovery audit is append-only and never rewrites the original decision or
// its attempt. Verification honors an earlier decision only when a recovery record
// connects that decision's attempt to the current attempt and the decision's
// criterion text, manual-evidence description, and evidence still match exactly.

const acceptanceRecoveryAuditName = "acceptance_recoveries.jsonl"

// acceptanceRecoveryPath is a task's durable, append-only acceptance-recovery audit.
func acceptanceRecoveryPath(projectDir, taskID string) string {
	return filepath.Join(projectDir, stateDirName, "runs", taskID, acceptanceRecoveryAuditName)
}

// readAcceptanceRecoveries reads a task's recorded acceptance recoveries in file
// order. A missing audit is no recoveries, not an error.
func readAcceptanceRecoveries(projectDir, taskID string) ([]outcome.AcceptanceRecovery, error) {
	return readJSONL[outcome.AcceptanceRecovery](acceptanceRecoveryPath(projectDir, taskID))
}

// recordAcceptanceRecoveriesOnApproval appends the recoveries authorized by a
// successful `sop approve`. It preserves every accepted decision recorded for an
// earlier attempt whose criterion and evidence are unchanged, and records the
// attempt transition that makes it applicable again. It is idempotent: an already
// recorded transition is not appended twice.
func recordAcceptanceRecoveriesOnApproval(dir string, st *store.Store, taskID, by string) error {
	task, err := st.Get(taskID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	if task == nil || task.Attempt <= 0 {
		return nil
	}
	res, err := readOutcomeArtifact(dir, taskID)
	if err != nil {
		return err
	}
	if res == nil {
		return nil
	}
	decisions, err := readAcceptanceDecisions(dir, taskID)
	if err != nil {
		return err
	}
	existing, err := readAcceptanceRecoveries(dir, taskID)
	if err != nil {
		return err
	}
	for _, d := range decisions {
		rec, ok := acceptanceRecoveryFor(taskID, d, res, task.Attempt, by, "approve")
		if !ok || recoveryAlreadyRecorded(existing, rec) {
			continue
		}
		if err := appendJSONL(acceptanceRecoveryPath(dir, taskID), rec); err != nil {
			return err
		}
		existing = append(existing, rec)
	}
	return nil
}

// recordAcceptanceRecoveryForDecision appends the recovery for a decision the
// operator recorded after the attempt had already advanced (the approval-first
// order). It is best-effort at the call site: the decision itself is already
// durable, so a failure is surfaced without discarding it.
func recordAcceptanceRecoveryForDecision(dir, taskID string, d outcome.AcceptanceDecision, res *outcomeArtifact, by string) error {
	toAttempt := currentTaskAttempt(dir, taskID)
	if toAttempt <= 0 {
		return nil
	}
	rec, ok := acceptanceRecoveryFor(taskID, d, res, toAttempt, by, "accept")
	if !ok {
		return nil
	}
	existing, err := readAcceptanceRecoveries(dir, taskID)
	if err != nil {
		return err
	}
	if recoveryAlreadyRecorded(existing, rec) {
		return nil
	}
	return appendJSONL(acceptanceRecoveryPath(dir, taskID), rec)
}

// acceptanceRecoveryFor builds the recovery for one accepted decision when the
// task's current attempt is later than the reviewed attempt and the decision is
// still bound to the outcome's manual mapping. Any changed criterion text or
// manual-evidence description yields no recovery, so acceptance cannot carry over
// to changed evidence.
func acceptanceRecoveryFor(taskID string, d outcome.AcceptanceDecision, res *outcomeArtifact, toAttempt int, by, source string) (outcome.AcceptanceRecovery, bool) {
	if res == nil || d.Decision != outcome.DecisionAccept {
		return outcome.AcceptanceRecovery{}, false
	}
	if d.Task != "" && d.Task != taskID {
		return outcome.AcceptanceRecovery{}, false
	}
	if d.Attempt < 0 || toAttempt <= d.Attempt {
		return outcome.AcceptanceRecovery{}, false
	}
	if strings.TrimSpace(d.CriterionDescription) == "" || strings.TrimSpace(d.ManualEvidence) == "" {
		return outcome.AcceptanceRecovery{}, false
	}
	for _, m := range res.Mappings {
		if m.CriterionID != d.CriterionID || m.Kind != outcome.EvidenceManual {
			continue
		}
		if m.Description != d.CriterionDescription || m.ManualEvidence != d.ManualEvidence {
			continue
		}
		return outcome.AcceptanceRecovery{
			Task:                 taskID,
			CriterionID:          d.CriterionID,
			CriterionDescription: d.CriterionDescription,
			ManualEvidence:       d.ManualEvidence,
			Evidence:             d.Evidence,
			FromAttempt:          d.Attempt,
			ToAttempt:            toAttempt,
			By:                   strings.TrimSpace(by),
			Source:               source,
			RecordedAt:           time.Now().UTC().Format(time.RFC3339),
		}, true
	}
	return outcome.AcceptanceRecovery{}, false
}

// recoveryAlreadyRecorded reports whether the same decision-to-attempt transition
// is already in the append-only audit. Provenance (who/when/source) is not part of
// the identity, so a repeated approve never duplicates the transition.
func recoveryAlreadyRecorded(existing []outcome.AcceptanceRecovery, rec outcome.AcceptanceRecovery) bool {
	for _, r := range existing {
		if r.Task == rec.Task &&
			r.CriterionID == rec.CriterionID &&
			r.CriterionDescription == rec.CriterionDescription &&
			r.ManualEvidence == rec.ManualEvidence &&
			r.Evidence == rec.Evidence &&
			r.FromAttempt == rec.FromAttempt &&
			r.ToAttempt == rec.ToAttempt {
			return true
		}
	}
	return false
}

// currentTaskAttempt reads the task's current attempt without creating or
// initializing state when the project has none.
func currentTaskAttempt(dir, taskID string) int {
	path := statePath(dir)
	if _, err := os.Stat(path); err != nil {
		return 0
	}
	st, err := store.Open(path)
	if err != nil {
		return 0
	}
	defer st.Close()
	task, err := st.Get(taskID)
	if err != nil || task == nil {
		return 0
	}
	return task.Attempt
}

// acceptanceRecoveryReadReason is the fail-closed diagnostic recorded on a verdict
// when the append-only recovery audit cannot be read.
func acceptanceRecoveryReadReason(err error) string {
	return fmt.Sprintf("acceptance recoveries could not be read: %v", err)
}
