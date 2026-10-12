package cli

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
)

// TestDecisionEvidenceCapturesSanitizedRecord proves the SEAM-004 boundary records a
// sanitized evidence record for every CONSULTED outcome, that the record matches the
// actual provider result, and that a rejected or unavailable result never records a
// choice/confidence — so the record can never imply successful evidence capture.
func TestDecisionEvidenceCapturesSanitizedRecord(t *testing.T) {
	cls := autoFixClassification()

	cases := []struct {
		name        string
		dec         decision.Decision
		perr        error
		wantOutcome string
		wantChoice  string
	}{
		{"benign-low", decision.Decision{Choice: decision.Low, Confidence: 0.2}, nil, "benign", "LOW"},
		{"adverse-high", decision.Decision{Choice: decision.High, Confidence: 0.9}, nil, "accepted", "HIGH"},
		{"invalid", decision.Decision{Choice: "NOPE", Confidence: 0.5}, nil, "rejected", ""},
		{"unavailable", decision.Decision{}, decision.ErrProviderFailure, "unavailable", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d deps
			if tc.perr != nil {
				d = decisionEvidenceDeps(failProvider(tc.perr), nil)
			} else {
				d = decisionEvidenceDeps(&fakeDecisionProvider{dec: tc.dec}, nil)
			}
			var doc *decisionEvidenceDoc
			applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), d, benignSpec(),
				decisionEvidenceResult(cls), func(dd *decisionEvidenceDoc) { doc = dd })

			if doc == nil {
				t.Fatalf("no evidence record captured for %s", tc.name)
			}
			if doc.Version != decisionEvidenceVersion || doc.Contract != decisionEvidenceContract || !doc.Consulted {
				t.Fatalf("doc header = %+v, want versioned/consulted", doc)
			}
			if doc.OutcomeClass != tc.wantOutcome {
				t.Fatalf("outcome = %q, want %q (%+v)", doc.OutcomeClass, tc.wantOutcome, doc)
			}
			if doc.Choice != tc.wantChoice {
				t.Errorf("choice = %q, want %q", doc.Choice, tc.wantChoice)
			}
			if doc.BaselineAction == "" || doc.FinalAction == "" {
				t.Errorf("baseline/final governance not recorded: %+v", doc)
			}
			// A rejected/unavailable outcome must not carry a choice or confidence.
			if tc.wantChoice == "" && (doc.Choice != "" || doc.Confidence != 0) {
				t.Errorf("rejected/unavailable record leaked choice/confidence: %+v", doc)
			}
			// Monotonic governance: benign evidence never escalates; adverse evidence does.
			if tc.wantOutcome == "benign" && doc.Escalated {
				t.Errorf("benign evidence must not escalate: %+v", doc)
			}
			if tc.wantOutcome == "accepted" && (!doc.Escalated || doc.FinalAction == doc.BaselineAction) {
				t.Errorf("adverse evidence must escalate to a distinct boundary: %+v", doc)
			}
			// Credential-free, and free of raw model text, provider names, and paths.
			b, _ := json.Marshal(doc)
			low := strings.ToLower(string(b))
			for _, bad := range []string{"api_key", "apikey", "bearer", "password", "secret", "token", "http://", "https://", "/users/", "metadata"} {
				if strings.Contains(low, bad) {
					t.Errorf("evidence record contains %q: %s", bad, b)
				}
			}
		})
	}
}

// TestDecisionEvidenceCaptureIsSideEffectOnly proves observation is a SIDE EFFECT:
// the returned governed result is unchanged (the checkpoint stays a strict no-op for
// a caller that does not observe), and a disabled or absent checkpoint records
// nothing at all.
func TestDecisionEvidenceCaptureIsSideEffectOnly(t *testing.T) {
	in := decisionEvidenceResult(autoFixClassification())

	captured := false
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true),
		decisionEvidenceDeps(&fakeDecisionProvider{dec: decision.Decision{Choice: decision.Low, Confidence: 0.2}}, nil),
		benignSpec(), in, func(*decisionEvidenceDoc) { captured = true })
	if !captured {
		t.Error("a consulted benign checkpoint must record evidence")
	}
	if !reflect.DeepEqual(out, in) {
		t.Error("observing must not change the returned governed result")
	}

	for _, tc := range []struct {
		name string
		cfg  config.Config
		dep  deps
	}{
		{"disabled", decisionEvidenceConfig(false), decisionEvidenceDeps(&fakeDecisionProvider{dec: decision.Decision{Choice: decision.High, Confidence: 0.9}}, nil)},
		{"no-provider", decisionEvidenceConfig(true), decisionEvidenceDeps(nil, nil)},
	} {
		got := false
		applyDecisionEvidence(context.Background(), tc.cfg, tc.dep, benignSpec(),
			decisionEvidenceResult(autoFixClassification()), func(*decisionEvidenceDoc) { got = true })
		if got {
			t.Errorf("%s checkpoint must record nothing", tc.name)
		}
	}
}
