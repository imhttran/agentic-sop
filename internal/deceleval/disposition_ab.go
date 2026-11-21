package deceleval

// Decision-model A/B benchmark corpus and scoring (evaluation-only).
//
// This file adds a second, independently labeled corpus for comparing two served
// models in SOP's structured-decision role. It does not modify the frozen DME
// corpus (evals/decision-model/corpus.json) or its freeze. Expected actions are
// authored from SOP's deterministic rules before either model runs.
//
// Reachability note. The six-action vocabulary below is the governed
// failure-disposition vocabulary (internal/failure.Disposition), which
// autonomy.Decide plus dispositionForAction can return in full. The production
// SEAM-004 change-risk checkpoint is narrower: it offers only LOW/MEDIUM/HIGH/HUMAN
// and the model's answer can only add NEEDS_HUMAN. This benchmark asks the model
// for the governed disposition directly, so all six actions are supported by the
// evaluated contract. TestABGovernedPathCanReachEveryAction asserts the mapping.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// AB corpus splits and question identity.
const (
	ABSplitSmoke   = "smoke"
	ABSplitHeldout = "heldout"
	ABQuestionID   = "failure-disposition"
)

// ABChoices is the closed six-action governed disposition set, in a fixed order
// used for every request so both models see byte-identical choices.
var ABChoices = []string{
	string(AutoFix),
	string(Continue),
	string(Retry),
	string(Replan),
	string(NeedsHuman),
	string(Block),
}

// ABCriteria is the frozen question criterion sent to both models. It states the
// task without naming a case's expected answer.
const ABCriteria = "Classify the failure into exactly one governed SOP recovery action: safe deterministic repair (AUTO_FIX), continued automated work (CONTINUE), a transient retry (RETRY), invalid plan assumptions (REPLAN), a genuine human decision (NEEDS_HUMAN), or a terminal automation stop (BLOCK)."

// ABPrompt is the frozen structured-decision prompt definition: the question kind,
// the criterion, and the closed choice set. The per-case observable is data, not
// part of the frozen prompt.
type ABPrompt struct {
	Kind     string   `json:"kind"`
	Criteria string   `json:"criteria"`
	Choices  []string `json:"choices"`
}

// ABPromptSpec returns the canonical prompt definition.
func ABPromptSpec() ABPrompt {
	return ABPrompt{
		Kind:     ABQuestionID,
		Criteria: ABCriteria,
		Choices:  append([]string(nil), ABChoices...),
	}
}

// ABPromptSHA256 freezes the prompt definition (kind, criterion, choices).
func ABPromptSHA256() string {
	data, _ := json.Marshal(ABPromptSpec())
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// BuildABRequest builds the production structured-decision request for one
// scenario: the observable is the state, and the frozen criterion/choices are the
// single question. It deliberately carries no expected action and no rationale.
func BuildABRequest(s Scenario) decision.DecisionRequest {
	spec := ABPromptSpec()
	return decision.DecisionRequest{
		State: s.Observable,
		Questions: []decision.Question{{
			ID:       spec.Kind,
			Type:     decision.QuestionChoice,
			Criteria: spec.Criteria,
			Choices:  append([]string(nil), spec.Choices...),
		}},
	}
}

// ABCorpus is the versioned A/B corpus. It reuses the DME scenario shape but has
// its own split vocabulary (smoke|heldout) and its own freeze file.
type ABCorpus struct {
	Version     int        `json:"version"`
	Description string     `json:"description"`
	Actions     []string   `json:"actions"`
	Scenarios   []Scenario `json:"scenarios"`
}

// LoadAB reads and strictly decodes the A/B corpus. Unknown keys are rejected.
func LoadAB(path string) (*ABCorpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var c ABCorpus
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("deceleval: decode A/B corpus: %w", err)
	}
	return &c, nil
}

