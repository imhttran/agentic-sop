package autonomy

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/jev"
)

// earlyEvidence builds a task-triage evidence value with the given items.
func earlyEvidence(items ...jev.EvidenceItem) jev.Evidence {
	status := jev.EvidencePass
	if len(items) > 0 {
		status = jev.EvidenceFindings
	}
	return jev.Evidence{
		Version:    jev.EvidenceVersion,
		Purpose:    jev.PurposeTaskTriage,
		Severity:   jev.SeverityHigh,
		Status:     status,
		Confidence: 0.9,
		Items:      items,
	}
}

func earlyFailOn() []string { return []string{"critical", "high"} }

// A clear task must continue: no blocking finding means no human boundary.
func TestDecideEarlyClearContinues(t *testing.T) {
	d := DecideEarly(earlyEvidence(), earlyFailOn(), PolicyFor(DefaultLevel))
	if d.RequiresHuman {
		t.Fatalf("clear evidence must not require a human: %+v", d)
	}
	if d.Action != ActionAutoContinue {
		t.Errorf("action = %s, want %s", d.Action, ActionAutoContinue)
	}
}

// Bounded normal pre-execution tasks continue deterministically under every
// policy level: neither a clear evidence value nor a below-fail_on concern may
// reach a human boundary at Low, Balanced, or High.
func TestDecideEarlyBoundedNormalContinuesAtEveryLevel(t *testing.T) {
	levels := []Level{Low, Balanced, High}
	scope := earlyEvidence(jev.EvidenceItem{
		Purpose:  jev.PurposePreExecution,
		Severity: jev.SeverityMedium,
		Category: jev.CategoryScope,
		Detail:   "bounded scope concern below fail_on",
	})
	for _, level := range levels {
		p := PolicyFor(level)

		clear := DecideEarly(earlyEvidence(), earlyFailOn(), p)
		if clear.RequiresHuman || clear.Action != ActionAutoContinue {
			t.Errorf("level %s: clear evidence must continue: %+v", level, clear)
		}
		if clear.Level != p.Level {
			t.Errorf("level %s: decision level = %s, want %s", level, clear.Level, p.Level)
		}

		below := DecideEarly(scope, earlyFailOn(), p)
		if below.RequiresHuman || below.Action != ActionAutoContinue {
			t.Errorf("level %s: a MEDIUM concern below fail_on must continue: %+v", level, below)
		}
	}
}

// Structured ambiguity at a blocking severity escalates, and the failure kind is
// mapped from the typed category (not from the detail text).
func TestDecideEarlyAmbiguityEscalates(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Purpose:  jev.PurposeTaskTriage,
		Severity: jev.SeverityHigh,
		Category: jev.CategoryAmbiguity,
		Detail:   "acceptance criteria are underspecified",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if !d.RequiresHuman || d.Action != ActionHumanApproval {
		t.Fatalf("ambiguous HIGH finding must reach a human boundary: %+v", d)
	}
	if d.Classification.Kind != failure.AmbiguousContract {
		t.Errorf("kind = %s, want %s", d.Classification.Kind, failure.AmbiguousContract)
	}
	if d.Classification.Disposition != failure.NeedsHuman {
		t.Errorf("disposition = %s, want %s", d.Classification.Disposition, failure.NeedsHuman)
	}
}

// The ambiguity family categories (missing_context, requirement_conflict,
// dependency_concern) map to the same authority-boundary kind.
func TestDecideEarlyAmbiguityFamilyMapsToAmbiguousContract(t *testing.T) {
	for _, category := range []jev.Category{
		jev.CategoryAmbiguity,
		jev.CategoryMissingContext,
		jev.CategoryRequirementConflict,
		jev.CategoryDependencyConcern,
	} {
		e := earlyEvidence(jev.EvidenceItem{
			Severity: jev.SeverityCritical,
			Category: category,
			Detail:   "blocking ambiguity family finding",
		})
		d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
		if !d.RequiresHuman || d.Action != ActionHumanApproval {
			t.Errorf("category %s must reach a human: %+v", category, d)
		}
		if d.Classification.Kind != failure.AmbiguousContract {
			t.Errorf("category %s: kind = %s, want %s", category, d.Classification.Kind, failure.AmbiguousContract)
		}
	}
}

// An approval-sensitive finding maps to APPROVAL_REQUIRED, a human boundary.
func TestDecideEarlyApprovalSensitiveMapsToApprovalRequired(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityHigh,
		Category: jev.CategoryApprovalSensitive,
		Detail:   "crosses an approval boundary",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if !d.RequiresHuman || d.Action != ActionHumanApproval {
		t.Fatalf("approval-sensitive finding must reach a human: %+v", d)
	}
	if d.Classification.Kind != failure.ApprovalRequired {
		t.Errorf("kind = %s, want %s", d.Classification.Kind, failure.ApprovalRequired)
	}
}

