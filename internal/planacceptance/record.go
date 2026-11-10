package planacceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/outcome"
)

// RecordAnnotation appends one auditable annotation that preserves the archived
// manual-evidence text and hash. It is idempotent for identical content.
func RecordAnnotation(ctx *Context, ann Annotation, designated []string) (Annotation, error) {
	m, ok := findMapping(ctx.Mappings, ann.CriterionID)
	if !ok {
		return Annotation{}, fmt.Errorf("criterion %q is not in the archived plan", ann.CriterionID)
	}
	if m.Kind != outcome.EvidenceManual {
		return Annotation{}, fmt.Errorf("criterion %q is check-mapped; annotations apply only to manual criteria", ann.CriterionID)
	}
	if strings.TrimSpace(ann.ReviewedEvidence) == "" {
		return Annotation{}, errors.New("reviewed evidence is required")
	}
	if strings.TrimSpace(ann.Reason) == "" {
		return Annotation{}, errors.New("an annotation requires a reason")
	}
	if !outcome.IsDesignatedApprover(ann.Approver, designated) {
		return Annotation{}, fmt.Errorf("%q is not a designated approver", ann.Approver)
	}
	if err := VerifyEvidenceRefs(ctx.ProjectDir, ann.EvidenceRefs); err != nil {
		return Annotation{}, err
	}
	ann.Kind = "manual_evidence_annotation"
	ann.PlanIdentity = ctx.PlanIdentity
	ann.CriterionID = m.CriterionID
	ann.ArchivedManualEvidence = m.ManualEvidence
	ann.ArchivedEvidenceSHA256 = sha256Hex(m.ManualEvidence)
	ann.ReviewedEvidenceSHA256 = sha256Hex(ann.ReviewedEvidence)
	ann.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	ann.ID = annotationIdentity(ann)

	var stored Annotation
	err := withProjectLock(ctx.ProjectDir, func() error {
		existing, err := ReadAnnotations(ctx)
		if err != nil {
			return err
		}
		for _, a := range existing {
			if a.ID == ann.ID {
				stored = a
				return nil
			}
		}
		if err := appendRecord(annotationsPath(ctx), ann); err != nil {
			return err
		}
		stored = ann
		return nil
	})
	if err != nil {
		return Annotation{}, err
	}
	return stored, nil
}

func annotationIdentity(ann Annotation) string {
	type identity struct {
		Kind                   string                `json:"kind"`
		PlanIdentity           string                `json:"plan_identity"`
		CriterionID            string                `json:"criterion_id"`
		ArchivedManualEvidence string                `json:"archived_manual_evidence"`
		ArchivedEvidenceSHA256 string                `json:"archived_evidence_sha256"`
		ReviewedEvidence       string                `json:"reviewed_evidence"`
		ReviewedEvidenceSHA256 string                `json:"reviewed_evidence_sha256"`
		Reason                 string                `json:"reason"`
		EvidenceRefs           []outcome.EvidenceRef `json:"evidence_refs"`
		Approver               string                `json:"approver"`
	}
	data, _ := json.Marshal(identity{
		Kind:                   ann.Kind,
		PlanIdentity:           ann.PlanIdentity,
		CriterionID:            ann.CriterionID,
		ArchivedManualEvidence: ann.ArchivedManualEvidence,
		ArchivedEvidenceSHA256: ann.ArchivedEvidenceSHA256,
		ReviewedEvidence:       ann.ReviewedEvidence,
		ReviewedEvidenceSHA256: ann.ReviewedEvidenceSHA256,
		Reason:                 ann.Reason,
		EvidenceRefs:           ann.EvidenceRefs,
		Approver:               ann.Approver,
	})
	return sha256Hex(string(data))
}

