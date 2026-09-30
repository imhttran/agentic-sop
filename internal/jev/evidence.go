package jev

// Structured early JEV analysis evidence (P3-002).
//
// This file defines the machine-readable, versioned contract an early JEV
// analysis result uses to describe itself, so SOP policy can consume it
// deterministically:
//
//   - Purpose, Severity, and Status are closed enumerations. Unknown values are
//     rejected (fail closed) — never defaulted, coerced, or silently accepted.
//   - Validation never derives meaning from summary text. Summary prose is
//     descriptive only and is explicitly not a decision input: no lifecycle
//     action, status, or severity is inferred from it.
//   - The contract is additive: it does not change the existing Analyzer
//     interface or the existing Result fields, so existing Analyzer/Result
//     consumers remain backward compatible.
//
// The contract is data only. It carries no authority: it is never a lifecycle
// command and exposes no method that transitions task state, mutates SOP
// persistence, marks validation/review successful, or performs Git/PR/merge
// actions. SOP remains the lifecycle owner and decides what a validated
// evidence value implies (if anything).

import (
	"encoding/json"
	"fmt"
)

// EvidenceVersion is the schema version of the structured evidence contract.
const EvidenceVersion = 1

// Purpose is the closed enumeration of what an early JEV analysis was run to
// establish. It is a machine-readable value, never free text. An unknown
// purpose fails closed.
type Purpose string

const (
	// PurposeQuality: assess implementation quality.
	PurposeQuality Purpose = "QUALITY"
	// PurposeCorrectness: assess whether the implementation matches the task.
	PurposeCorrectness Purpose = "CORRECTNESS"
	// PurposeSecurity: assess security-relevant concerns.
	PurposeSecurity Purpose = "SECURITY"
	// PurposeRegression: assess risk of regression.
	PurposeRegression Purpose = "REGRESSION"
	// PurposeTaskTriage: assess a deterministically selected task before
	// implementation begins (Phase 3 early checkpoint).
	PurposeTaskTriage Purpose = "TASK_TRIAGE"
	// PurposePreExecution: assess the proposed execution context immediately before
	// implementation begins (Phase 3 early checkpoint).
	PurposePreExecution Purpose = "PRE_EXECUTION"
)

// purposeRank is the closed set of known purposes. Membership in the map is the
// single definition of validity.
var purposeRank = map[Purpose]bool{
	PurposeQuality:      true,
	PurposeCorrectness:  true,
	PurposeSecurity:     true,
	PurposeRegression:   true,
	PurposeTaskTriage:   true,
	PurposePreExecution: true,
}

// Valid reports whether p is a known purpose. Unknown values are invalid so
// callers can fail closed rather than default.
func (p Purpose) Valid() bool { return purposeRank[p] }

// Severity is the closed enumeration of how serious an early JEV analysis
// finding is. It mirrors the review severity vocabulary so a single set of
// values is used across the boundary, and it fails closed on unknown values.
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityHigh     Severity = "HIGH"
	SeverityCritical Severity = "CRITICAL"
)

// severityRankE is the closed set of known severities.
var severityRankE = map[Severity]bool{
	SeverityInfo:     true,
	SeverityLow:      true,
	SeverityMedium:   true,
	SeverityHigh:     true,
	SeverityCritical: true,
}

// Valid reports whether s is a known severity. Unknown values are invalid so
// callers can fail closed rather than default.
func (s Severity) Valid() bool { return severityRankE[s] }

// EvidenceStatus is the closed enumeration of how an early JEV analysis ended.
// It is a result value, not a lifecycle transition: SOP policy decides what a
// status implies for a task. An unknown status fails closed.
type EvidenceStatus string

const (
	// EvidencePass: analysis completed and found nothing worth reporting.
	EvidencePass EvidenceStatus = "PASS"
	// EvidenceFindings: analysis completed and reported one or more findings.
	EvidenceFindings EvidenceStatus = "FINDINGS"
	// EvidenceIncomplete: analysis could not finish; it is never a pass.
	EvidenceIncomplete EvidenceStatus = "INCOMPLETE"
	// EvidenceError: analysis failed to run; it is never a pass.
	EvidenceError EvidenceStatus = "ERROR"
)

// evidenceStatusRank is the closed set of known statuses.
var evidenceStatusRank = map[EvidenceStatus]bool{
	EvidencePass:       true,
	EvidenceFindings:   true,
	EvidenceIncomplete: true,
	EvidenceError:      true,
}

// Valid reports whether s is a known evidence status. Unknown values are
// invalid so callers can fail closed rather than default.
func (s EvidenceStatus) Valid() bool { return evidenceStatusRank[s] }

// Category is the closed enumeration of what an early JEV analysis finding is
// about. Policy maps these typed values (never free-text topics or words) to a
// deterministic disposition, so an early finding is machine-readable. An unknown
// non-empty category fails closed rather than being silently accepted.
type Category string

