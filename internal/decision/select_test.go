package decision

import "testing"

// TestSelectClearWinner: a strong, low-risk, reversible approach beats a partial,
// high-risk, irreversible one and routes to the small model (routine).
func TestSelectClearWinner(t *testing.T) {
	sel := Select([]Alternative{
		{Name: "rewrite", Evidence: EvidencePartial, Risk: RiskHigh, Reversible: false, Cost: 1},
		{Name: "refactor", Evidence: EvidenceStrong, Risk: RiskLow, Reversible: true, Cost: 3},
	})
	if sel.Approach != "refactor" {
		t.Fatalf("approach=%q, want refactor", sel.Approach)
	}
	if sel.Choice != Low || sel.Target != SmallModel {
		t.Fatalf("choice=%s target=%s, want LOW/SMALL_MODEL", sel.Choice, sel.Target)
	}
}

// TestSelectClosePrefersReversible: when the leading alternatives are close (same
// evidence), the safest reversible one wins even though the other is cheaper.
func TestSelectClosePrefersReversible(t *testing.T) {
	sel := Select([]Alternative{
		{Name: "fast", Evidence: EvidenceStrong, Risk: RiskLow, Reversible: false, Cost: 0.1},
		{Name: "safe", Evidence: EvidenceStrong, Risk: RiskMedium, Reversible: true, Cost: 1.0},
	})
	if sel.Approach != "safe" {
		t.Fatalf("approach=%q, want safe (safest reversible)", sel.Approach)
	}
}

// TestSelectInsufficientEvidenceEscalates: with no deterministic support, SOP
// escalates rather than guessing.
func TestSelectInsufficientEvidenceEscalates(t *testing.T) {
	sel := Select([]Alternative{
		{Name: "a", Evidence: EvidenceNone, Risk: RiskLow, Reversible: true},
		{Name: "b", Evidence: EvidenceNone, Risk: RiskLow, Reversible: true},
	})
	if sel.Choice != Human || sel.Target != HumanTarget {
		t.Fatalf("choice=%s target=%s, want HUMAN/HUMAN", sel.Choice, sel.Target)
	}
}

// TestSelectAuthorityBoundaryEscalates: an approach that crosses an authority
// boundary is never auto-selected.
func TestSelectAuthorityBoundaryEscalates(t *testing.T) {
	sel := Select([]Alternative{
		{Name: "drop-db", Evidence: EvidenceStrong, Risk: RiskIrreversible, Boundary: BoundarySecurity},
	})
	if sel.Choice != Human || sel.Target != HumanTarget {
		t.Fatalf("choice=%s target=%s, want HUMAN/HUMAN", sel.Choice, sel.Target)
	}
	if sel.Approach != "" {
		t.Fatalf("approach=%q, want empty", sel.Approach)
	}
}

// TestSelectBoundaryExcludedWhenSafeExists: a boundary approach is excluded, and a
// routine alternative is selected instead.
func TestSelectBoundaryExcludedWhenSafeExists(t *testing.T) {
	sel := Select([]Alternative{
		{Name: "scope-creep", Evidence: EvidenceStrong, Risk: RiskLow, Reversible: true, Boundary: BoundaryScope},
		{Name: "minimal", Evidence: EvidenceStrong, Risk: RiskLow, Reversible: true},
	})
	if sel.Approach != "minimal" {
		t.Fatalf("approach=%q, want minimal", sel.Approach)
	}
}
