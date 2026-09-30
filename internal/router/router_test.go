package router

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
)

// clearEvidence is a validated PASS analysis: nothing to report.
func clearEvidence() *jev.Evidence {
	return &jev.Evidence{
		Version:  jev.EvidenceVersion,
		Purpose:  jev.PurposeTaskTriage,
		Severity: jev.SeverityInfo,
		Status:   jev.EvidencePass,
	}
}

// findingsEvidence is a validated FINDINGS analysis with the given typed items.
func findingsEvidence(items ...jev.EvidenceItem) *jev.Evidence {
	ev := &jev.Evidence{
		Version:  jev.EvidenceVersion,
		Purpose:  jev.PurposePreExecution,
		Severity: jev.SeverityInfo,
		Status:   jev.EvidenceFindings,
		Items:    items,
	}
	for _, it := range items {
		if severityScore(it.Severity) > severityScore(ev.Severity) {
			ev.Severity = it.Severity
		}
	}
	return ev
}

func item(category jev.Category, severity jev.Severity) jev.EvidenceItem {
	return jev.EvidenceItem{
		Purpose:  jev.PurposePreExecution,
		Severity: severity,
		Category: category,
		Detail:   "detail",
		Evidence: "evidence",
	}
}

func TestDecideTable(t *testing.T) {
	tests := []struct {
		name   string
		ev     *jev.Evidence
		task   TaskSignals
		want   model.Class
		reason string
	}{
		{
			name:   "clear small task routes SMALL",
			ev:     clearEvidence(),
			task:   TaskSignals{AcceptanceCriteria: 2},
			want:   model.ClassSmall,
			reason: ReasonSmallTask,
		},
		{
			name:   "single-file low-risk change routes SMALL",
			ev:     clearEvidence(),
			task:   TaskSignals{AcceptanceCriteria: 1, FilesAffected: 1},
			want:   model.ClassSmall,
			reason: ReasonSmallTask,
		},
		{
			name:   "normal feature (many criteria) routes MEDIUM",
			ev:     clearEvidence(),
			task:   TaskSignals{AcceptanceCriteria: 6},
			want:   model.ClassMedium,
			reason: ReasonDefault,
		},
		{
			name:   "clear task with a dependency routes MEDIUM",
			ev:     clearEvidence(),
			task:   TaskSignals{AcceptanceCriteria: 2, Dependencies: 1},
			want:   model.ClassMedium,
			reason: ReasonDefault,
		},
		{
			name:   "moderate multi-file change routes MEDIUM",
			ev:     clearEvidence(),
			task:   TaskSignals{AcceptanceCriteria: 4, FilesAffected: 3},
			want:   model.ClassMedium,
			reason: ReasonMultiFile,
		},
		{
			name:   "scope concern routes LARGE",
			ev:     findingsEvidence(item(jev.CategoryScope, jev.SeverityLow)),
			task:   TaskSignals{AcceptanceCriteria: 4},
			want:   model.ClassLarge,
			reason: ReasonCrossCutting,
		},
		{
			name:   "high-risk security change routes LARGE",
			ev:     findingsEvidence(item(jev.CategorySecurity, jev.SeverityHigh)),
			task:   TaskSignals{AcceptanceCriteria: 1},
			want:   model.ClassLarge,
			reason: ReasonRiskHigh,
		},
		{
			name:   "destructive concern routes LARGE",
			ev:     findingsEvidence(item(jev.CategoryDestructive, jev.SeverityMedium)),
			task:   TaskSignals{},
			want:   model.ClassLarge,
			reason: ReasonRiskHigh,
		},
		{
			name:   "requirement conflict routes LARGE",
			ev:     findingsEvidence(item(jev.CategoryRequirementConflict, jev.SeverityLow)),
			task:   TaskSignals{},
			want:   model.ClassLarge,
			reason: ReasonComplexityHigh,
		},
		{
			name:   "missing context routes LARGE",
			ev:     findingsEvidence(item(jev.CategoryMissingContext, jev.SeverityLow)),
			task:   TaskSignals{},
			want:   model.ClassLarge,
			reason: ReasonComplexityHigh,
		},
		{
			name:   "conflicting evidence: low complexity + high risk routes LARGE",
			ev:     findingsEvidence(item(jev.CategorySecurity, jev.SeverityCritical)),
			task:   TaskSignals{AcceptanceCriteria: 1},
			want:   model.ClassLarge,
			reason: ReasonRiskHigh,
		},
		{
			name:   "JEV unavailable routes MEDIUM",
			ev:     nil,
			task:   TaskSignals{AcceptanceCriteria: 1},
			want:   model.ClassMedium,
			reason: ReasonJEVUnavailable,
		},
		{
			name:   "invalid evidence status routes MEDIUM",
			ev:     &jev.Evidence{Version: jev.EvidenceVersion, Status: "NOT_A_STATUS"},
			task:   TaskSignals{AcceptanceCriteria: 1},
			want:   model.ClassMedium,
			reason: ReasonJEVUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(SignalsFrom(tt.ev, tt.task))
			if got.Class != tt.want {
				t.Fatalf("class = %q, want %q (reasons %v)", got.Class, tt.want, got.Reasons)
			}
			if !contains(got.Reasons, tt.reason) {
				t.Fatalf("reasons = %v, want to contain %q", got.Reasons, tt.reason)
			}
			for _, r := range got.Reasons {
				if r == "" {
					t.Fatalf("empty reason phrase in %v", got.Reasons)
				}
			}
		})
	}
}

