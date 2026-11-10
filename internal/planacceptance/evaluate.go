package planacceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// CriterionStatus is the per-criterion acceptance state. It separates mapping,
// eligibility, acceptance, and stale/corrupt conditions.
type CriterionStatus string

const (
	StatusManuallyAccepted CriterionStatus = "manually-accepted"
	StatusCheckPassed      CriterionStatus = "check-passed"
	StatusDecisionEligible CriterionStatus = "decision-eligible"
	StatusCheckUnverified  CriterionStatus = "check-unverified"
	StatusRejected         CriterionStatus = "rejected"
	StatusStale            CriterionStatus = "stale"
	StatusUnmapped         CriterionStatus = "unmapped"
)

// CriterionReport is one evaluated historical criterion.
type CriterionReport struct {
	Mapping  Mapping         `json:"mapping"`
	Status   CriterionStatus `json:"status"`
	Evidence string          `json:"evidence"`
	Revision string          `json:"revision,omitempty"`
}

// Report is the deterministic historical-plan acceptance report.
type Report struct {
	PlanID         string            `json:"plan_id"`
	Source         string            `json:"source"`
	Disposition    string            `json:"disposition"`
	RecordedAt     string            `json:"recorded_at"`
	PlanIdentity   string            `json:"plan_identity"`
	ArchiveDir     string            `json:"archive_dir"`
	CurrentHead    string            `json:"current_head"`
	Criteria       []CriterionReport `json:"criteria"`
	Counts         Counts            `json:"counts"`
	Accepted       int               `json:"accepted"`
	CheckRevisions []string          `json:"check_revisions"`
	Corrupt        *CorruptError     `json:"-"`
}

type criterionError struct {
	status CriterionStatus
	err    error
}

func (e *criterionError) Error() string { return e.err.Error() }
func (e *criterionError) Unwrap() error { return e.err }

// Evaluate builds one deterministic evaluation for the historical plan. It
// reuses outcome.Verify for per-criterion verdicts and never changes lifecycle.
func Evaluate(ctx *Context, designated []string) (*Report, error) {
	rep := &Report{
		PlanID:       ctx.Archive.PlanID,
		Source:       ctx.Archive.Source,
		Disposition:  ctx.Archive.Disposition,
		RecordedAt:   ctx.Archive.RecordedAt,
		PlanIdentity: ctx.PlanIdentity,
		ArchiveDir:   ctx.Archive.ArchiveDir,
		Counts:       CountsFromMappings(ctx.Mappings),
	}
	if head, err := git.New(ctx.ProjectDir).Head(context.Background()); err == nil {
		rep.CurrentHead = head
	}

	decisions, err := ReadDecisions(ctx)
	if err != nil {
		return corruptReport(rep, err)
	}
	annotations, err := ReadAnnotations(ctx)
	if err != nil {
		return corruptReport(rep, err)
	}
	checks, err := ReadChecks(ctx)
	if err != nil {
		return corruptReport(rep, err)
	}

	criteria := make([]outcome.Criterion, 0, len(ctx.Mappings))
	errs := make([]error, len(ctx.Mappings))
	for i, m := range ctx.Mappings {
		mp := m
		idx := i
		criteria = append(criteria, outcome.Criterion{
			ID:          mp.CriterionID,
			Required:    true,
			Description: mp.Description,
			Check: func() error {
				var err error
				switch mp.Kind {
				case outcome.EvidenceManual:
					err = evaluateManual(ctx, mp, decisions, annotations, designated)
				case outcome.EvidenceCheck:
					err = evaluateCheck(ctx, mp, checks)
				default:
					err = &criterionError{status: StatusUnmapped, err: errors.New("criterion has no manual or check mapping")}
				}
				errs[idx] = err
				return err
			},
		})
	}

	res := outcome.Verify(criteria)
	seen := map[string]struct{}{}
	for i, cr := range res.Criteria {
		m := ctx.Mappings[i]
		status := StatusUnmapped
		var statusErr *criterionError
		switch {
		case cr.Met && m.Kind == outcome.EvidenceManual:
			status = StatusManuallyAccepted
			rep.Counts.ManualAccepted++
			rep.Accepted++
		case cr.Met && m.Kind == outcome.EvidenceCheck:
			status = StatusCheckPassed
			rep.Counts.CheckPassed++
			rep.Accepted++
		case errors.As(errs[i], &statusErr):
			status = statusErr.status
		default:
			status = statusFromEvidence(cr.Evidence, m.Kind)
		}
		switch status {
		case StatusStale:
			rep.Counts.Stale++
		case StatusRejected:
			rep.Counts.Rejected++
		case StatusCheckUnverified:
			rep.Counts.Unverified++
		}
		rev := ""
		if status == StatusCheckPassed {
			rev = checkRevisionFor(m, checks, ctx.PlanIdentity)
			if rev != "" {
				seen[rev] = struct{}{}
			}
		}
		evidence := cr.Evidence
		switch status {
		case StatusManuallyAccepted:
			if d, ok := latestDecisionFor(m.CriterionID, ctx.PlanIdentity, decisions); ok {
				evidence = fmt.Sprintf("accepted by %s: %s", d.Approver, d.Evidence)
				if d.AnnotationID != "" {
					evidence += fmt.Sprintf(" (annotation %s)", d.AnnotationID)
				}
			}
		case StatusCheckPassed:
			evidence = fmt.Sprintf("passed; tested revision %s", rev)
		}
		rep.Criteria = append(rep.Criteria, CriterionReport{Mapping: m, Status: status, Evidence: evidence, Revision: rev})
	}
	for rev := range seen {
		rep.CheckRevisions = append(rep.CheckRevisions, rev)
	}
	sort.Strings(rep.CheckRevisions)
	return rep, nil
}