const (
	// CategoryAmbiguity: the task or requirement is underspecified.
	CategoryAmbiguity Category = "ambiguity"
	// CategoryMissingContext: required context is absent.
	CategoryMissingContext Category = "missing_context"
	// CategoryScope: the change or task scope is broader than intended.
	CategoryScope Category = "scope"
	// CategoryRequirementConflict: requirements contradict one another.
	CategoryRequirementConflict Category = "requirement_conflict"
	// CategoryDependencyConcern: a dependency is missing or unsafe to rely on.
	CategoryDependencyConcern Category = "dependency_concern"
	// CategorySecurity: a security-relevant concern.
	CategorySecurity Category = "security"
	// CategoryDestructive: the operation may be destructive or irreversible.
	CategoryDestructive Category = "destructive"
	// CategoryCredentialSensitivity: credential- or secret-sensitive work.
	CategoryCredentialSensitivity Category = "credential_sensitivity"
	// CategoryUnexpectedArea: the change touches an unexpected repository area.
	CategoryUnexpectedArea Category = "unexpected_area"
	// CategoryApprovalSensitive: the operation crosses an approval boundary.
	CategoryApprovalSensitive Category = "approval_sensitive"
)

// categoryRank is the closed set of known categories. Membership in the map is
// the single definition of validity.
var categoryRank = map[Category]bool{
	CategoryAmbiguity:             true,
	CategoryMissingContext:        true,
	CategoryScope:                 true,
	CategoryRequirementConflict:   true,
	CategoryDependencyConcern:     true,
	CategorySecurity:              true,
	CategoryDestructive:           true,
	CategoryCredentialSensitivity: true,
	CategoryUnexpectedArea:        true,
	CategoryApprovalSensitive:     true,
}

// Valid reports whether c is a known category. An empty category is treated as
// unspecified (valid); any other unknown value is invalid so callers can fail
// closed rather than default.
func (c Category) Valid() bool { return c == "" || categoryRank[c] }

// EvidenceItem is one structured early JEV analysis finding: a validated
// purpose/severity pair with supporting detail. It is plain data.
type EvidenceItem struct {
	// Purpose is what the analysis established for this item.
	Purpose Purpose
	// Severity is how serious the item is.
	Severity Severity
	// Category classifies the item with a typed value (for example scope,
	// security). Policy maps categories, never free text.
	Category Category
	// Detail is the human-readable description of the item.
	Detail string
	// Evidence is the supporting detail that justifies the item.
	Evidence string
}

// earlyEvidence is the structured early JEV analysis result. It is
// machine-readable data consumed by SOP policy; it is never a lifecycle
// command. Summary is descriptive only and is explicitly not a decision input.
type earlyEvidence struct {
	Version  int
	Purpose  Purpose
	Severity Severity
	Status   EvidenceStatus
	// Confidence is a bounded (0.0 <= c <= 1.0) advisory confidence for the
	// analysis. It is structured data, never prose, and it never directly controls
	// lifecycle state: SOP policy MAY combine it with category, severity, risk, and
	// configured thresholds, but a bare confidence threshold is not a decision rule
	// (FR-P3-8). A zero value means "not stated".
	Confidence float64
	Items      []EvidenceItem
	Summary    string
}

// Evidence is the exported structured early JEV analysis evidence value.
type Evidence = earlyEvidence

// Validate rejects malformed or unknown structured evidence. Callers must treat
// an invalid value as a failure (fail closed) rather than a pass: malformed
// evidence must never silently become PASS.
//
// Validation is entirely structural. It never inspects Summary — summary text
// is descriptive only and is not a decision input, so no lifecycle action,
// status, or severity is derived from it.
func (e Evidence) Validate() error {
	if e.Version != EvidenceVersion {
		return errInvalidVersion(e.Version)
	}
	if !e.Status.Valid() {
		return errInvalidEvidenceStatus(string(e.Status))
	}
	if !e.Purpose.Valid() {
		return errInvalidPurpose(string(e.Purpose))
	}
	if !e.Severity.Valid() {
		return errInvalidEvidenceSeverity(string(e.Severity))
	}
	if e.Confidence < 0 || e.Confidence > 1 {
		return errInvalidConfidence(e.Confidence)
	}
	for i, item := range e.Items {
		if !item.Purpose.Valid() {
			return errInvalidPurpose(fmt.Sprintf("item[%d] purpose %q", i, item.Purpose))
		}
		if !item.Severity.Valid() {
			return errInvalidEvidenceSeverity(fmt.Sprintf("item[%d] severity %q", i, item.Severity))
		}
		if !item.Category.Valid() {
			return errInvalidCategory(fmt.Sprintf("item[%d] category %q", i, item.Category))
		}
	}
	switch e.Status {
	case EvidencePass:
		if len(e.Items) > 0 {
			return errEvidencePassWithItems
		}
	case EvidenceFindings:
		if len(e.Items) == 0 {
			return errEvidenceFindingsWithoutItems
		}
	}
	return nil
}

// MarshalEvidence serializes structured evidence to JSON following the
// repository's existing serialization convention (encoding/json), so the
// machine-readable form is stable and deterministic.
func MarshalEvidence(e Evidence) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

// UnmarshalEvidence deserializes structured evidence and validates it, so an
// unknown purpose, severity, or status in the serialized form fails closed
// rather than being silently accepted.
func UnmarshalEvidence(data []byte) (Evidence, error) {
	var e Evidence
	if err := json.Unmarshal(data, &e); err != nil {
		return Evidence{}, fmt.Errorf("jev: evidence is not valid JSON: %w", err)
	}
	if err := e.Validate(); err != nil {
		return Evidence{}, err
	}
	return e, nil
}