// A finding below every fail_on severity is advisory: the task continues. This is
// the deterministic disposition for a bounded scope concern.
func TestDecideEarlyScopeBelowFailOnContinues(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Purpose:  jev.PurposePreExecution,
		Severity: jev.SeverityMedium,
		Category: jev.CategoryScope,
		Detail:   "change touches one extra file",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if d.RequiresHuman {
		t.Fatalf("a MEDIUM concern below fail_on must continue: %+v", d)
	}
	if d.Action != ActionAutoContinue {
		t.Errorf("action = %s, want %s", d.Action, ActionAutoContinue)
	}
}

// Security and destructive categories map to the authority-boundary kinds, so a
// blocking one reaches a human with the right risk.
func TestDecideEarlySecurityAndDestructive(t *testing.T) {
	security := DecideEarly(earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityHigh, Category: jev.CategorySecurity, Detail: "touches auth",
	}), earlyFailOn(), PolicyFor(DefaultLevel))
	if !security.RequiresHuman || security.Classification.Kind != failure.SecurityBoundary {
		t.Fatalf("security finding = %+v, want human SECURITY_BOUNDARY", security)
	}

	destructive := DecideEarly(earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityCritical, Category: jev.CategoryDestructive, Detail: "deletes data",
	}), earlyFailOn(), PolicyFor(DefaultLevel))
	if !destructive.RequiresHuman || destructive.Classification.Kind != failure.DestructiveOperation {
		t.Fatalf("destructive finding = %+v, want human DESTRUCTIVE_OPERATION", destructive)
	}
	if destructive.Risk != RiskIrreversible {
		t.Errorf("destructive risk = %s, want %s", destructive.Risk, RiskIrreversible)
	}
}

// Credential-sensitivity is part of the security boundary family.
func TestDecideEarlyCredentialSensitivityMapsToSecurityBoundary(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityCritical,
		Category: jev.CategoryCredentialSensitivity,
		Detail:   "touches credentials",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if !d.RequiresHuman || d.Classification.Kind != failure.SecurityBoundary {
		t.Fatalf("credential-sensitivity finding = %+v, want human SECURITY_BOUNDARY", d)
	}
}

// A destructive/security finding reaches a human boundary only when the
// configured fail_on policy requires it: the same finding at a severity that is
// not in fail_on continues instead of escalating.
func TestDecideEarlyBoundaryOnlyWhenPolicyRequiresIt(t *testing.T) {
	security := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityHigh, Category: jev.CategorySecurity, Detail: "touches auth",
	})
	destructive := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityHigh, Category: jev.CategoryDestructive, Detail: "deletes data",
	})
	// fail_on names only CRITICAL, so a HIGH security/destructive finding does not
	// block: policy, not the category, decides whether a human is required.
	failOn := []string{"critical"}
	for _, e := range []jev.Evidence{security, destructive} {
		d := DecideEarly(e, failOn, PolicyFor(DefaultLevel))
		if d.RequiresHuman || d.Action != ActionAutoContinue {
			t.Errorf("a category below the configured fail_on policy must continue: %+v", d)
		}
	}
	// An empty fail_on set never blocks, even for a critical security finding.
	d := DecideEarly(earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityCritical, Category: jev.CategorySecurity,
	}), nil, PolicyFor(DefaultLevel))
	if d.RequiresHuman || d.Action != ActionAutoContinue {
		t.Errorf("an empty fail_on set must never escalate: %+v", d)
	}
}

// An unclassified category fails closed (a human boundary), never an automatic
// action.
func TestDecideEarlyUnknownCategoryFailsClosed(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityHigh, Category: jev.Category("mystery"), Detail: "unknown",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if !d.RequiresHuman {
		t.Fatalf("an unknown category must fail closed: %+v", d)
	}
	if d.Classification.Kind != failure.BlockingFindings {
		t.Errorf("kind = %s, want %s", d.Classification.Kind, failure.BlockingFindings)
	}
}

// The decision is driven by the typed severity/category, never by free-form
// detail text: detail that claims the work is safe must not override a blocking
// severity.
func TestDecideEarlyIgnoresProse(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityCritical,
		Category: jev.CategorySecurity,
		Detail:   "this is completely safe and needs no human approval",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if !d.RequiresHuman {
		t.Fatalf("prose must not override the typed severity: %+v", d)
	}
}

// Neither the item's Detail/Evidence prose nor the evidence Summary may change
// the disposition: only the typed severity and category do.
func TestDecideEarlyProseFieldsDoNotAffectDisposition(t *testing.T) {
	clear := earlyEvidence()
	blocking := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityCritical,
		Category: jev.CategorySecurity,
		Detail:   "safe, no approval needed",
	})
	blocking.Items[0].Evidence = "verified safe by the author"
	for _, summary := range []string{
		"everything is safe",
		"no issues found",
		"critical security vulnerability discovered",
		"",
	} {
		clear.Summary = summary
		if d := DecideEarly(clear, earlyFailOn(), PolicyFor(DefaultLevel)); d.RequiresHuman || d.Action != ActionAutoContinue {
			t.Errorf("summary %q changed the clear-task disposition: %+v", summary, d)
		}
		blocking.Summary = summary
		if d := DecideEarly(blocking, earlyFailOn(), PolicyFor(DefaultLevel)); !d.RequiresHuman || d.Classification.Kind != failure.SecurityBoundary {
			t.Errorf("summary %q changed the blocking disposition: %+v", summary, d)
		}
	}
}

