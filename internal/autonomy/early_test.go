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
}
