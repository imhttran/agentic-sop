package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/durablefs"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/quality"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// stageRecoveryVersion is the schema version of the append-only stage-recovery
// audit written beside a repaired run's other artifacts.
const stageRecoveryVersion = 1

const (
	// stageRecoveryAuditName is the append-only audit of completed stage repairs.
	stageRecoveryAuditName = "stage-recoveries.jsonl"
	// stageRecoveryPendingName marks a repair whose stage may already be PASSED but
	// whose audit append has not yet succeeded. It lets a retry finish the audit
	// without duplicating a record.
	stageRecoveryPendingName = "stage-recovery.pending.json"
)

// stageRecovery records one explicit, auditable repair of a stale accepted-outcome
// run stage. It is diagnostic provenance only: it never changes the task status,
// attempt, outcome, acceptance decisions, or validation evidence.
type stageRecovery struct {
	ID              string   `json:"id"`
	Version         int      `json:"version"`
	Task            string   `json:"task"`
	FromStage       string   `json:"from_stage"`
	ToStage         string   `json:"to_stage"`
	OutcomeStatus   string   `json:"outcome_status"`
	Validation      string   `json:"validation"`
	ContentIdentity string   `json:"content_identity,omitempty"`
	Criteria        []string `json:"criteria,omitempty"`
	Reason          string   `json:"reason"`
	By              string   `json:"by"`
	RecordedAt      string   `json:"recorded_at"`
}

// appendStageRecoveryRecord is the audit append seam. Tests replace it to exercise
// an interrupted audit without changing production behavior.
var appendStageRecoveryRecord = appendJSONL

const taskRecoverStageUsage = "usage: sop task recover-stage <task-id> --by NAME --reason R"

