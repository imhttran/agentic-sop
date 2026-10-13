package deceleval

// Failure-recovery advisory question (DME follow-up experiment).
//
// The production SEAM-004 checkpoint asks a change-RISK question (LOW/MEDIUM/HIGH),
// which the local models answer HIGH almost always, escalating benign failures.
// This experiment asks a failure-RECOVERY question with three outcomes that map
// onto the three recovery categories:
//
//	AUTO_FIX : safe deterministic recovery (the fix loop / retry can repair it)
//	CONTINUE : continued automated investigation (productive but unfinished work)
//	HUMAN    : genuine human intervention required
//
// It is isolated and reversible tooling: it does NOT change the production
// question, the bridge profile, governance, or the escalation thresholds. The
// experiment applies the SAME add-attention semantics (a HUMAN recommendation
// escalates; other recommendations add nothing) so results are comparable.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// RecoveryKind is the experiment's question kind.
const RecoveryKind = "failure-recovery"

// Recovery choices, in order.
const (
	RecoveryAutoFix  = "AUTO_FIX"
	RecoveryContinue = "CONTINUE"
	RecoveryHuman    = "HUMAN"
)

// RecoveryChoices is the closed choice set for the recovery question.
var RecoveryChoices = []string{RecoveryAutoFix, RecoveryContinue, RecoveryHuman}

// recoveryRequest / recoveryQuestion are the ADAPTER's decision-contract wire
// shapes (sop-decision-adapters/decision), sent directly to the adapter for this
// isolated experiment. They are not SOP policy types.
type recoveryRequest struct {
	State     string             `json:"state"`
	Questions []recoveryQuestion `json:"questions"`
}

type recoveryQuestion struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Criteria string   `json:"criteria"`
	Choices  []string `json:"choices"`
}

// BuildRecoveryRequest builds the failure-recovery question for one scenario. The
// scenario's observable failure text is the question's state/criteria.
func BuildRecoveryRequest(s Scenario) recoveryRequest {
	return recoveryRequest{
		State: s.Observable,
		Questions: []recoveryQuestion{{
			ID:       RecoveryKind,
			Type:     "choice",
			Criteria: "Classify how SOP should recover from this failure: safe deterministic recovery (AUTO_FIX), continued automated investigation (CONTINUE), or genuine human intervention (HUMAN).",
			Choices:  append([]string(nil), RecoveryChoices...),
		}},
	}
}

// MarshalRecoveryRequest encodes the recovery-question wire bytes to send to the
// adapter.
func MarshalRecoveryRequest(s Scenario) ([]byte, error) {
	return json.Marshal(BuildRecoveryRequest(s))
}

// RecoveryRecommendationAction maps the model's recovery choice to the failure
// disposition vocabulary. An empty/unknown choice yields "" (no recommendation).
func RecoveryRecommendationAction(choice string) string {
	switch choice {
	case RecoveryAutoFix:
		return "AUTO_FIX"
	case RecoveryContinue:
		return "CONTINUE"
	case RecoveryHuman:
		return "NEEDS_HUMAN"
	default:
		return ""
	}
}

// RecoveryChoiceValid reports whether choice is one of the closed recovery set.
func RecoveryChoiceValid(choice string) bool {
	for _, c := range RecoveryChoices {
		if c == choice {
			return true
		}
	}
	return false
}

// CanonicalToNativeChoice maps a canonical decision choice (returned through the
// real advisory path, which reduces the native recovery choice onto the canonical
// choice set) back to the experiment's native recovery choice. The mapping is an
// exact inverse of the eval-only registry's choice map.
func CanonicalToNativeChoice(canonical string) string {
	switch canonical {
	case "LOW":
		return RecoveryAutoFix
	case "MEDIUM":
		return RecoveryContinue
	case "HUMAN":
		return RecoveryHuman
	default:
		return ""
	}
}

// RecoveryPrompt is the frozen evaluation prompt definition for the
// failure-recovery experiment: the question kind, the adapter-facing choices, and
// the eval-only choice map onto canonical choices. The per-scenario subject (the
// failure observable) is data, not part of the frozen prompt.
type RecoveryPrompt struct {
	Kind      string            `json:"kind"`
	Choices   []string          `json:"choices"`
	ChoiceMap map[string]string `json:"choice_map"`
}

// RecoveryPromptSpec returns the canonical prompt definition.
func RecoveryPromptSpec() RecoveryPrompt {
	return RecoveryPrompt{
		Kind:    RecoveryKind,
		Choices: append([]string(nil), RecoveryChoices...),
		// AUTO_FIX/CONTINUE are benign canonical choices; only HUMAN is adverse.
		ChoiceMap: map[string]string{"AUTO_FIX": "LOW", "CONTINUE": "MEDIUM", "HUMAN": "HUMAN"},
	}
}