// TestReasonsAreMeaningful pins the routing explanations so a decision cannot
// become opaque: every class names WHY it was chosen.
func TestReasonsAreMeaningful(t *testing.T) {
	got := Decide(SignalsFrom(findingsEvidence(item(jev.CategorySecurity, jev.SeverityHigh)), TaskSignals{}))
	if len(got.Reasons) == 0 || got.Reasons[0] != ReasonRiskHigh {
		t.Fatalf("reasons = %v, want [%q]", got.Reasons, ReasonRiskHigh)
	}
}

func TestSignalsFromDerivation(t *testing.T) {
	ev := findingsEvidence(
		item(jev.CategorySecurity, jev.SeverityMedium),
		item(jev.CategoryMissingContext, jev.SeverityLow),
	)
	ev.Confidence = 0.8
	s := SignalsFrom(ev, TaskSignals{AcceptanceCriteria: 2, Dependencies: 1, FilesAffected: 3})

	if !s.JEVAvailable {
		t.Error("JEVAvailable = false, want true")
	}
	if s.Risk != LevelHigh {
		t.Errorf("Risk = %q, want HIGH (security family floors to HIGH)", s.Risk)
	}
	if s.Complexity != LevelHigh {
		t.Errorf("Complexity = %q, want HIGH (missing context floors to HIGH)", s.Complexity)
	}
	if !s.RequiresContext {
		t.Error("RequiresContext = false, want true")
	}
	if s.Scope != ScopeMulti {
		t.Errorf("Scope = %q, want MULTI_FILE", s.Scope)
	}
	if s.Confidence != 0.8 {
		t.Errorf("Confidence = %v, want 0.8", s.Confidence)
	}
}

func TestSignalsMergeEscalatesOnly(t *testing.T) {
	low := SignalsFrom(clearEvidence(), TaskSignals{})
	high := SignalsFrom(findingsEvidence(item(jev.CategorySecurity, jev.SeverityHigh)), TaskSignals{})

	merged := low.Merge(high)
	if merged.Risk != LevelHigh || !merged.JEVAvailable {
		t.Fatalf("merged = %+v, want the escalated risk", merged)
	}
	// Merge must never downgrade.
	if got := high.Merge(low); got.Risk != LevelHigh {
		t.Fatalf("reverse merge downgraded risk to %q", got.Risk)
	}
}

func TestSignalsFromNilTask(t *testing.T) {
	s := SignalsFrom(nil, TaskSignals{AcceptanceCriteria: 2})
	if s.JEVAvailable {
		t.Fatal("JEVAvailable = true without evidence")
	}
	if s.Risk != LevelLow || s.Complexity != LevelLow || s.Scope != ScopeSingle {
		t.Fatalf("zero signals not neutral: %+v", s)
	}
}

// TestConfidenceAloneDoesNotRoute proves a bare confidence value is never a
// routing rule (Phase 3.5 §9): the same class is chosen regardless of confidence.
func TestConfidenceAloneDoesNotRoute(t *testing.T) {
	for _, conf := range []float64{0, 0.1, 0.69, 0.7, 0.99, 1} {
		ev := clearEvidence()
		ev.Confidence = conf
		if got := Decide(SignalsFrom(ev, TaskSignals{AcceptanceCriteria: 2})); got.Class != model.ClassSmall {
			t.Fatalf("confidence %v changed the class to %q", conf, got.Class)
		}
	}
}

func TestLevelAndScopeValid(t *testing.T) {
	for _, l := range []Level{LevelLow, LevelMedium, LevelHigh} {
		if !l.Valid() {
			t.Errorf("Level %q should be valid", l)
		}
	}
	if Level("nope").Valid() {
		t.Error("unknown level must be invalid")
	}
	for _, s := range []Scope{ScopeSingle, ScopeMulti, ScopeCross} {
		if !s.Valid() {
			t.Errorf("Scope %q should be valid", s)
		}
	}
	if Scope("nope").Valid() {
		t.Error("unknown scope must be invalid")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
