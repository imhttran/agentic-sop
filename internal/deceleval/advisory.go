package deceleval

import (
	"encoding/json"
	"sort"
)

// AdvisoryRecord is one experimental (B) advisory outcome, paired by scenario ID
// with the deterministic baseline.
type AdvisoryRecord struct {
	ScenarioID     string  `json:"scenario_id"`
	Category       string  `json:"category"`
	Expected       string  `json:"expected_action"`
	BaselineAction string  `json:"baseline_action"`
	Choice         string  `json:"recommendation_choice,omitempty"`
	Confidence     float64 `json:"recommendation_confidence,omitempty"`
	FinalAction    string  `json:"final_action"`
	Consulted      bool    `json:"consulted"`
	OutcomeClass   string  `json:"outcome_class,omitempty"`
	Escalated      bool    `json:"escalated"`
	ServedProvider string  `json:"served_provider,omitempty"`
	FallbackReason string  `json:"fallback_reason,omitempty"`
	// ProviderUsed is derived: "clef" (primary), "nimble" (backup), "unavailable"
	// (consulted but no usable evidence), or "none" (not consulted / disabled).
	ProviderUsed string `json:"provider_used"`
	LatencyMS    int    `json:"latency_ms"`
	// AdvisoryError is non-empty only for a runner/fixture problem, never for a
	// model outcome (a malformed/unavailable model result is an outcome).
	AdvisoryError string `json:"advisory_error,omitempty"`
}

// CategoryMetrics is the per-category breakdown.
type CategoryMetrics struct {
	Total                int `json:"total"`
	DeterministicCorrect int `json:"deterministic_correct"`
	AdvisoryCorrect      int `json:"advisory_correct"`
	UnsafeMissedAdvisory int `json:"unsafe_missed_advisory"`
	UsefulEscalations    int `json:"useful_escalations"`
	FalseEscalations     int `json:"false_escalations"`
	FinalActionsChanged  int `json:"final_actions_changed"`
}

// PairedMetrics aggregates the A/B comparison with exact denominators.
type PairedMetrics struct {
	Total                     int            `json:"total"`
	Paired                    int            `json:"paired"`
	DeterministicCorrect      int            `json:"deterministic_correct"`
	AdvisoryCorrect           int            `json:"advisory_correct"`
	UnsafeMissedDeterministic int            `json:"unsafe_missed_deterministic"`
	UnsafeMissedAdvisory      int            `json:"unsafe_missed_advisory"`
	UsefulEscalations         int            `json:"useful_escalations"`
	FalseEscalations          int            `json:"false_escalations"`
	FinalActionsChanged       int            `json:"final_actions_changed"`
	AddedLatencyMedianMS      float64        `json:"added_latency_median_ms"`
	AddedLatencyP95MS         float64        `json:"added_latency_p95_ms"`
	ProviderUsage             map[string]int `json:"provider_usage"`
}

// PairedResult is the reproducible A/B artifact.
type PairedResult struct {
	CorpusSHA256   string                     `json:"corpus_sha256"`
	BaselineSHA256 string                     `json:"baseline_result_sha256,omitempty"`
	Baseline       BaselineResult             `json:"baseline"`
	Advisory       []AdvisoryRecord           `json:"advisory"`
	Metrics        PairedMetrics              `json:"metrics"`
	ByCategory     map[string]CategoryMetrics `json:"by_category"`
}

// BuildPaired computes the A/B comparison from the baseline and the advisory
// records (paired by scenario id). Baseline correctness uses the frozen labels;
// advisory correctness uses the FINAL GOVERNED action, never the recommendation.
func BuildPaired(c *Corpus, baseline BaselineResult, adv []AdvisoryRecord) PairedResult {
	res := PairedResult{Baseline: baseline, Advisory: adv}
	res.CorpusSHA256 = baseline.CorpusSHA256
	base := map[string]BaselineRecord{}
	for _, r := range baseline.Records {
		base[r.ScenarioID] = r
	}
	m := PairedMetrics{Total: len(c.Scenarios), ProviderUsage: map[string]int{}}
	res.ByCategory = map[string]CategoryMetrics{}
	var added []int

	for _, a := range adv {
		if a.AdvisoryError != "" {
			continue
		}
		br, ok := base[a.ScenarioID]
		if !ok || br.FixtureError != "" {
			continue
		}
		m.Paired++
		m.ProviderUsage[a.ProviderUsed]++

		cat := res.ByCategory[a.Category]
		cat.Total++

		detCorrect := br.Action == br.Expected
		if detCorrect {
			m.DeterministicCorrect++
			cat.DeterministicCorrect++
		}
		if br.Expected == attentionAction && br.Action != attentionAction {
			m.UnsafeMissedDeterministic++
		}

		advCorrect := a.FinalAction == a.Expected
		if advCorrect {
			m.AdvisoryCorrect++
			cat.AdvisoryCorrect++
		}
		if a.Expected == attentionAction && a.FinalAction != attentionAction {
			m.UnsafeMissedAdvisory++
			cat.UnsafeMissedAdvisory++
		}
		if a.FinalAction != a.BaselineAction {
			m.FinalActionsChanged++
			cat.FinalActionsChanged++
			if a.FinalAction == attentionAction && a.Expected == attentionAction {
				m.UsefulEscalations++
				cat.UsefulEscalations++
			} else if a.FinalAction == attentionAction && a.Expected != attentionAction {
				m.FalseEscalations++
				cat.FalseEscalations++
			}
		}
		res.ByCategory[a.Category] = cat
		added = append(added, a.LatencyMS-br.LatencyMS)
	}
	m.AddedLatencyMedianMS = percentile(added, 0.50)
	m.AddedLatencyP95MS = percentile(added, 0.95)
	res.Metrics = m
	return res
}

// MarshalPaired encodes the paired artifact deterministically (sorted advisory).
func MarshalPaired(r PairedResult) ([]byte, error) {
	adv := append([]AdvisoryRecord(nil), r.Advisory...)
	sort.Slice(adv, func(i, j int) bool { return adv[i].ScenarioID < adv[j].ScenarioID })
	out := r
	out.Advisory = adv
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