// RecoveryPromptSHA256 is the freeze anchor for the selected prompt: a stable
// sha256 of the canonical JSON of RecoveryPromptSpec.
func RecoveryPromptSHA256() string {
	data, _ := json.Marshal(RecoveryPromptSpec())
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// adapterAnswer / adapterResult mirror the adapter's normalized DecisionResult
// wire shape (sop-decision-adapters/decision). Only the fields this experiment
// reads are declared.
type adapterAnswer struct {
	Type       string   `json:"type"`
	Choice     string   `json:"choice"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type adapterResult struct {
	Provider    string                   `json:"provider"`
	Model       string                   `json:"model,omitempty"`
	Answers     map[string]adapterAnswer `json:"answers"`
	Diagnostics map[string]string        `json:"diagnostics,omitempty"`
}

// RecoveryAnswer is the parsed recovery decision from the adapter.
type RecoveryAnswer struct {
	Choice         string
	Confidence     *float64
	ServedProvider string
	Model          string
	FallbackReason string
}

// ParseRecoveryResult parses the adapter's DecisionResult JSON for the recovery
// question. The serving backend is taken from the fallback diagnostics when the
// backup served (the composite masks its provider name to the profile), else
// from the result's provider. Any incomplete or non-conforming response is an
// error, which the runner records as an unavailable/malformed OUTCOME, never as
// a decision.
func ParseRecoveryResult(data []byte) (RecoveryAnswer, error) {
	var res adapterResult
	if err := json.Unmarshal(data, &res); err != nil {
		return RecoveryAnswer{}, fmt.Errorf("decode adapter result: %w", err)
	}
	ans, ok := res.Answers[RecoveryKind]
	if !ok {
		return RecoveryAnswer{}, fmt.Errorf("adapter result omitted question %q", RecoveryKind)
	}
	if !RecoveryChoiceValid(ans.Choice) {
		return RecoveryAnswer{}, fmt.Errorf("adapter returned unknown choice %q", ans.Choice)
	}
	served := ""
	if res.Diagnostics != nil {
		served = res.Diagnostics["served_provider"]
	}
	if served == "" {
		served = res.Provider
	}
	fallback := ""
	if res.Diagnostics != nil {
		fallback = res.Diagnostics["fallback"]
	}
	return RecoveryAnswer{
		Choice:         ans.Choice,
		Confidence:     ans.Confidence,
		ServedProvider: served,
		Model:          res.Model,
		FallbackReason: fallback,
	}, nil
}

// RecoveryObservation is one scenario's model answer for the recovery question.
type RecoveryObservation struct {
	ScenarioID string   `json:"scenario_id"`
	Category   string   `json:"category"`
	Split      string   `json:"split"`
	Expected   string   `json:"expected_action"`
	Choice     string   `json:"choice,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	// Recommendation is the mapped action ("" when no usable choice was returned).
	Recommendation string `json:"recommendation_action,omitempty"`
	// FinalAction is the add-attention-clamped governed action (a HUMAN
	// recommendation escalates to NEEDS_HUMAN; any other recommendation keeps the
	// deterministic baseline).
	FinalAction    string `json:"final_action"`
	BaselineAction string `json:"baseline_action"`
	// Consulted is true only when a usable choice was returned.
	Consulted bool `json:"consulted"`
	// OutcomeClass classifies the consultation: "decision", "malformed"
	// (non-conforming response), "unavailable" (adapter error), or "runner_error"
	// (a fixture/runner problem, never a model outcome).
	OutcomeClass   string `json:"outcome_class,omitempty"`
	ServedProvider string `json:"served_provider,omitempty"`
	Model          string `json:"model,omitempty"`
	FallbackReason string `json:"fallback_reason,omitempty"`
	LatencyMS      int    `json:"latency_ms"`
	// Detail is non-secret diagnostic text for an unavailable/malformed outcome.
	Detail string `json:"detail,omitempty"`
	// Error is non-empty only for a runner/fixture problem, never a model outcome.
	Error string `json:"error,omitempty"`
}

// RecoverySplitMetrics is the per-split comparison.
type RecoverySplitMetrics struct {
	N                 int `json:"n"`
	Consulted         int `json:"consulted"`
	BaselineCorrect   int `json:"baseline_correct"`
	RecommendCorrect  int `json:"recommendation_correct"`
	FinalCorrect      int `json:"final_correct"`
	UnsafeMissed      int `json:"unsafe_missed"`
	UsefulEscalations int `json:"useful_escalations"`
	FalseEscalations  int `json:"false_escalations"`
	NoRecommendation  int `json:"no_recommendation"`
	// OperationalAgree counts consulted decisions whose "needs human?" answer
	// matches the label's, i.e. (recommendation==NEEDS_HUMAN)==(expected==NEEDS_HUMAN).
	// It is the coarser operational question, distinct from an exact choice match.
	OperationalAgree int `json:"operational_agree"`
	// NotEligible counts scenarios the real advisory gate did not consult because
	// the deterministic baseline already withholds execution. A not-eligible
	// scenario is NOT missing data and NOT an unavailable provider.
	NotEligible   int            `json:"not_eligible"`
	ByChoice      map[string]int `json:"by_choice"`
	ProviderUsage map[string]int `json:"provider_usage"`
	// Latency is measured over consulted decisions only.
	LatencyMedianMS float64 `json:"latency_median_ms"`
	LatencyP95MS    float64 `json:"latency_p95_ms"`
}

// RecoveryResult is the experiment artifact.
type RecoveryResult struct {
	CorpusSHA256 string                          `json:"corpus_sha256"`
	PromptSHA256 string                          `json:"prompt_sha256"`
	Records      []RecoveryObservation           `json:"records"`
	BySplit      map[string]RecoverySplitMetrics `json:"by_split"`
}

// BuildRecovery pairs the recovery-question observations with the deterministic
// baseline and computes per-split metrics. FinalAction is clamped so the advisory
// can only ADD attention.
func BuildRecovery(c *Corpus, baseline BaselineResult, obs []RecoveryObservation) RecoveryResult {
	res := RecoveryResult{CorpusSHA256: baseline.CorpusSHA256, PromptSHA256: RecoveryPromptSHA256(), Records: obs, BySplit: map[string]RecoverySplitMetrics{}}
	base := map[string]BaselineRecord{}
	for _, r := range baseline.Records {
		base[r.ScenarioID] = r
	}
	lat := map[string][]int{}
	for i := range res.Records {
		o := &res.Records[i]
		br, ok := base[o.ScenarioID]
		if !ok {
			continue
		}
		o.BaselineAction = br.Action
		// Add-attention clamp: only a HUMAN recommendation may change the action.
		o.FinalAction = br.Action
		if o.Recommendation == "NEEDS_HUMAN" {
			o.FinalAction = "NEEDS_HUMAN"
		}
		m := res.BySplit[o.Split]
		if m.ByChoice == nil {
			m.ByChoice = map[string]int{}
		}
		if m.ProviderUsage == nil {
			m.ProviderUsage = map[string]int{}
		}
		m.N++
		if o.Consulted {
			m.Consulted++
			m.ProviderUsage[o.ServedProvider]++
			lat[o.Split] = append(lat[o.Split], o.LatencyMS)
		} else if o.OutcomeClass == "not_eligible" {
			m.NotEligible++
			m.ProviderUsage["not_eligible"]++
		} else {
			m.ProviderUsage["unavailable"]++
		}
		if o.Choice != "" {
			m.ByChoice[o.Choice]++
		}
		if br.Action == br.Expected {
			m.BaselineCorrect++
		}
		if o.Recommendation != "" && o.Recommendation == o.Expected {
			m.RecommendCorrect++
		}
		if o.Recommendation == "" && o.OutcomeClass != "not_eligible" {
			m.NoRecommendation++
		}
		if o.Consulted && (o.Recommendation == attentionAction) == (o.Expected == attentionAction) {
			m.OperationalAgree++
		}
		if o.FinalAction == o.Expected {
			m.FinalCorrect++
		}
		if o.Expected == attentionAction && o.FinalAction != attentionAction {
			m.UnsafeMissed++
		}
		if o.FinalAction != br.Action {
			if o.FinalAction == attentionAction && o.Expected == attentionAction {
				m.UsefulEscalations++
			} else if o.FinalAction == attentionAction && o.Expected != attentionAction {
				m.FalseEscalations++
			}
		}
		res.BySplit[o.Split] = m
	}
	for s, m := range res.BySplit {
		m.LatencyMedianMS = percentile(lat[s], 0.50)
		m.LatencyP95MS = percentile(lat[s], 0.95)
		res.BySplit[s] = m
	}
	return res
}

// MarshalRecovery encodes the experiment artifact deterministically (sorted
// records).
func MarshalRecovery(r RecoveryResult) ([]byte, error) {
	recs := append([]RecoveryObservation(nil), r.Records...)
	sort.Slice(recs, func(i, j int) bool { return recs[i].ScenarioID < recs[j].ScenarioID })
	out := r
	out.Records = recs
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
