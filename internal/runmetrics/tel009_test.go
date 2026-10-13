package runmetrics

import "testing"

// TestEscalationCountsModelEscalationOnly proves decision_escalation_rate counts
// model-class escalation (routing source "escalation" or an escalated retry under
// attempts/) and never a human approval. A NEEDS_HUMAN run with no routing or
// attempt evidence is not in the denominator and not counted; human approvals are
// reported separately by human_approval_frequency.
func TestEscalationCountsModelEscalationOnly(t *testing.T) {
	agg := fixture(t, "escalation")
	r := findDecision(t, agg, MetricEscalation)
	if r.Numerator != 2 || r.Denominator != 2 || r.Value != "100.0" {
		t.Errorf("escalation = %+v, want num=2 den=2 value=100.0 (routing escalation + escalated retry)", r)
	}
	// RUN-E3 records NEEDS_HUMAN but no routing/attempt evidence: it is excluded.
	if r.ArtifactsPresent != 2 {
		t.Errorf("escalation artifacts_present = %d, want 2 (the NEEDS_HUMAN run is not escalation evidence)", r.ArtifactsPresent)
	}
}

// TestEscalationUnavailableWithoutEvidence proves a project with no routing or
// attempt evidence reports escalation UNAVAILABLE, never a zero rate.
func TestEscalationUnavailableWithoutEvidence(t *testing.T) {
	agg := fixture(t, "present") // RUN-A has report.json but no routing.json or attempts/.
	r := findDecision(t, agg, MetricEscalation)
	if r.Value != UNAVAILABLE || r.Numerator != 0 || r.Denominator != 0 {
		t.Errorf("escalation = %+v, want UNAVAILABLE with no fraction", r)
	}
}

// TestOverrideProvenanceReconciled proves decision_override_rate counts a manual
// override recorded in EITHER artifacts: routing.json source "manual_override" or
// model-selection.json source "cli". RUN-O1 records the override only in routing.json
// (its selection says "env"); RUN-O2 records it only in the selection ("cli"). Both
// count, so the numerator is 2 over the 2 runs that recorded provenance.
func TestOverrideProvenanceReconciled(t *testing.T) {
	agg := fixture(t, "override")
	r := findDecision(t, agg, MetricOverride)
	if r.Numerator != 2 || r.Denominator != 2 || r.Value != "100.0" {
		t.Errorf("override = %+v, want num=2 den=2 value=100.0 (routing.json and model-selection provenance)", r)
	}
}

// TestModelSelectionVersionRejected proves an unknown model-selection schema version
// is present but UNAVAILABLE, never interpreted; a run with no version (historical)
// is accepted as the initial version.
func TestModelSelectionVersionRejected(t *testing.T) {
	agg := fixture(t, "mselver")
	tier := findDecision(t, agg, MetricModelTier)
	if tier.ArtifactsPresent != 1 {
		t.Errorf("model tier artifacts_present = %d, want 1 (file present)", tier.ArtifactsPresent)
	}
	if tier.ValidMeasurements != 0 || tier.Value != UNAVAILABLE {
		t.Errorf("model tier = %+v, want valid=0 value=UNAVAILABLE (unknown schema version)", tier)
	}
	// The historical no-version fixture is still measured.
	if got := findDecision(t, fixture(t, "decisions"), MetricModelTier); got.Value == UNAVAILABLE {
		t.Errorf("historical no-version model-selection must stay measured: %+v", got)
	}
}

// TestRetryEfficiencyIsRate proves retry_efficiency reports a rate (a percentage of
// eligible runs), not a bare count, and its unit is a percentage.
func TestRetryEfficiencyIsRate(t *testing.T) {
	agg := fixture(t, "present")
	r := findMetric(t, agg, MetricRetry)
	if r.Unit != "%" {
		t.Errorf("retry unit = %q, want %%", r.Unit)
	}
	if r.Denominator <= 0 {
		t.Fatalf("retry = %+v, want a positive denominator", r)
	}
	if r.Numerator == r.Denominator && r.Value == "100.0" {
		// The known corpus shape: every eligible run recorded a retry signal.
		return
	}
	if r.Value == UNAVAILABLE {
		t.Errorf("retry = %+v, want a measured rate", r)
	}
}
