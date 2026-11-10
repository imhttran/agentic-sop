package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/quality"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// stageRecoveryVersion is the schema version of the append-only stage-recovery
// audit written beside a repaired run's other artifacts.
const stageRecoveryVersion = 1

// stageRecovery records one explicit, auditable repair of a stale accepted-outcome
// run stage. It is diagnostic provenance only: it never changes the task status,
// attempt, outcome, acceptance decisions, or validation evidence.
type stageRecovery struct {
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

const taskRecoverStageUsage = "usage: sop task recover-stage <task-id> --by NAME --reason R"

// runTaskRecoverStage repairs the legacy accepted-outcome run-stage defect without
// rerunning planning, implementation, validation, or acceptance. It is deliberately
// narrow: the task must already be satisfied, have a persisted VERIFIED outcome whose
// validation passed, carry an APPROVED approval gate, and have a run stage of exactly
// WAITING_FOR_HUMAN. The accepted outcome is re-derived against the current repository
// content so stale acceptance bindings fail closed. Only the existing non-destructive
// PASSED stage checkpoint is written, plus an append-only audit record.
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
	if stage == runpkg.Passed {
		fmt.Fprintf(stdout, "task %s: run stage is already %s; no recovery recorded\n", id, runpkg.Passed)
		return exitOK
	}
	if stage != runpkg.WaitingForHuman {
		fmt.Fprintf(stderr, "task recover-stage: %s run stage is %s; only %s can be repaired\n", id, stage, runpkg.WaitingForHuman)
		return exitError
	}

	rec := runpkg.At(runpkg.Dir(dir, id))
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

	if err := runpkg.SetStageOnExisting(dir, id, runpkg.Passed); err != nil {
		fmt.Fprintf(stderr, "task recover-stage: persist stage: %v\n", err)
		return exitError
	}

	record := stageRecovery{
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
	if err := appendJSONL(filepath.Join(rec.Dir(), "stage-recoveries.jsonl"), record); err != nil {
		fmt.Fprintf(stdout, "warning: stage-recovery audit not persisted: %v\n", err)
	}

	fmt.Fprintf(stdout, "task %s: run stage repaired %s -> %s (outcome %s)\n", id, runpkg.WaitingForHuman, runpkg.Passed, out.Status)
	fmt.Fprintf(stdout, "  by: %s\n", by)
	fmt.Fprintf(stdout, "  reason: %s\n", reason)
	return exitOK
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