// runTaskRecoverStage repairs the legacy accepted-outcome run-stage defect without
// rerunning planning, implementation, validation, or acceptance. It is deliberately
// narrow: the task must already be satisfied, have a persisted VERIFIED outcome whose
// validation passed, carry an APPROVED approval gate, and have a run stage of exactly
// WAITING_FOR_HUMAN. The accepted outcome is re-derived against the current repository
// content so stale acceptance bindings fail closed. Only the existing non-destructive
// PASSED stage checkpoint is written, plus an append-only audit record.
//
// The repair is two-phase: a pending marker is written before the stage changes, and
// the audit record is appended after. If the audit append fails, the stage is already
// PASSED but the pending marker remains; a retry sees PASSED plus the marker, appends
// the missing record once, and removes the marker. A retry after a stage-write failure
// reuses the same pending record and completes the transition.
func runTaskRecoverStage(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	id, by, reason, ok := parseTaskRecoverStageArgs(args, stderr)
	if !ok {
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		fmt.Fprintf(stderr, "task recover-stage: %v\n", err)
		return exitError
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "task recover-stage: %v\n", err)
		return exitError
	}
	defer st.Close()

	task, err := st.Get(id)
	if store.IsNotFound(err) {
		fmt.Fprintf(stderr, "task recover-stage: task %s not found\n", id)
		return exitError
	}
	if err != nil {
		fmt.Fprintf(stderr, "task recover-stage: %v\n", err)
		return exitError
	}
	if !task.IsSatisfied() {
		fmt.Fprintf(stderr, "task recover-stage: task %s is not complete (%s)\n", id, task.Status)
		return exitError
	}

	out, err := readOutcomeArtifact(dir, id)
	if err != nil {
		fmt.Fprintf(stderr, "task recover-stage: %v\n", err)
		return exitError
	}
	if out == nil || out.Status != outcome.Verified || out.Validation == nil || !out.Validation.Passed {
		fmt.Fprintf(stderr, "task recover-stage: %s has no VERIFIED accepted outcome with passed validation\n", id)
		return exitError
	}

	stage, ok := runpkg.Load(dir, id)
	if !ok {
		fmt.Fprintf(stderr, "task recover-stage: %s has no persisted run state\n", id)
		return exitError
	}

	rec := runpkg.At(runpkg.Dir(dir, id))
	auditPath := filepath.Join(rec.Dir(), stageRecoveryAuditName)
	pendingPath := filepath.Join(rec.Dir(), stageRecoveryPendingName)
	existing, err := readJSONL[stageRecovery](auditPath)
	if err != nil {
		fmt.Fprintf(stderr, "task recover-stage: read recovery audit: %v\n", err)
		return exitError
	}
	pending, pendingOK, err := readStageRecoveryPending(pendingPath)
	if err != nil {
		fmt.Fprintf(stderr, "task recover-stage: %v\n", err)
		return exitError
	}

	if stage == runpkg.Passed {
		if !pendingOK {
			fmt.Fprintf(stdout, "task %s: run stage is already %s; no recovery recorded\n", id, runpkg.Passed)
			return exitOK
		}
		if pending.Task != id {
			fmt.Fprintf(stderr, "task recover-stage: pending recovery belongs to %s, not %s\n", pending.Task, id)
			return exitError
		}
		if err := appendStageRecoveryIfMissing(auditPath, existing, pending); err != nil {
			fmt.Fprintf(stderr, "task recover-stage: finish interrupted audit: %v\n", err)
			return exitError
		}
		if err := os.Remove(pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "task recover-stage: recovery audit recorded, but the pending marker was not removed: %v; retry to finish\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "task %s: completed interrupted stage-recovery audit\n", id)
		return exitOK
	}
	if stage != runpkg.WaitingForHuman {
		fmt.Fprintf(stderr, "task recover-stage: %s run stage is %s; only %s can be repaired\n", id, stage, runpkg.WaitingForHuman)
		return exitError
	}

	req, ok := rec.Approval()
	if !ok || req.Status != domain.ApprovalApproved {
		fmt.Fprintf(stderr, "task recover-stage: %s has no APPROVED approval gate; refusing recovery\n", id)
		return exitError
	}

	// Re-derive the accepted outcome against the current content. A stale
	// acceptance decision, changed criterion text, or changed repository identity
	// yields PARTIAL and fails closed, so the repair cannot carry acceptance to
	// different evidence.
	spec := specFromTask(task)
	spec.Deliverables = planTaskDeliverables(dir, task)
	spec.DeliverableGroups = deliverableGroups(spec)
	spec.AcceptanceChecks = planTaskAcceptanceChecks(dir, task)
	spec.AcceptanceMapping = planTaskAcceptanceMapping(dir, task)
	if len(spec.AcceptanceMapping) == 0 {
		fmt.Fprintf(stderr, "task recover-stage: %s has no acceptance mapping\n", id)
		return exitError
	}
	derived := deriveGatedOutcome(*cfg, spec, dir, quality.Result{Decision: quality.Pass, Reasons: []string{"all required checks passed"}})
	if derived == nil || derived.Status != outcome.Verified {
		fmt.Fprintf(stderr, "task recover-stage: %s accepted outcome is not VERIFIED against the current content; re-run required\n", id)
		return exitError
	}

	var criteria []string
	identity := ""
	for _, m := range derived.Mappings {
		if m.Kind == outcome.EvidenceManual {
			criteria = append(criteria, m.CriterionID)
			if identity == "" {
				identity = m.ContentIdentity
			}
		}
	}
	if len(criteria) == 0 {
		fmt.Fprintf(stderr, "task recover-stage: %s has no manual criteria\n", id)
		return exitError
	}
	persistedIdentity := ""
	for _, m := range out.Mappings {
		if m.Kind == outcome.EvidenceManual {
			persistedIdentity = m.ContentIdentity
			break
		}
	}
	if persistedIdentity != "" && identity != persistedIdentity {
		fmt.Fprintf(stderr, "task recover-stage: repository content changed since %s's outcome; re-run required\n", id)
		return exitError
	}

	record := stageRecovery{
		ID:              stageRecoveryID(id, string(runpkg.WaitingForHuman), string(runpkg.Passed), identity, criteria),
		Version:         stageRecoveryVersion,
		Task:            id,
		FromStage:       string(runpkg.WaitingForHuman),
		ToStage:         string(runpkg.Passed),
		OutcomeStatus:   string(out.Status),
		Validation:      "passed",
		ContentIdentity: identity,
		Criteria:        criteria,
		Reason:          reason,
		By:              by,
		RecordedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if pendingOK {
		if pending.Task != id || pending.ID != record.ID || pending.ContentIdentity != identity || strings.Join(pending.Criteria, "\x00") != strings.Join(criteria, "\x00") {
			fmt.Fprintf(stderr, "task recover-stage: pending recovery does not match the current content; re-run required\n")
			return exitError
		}
		record = pending
	} else {
		if err := writeStageRecoveryPending(pendingPath, record); err != nil {
			fmt.Fprintf(stderr, "task recover-stage: record pending recovery: %v\n", err)
			return exitError
		}
	}

	if err := runpkg.SetStageOnExisting(dir, id, runpkg.Passed); err != nil {
		fmt.Fprintf(stderr, "task recover-stage: persist stage: %v\n", err)
		return exitError
	}
	if err := appendStageRecoveryIfMissing(auditPath, existing, record); err != nil {
		fmt.Fprintf(stderr, "task recover-stage: stage repaired, but the recovery audit was not persisted: %v; retry to finish the audit\n", err)
		return exitError
	}
	if err := os.Remove(pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stderr, "task recover-stage: stage repaired and audit recorded, but the pending marker was not removed: %v; retry to finish\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "task %s: run stage repaired %s -> %s (outcome %s)\n", id, runpkg.WaitingForHuman, runpkg.Passed, out.Status)
	fmt.Fprintf(stdout, "  by: %s\n", by)
	fmt.Fprintf(stdout, "  reason: %s\n", reason)
	return exitOK
}

// stageRecoveryID is the stable idempotency key for one repair of a task's recorded
// outcome to the terminal PASSED stage. It deliberately excludes reason/by so a
// retry with different operator prose still recognizes the same repair.
func stageRecoveryID(task, from, to, identity string, criteria []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "task=%s\nfrom=%s\nto=%s\nidentity=%s\ncriteria=%s\n", task, from, to, identity, strings.Join(criteria, "\x00"))
	return hex.EncodeToString(h.Sum(nil))
}

// appendStageRecoveryIfMissing appends rec once. A record with the same idempotency
// key already present is not duplicated.
func appendStageRecoveryIfMissing(path string, existing []stageRecovery, rec stageRecovery) error {
	for _, r := range existing {
		if r.ID == rec.ID {
			return nil
		}
	}
	return appendStageRecoveryRecord(path, rec)
}

// writeStageRecoveryPending durably records the intended repair before the stage
// changes, so an audit-write failure is recoverable by a retry.
func writeStageRecoveryPending(path string, rec stageRecovery) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pending recovery: %w", err)
	}
	return durablefs.WriteFile(path, append(data, '\n'))
}