func corruptReport(rep *Report, err error) (*Report, error) {
	var corrupt *CorruptError
	if errors.As(err, &corrupt) {
		rep.Corrupt = corrupt
		return rep, nil
	}
	return nil, err
}

func statusFromEvidence(evidence string, kind outcome.EvidenceKind) CriterionStatus {
	if kind == outcome.EvidenceCheck {
		return StatusCheckUnverified
	}
	return StatusDecisionEligible
}

func evaluateManual(ctx *Context, m Mapping, decisions []outcome.AcceptanceDecision, annotations []Annotation, designated []string) error {
	var latest *outcome.AcceptanceDecision
	foreign := false
	for i := len(decisions) - 1; i >= 0; i-- {
		d := decisions[i]
		if d.CriterionID != m.CriterionID {
			continue
		}
		if d.PlanIdentity != ctx.PlanIdentity {
			foreign = true
			continue
		}
		latest = &d
		break
	}
	if latest == nil {
		if foreign {
			return &criterionError{status: StatusStale, err: errors.New("a decision exists only for a different plan identity")}
		}
		return &criterionError{status: StatusDecisionEligible, err: errors.New("no manual acceptance decision is recorded")}
	}
	d := *latest

	effective := m.ManualEvidence
	if strings.TrimSpace(d.AnnotationID) != "" {
		ann, ok := findAnnotation(annotations, d.AnnotationID)
		if !ok {
			return &criterionError{status: StatusStale, err: fmt.Errorf("annotation %s is missing", d.AnnotationID)}
		}
		if err := validateAnnotation(ctx, m, ann, d); err != nil {
			return &criterionError{status: StatusStale, err: err}
		}
		effective = ann.ReviewedEvidence
	}

	mp := m
	mp.ManualEvidence = effective
	mapping := outcome.CriterionMapping{
		CriterionID:    mp.CriterionID,
		Description:    mp.Description,
		Required:       true,
		Kind:           outcome.EvidenceManual,
		ManualEvidence: effective,
		PlanIdentity:   ctx.PlanIdentity,
	}
	d.ManualEvidence = effective
	d.CriterionDescription = mp.Description
	d.PlanIdentity = ctx.PlanIdentity
	if err := outcome.ValidateAcceptanceDecision(d, []outcome.CriterionMapping{mapping}, designated); err != nil {
		return &criterionError{status: StatusStale, err: err}
	}
	if err := VerifyEvidenceRefs(ctx.ProjectDir, d.EvidenceRefs); err != nil {
		return &criterionError{status: StatusStale, err: fmt.Errorf("evidence no longer verifies: %w", err)}
	}
	if d.Decision != outcome.DecisionAccept {
		return &criterionError{status: StatusRejected, err: fmt.Errorf("the manual decision is %q, not accept", d.Decision)}
	}
	return nil
}

