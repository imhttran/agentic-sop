package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// SOP-003 manual acceptance: `sop outcome <task> --accept <criterion> --evidence E
// --by NAME` appends a designated approver's explicit, traceable decision on a
// criterion mapped to manual evidence. The audit is append-only and outcome.json is
// never rewritten; the next derived outcome reads the decision.

const acceptanceDecisionAuditName = "acceptance_decisions.jsonl"

// acceptanceDecisionPath is a task's durable, append-only acceptance-decision audit.
func acceptanceDecisionPath(projectDir, taskID string) string {
	return filepath.Join(projectDir, stateDirName, "runs", taskID, acceptanceDecisionAuditName)
}

// readAcceptanceDecisions reads a task's recorded acceptance decisions in file order.
func readAcceptanceDecisions(projectDir, taskID string) ([]outcome.AcceptanceDecision, error) {
	return readJSONL[outcome.AcceptanceDecision](acceptanceDecisionPath(projectDir, taskID))
}

// recordAcceptanceDecision validates and appends one accept decision. A decision by a
// non-designated approver, without evidence, or for a criterion the recorded outcome
// does not map to manual evidence is rejected and nothing is written.
func recordAcceptanceDecision(dir, taskID string, res *outcomeArtifact, criterionID, evidence, by string) (outcome.AcceptanceDecision, error) {
	var designated []string
	if cfg, err := config.LoadDir(dir); err == nil {
		designated = cfg.Human.DesignatedApprovers
	} else if !errors.Is(err, config.ErrNotFound) {
		return outcome.AcceptanceDecision{}, err
	}
	d := outcome.AcceptanceDecision{
		Task:        taskID,
		CriterionID: criterionID,
		Evidence:    strings.TrimSpace(evidence),
		Decision:    outcome.DecisionAccept,
		Approver:    strings.TrimSpace(by),
		RecordedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	// Bind the decision to the exact mapping the operator reviewed: its criterion
	// text, manual-evidence description, and attempt. Without this, a decision
	// recorded for one version of a criterion could satisfy a changed criterion or
	// a later attempt.
	var mapping *outcome.CriterionMapping
	for i := range res.Mappings {
		if res.Mappings[i].CriterionID == criterionID && res.Mappings[i].Kind == outcome.EvidenceManual {
			mapping = &res.Mappings[i]
			break
		}
	}
	if mapping == nil {
		return outcome.AcceptanceDecision{}, fmt.Errorf("criterion %q is not mapped to manual evidence", criterionID)
	}
	d.CriterionDescription = mapping.Description
	d.ManualEvidence = mapping.ManualEvidence
	d.Attempt = mapping.Attempt
	if err := outcome.ValidateAcceptanceDecision(d, res.Mappings, designated); err != nil {
		return outcome.AcceptanceDecision{}, err
	}
	if err := appendJSONL(acceptanceDecisionPath(dir, taskID), d); err != nil {
		return outcome.AcceptanceDecision{}, fmt.Errorf("record acceptance decision: %w", err)
	}
	return d, nil
}