// readStageRecoveryPending reads a pending repair marker, or reports it absent.
func readStageRecoveryPending(path string) (stageRecovery, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stageRecovery{}, false, nil
	}
	if err != nil {
		return stageRecovery{}, false, err
	}
	var rec stageRecovery
	if err := json.Unmarshal(data, &rec); err != nil {
		return stageRecovery{}, false, fmt.Errorf("parse pending stage recovery: %w", err)
	}
	if strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.Task) == "" {
		return stageRecovery{}, false, errors.New("pending stage recovery is incomplete")
	}
	return rec, true, nil
}

// parseTaskRecoverStageArgs parses
// `sop task recover-stage <task-id> --by NAME --reason R`. Both --by and --reason
// are required so a repair is always attributable and explained.
func parseTaskRecoverStageArgs(args []string, stderr io.Writer) (id, by, reason string, ok bool) {
	fail := func() (string, string, string, bool) {
		fmt.Fprintln(stderr, taskRecoverStageUsage)
		return "", "", "", false
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func() (string, bool) {
			if i+1 >= len(args) {
				return "", false
			}
			i++
			return args[i], true
		}
		switch {
		case a == "--by":
			v, valid := value()
			if !valid {
				return fail()
			}
			by = v
		case strings.HasPrefix(a, "--by="):
			by = strings.TrimPrefix(a, "--by=")
		case a == "--reason":
			v, valid := value()
			if !valid {
				return fail()
			}
			reason = v
		case strings.HasPrefix(a, "--reason="):
			reason = strings.TrimPrefix(a, "--reason=")
		case strings.HasPrefix(a, "-"):
			return fail()
		default:
			if id != "" {
				return fail()
			}
			id = a
		}
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(by) == "" || strings.TrimSpace(reason) == "" {
		return fail()
	}
	return id, strings.TrimSpace(by), strings.TrimSpace(reason), true
}
