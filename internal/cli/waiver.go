package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// This file implements the SOP-HARD-04 supported recovery surface:
//
//   - `sop verify <task>` — strictly read-only: it reprints a task's recorded
//     objective-level verdict per criterion and never writes SOP state.
//   - `sop outcome <task> --waive <criterion> --reason R --by NAME` — records a
//     designated-approver, criterion-scoped waiver of an unmet required criterion as
//     durable, append-only audit evidence.
//
// A waiver is never implied and never silently applied: only a designated approver
// (internal/config.Human.DesignatedApprovers), with an explicit reason and timestamp,
// covering a criterion that really is an unmet REQUIRED criterion of the recorded
// verdict, is accepted. The audit file is opened O_APPEND, so it is only ever
// extended — never rewritten — and historical outcomes stay intact.

const waiverAuditName = "waivers.jsonl"

// outcomeArtifactPath is a task's recorded objective-level verdict.
func outcomeArtifactPath(projectDir, taskID string) string {
	return filepath.Join(projectDir, stateDirName, "runs", taskID, "outcome.json")
}

// waiverAuditPath is a task's durable, append-only waiver audit.
func waiverAuditPath(projectDir, taskID string) string {
	return filepath.Join(projectDir, stateDirName, "runs", taskID, waiverAuditName)
}

