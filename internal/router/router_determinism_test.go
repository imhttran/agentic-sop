package router

import (
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
)

// This file pins the compatibility, escalation, fallback, override and secrecy
// properties of the deterministic router (Phase 3.5). Every case is a pure,
// in-memory function call: no provider, network, or clock is involved, so the
// suite is deterministic and offline.

// decided is a small helper that derives signals and decides in one step, the
// same composition the CLI seam uses.
func decided(ev *jev.Evidence, task TaskSignals) Decision {
	return Decide(SignalsFrom(ev, task))
}

// TestDecideRepeatDeterminism proves the router is a pure function: identical
// inputs always yield the identical class and the identical reasons slice.
func TestDecideRepeatDeterminism(t *testing.T) {
	inputs := []struct {
		name string
		ev   *jev.Evidence
		task TaskSignals
	}{
		{"small", clearEvidence(), TaskSignals{AcceptanceCriteria: 2}},
		{"medium-default", clearEvidence(), TaskSignals{AcceptanceCriteria: 6}},
		{"multi-file", clearEvidence(), TaskSignals{AcceptanceCriteria: 4, FilesAffected: 3}},
		{"large-risk", findingsEvidence(item(jev.CategorySecurity, jev.SeverityHigh)), TaskSignals{}},
		{"large-complexity", findingsEvidence(item(jev.CategoryRequirementConflict, jev.SeverityLow)), TaskSignals{}},
		{"large-cross", findingsEvidence(item(jev.CategoryScope, jev.SeverityLow)), TaskSignals{}},
		{"fallback-nil", nil, TaskSignals{AcceptanceCriteria: 1}},
		{"fallback-invalid", &jev.Evidence{Version: jev.EvidenceVersion, Status: "NOT_A_STATUS"}, TaskSignals{AcceptanceCriteria: 1}},
	}
	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			first := decided(in.ev, in.task)
			for i := 0; i < 5; i++ {
				got := decided(in.ev, in.task)
				if got.Class != first.Class {
					t.Fatalf("call %d class = %q, want %q", i, got.Class, first.Class)
				}
				if !reflect.DeepEqual(got.Reasons, first.Reasons) {
					t.Fatalf("call %d reasons = %v, want %v", i, got.Reasons, first.Reasons)
				}
			}
		})
	}
}

// TestEscalationPrecedence proves that high risk, high complexity, and
// cross-cutting each independently escalate to LARGE with the exact fixed reason
// phrase, even when the task structure would otherwise qualify as SMALL.
func TestEscalationPrecedence(t *testing.T) {
	smallTask := TaskSignals{AcceptanceCriteria: 1, FilesAffected: 1}
	tests := []struct {
		name   string
		ev     *jev.Evidence
		want   model.Class
		reason string
	}{
		{
			name:   "high risk escalates a small-looking task",
			ev:     findingsEvidence(item(jev.CategorySecurity, jev.SeverityHigh)),
			want:   model.ClassLarge,
			reason: ReasonRiskHigh,
		},
		{
			name:   "high complexity escalates a small-looking task",
			ev:     findingsEvidence(item(jev.CategoryAmbiguity, jev.SeverityLow)),
			want:   model.ClassLarge,
			reason: ReasonComplexityHigh,
		},
		{
			name:   "cross-cutting escalates a small-looking task",
			ev:     findingsEvidence(item(jev.CategoryUnexpectedArea, jev.SeverityInfo)),
			want:   model.ClassLarge,
			reason: ReasonCrossCutting,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decided(tt.ev, smallTask)
			if got.Class != tt.want {
				t.Fatalf("class = %q, want %q (reasons %v)", got.Class, tt.want, got.Reasons)
			}
			if len(got.Reasons) != 1 || got.Reasons[0] != tt.reason {
				t.Fatalf("reasons = %v, want [%q]", got.Reasons, tt.reason)
			}
		})
	}
}

// TestEscalationOrdering pins the documented precedence order: a high-risk
// signal wins over high complexity, which wins over cross-cutting scope, so the
// reported reason is stable when several escalations apply at once.
func TestEscalationOrdering(t *testing.T) {
	ev := findingsEvidence(
		item(jev.CategorySecurity, jev.SeverityHigh),            // risk
		item(jev.CategoryRequirementConflict, jev.SeverityHigh), // complexity
		item(jev.CategoryScope, jev.SeverityHigh),               // cross-cutting
	)
	got := decided(ev, TaskSignals{})
	if got.Class != model.ClassLarge {
		t.Fatalf("class = %q, want large", got.Class)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != ReasonRiskHigh {
		t.Fatalf("reasons = %v, want risk escalation to take precedence", got.Reasons)
	}
}

// TestFallbackToMedium proves nil and invalid evidence both fall back to MEDIUM
// with the JEV-unavailable reason, even when the task structure looks small —
// the router never selects SMALL without typed JEV evidence.
func TestFallbackToMedium(t *testing.T) {
	smallTask := TaskSignals{AcceptanceCriteria: 1, Dependencies: 0, FilesAffected: 1}
	tests := []struct {
		name string
		ev   *jev.Evidence
	}{
		{"nil evidence", nil},
		{"invalid evidence status", &jev.Evidence{Version: jev.EvidenceVersion, Status: "NOT_A_STATUS"}},
		{"unset evidence status", &jev.Evidence{Version: jev.EvidenceVersion}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decided(tt.ev, smallTask)
			if got.Class != model.ClassMedium {
				t.Fatalf("class = %q, want medium", got.Class)
			}
			if len(got.Reasons) != 1 || got.Reasons[0] != ReasonJEVUnavailable {
				t.Fatalf("reasons = %v, want [%q]", got.Reasons, ReasonJEVUnavailable)
			}
		})
	}
}

