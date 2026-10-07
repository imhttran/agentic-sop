package decision

import (
	"errors"
	"math"
	"testing"
)

// SEAM-005 validator hardening. The validator is the SOP-owned fail-closed boundary
// between an untrusted provider result and SOP policy. These tests pin the cases
// SEAM-004's live path now depends on: enum/status, allowed-choice, confidence
// bounds, required-field combinations, contradictory fields, and protocol version.

// TestValidateResultStatusVocabularyIsClosed proves the status vocabulary is exact
// and case-sensitive: any near-miss is an unknown status and fails closed.
func TestValidateResultStatusVocabularyIsClosed(t *testing.T) {
	req := Request{UseCase: "k", Choices: []Choice{Low, Medium, High, Human}}
	for _, status := range []Status{StatusOK, StatusUnsupported, StatusError, StatusIndeterminate} {
		if !status.Valid() {
			t.Errorf("%q must be a valid status", status)
		}
	}
	for _, status := range []Status{"", "ok", "Ok", "OK ", "MAYBE", "PASS", "APPROVE", "TIMEOUT"} {
		if status.Valid() {
			t.Errorf("%q must not be a valid status", status)
		}
		res := Result{ContractVersion: ContractVersion, Status: status, Choice: Low, Confidence: f64(0.9)}
		if err := req.ValidateResult(res); !errors.Is(err, ErrInvalidResult) {
			t.Errorf("unknown status %q must fail closed, got %v", status, err)
		}
	}
}

// TestValidateResultContractVersionIsExact proves a mismatched or missing protocol
// version fails closed.
func TestValidateResultContractVersionIsExact(t *testing.T) {
	req := Request{UseCase: "k"}
	for _, v := range []int{0, -1, ContractVersion + 1, 99} {
		res := Result{ContractVersion: v, Status: StatusOK, Choice: Low, Confidence: f64(0.9)}
		if err := req.ValidateResult(res); !errors.Is(err, ErrInvalidResult) {
			t.Errorf("contract version %d must fail closed, got %v", v, err)
		}
	}
}

// TestValidateResultConfidenceBounds proves every non-finite or out-of-range
// confidence fails closed, and only a finite value in [0,1] is accepted.
func TestValidateResultConfidenceBounds(t *testing.T) {
	req := Request{UseCase: "k"}
	for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN(), -0.0001, 1.0001, 2} {
		res := Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: Low, Confidence: f64(v)}
		if err := req.ValidateResult(res); !errors.Is(err, ErrInvalidResult) {
			t.Errorf("confidence %v must fail closed, got %v", v, err)
		}
	}
	for _, v := range []float64{0, 0.5, 1} {
		res := Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: Low, Confidence: f64(v)}
		if err := req.ValidateResult(res); err != nil {
			t.Errorf("confidence %v must be accepted, got %v", v, err)
		}
	}
}

// TestValidateResultRequiredFields proves a success status missing a required field
// fails closed: no choice, and no confidence, are each unusable evidence.
func TestValidateResultRequiredFields(t *testing.T) {
	req := Request{UseCase: "k"}
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Confidence: f64(0.9)}); !errors.Is(err, ErrInvalidResult) {
		t.Errorf("OK without a choice must fail closed, got %v", err)
	}
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: Low}); !errors.Is(err, ErrIndeterminate) {
		t.Errorf("OK without a confidence must be indeterminate, got %v", err)
	}
}

// TestValidateResultContradictoryFields proves contradictory combinations never
// become a success: a success status carrying an error, and every non-OK status
// even when it also carries a plausible choice and confidence.
func TestValidateResultContradictoryFields(t *testing.T) {
	req := Request{UseCase: "k", Choices: []Choice{Low, Medium, High, Human}}
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: Low, Confidence: f64(0.9), Error: "boom"}); !errors.Is(err, ErrInvalidResult) {
		t.Errorf("OK with an error must fail closed, got %v", err)
	}
	for _, tc := range []struct {
		status Status
		want   error
	}{
		{StatusUnsupported, ErrUnsupportedCapability},
		{StatusError, ErrProviderResult},
		{StatusIndeterminate, ErrIndeterminate},
	} {
		// A hostile provider may attach a plausible choice/confidence to a non-OK
		// status; it must still never be a usable decision.
		res := Result{ContractVersion: ContractVersion, Status: tc.status, Choice: High, Confidence: f64(1.0), Error: "x"}
		if err := req.ValidateResult(res); !errors.Is(err, tc.want) {
			t.Errorf("status %q with a success payload must fail as %v, got %v", tc.status, tc.want, err)
		}
	}
}

// TestValidateResultNonOKNeverSucceeds is the property form: for every non-OK
// status, the validator never returns nil, regardless of the payload.
func TestValidateResultNonOKNeverSucceeds(t *testing.T) {
	req := Request{UseCase: "k", Choices: []Choice{Low, Medium, High, Human}}
	for _, status := range []Status{StatusUnsupported, StatusError, StatusIndeterminate} {
		for _, choice := range []Choice{"", Low, High, Human, "APPROVE"} {
			for _, conf := range []*float64{nil, f64(0), f64(0.9), f64(1.0), f64(math.NaN())} {
				res := Result{ContractVersion: ContractVersion, Status: status, Choice: choice, Confidence: conf}
				if err := req.ValidateResult(res); err == nil {
					t.Fatalf("non-OK status %q produced a usable decision: %+v", status, res)
				}
			}
		}
	}
}

// TestValidateResultAllowedChoiceMembership proves a known-but-not-allowed choice is
// rejected when the request declares an allowed set, and that membership is exact
// (case-sensitive).
func TestValidateResultAllowedChoiceMembership(t *testing.T) {
	req := Request{UseCase: "k", Choices: []Choice{Low, Medium}}
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: High, Confidence: f64(0.9)}); !errors.Is(err, ErrInvalidResult) {
		t.Errorf("a choice outside the allowed set must fail closed, got %v", err)
	}
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: Medium, Confidence: f64(0.9)}); err != nil {
		t.Errorf("an allowed choice must be accepted, got %v", err)
	}
	// A non-member value that is not even a known choice is still rejected.
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: "low", Confidence: f64(0.9)}); !errors.Is(err, ErrInvalidResult) {
		t.Errorf("an unknown choice must fail closed, got %v", err)
	}
}

// TestValidateDecisionConfidenceBounds proves the in-process Validate boundary also
// rejects non-finite confidence.
func TestValidateDecisionConfidenceBounds(t *testing.T) {
	for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN(), -0.1, 1.1} {
		if err := Validate(Decision{Choice: Low, Confidence: v}); !errors.Is(err, ErrInvalidResult) {
			t.Errorf("Validate must reject confidence %v, got %v", v, err)
		}
	}
	if err := Validate(Decision{Choice: Human, Confidence: 1.0}); err != nil {
		t.Errorf("a valid decision must pass: %v", err)
	}
}

// TestValidateResultKindEchoMismatch proves a kind echo that does not match the
// request is rejected, while an empty echo (kind is optional) is accepted.
func TestValidateResultKindEchoMismatch(t *testing.T) {
	req := Request{UseCase: "implementation-risk"}
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Kind: "other", Choice: Low, Confidence: f64(0.9)}); !errors.Is(err, ErrInvalidResult) {
		t.Errorf("a mismatched kind echo must fail closed, got %v", err)
	}
	if err := req.ValidateResult(Result{ContractVersion: ContractVersion, Status: StatusOK, Choice: Low, Confidence: f64(0.9)}); err != nil {
		t.Errorf("an absent kind echo must be accepted, got %v", err)
	}
}