func evaluateCheck(ctx *Context, m Mapping, checks []CheckEvidence) error {
	var latest *CheckEvidence
	foreign := false
	for i := len(checks) - 1; i >= 0; i-- {
		c := checks[i]
		if c.CriterionID != m.CriterionID {
			continue
		}
		if c.PlanIdentity != ctx.PlanIdentity {
			foreign = true
			continue
		}
		latest = &c
		break
	}
	if latest == nil {
		if foreign {
			return &criterionError{status: StatusStale, err: errors.New("check evidence exists only for a different plan identity")}
		}
		return &criterionError{status: StatusCheckUnverified, err: errors.New("no check evidence is recorded")}
	}
	c := *latest
	switch {
	case c.Kind != "historical_check":
		return &criterionError{status: StatusStale, err: errors.New("unexpected check record kind")}
	case c.CheckRef != m.CheckRef:
		return &criterionError{status: StatusStale, err: fmt.Errorf("check ref %q does not match %q", c.CheckRef, m.CheckRef)}
	case c.Command != m.Command:
		return &criterionError{status: StatusStale, err: fmt.Errorf("check command %q does not match the archived command", c.Command)}
	case c.Class != "SAFE":
		return &criterionError{status: StatusStale, err: fmt.Errorf("check was recorded with class %q, not SAFE", c.Class)}
	case !c.Executed:
		return &criterionError{status: StatusCheckUnverified, err: errors.New("check was not executed")}
	case !c.Passed:
		return &criterionError{status: StatusRejected, err: fmt.Errorf("check failed (exit %d)", c.ExitCode)}
	case !c.SourceUnchanged:
		return &criterionError{status: StatusStale, err: errors.New("the checked source changed during execution")}
	case c.TestedDirty:
		return &criterionError{status: StatusStale, err: errors.New("the check ran on a dirty workspace")}
	case strings.TrimSpace(c.TestedRevision) == "":
		return &criterionError{status: StatusStale, err: errors.New("check has no tested revision")}
	}
	if c.OutputSHA256 != "" && sha256Hex(c.Output) != strings.ToLower(c.OutputSHA256) {
		return &criterionError{status: StatusStale, err: errors.New("check output excerpt changed since it was recorded")}
	}
	if c.LogPath != "" {
		if err := verifyCheckLog(ctx, c); err != nil {
			return &criterionError{status: StatusStale, err: err}
		}
	}
	return nil
}

func latestDecisionFor(criterionID, planIdentity string, decisions []outcome.AcceptanceDecision) (outcome.AcceptanceDecision, bool) {
	for i := len(decisions) - 1; i >= 0; i-- {
		d := decisions[i]
		if d.CriterionID == criterionID && d.PlanIdentity == planIdentity {
			return d, true
		}
	}
	return outcome.AcceptanceDecision{}, false
}

func verifyCheckLog(ctx *Context, c CheckEvidence) error {
	abs, err := resolveUnderRoot(ctx.ProjectDir, c.LogPath)
	if err != nil {
		return fmt.Errorf("check log: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("check log missing: %w", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(c.LogSHA256) {
		return errors.New("check log changed since it was recorded")
	}
	return nil
}

func findAnnotation(annotations []Annotation, id string) (Annotation, bool) {
	for _, a := range annotations {
		if a.ID == id {
			return a, true
		}
	}
	return Annotation{}, false
}

func validateAnnotation(ctx *Context, m Mapping, ann Annotation, d outcome.AcceptanceDecision) error {
	archivedHash := sha256Hex(m.ManualEvidence)
	if ann.ID != d.AnnotationID {
		return errors.New("decision annotation id does not match the annotation")
	}
	if ann.PlanIdentity != ctx.PlanIdentity {
		return errors.New("annotation belongs to a different plan identity")
	}
	if ann.CriterionID != m.CriterionID {
		return fmt.Errorf("annotation names criterion %s, not %s", ann.CriterionID, m.CriterionID)
	}
	if ann.ArchivedManualEvidence != m.ManualEvidence {
		return errors.New("annotation does not preserve the archived manual evidence")
	}
	if ann.ArchivedEvidenceSHA256 != archivedHash {
		return errors.New("annotation archived evidence hash does not match the archived text")
	}
	if ann.ReviewedEvidence != d.ManualEvidence {
		return errors.New("decision reviewed evidence does not match the annotation")
	}
	if ann.ReviewedEvidenceSHA256 != sha256Hex(ann.ReviewedEvidence) {
		return errors.New("annotation reviewed-evidence hash does not match its text")
	}
	if strings.TrimSpace(ann.Reason) == "" {
		return errors.New("annotation reason is empty")
	}
	if err := VerifyEvidenceRefs(ctx.ProjectDir, ann.EvidenceRefs); err != nil {
		return fmt.Errorf("annotation evidence no longer verifies: %w", err)
	}
	return nil
}

func checkRevisionFor(m Mapping, checks []CheckEvidence, planIdentity string) string {
	for i := len(checks) - 1; i >= 0; i-- {
		if checks[i].CriterionID == m.CriterionID && checks[i].PlanIdentity == planIdentity {
			return checks[i].TestedRevision
		}
	}
	return ""
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