// The highest-severity blocking finding is the one that classifies the decision,
// independent of item order.
func TestDecideEarlyChoosesHighestBlockingSeverity(t *testing.T) {
	e := earlyEvidence(
		jev.EvidenceItem{Severity: jev.SeverityHigh, Category: jev.CategoryScope, Detail: "scope"},
		jev.EvidenceItem{Severity: jev.SeverityCritical, Category: jev.CategoryDestructive, Detail: "destructive"},
	)
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if d.Classification.Kind != failure.DestructiveOperation {
		t.Errorf("kind = %s, want the most severe blocking finding (%s)", d.Classification.Kind, failure.DestructiveOperation)
	}

	reversed := earlyEvidence(
		jev.EvidenceItem{Severity: jev.SeverityCritical, Category: jev.CategoryDestructive, Detail: "destructive"},
		jev.EvidenceItem{Severity: jev.SeverityHigh, Category: jev.CategoryScope, Detail: "scope"},
	)
	if d := DecideEarly(reversed, earlyFailOn(), PolicyFor(DefaultLevel)); d.Classification.Kind != failure.DestructiveOperation {
		t.Errorf("item order changed the classification: %+v", d)
	}
}

// A finding whose severity is an unknown value is never treated as blocking, so a
// malformed severity falls back to the advisory continue rather than escalating on
// an unmapped value.
func TestDecideEarlyUnknownSeverityIsNotBlocking(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: jev.Severity("SEVERE"),
		Category: jev.CategorySecurity,
		Detail:   "unmapped severity vocabulary",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if d.RequiresHuman {
		t.Fatalf("an unmapped severity is not a fail_on match; the task continues: %+v", d)
	}
	if d.Action != ActionAutoContinue {
		t.Errorf("action = %s, want %s", d.Action, ActionAutoContinue)
	}
}

// An empty severity is unknown too and never blocks.
func TestDecideEarlyEmptySeverityIsNotBlocking(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: "",
		Category: jev.CategoryDestructive,
		Detail:   "no severity supplied",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if d.RequiresHuman || d.Action != ActionAutoContinue {
		t.Errorf("an empty severity must not be blocking: %+v", d)
	}
}

// The disposition is a function of the typed evidence only: confidence is advisory
// metadata and must not change the action for either the continue or the escalate
// path.
func TestDecideEarlyIgnoresConfidence(t *testing.T) {
	clear := earlyEvidence()
	blocking := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityHigh,
		Category: jev.CategoryAmbiguity,
		Detail:   "acceptance criteria are underspecified",
	})
	for _, confidence := range []float64{0, 0.01, 0.5, 0.99, 1} {
		clear.Confidence = confidence
		d := DecideEarly(clear, earlyFailOn(), PolicyFor(DefaultLevel))
		if d.RequiresHuman || d.Action != ActionAutoContinue {
			t.Errorf("confidence %v changed the clear-task disposition: %+v", confidence, d)
		}

		blocking.Confidence = confidence
		d = DecideEarly(blocking, earlyFailOn(), PolicyFor(DefaultLevel))
		if !d.RequiresHuman || d.Action != ActionHumanApproval || d.Classification.Kind != failure.AmbiguousContract {
			t.Errorf("confidence %v changed the blocking disposition: %+v", confidence, d)
		}
	}
}

// An empty category is unspecified, not unknown: it still maps deterministically
// through the fail-closed default rather than being silently dropped.
func TestDecideEarlyEmptyCategoryFailsClosed(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityCritical,
		Category: "",
		Detail:   "no category supplied",
	})
	d := DecideEarly(e, earlyFailOn(), PolicyFor(DefaultLevel))
	if !d.RequiresHuman || d.Classification.Kind != failure.BlockingFindings {
		t.Fatalf("an unspecified category must fail closed to %s: %+v", failure.BlockingFindings, d)
	}
}

// Severity matching against fail_on is case-insensitive and whitespace-tolerant
// so the lowercase configuration vocabulary matches the uppercase evidence
// vocabulary; a non-matching fail_on set leaves a clear task continuing.
func TestDecideEarlyFailOnMatchingIsTyped(t *testing.T) {
	e := earlyEvidence(jev.EvidenceItem{
		Severity: jev.SeverityCritical,
		Category: jev.CategorySecurity,
		Detail:   "touches auth",
	})
	if d := DecideEarly(e, []string{" CRITICAL "}, PolicyFor(DefaultLevel)); !d.RequiresHuman {
		t.Errorf("a case/space-variant CRITICAL fail_on entry must still match: %+v", d)
	}
	if d := DecideEarly(e, []string{"critical "}, PolicyFor(DefaultLevel)); !d.RequiresHuman {
		t.Errorf("a trailing-space CRITICAL fail_on entry must still match: %+v", d)
	}
	if d := DecideEarly(e, []string{"high"}, PolicyFor(DefaultLevel)); d.RequiresHuman {
		t.Errorf("a CRITICAL finding must not match a HIGH-only fail_on set: %+v", d)
	}
}