// readOutcomeArtifact reads a task's recorded verdict, or nil when none exists
// (the task has not produced an outcome yet).
func readOutcomeArtifact(projectDir, taskID string) (*outcomeArtifact, error) {
	data, err := os.ReadFile(outcomeArtifactPath(projectDir, taskID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var res outcomeArtifact
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// readWaiverRecords reads a task's append-only waiver audit in file order, or nil
// when no audit exists. It is read-only.
func readWaiverRecords(projectDir, taskID string) ([]outcome.WaiverRecord, error) {
	data, err := os.ReadFile(waiverAuditPath(projectDir, taskID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []outcome.WaiverRecord
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec outcome.WaiverRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// appendWaiverRecord appends one record to the durable audit. The file is opened
// O_APPEND|O_CREATE, so an existing audit is only ever extended — never rewritten or
// reordered — and no other evidence is touched.
func appendWaiverRecord(projectDir, taskID string, rec outcome.WaiverRecord) error {
	path := waiverAuditPath(projectDir, taskID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// recordedWaiverForTask returns the combined designated-approver waiver recorded for
// a task, or nil when none is recorded. It is the run-path reader that lets a
// recorded waiver resolve a gated PARTIAL to LOCAL_DONE.
func recordedWaiverForTask(projectDir, taskID string) *outcome.Waiver {
	recs, err := readWaiverRecords(projectDir, taskID)
	if err != nil {
		return nil
	}
	return outcome.WaiverFromRecords(recs)
}

// runVerify prints a task's recorded objective-level verdict per criterion. It is
// strictly read-only: it reads the recorded outcome artifact and the append-only
// waiver audit, and writes nothing — no lifecycle transition, no state mutation.
func runVerify(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop verify <task-id>")
		return exitUsage
	}
	taskID := args[0]
	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}
	res, err := readOutcomeArtifact(dir, taskID)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitError
	}
	if res == nil {
		fmt.Fprintf(stdout, "verify %s: no recorded outcome (run the task first)\n", taskID)
		return exitOK
	}
	return printRecordedOutcome(stdout, stderr, dir, taskID, res)
}

// runOutcome prints a task's recorded outcome, or — with --waive — records a
// designated-approver, criterion-scoped waiver of an unmet required criterion as
// durable, append-only audit evidence. A non-designated approver, a missing reason,
// or a criterion that is not an unmet required criterion is rejected without writing
// anything: a waiver is never implied and never silently applied.
func runOutcome(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	taskID, waive, reason, by, scope := "", "", "", "", ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		takeValue := func() (string, bool) {
			if i+1 >= len(args) {
				return "", false
			}
			i++
			return args[i], true
		}
		switch {
		case a == "--waive":
			v, ok := takeValue()
			if !ok {
				return outcomeUsage(stderr)
			}
			waive = v
		case strings.HasPrefix(a, "--waive="):
			waive = strings.TrimPrefix(a, "--waive=")
		case a == "--reason":
			v, ok := takeValue()
			if !ok {
				return outcomeUsage(stderr)
			}
			reason = v
		case strings.HasPrefix(a, "--reason="):
			reason = strings.TrimPrefix(a, "--reason=")
		case a == "--by":
			v, ok := takeValue()
			if !ok {
				return outcomeUsage(stderr)
			}
			by = v
		case strings.HasPrefix(a, "--by="):
			by = strings.TrimPrefix(a, "--by=")
		case a == "--scope":
			v, ok := takeValue()
			if !ok {
				return outcomeUsage(stderr)
			}
			scope = v
		case strings.HasPrefix(a, "--scope="):
			scope = strings.TrimPrefix(a, "--scope=")
		case strings.HasPrefix(a, "-"):
			return outcomeUsage(stderr)
		default:
			if taskID != "" {
				return outcomeUsage(stderr)
			}
			taskID = a
		}
	}
	if taskID == "" {
		return outcomeUsage(stderr)
	}
	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}
	res, err := readOutcomeArtifact(dir, taskID)
	if err != nil {
		fmt.Fprintf(stderr, "outcome: %v\n", err)
		return exitError
	}
	if res == nil {
		fmt.Fprintf(stderr, "outcome: no recorded outcome for %s; run the task first\n", taskID)
		return exitError
	}
	if waive == "" {
		return printRecordedOutcome(stdout, stderr, dir, taskID, res)
	}
	rec, err := recordWaiver(dir, taskID, res, waive, scope, reason, by)
	if err != nil {
		fmt.Fprintf(stderr, "outcome: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "recorded waiver: task=%s criterion=%s approver=%s recorded_at=%s\n", rec.Task, rec.CriterionID, rec.Approver, rec.RecordedAt)
	if rec.Scope != "" {
		fmt.Fprintf(stdout, "  scope: %s\n", rec.Scope)
	}
	fmt.Fprintf(stdout, "  reason: %s\n", rec.Reason)
	fmt.Fprintf(stdout, "  waived outcome: %s\n", rec.Outcome)
	return exitOK
}

// recordWaiver validates and appends one designated-approver waiver record. It
// enforces: the criterion is an unmet REQUIRED criterion of the recorded verdict; the
// approver is designated; a reason and timestamp are present. It writes only the
// append-only audit record.
func recordWaiver(dir, taskID string, res *outcomeArtifact, criterionID, scope, reason, by string) (outcome.WaiverRecord, error) {
	cr, ok := unmetRequiredCriterion(res, criterionID)
	if !ok {
		return outcome.WaiverRecord{}, fmt.Errorf("criterion %q is not an unmet required criterion of %s", criterionID, taskID)
	}
	var designated []string
	if cfg, err := config.LoadDir(dir); err == nil {
		designated = cfg.Human.DesignatedApprovers
	} else if !errors.Is(err, config.ErrNotFound) {
		return outcome.WaiverRecord{}, err
	}
	if strings.TrimSpace(scope) == "" {
		scope = cr.Description
	}
	now := time.Now().UTC().Format(time.RFC3339)
	req := outcome.GrantRequest{
		Task:        taskID,
		CriterionID: criterionID,
		Scope:       strings.TrimSpace(scope),
		Reason:      strings.TrimSpace(reason),
		Approver:    strings.TrimSpace(by),
	}
	if err := outcome.ValidateGrant(req, designated, now); err != nil {
		return outcome.WaiverRecord{}, err
	}
	rec := outcome.NewGrant(req, res.Status, now)
	if err := appendWaiverRecord(dir, taskID, rec); err != nil {
		return outcome.WaiverRecord{}, err
	}
	return rec, nil
}

// unmetRequiredCriterion returns the recorded criterion with the given id when it is
// a required criterion that was not met.
func unmetRequiredCriterion(res *outcomeArtifact, id string) (outcome.CriterionResult, bool) {
	for _, cr := range res.Criteria {
		if cr.ID == id && cr.Required && !cr.Met {
			return cr, true
		}
	}
	return outcome.CriterionResult{}, false
}

// printRecordedOutcome renders a task's recorded verdict, its per-criterion results,
// and any recorded waivers. It is shared by `sop verify` and `sop outcome`.
func printRecordedOutcome(stdout, stderr io.Writer, dir, taskID string, res *outcomeArtifact) int {
	fmt.Fprintf(stdout, "outcome %s\n", taskID)
	fmt.Fprintf(stdout, "  status: %s\n", res.Status)
	if res.Lifecycle != "" {
		fmt.Fprintf(stdout, "  lifecycle: %s\n", res.Lifecycle)
	}
	if res.WaiverDenied {
		fmt.Fprintln(stdout, "  waiver: denied — a required deliverable/check is unmet and no valid designated-approver waiver exists")
	}
	for _, cr := range res.Criteria {
		fmt.Fprintf(stdout, "  - %s %s: %s (%s)\n", cr.ID, criterionState(cr), cr.Description, cr.Evidence)
	}
	recs, err := readWaiverRecords(dir, taskID)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return exitError
	}
	if len(recs) > 0 {
		fmt.Fprintln(stdout, "  waivers:")
		for _, r := range recs {
			fmt.Fprintf(stdout, "    - %s by %s at %s: %s\n", r.CriterionID, r.Approver, r.RecordedAt, r.Reason)
		}
	}
	return exitOK
}

// criterionState labels a recorded criterion result the way the run report does.
func criterionState(cr outcome.CriterionResult) string {
	switch {
	case cr.Met:
		return "met"
	case !cr.Required:
		return "informational"
	default:
		return "not met"
	}
}

func outcomeUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: sop outcome <task-id>")
	fmt.Fprintln(stderr, "       sop outcome <task-id> --waive <criterion-id> --reason R --by NAME [--scope S]")
	return exitUsage
}
