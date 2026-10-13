package deceleval

// JEV shadow-mode evaluation (eval-only).
//
// This runs the REAL JEV analyzer over the frozen corpus and maps its structured
// result to a failure disposition through the EXISTING deterministic early-JEV
// policy (autonomy.DecideEarly) — the repository's sanctioned
// "JEV evidence -> structured classification -> deterministic SOP policy ->
// lifecycle action" path. It is shadow-only: JEV's recommendation is recorded and
// compared, never applied. The deterministic Go decision is never changed.
//
// JEV itself emits only PASS/FINDINGS/INCOMPLETE/ERROR; it has no disposition
// vocabulary. The disposition is therefore derived, and the derivation is the
// existing policy, not a new one.

import (
	"encoding/json"
	"sort"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/jev"
)

// JEVEvaluationRevision identifies this experiment's inputs and mapping so a run
// is reproducible.
const JEVEvaluationRevision = "jev-shadow-v1"

// JEVInputMode documents what bounded context each scenario is given: only the
// scenario's observable failure text as the task. No label or category is sent,
// so JEV never sees the expected answer.
const JEVInputMode = "task=observable"

// JEVDispositionForAction maps an autonomy action to the failure-disposition
// vocabulary (mirrors cli.dispositionForAction).
func JEVDispositionForAction(a autonomy.Action) string {
	switch a {
	case autonomy.ActionAutoRetry:
		return string(failure.Retry)
	case autonomy.ActionAutoContinue:
		return string(failure.Continue)
	case autonomy.ActionAutoFix:
		return string(failure.AutoFix)
	case autonomy.ActionAutoReconcile:
		return string(failure.Replan)
	case autonomy.ActionTerminal:
		return string(failure.Block)
	case autonomy.ActionHumanApproval:
		return string(failure.NeedsHuman)
	default:
		return string(a)
	}
}

// MapJEVHypotheticalDirective maps a validated JEV result to the failure
// disposition that autonomy.DecideEarly WOULD produce if the recovery policy
// consumed JEV evidence directly. It is HYPOTHETICAL evaluation tooling.
//
// It is NOT the production early-JEV behavior. Production
// (internal/cli/early_jev.go runJEVCheckpoint) short-circuits any fail-closed,
// ERROR, or INCOMPLETE result (and any result without valid structured evidence)
// into a non-escalating ProviderFailed outcome BEFORE policy is applied, so
// production never routes INCOMPLETE/ERROR into DecideEarly; the quality-gate path
// (quality.NewJEVEvidence) separately fails closed on them. This function
// deliberately bypasses that guard so the experiment can characterize "JEV used as
// a failure-disposition classifier". Its output MUST NOT be read as production
// behavior; see docs/reference/DECISION-MODEL-FINDINGS.md §5.
//
// ok is false when JEV produced no usable structured evidence (no actionable
// recommendation).
func MapJEVHypotheticalDirective(res jev.Result, failOn []string, p autonomy.Policy) (directive string, ok bool) {
	if res.StructuredEvidence == nil {
		return "", false
	}
	return JEVDispositionForAction(autonomy.DecideEarly(*res.StructuredEvidence, failOn, p).Action), true
}

