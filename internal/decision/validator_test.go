package decision

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func f64(v float64) *float64 { return &v }

func TestKnownChoice(t *testing.T) {
	for _, c := range []Choice{Low, Medium, High, Human} {
		if !c.Known() {
			t.Errorf("%q must be a known choice", c)
		}
	}
	for _, c := range []Choice{"", "APPROVE", "BLOCK", "low", "INDETERMINATE"} {
		if c.Known() {
			t.Errorf("%q must not be a known choice", c)
		}
	}
}

func TestValidConfidence(t *testing.T) {
	for _, v := range []float64{0, 0.5, 1} {
		if !ValidConfidence(v) {
			t.Errorf("%v must be a valid confidence", v)
		}
	}
	for _, v := range []float64{-0.1, 1.0001, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if ValidConfidence(v) {
			t.Errorf("%v must be an invalid confidence", v)
		}
	}
}

func TestValidateDecision(t *testing.T) {
	if err := Validate(Decision{Choice: Low, Confidence: 0.9}); err != nil {
		t.Fatalf("valid decision rejected: %v", err)
	}
	if err := Validate(Decision{Choice: "APPROVE", Confidence: 0.9}); !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("unknown choice must fail closed: %v", err)
	}
	if err := Validate(Decision{Choice: Low, Confidence: 1.5}); !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("invalid confidence must fail closed: %v", err)
	}
}

func TestValidateResult(t *testing.T) {
	req := Request{UseCase: "implementation-risk", Choices: []Choice{Low, Medium, High}}
	for _, tc := range []struct {
		name string
		res  Result
		want error
	}{
		{"valid ok", Result{ContractVersion: 1, Status: StatusOK, Kind: "implementation-risk", Choice: Medium, Confidence: f64(0.8)}, nil},
		{"ok without kind echo is allowed", Result{ContractVersion: 1, Status: StatusOK, Choice: Low, Confidence: f64(0.9)}, nil},
		{"unsupported contract version", Result{ContractVersion: 2, Status: StatusOK, Choice: Low, Confidence: f64(0.9)}, ErrInvalidResult},
		{"unknown status", Result{ContractVersion: 1, Status: "MAYBE", Choice: Low, Confidence: f64(0.9)}, ErrInvalidResult},
		{"unknown choice", Result{ContractVersion: 1, Status: StatusOK, Choice: "APPROVE", Confidence: f64(0.9)}, ErrInvalidResult},
		{"choice outside allowed set", Result{ContractVersion: 1, Status: StatusOK, Choice: Human, Confidence: f64(0.9)}, ErrInvalidResult},
		{"invalid confidence high", Result{ContractVersion: 1, Status: StatusOK, Choice: Low, Confidence: f64(1.5)}, ErrInvalidResult},
		{"invalid confidence nan", Result{ContractVersion: 1, Status: StatusOK, Choice: Low, Confidence: f64(math.NaN())}, ErrInvalidResult},
		{"missing confidence is indeterminate", Result{ContractVersion: 1, Status: StatusOK, Choice: Low, Confidence: nil}, ErrIndeterminate},
		{"ok with error is inconsistent", Result{ContractVersion: 1, Status: StatusOK, Choice: Low, Confidence: f64(0.9), Error: "boom"}, ErrInvalidResult},
		{"kind mismatch", Result{ContractVersion: 1, Status: StatusOK, Kind: "other", Choice: Low, Confidence: f64(0.9)}, ErrInvalidResult},
		{"unsupported capability", Result{ContractVersion: 1, Status: StatusUnsupported, Error: "nope"}, ErrUnsupportedCapability},
		{"provider error", Result{ContractVersion: 1, Status: StatusError, Error: "boom"}, ErrProviderResult},
		{"indeterminate", Result{ContractVersion: 1, Status: StatusIndeterminate}, ErrIndeterminate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := req.ValidateResult(tc.res)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

// TestAllowedChoiceNilPermitsAnyKnownChoice proves that when a request declares
// no allowed set, any SOP-recognized choice is accepted (allowed-choice
// membership is only enforced when the request declares one).
func TestAllowedChoiceNilPermitsAnyKnownChoice(t *testing.T) {
	req := Request{UseCase: "k"}
	if err := req.ValidateResult(Result{ContractVersion: 1, Status: StatusOK, Choice: Human, Confidence: f64(0.9)}); err != nil {
		t.Fatalf("a known choice with no allowed set must be accepted: %v", err)
	}
}

// TestDeterministicProviderSatisfiesContract proves the in-process deterministic
// provider returns evidence that passes the SOP-owned validator.
func TestDeterministicProviderSatisfiesContract(t *testing.T) {
	p := DeterministicProvider{}
	d, err := p.Decide(nil, Request{UseCase: "k", Subject: "small change", Signals: map[string]float64{"files": 1}})
	if err != nil {
		t.Fatalf("deterministic provider errored: %v", err)
	}
	if err := Validate(d); err != nil {
		t.Fatalf("deterministic provider output failed validation: %v", err)
	}
	if err := (Request{UseCase: "k"}).ValidateResult(Result{
		ContractVersion: ContractVersion, Status: StatusOK, Choice: d.Choice, Confidence: &d.Confidence,
	}); err != nil {
		t.Fatalf("deterministic provider output rejected by ValidateResult: %v", err)
	}
}

// TestDecisionCarriesNoLifecycleAuthority proves structurally that a decision
// (provider evidence) has no field that could carry a SOP lifecycle action,
// approval, or authorization.
func TestDecisionCarriesNoLifecycleAuthority(t *testing.T) {
	want := map[string]bool{"Choice": true, "Confidence": true, "Metadata": true}
	typ := reflect.TypeOf(Decision{})
	got := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		got[typ.Field(i).Name] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Decision fields = %v, want %v (a decision must carry evidence only)", got, want)
	}
}
