package outcome

import (
	"errors"
	"testing"
)

func ok() error   { return nil }
func fail() error { return errors.New("not satisfied") }

func TestAllRequiredMetIsVerified(t *testing.T) {
	res := Verify([]Criterion{
		{ID: "a", Required: true, Check: ok},
		{ID: "b", Required: true, Check: ok},
		{ID: "c", Boundary: true, Check: ok},
	})
	if res.Status != Verified {
		t.Fatalf("status = %s, want VERIFIED; reasons=%v", res.Status, res.Reasons)
	}
}

func TestUnmetRequiredIsPartial(t *testing.T) {
	res := Verify([]Criterion{
		{ID: "a", Required: true, Check: ok},
		{ID: "b", Required: true, Check: fail},
	})
	if res.Status != Partial {
		t.Fatalf("status = %s, want PARTIAL; reasons=%v", res.Status, res.Reasons)
	}
}

func TestBoundaryViolationIsHold(t *testing.T) {
	res := Verify([]Criterion{
		{ID: "a", Required: true, Check: ok},
		{ID: "protected", Boundary: true, Check: fail},
	})
	if res.Status != Hold {
		t.Fatalf("status = %s, want HOLD; reasons=%v", res.Status, res.Reasons)
	}
}

// TestNilCheckCannotBeSatisfiedByAClaim proves an unverifiable criterion counts as
// unmet: no model claim can turn it into VERIFIED.
func TestNilCheckCannotBeSatisfiedByAClaim(t *testing.T) {
	res := Verify([]Criterion{{ID: "claimed", Required: true}})
	if res.Status != Partial {
		t.Fatalf("status = %s, want PARTIAL", res.Status)
	}
	if res.Criteria[0].Met {
		t.Fatal("a nil check must not be reported as met")
	}
}

func TestNoRequiredCriterionIsPartial(t *testing.T) {
	res := Verify([]Criterion{{ID: "optional", Check: ok}})
	if res.Status != Partial {
		t.Fatalf("status = %s, want PARTIAL", res.Status)
	}
}