// RecordDecision appends one designated-approver decision bound to the plan
// identity, the exact annotation (when used), and verified evidence references.
func RecordDecision(ctx *Context, d outcome.AcceptanceDecision, designated []string) (outcome.AcceptanceDecision, error) {
	m, ok := findMapping(ctx.Mappings, d.CriterionID)
	if !ok {
		return outcome.AcceptanceDecision{}, fmt.Errorf("criterion %q is not in the archived plan", d.CriterionID)
	}
	if m.Kind != outcome.EvidenceManual {
		return outcome.AcceptanceDecision{}, fmt.Errorf("criterion %q is check-mapped; manual acceptance is not allowed", d.CriterionID)
	}
	if !outcome.IsDesignatedApprover(d.Approver, designated) {
		return outcome.AcceptanceDecision{}, fmt.Errorf("%q is not a designated approver", d.Approver)
	}
	if strings.TrimSpace(d.Evidence) == "" {
		return outcome.AcceptanceDecision{}, errors.New("an acceptance decision requires evidence")
	}
	if err := VerifyEvidenceRefs(ctx.ProjectDir, d.EvidenceRefs); err != nil {
		return outcome.AcceptanceDecision{}, err
	}

	effective := m.ManualEvidence
	if strings.TrimSpace(d.AnnotationID) != "" {
		annotations, err := ReadAnnotations(ctx)
		if err != nil {
			return outcome.AcceptanceDecision{}, err
		}
		ann, ok := findAnnotation(annotations, d.AnnotationID)
		if !ok {
			return outcome.AcceptanceDecision{}, fmt.Errorf("annotation %q is not recorded", d.AnnotationID)
		}
		d.ManualEvidence = ann.ReviewedEvidence
		if err := validateAnnotation(ctx, m, ann, d); err != nil {
			return outcome.AcceptanceDecision{}, err
		}
		effective = ann.ReviewedEvidence
	}

	d.Task = m.TaskID
	d.CriterionDescription = m.Description
	d.ManualEvidence = effective
	d.PlanIdentity = ctx.PlanIdentity
	d.ContentIdentity = ""
	d.Attempt = 0
	d.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	mapping := outcome.CriterionMapping{
		CriterionID:    m.CriterionID,
		Description:    m.Description,
		Required:       true,
		Kind:           outcome.EvidenceManual,
		ManualEvidence: effective,
		PlanIdentity:   ctx.PlanIdentity,
	}
	if err := outcome.ValidateAcceptanceDecision(d, []outcome.CriterionMapping{mapping}, designated); err != nil {
		return outcome.AcceptanceDecision{}, err
	}

	var stored outcome.AcceptanceDecision
	err := withProjectLock(ctx.ProjectDir, func() error {
		existing, err := ReadDecisions(ctx)
		if err != nil {
			return err
		}
		for _, old := range existing {
			if sameDecision(old, d) {
				stored = old
				return nil
			}
		}
		if err := appendRecord(decisionsPath(ctx), d); err != nil {
			return err
		}
		stored = d
		return nil
	})
	if err != nil {
		return outcome.AcceptanceDecision{}, err
	}
	return stored, nil
}

func sameDecision(a, b outcome.AcceptanceDecision) bool {
	if a.CriterionID != b.CriterionID || a.PlanIdentity != b.PlanIdentity || a.Decision != b.Decision || a.Approver != b.Approver || a.Evidence != b.Evidence || a.AnnotationID != b.AnnotationID {
		return false
	}
	if len(a.EvidenceRefs) != len(b.EvidenceRefs) {
		return false
	}
	for i := range a.EvidenceRefs {
		if a.EvidenceRefs[i] != b.EvidenceRefs[i] {
			return false
		}
	}
	return true
}

// RecordCheck appends one executed check result. Failed or source-changed results
// are recorded for audit and are never accepted by the evaluator.
func RecordCheck(ctx *Context, c CheckEvidence) (CheckEvidence, error) {
	m, ok := findMapping(ctx.Mappings, c.CriterionID)
	if !ok {
		return CheckEvidence{}, fmt.Errorf("criterion %q is not in the archived plan", c.CriterionID)
	}
	if m.Kind != outcome.EvidenceCheck {
		return CheckEvidence{}, fmt.Errorf("criterion %q is not check-mapped", c.CriterionID)
	}
	if c.CheckRef != m.CheckRef || c.Command != m.Command {
		return CheckEvidence{}, fmt.Errorf("check evidence does not match the archived check for %q", c.CriterionID)
	}
	if c.Class != "SAFE" {
		return CheckEvidence{}, fmt.Errorf("check class %q is not SAFE", c.Class)
	}
	if strings.TrimSpace(c.TestedRevision) == "" {
		return CheckEvidence{}, errors.New("check evidence requires a tested revision")
	}
	if c.LogPath != "" {
		if err := verifyCheckLog(ctx, c); err != nil {
			return CheckEvidence{}, err
		}
	}
	c.Kind = "historical_check"
	c.PlanIdentity = ctx.PlanIdentity
	c.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	err := withProjectLock(ctx.ProjectDir, func() error {
		if _, err := ReadChecks(ctx); err != nil {
			return err
		}
		return appendRecord(checksPath(ctx), c)
	})
	if err != nil {
		return CheckEvidence{}, err
	}
	return c, nil
}

func findMapping(mappings []Mapping, criterionID string) (Mapping, bool) {
	for _, m := range mappings {
		if m.CriterionID == criterionID || m.DisplayID == criterionID {
			return m, true
		}
	}
	return Mapping{}, false
}
