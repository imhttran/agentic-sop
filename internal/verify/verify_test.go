package verify

import (
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/router"
)

// lowEvidence is the affirmatively-low typed evidence a LOW tier may be derived
// from: typed low risk on every signal with a trivial task structure.
func lowEvidence() router.Signals {
	return router.Signals{
		JEVAvailable:       true,
		Risk:               router.LevelLow,
		Complexity:         router.LevelLow,
		Scope:              router.ScopeSingle,
		FilesAffected:      smallMaxFiles,
		AcceptanceCriteria: smallMaxCriteria,
	}
}

func TestDeriveTable(t *testing.T) {
	cases := []struct {
		name        string
		signals     router.Signals
		verifyFirst bool
		want        Tier
		wantReason  string
	}{
		{
			name:       "high risk floors the tier",
			signals:    router.Signals{Risk: router.LevelHigh},
			want:       High,
			wantReason: ReasonRiskHigh,
		},
		{
			name:       "high complexity floors the tier",
			signals:    router.Signals{Complexity: router.LevelHigh},
			want:       High,
			wantReason: ReasonComplexityHigh,
		},
		{
			name:       "cross-cutting floors the tier",
			signals:    router.Signals{CrossCutting: true, Scope: router.ScopeCross},
			want:       High,
			wantReason: ReasonCrossCutting,
		},
		{
			name:        "high risk overrides an explicit verify-first declaration",
			signals:     router.Signals{Risk: router.LevelHigh},
			verifyFirst: true,
			want:        High,
			wantReason:  ReasonRiskHigh,
		},
		{
			name:        "verify-first without evidence is LOW",
			signals:     router.Signals{},
			verifyFirst: true,
			want:        Low,
			wantReason:  ReasonVerifyFirst,
		},
		{
			name:       "affirmatively low typed evidence is LOW",
			signals:    lowEvidence(),
			want:       Low,
			wantReason: ReasonLowRisk,
		},
		{
			name:       "no typed evidence defaults to MEDIUM",
			signals:    router.Signals{},
			want:       Medium,
			wantReason: ReasonNoEvidence,
		},
		{
			name: "evidence present but not affirmatively small stays MEDIUM",
			signals: router.Signals{
				JEVAvailable:  true,
				Risk:          router.LevelLow,
				Complexity:    router.LevelLow,
				Scope:         router.ScopeMulti, // multi-file: not affirmatively small
				FilesAffected: 4,
			},
			want:       Medium,
			wantReason: ReasonDefault,
		},
		{
			name: "evidence present but context-required stays MEDIUM",
			signals: router.Signals{
				JEVAvailable:    true,
				Risk:            router.LevelLow,
				Complexity:      router.LevelLow,
				Scope:           router.ScopeSingle,
				RequiresContext: true,
			},
			want:       Medium,
			wantReason: ReasonDefault,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reasons := Derive(tc.signals, tc.verifyFirst)
			if got != tc.want {
				t.Fatalf("Derive() tier = %q, want %q (reasons %v)", got, tc.want, reasons)
			}
			if len(reasons) != 1 || reasons[0] != tc.wantReason {
				t.Fatalf("Derive() reasons = %v, want [%q]", reasons, tc.wantReason)
			}
		})
	}
}

// TestDeriveIsDeterministic proves identical inputs always yield the identical tier
// and reasons.
func TestDeriveIsDeterministic(t *testing.T) {
	for _, s := range []router.Signals{{}, lowEvidence(), {Risk: router.LevelHigh}} {
		t1, r1 := Derive(s, false)
		t2, r2 := Derive(s, false)
		if t1 != t2 || !reflect.DeepEqual(r1, r2) {
			t.Fatalf("Derive not deterministic for %+v: (%q,%v) vs (%q,%v)", s, t1, r1, t2, r2)
		}
	}
}

// TestMaxIsEscalateOnly proves the merge can only raise a tier, never lower it.
func TestMaxIsEscalateOnly(t *testing.T) {
	cases := []struct{ a, b, want Tier }{
		{Low, Medium, Medium},
		{Medium, Low, Medium},
		{High, Low, High},
		{Low, High, High},
		{Medium, High, High},
		{High, High, High},
		{Low, Low, Low},
	}
	for _, tc := range cases {
		if got := Max(tc.a, tc.b); got != tc.want {
			t.Errorf("Max(%q,%q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
		if got := Max(tc.b, tc.a); got != tc.want {
			t.Errorf("Max(%q,%q) = %q, want %q (commutative)", tc.b, tc.a, got, tc.want)
		}
	}
}

// TestRequiresJEV proves JEV is required only at HIGH.
func TestRequiresJEV(t *testing.T) {
	cases := map[Tier]bool{Low: false, Medium: false, High: true}
	for tier, want := range cases {
		if got := tier.RequiresJEV(); got != want {
			t.Errorf("%q.RequiresJEV() = %v, want %v", tier, got, want)
		}
	}
}

// TestTierValid proves the tier set is closed.
func TestTierValid(t *testing.T) {
	for _, tier := range []Tier{Low, Medium, High} {
		if !tier.Valid() {
			t.Errorf("%q.Valid() = false, want true", tier)
		}
	}
	for _, tier := range []Tier{"", "low", "CRITICAL", "bogus"} {
		if tier.Valid() {
			t.Errorf("%q.Valid() = true, want false", tier)
		}
	}
}