// Validate reports the first A/B corpus contract violation, including the
// required balance (2 smoke and 6 held-out cases per declared action).
func (c ABCorpus) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("deceleval: unsupported A/B corpus version %d", c.Version)
	}
	declared := map[string]bool{}
	for _, a := range c.Actions {
		if declared[a] {
			return fmt.Errorf("deceleval: duplicate declared action %q", a)
		}
		declared[a] = true
	}
	if len(declared) == 0 {
		return fmt.Errorf("deceleval: A/B corpus declares no actions")
	}
	for _, want := range ABChoices {
		if !declared[want] {
			return fmt.Errorf("deceleval: A/B corpus is missing governed action %q", want)
		}
	}
	for a := range declared {
		if !isABChoice(a) {
			return fmt.Errorf("deceleval: unknown declared action %q", a)
		}
	}
	if len(c.Scenarios) == 0 {
		return fmt.Errorf("deceleval: A/B corpus has no scenarios")
	}

	seen := map[string]bool{}
	smoke := map[string]int{}
	heldout := map[string]int{}
	for i, s := range c.Scenarios {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("deceleval: A/B scenario %d has no id", i)
		}
		if seen[s.ID] {
			return fmt.Errorf("deceleval: duplicate A/B scenario id %q", s.ID)
		}
		seen[s.ID] = true
		if !categories[s.Category] {
			return fmt.Errorf("deceleval: A/B scenario %q: unknown category %q", s.ID, s.Category)
		}
		if !declared[string(s.Expected.Action)] {
			return fmt.Errorf("deceleval: A/B scenario %q: expected action %q is not declared", s.ID, s.Expected.Action)
		}
		if strings.TrimSpace(s.Expected.Rationale) == "" {
			return fmt.Errorf("deceleval: A/B scenario %q: expected rationale is required", s.ID)
		}
		if strings.TrimSpace(s.Observable) == "" {
			return fmt.Errorf("deceleval: A/B scenario %q: observable is required", s.ID)
		}
		if s.Source != "synthetic" && !strings.HasPrefix(s.Source, "real:") {
			return fmt.Errorf("deceleval: A/B scenario %q: source must be synthetic or real:<run-id>", s.ID)
		}
		switch s.Split {
		case ABSplitSmoke:
			smoke[string(s.Expected.Action)]++
		case ABSplitHeldout:
			heldout[string(s.Expected.Action)]++
		default:
			return fmt.Errorf("deceleval: A/B scenario %q: split must be %s or %s", s.ID, ABSplitSmoke, ABSplitHeldout)
		}
	}

	// Balance: every declared action appears exactly twice in smoke and exactly
	// six times in the held-out set. This is what makes the 12/36 counts and the
	// per-action denominators meaningful.
	for a := range declared {
		if smoke[a] != 2 {
			return fmt.Errorf("deceleval: A/B smoke split has %d %s cases, want 2", smoke[a], a)
		}
		if heldout[a] != 6 {
			return fmt.Errorf("deceleval: A/B held-out split has %d %s cases, want 6", heldout[a], a)
		}
	}
	if len(smoke) != len(declared) || len(heldout) != len(declared) {
		return fmt.Errorf("deceleval: A/B splits do not cover every declared action")
	}
	return nil
}

func isABChoice(a string) bool {
	for _, c := range ABChoices {
		if c == a {
			return true
		}
	}
	return false
}

// Split returns the scenarios in the requested split, preserving corpus order.
func (c ABCorpus) Split(name string) []Scenario {
	var out []Scenario
	for _, s := range c.Scenarios {
		if s.Split == name {
			out = append(out, s)
		}
	}
	return out
}

// ABObservation is one model's first-attempt result for one scenario, in a
// provider-neutral shape. Optional measurements are pointers: nil is "not
// reported", never a zero.
type ABObservation struct {
	SchemaValid   bool
	ActualAction  string
	ErrorCategory string
	ErrorDetail   string

	InputTokens     *int
	OutputTokens    *int
	TotalTokens     *int
	ReasoningTokens *int
	CachedTokens    *int

	TimeToFirstTokenMS         *float64
	TimeToFirstAnswerContentMS *float64
	DecodeTokensPerSecond      *float64
	ModelLoadDurationMS        *float64
}

// ABDecideFunc performs exactly one production-shaped request for a scenario.
type ABDecideFunc func(ctx context.Context, s Scenario) ABObservation

// ABAttempt is one JSONL result per attempted case.
type ABAttempt struct {
	CaseID      string `json:"case_id"`
	Split       string `json:"split"`
	Category    string `json:"category"`
	Model       string `json:"model"`
	Concurrency int    `json:"concurrency"`
	Attempt     int    `json:"attempt"`

	ExpectedAction string  `json:"expected_action"`
	ActualAction   *string `json:"actual_action"`
	SchemaValid    bool    `json:"schema_valid"`
	Correct        bool    `json:"correct"`
	ElapsedMS      int64   `json:"elapsed_ms"`
	ErrorCategory  *string `json:"error_category"`
	ErrorDetail    string  `json:"error_detail,omitempty"`

	InputTokens     *int `json:"input_tokens"`
	OutputTokens    *int `json:"output_tokens"`
	TotalTokens     *int `json:"total_tokens"`
	ReasoningTokens *int `json:"reasoning_tokens"`
	CachedTokens    *int `json:"cached_tokens"`

	TimeToFirstTokenMS         *float64 `json:"time_to_first_token_ms"`
	TimeToFirstAnswerContentMS *float64 `json:"time_to_first_answer_content_ms"`
	DecodeTokensPerSecond      *float64 `json:"decode_tokens_per_second"`
	ModelLoadDurationMS        *float64 `json:"model_load_duration_ms"`
}

// ABActionStat is the per-expected-action breakdown.
type ABActionStat struct {
	Attempted   int `json:"attempted"`
	SchemaValid int `json:"schema_valid"`
	Correct     int `json:"correct"`
	Wrong       int `json:"wrong"`
	Invalid     int `json:"invalid"`
	Timeout     int `json:"timeout"`
	OtherError  int `json:"other_error"`
}