// TestConfidenceNeverOverridesClass extends the confidence invariant: across a
// full sweep of confidence values the class and reasons are byte-identical for
// escalation-free evidence.
func TestConfidenceNeverOverridesClass(t *testing.T) {
	base := TaskSignals{AcceptanceCriteria: 2}
	var wantClass model.Class
	var wantReasons []string
	for i, conf := range []float64{0, 0.01, 0.5, 0.69, 0.7, 0.99, 1} {
		ev := clearEvidence()
		ev.Confidence = conf
		got := decided(ev, base)
		if i == 0 {
			wantClass, wantReasons = got.Class, got.Reasons
			continue
		}
		if got.Class != wantClass || !reflect.DeepEqual(got.Reasons, wantReasons) {
			t.Fatalf("confidence %v changed the decision: %q %v", conf, got.Class, got.Reasons)
		}
	}
}

// TestMergeIsMonotonic proves Merge can only escalate, never downgrade: for a
// sweep of signal pairs, merging in either order yields the escalated value.
func TestMergeIsMonotonic(t *testing.T) {
	mk := func(risk, comp Level, scope Scope, cc, rc bool) Signals {
		return Signals{JEVAvailable: true, Risk: risk, Complexity: comp, Scope: scope, CrossCutting: cc, RequiresContext: rc}
	}
	sets := []Signals{
		mk(LevelLow, LevelLow, ScopeSingle, false, false),
		mk(LevelLow, LevelMedium, ScopeMulti, false, true),
		mk(LevelMedium, LevelHigh, ScopeCross, true, false),
		mk(LevelHigh, LevelLow, ScopeSingle, true, true),
	}
	for i, a := range sets {
		for j, b := range sets {
			ab := a.Merge(b)
			ba := b.Merge(a)
			if ab.Risk != ba.Risk || ab.Complexity != ba.Complexity || ab.Scope != ba.Scope ||
				ab.CrossCutting != ba.CrossCutting || ab.RequiresContext != ba.RequiresContext {
				t.Fatalf("merge(%d,%d) not commutative: %+v vs %+v", i, j, ab, ba)
			}
			if levelRank(ab.Risk) < levelRank(a.Risk) || levelRank(ab.Risk) < levelRank(b.Risk) {
				t.Fatalf("merge(%d,%d) downgraded risk: %+v", i, j, ab)
			}
			if levelRank(ab.Complexity) < levelRank(a.Complexity) || levelRank(ab.Complexity) < levelRank(b.Complexity) {
				t.Fatalf("merge(%d,%d) downgraded complexity: %+v", i, j, ab)
			}
			if scopeRank(ab.Scope) < scopeRank(a.Scope) || scopeRank(ab.Scope) < scopeRank(b.Scope) {
				t.Fatalf("merge(%d,%d) narrowed scope: %+v", i, j, ab)
			}
			if !ab.CrossCutting && (a.CrossCutting || b.CrossCutting) {
				t.Fatalf("merge(%d,%d) dropped the cross-cutting flag", i, j)
			}
			if !ab.RequiresContext && (a.RequiresContext || b.RequiresContext) {
				t.Fatalf("merge(%d,%d) dropped the requires-context flag", i, j)
			}
		}
	}
}

// TestFreeFormTextNeverInfluencesRouting proves routing depends only on typed
// categories and severities: arbitrary Item.Detail/Item.Evidence strings —
// including secret-shaped values — never change the class or reasons.
func TestFreeFormTextNeverInfluencesRouting(t *testing.T) {
	task := TaskSignals{AcceptanceCriteria: 1, FilesAffected: 1}
	baseline := findingsEvidence(item(jev.CategorySecurity, jev.SeverityHigh))
	want := decided(baseline, task)

	decorated := findingsEvidence(item(jev.CategorySecurity, jev.SeverityHigh))
	decorated.Items[0].Detail = "SECRET-abc \"quoted\" multiline\nvalue"
	decorated.Items[0].Evidence = "token=super-secret-value"
	if got := decided(decorated, task); got.Class != want.Class || !reflect.DeepEqual(got.Reasons, want.Reasons) {
		t.Fatalf("free-form detail changed the decision: %q %v (want %q %v)", got.Class, got.Reasons, want.Class, want.Reasons)
	}

	// A free-form-only finding that carries no typed category and no escalating
	// severity must not escalate: an empty (unspecified) category is valid and an
	// INFO severity scores zero, so the router ignores the prose entirely.
	noise := findingsEvidence(jev.EvidenceItem{Purpose: jev.PurposePreExecution, Severity: jev.SeverityInfo, Detail: "urgent!!! critical secret leak"})
	if got := decided(noise, task); got.Class != model.ClassSmall {
		t.Fatalf("free-form text escalated: class = %q, want small", got.Class)
	}
}