// JEVObservation is one scenario's shadow observation.
type JEVObservation struct {
	ScenarioID string `json:"scenario_id"`
	Category   string `json:"category"`
	Split      string `json:"split"`
	Expected   string `json:"expected_action"`
	// DeterministicAction is the frozen deterministic Go baseline action.
	DeterministicAction string `json:"deterministic_action"`
	// Ran is true when a JEV analyzer was present and invoked.
	Ran bool `json:"ran"`
	// Available is true when the analyzer ran and returned a usable result.
	Available bool   `json:"available"`
	Status    string `json:"jev_status,omitempty"`
	// FindingCount and MaxSeverity summarize the raw JEV result.
	FindingCount int    `json:"finding_count"`
	MaxSeverity  string `json:"max_severity,omitempty"`
	// Directive is the disposition derived by autonomy.DecideEarly; empty when no
	// actionable recommendation was produced.
	Directive string `json:"jev_directive,omitempty"`
	// Actionable is true when a directive was derived.
	Actionable bool   `json:"actionable"`
	LatencyMS  int    `json:"latency_ms"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	ToolCalls  int    `json:"tool_calls,omitempty"`
	Error      string `json:"error,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// JEVMetrics aggregates the deterministic-vs-JEV comparison with exact
// denominators. "attention" means the NEEDS_HUMAN disposition; "withholds" means
// NEEDS_HUMAN or BLOCK.
type JEVMetrics struct {
	N             int `json:"n"`
	Available     int `json:"available"`
	Unavailable   int `json:"unavailable"`
	NotActionable int `json:"not_actionable"`

	ExactAgree       int `json:"exact_agree"`
	OperationalAgree int `json:"operational_agree"`

	// Attention semantics relative to the deterministic baseline.
	UsefulEscalations int `json:"useful_escalations"`
	FalseEscalations  int `json:"false_escalations"`
	UnsafeMissed      int `json:"unsafe_missed"`

	// Directional separation.
	Improved             int `json:"improved"`
	Worsened             int `json:"worsened"`
	AgreeWithDeterminism int `json:"agree_with_deterministic"`

	// Terminal-case coverage: scenarios whose label or baseline is BLOCK.
	TerminalTotal     int `json:"terminal_total"`
	TerminalEvaluated int `json:"terminal_evaluated"`

	// Directive distribution (derived dispositions) and raw status distribution.
	ByDirective map[string]int `json:"by_directive"`
	ByStatus    map[string]int `json:"by_status"`

	LatencyMedianMS float64 `json:"latency_median_ms"`
	LatencyP95MS    float64 `json:"latency_p95_ms"`
}

// JEVResult is the experiment artifact.
type JEVResult struct {
	CorpusSHA256       string                `json:"corpus_sha256"`
	EvaluationRevision string                `json:"evaluation_revision"`
	Implementation     string                `json:"implementation"`
	InputMode          string                `json:"input_mode"`
	Provider           string                `json:"provider,omitempty"`
	Model              string                `json:"model,omitempty"`
	FailOn             []string              `json:"fail_on"`
	Records            []JEVObservation      `json:"records"`
	Metrics            JEVMetrics            `json:"metrics"`
	BySplit            map[string]JEVMetrics `json:"by_split"`
}

// jevAttention reports whether a directive is the human boundary.
func jevAttention(d string) bool { return d == attentionAction }

// BuildJEV pairs the shadow observations with the deterministic baseline and
// computes the comparison metrics.
func BuildJEV(baseline BaselineResult, obs []JEVObservation, provider, model string, failOn []string) JEVResult {
	res := JEVResult{
		CorpusSHA256:       baseline.CorpusSHA256,
		EvaluationRevision: JEVEvaluationRevision,
		Implementation:     "internal/jev.OllamaAnalyzer",
		InputMode:          JEVInputMode,
		Provider:           provider,
		Model:              model,
		FailOn:             append([]string(nil), failOn...),
		Records:            obs,
		BySplit:            map[string]JEVMetrics{},
	}
	base := map[string]BaselineRecord{}
	for _, r := range baseline.Records {
		base[r.ScenarioID] = r
	}
	lat := map[string][]int{}
	allLat := []int{}
	acc := map[string]*JEVMetrics{"all": {}}
	accSplit := map[string]*JEVMetrics{}
	initM := func(m *JEVMetrics) {
		if m.ByDirective == nil {
			m.ByDirective = map[string]int{}
		}
		if m.ByStatus == nil {
			m.ByStatus = map[string]int{}
		}
	}
	for i := range res.Records {
		o := &res.Records[i]
		br, ok := base[o.ScenarioID]
		if !ok {
			continue
		}
		o.DeterministicAction = br.Action
		if o.Split == "" {
			o.Split = "eval"
		}
		if _, ok := accSplit[o.Split]; !ok {
			accSplit[o.Split] = &JEVMetrics{}
		}
		for _, m := range []*JEVMetrics{acc["all"], accSplit[o.Split]} {
			initM(m)
			m.N++
			if o.Ran {
				if o.Available {
					m.Available++
					m.ByStatus[o.Status]++
					lat[o.Split] = append(lat[o.Split], o.LatencyMS)
					allLat = append(allLat, o.LatencyMS)
				} else {
					m.Unavailable++
				}
			} else {
				m.Unavailable++
			}
			if o.Actionable {
				m.ByDirective[o.Directive]++
				if o.Directive == o.Expected {
					m.ExactAgree++
				}
				if jevAttention(o.Directive) == jevAttention(o.Expected) {
					m.OperationalAgree++
				}
				if jevAttention(o.Directive) && jevAttention(o.Expected) && !jevAttention(br.Action) {
					m.UsefulEscalations++
				}
				if jevAttention(o.Directive) && !jevAttention(o.Expected) {
					m.FalseEscalations++
				}
				if jevAttention(o.Expected) && !jevAttention(o.Directive) {
					m.UnsafeMissed++
				}
				if o.Directive == o.Expected && o.DeterministicAction != o.Expected {
					m.Improved++
				}
				if o.Directive != o.Expected && o.DeterministicAction == o.Expected {
					m.Worsened++
				}
				if o.Directive == o.DeterministicAction {
					m.AgreeWithDeterminism++
				}
			} else if o.Available {
				m.NotActionable++
			}
			if o.Expected == string(failure.Block) || o.DeterministicAction == string(failure.Block) {
				m.TerminalTotal++
				if o.Actionable {
					m.TerminalEvaluated++
				}
			}
		}
	}
	all := *acc["all"]
	all.LatencyMedianMS = percentile(allLat, 0.50)
	all.LatencyP95MS = percentile(allLat, 0.95)
	res.Metrics = all
	for s, m := range accSplit {
		mm := *m
		mm.LatencyMedianMS = percentile(lat[s], 0.50)
		mm.LatencyP95MS = percentile(lat[s], 0.95)
		res.BySplit[s] = mm
	}
	return res
}

// MarshalJEV encodes the artifact deterministically (sorted records).
func MarshalJEV(r JEVResult) ([]byte, error) {
	recs := append([]JEVObservation(nil), r.Records...)
	sort.Slice(recs, func(i, j int) bool { return recs[i].ScenarioID < recs[j].ScenarioID })
	out := r
	out.Records = recs
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