// ABBatchSummary is the deterministic metrics for one model at one concurrency.
type ABBatchSummary struct {
	Model       string `json:"model"`
	Concurrency int    `json:"concurrency"`
	Split       string `json:"split"`

	Cases     int  `json:"cases"`
	Attempted int  `json:"attempted"`
	Complete  bool `json:"complete"`

	SchemaValid      int `json:"schema_valid"`
	Correct          int `json:"correct"`
	InvalidResponses int `json:"invalid_responses"`
	WrongActions     int `json:"wrong_actions"`
	Timeouts         int `json:"timeouts"`
	OtherErrors      int `json:"other_errors"`
	Fallbacks        int `json:"fallbacks"`

	LatencyMedianMS  float64 `json:"latency_median_ms"`
	LatencySlowestMS int64   `json:"latency_slowest_ms"`
	SlowestCase      string  `json:"slowest_case"`

	// WallMS is the measured batch wall time (all requests in this batch), which
	// is the denominator for CorrectPerMinute. Cold load and warm-up are excluded.
	WallMS           int64   `json:"wall_ms"`
	CorrectPerMinute float64 `json:"correct_per_minute"`

	ByAction map[string]ABActionStat `json:"by_action"`

	ColdLoadMS *int64 `json:"cold_load_ms"`
	WarmupMS   *int64 `json:"warmup_ms"`
}

// ABDisagreement is a case whose expected action is BLOCK or NEEDS_HUMAN and whose
// first attempt did not select it.
type ABDisagreement struct {
	CaseID         string  `json:"case_id"`
	Model          string  `json:"model"`
	Concurrency    int     `json:"concurrency"`
	ExpectedAction string  `json:"expected_action"`
	SelectedAction *string `json:"selected_action"`
	SchemaValid    bool    `json:"schema_valid"`
	ErrorCategory  *string `json:"error_category"`
}

// SummarizeAB computes the per-batch summary from ordered attempts and the
// measured batch wall time.
func SummarizeAB(attempts []ABAttempt, cases int, wall time.Duration) ABBatchSummary {
	s := ABBatchSummary{
		Split:      firstSplit(attempts),
		Cases:      cases,
		Attempted:  len(attempts),
		Complete:   len(attempts) == cases,
		ByAction:   map[string]ABActionStat{},
		WallMS:     wall.Milliseconds(),
		ColdLoadMS: nil,
	}
	for _, a := range ABChoices {
		s.ByAction[a] = ABActionStat{}
	}
	var lat []int
	for _, a := range attempts {
		if s.Model == "" {
			s.Model = a.Model
		}
		if s.Concurrency == 0 {
			s.Concurrency = a.Concurrency
		}
		lat = append(lat, int(a.ElapsedMS))
		if a.ElapsedMS > s.LatencySlowestMS {
			s.LatencySlowestMS = a.ElapsedMS
			s.SlowestCase = a.CaseID
		}
		st := s.ByAction[a.ExpectedAction]
		st.Attempted++
		if a.SchemaValid {
			st.SchemaValid++
		}
		switch {
		case a.Correct:
			st.Correct++
		case a.ErrorCategory != nil && *a.ErrorCategory == "timeout":
			st.Timeout++
		case a.SchemaValid:
			st.Wrong++
		case a.ErrorCategory != nil && *a.ErrorCategory == "malformed":
			st.Invalid++
		default:
			st.OtherError++
		}
		s.ByAction[a.ExpectedAction] = st
	}
	for _, st := range s.ByAction {
		s.SchemaValid += st.SchemaValid
		s.Correct += st.Correct
		s.InvalidResponses += st.Invalid
		s.WrongActions += st.Wrong
		s.Timeouts += st.Timeout
		s.OtherErrors += st.OtherError
	}
	s.LatencyMedianMS = percentile(lat, 0.50)
	if s.WallMS > 0 {
		s.CorrectPerMinute = float64(s.Correct) / (float64(s.WallMS) / 60000.0)
	}
	return s
}

func firstSplit(attempts []ABAttempt) string {
	if len(attempts) == 0 {
		return ""
	}
	return attempts[0].Split
}

// ConsequentialDisagreements returns every non-correct case whose expected action
// is BLOCK or NEEDS_HUMAN. The selected action is included (nil when the first
// attempt was not schema-valid) so the operator can tell an unsafe automatic
// continuation from a malformed or timed-out attempt.
func ConsequentialDisagreements(attempts []ABAttempt) []ABDisagreement {
	var out []ABDisagreement
	for _, a := range attempts {
		if a.Correct {
			continue
		}
		if a.ExpectedAction != string(Block) && a.ExpectedAction != string(NeedsHuman) {
			continue
		}
		out = append(out, ABDisagreement{
			CaseID:         a.CaseID,
			Model:          a.Model,
			Concurrency:    a.Concurrency,
			ExpectedAction: a.ExpectedAction,
			SelectedAction: a.ActualAction,
			SchemaValid:    a.SchemaValid,
			ErrorCategory:  a.ErrorCategory,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		if out[i].Concurrency != out[j].Concurrency {
			return out[i].Concurrency < out[j].Concurrency
		}
		return out[i].CaseID < out[j].CaseID
	})
	return out
}
