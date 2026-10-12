// Package bridge is SOP's deterministic bridge between the canonical SEAM-002
// decision seam (internal/decision) and a provider-neutral, batched decision
// adapter profile.
//
// The canonical seam is one bounded decision per call; a batched adapter profile
// answers several questions per call against one state. This bridge owns the
// deterministic translation in both directions, validates a profile result
// structurally (schema, question identity, answer type, and capability semantics —
// never a provider name alone), and fails closed on anything it cannot interpret.
//
// It carries no authority: it never approves, escalates on its own, or executes.
// It produces canonical evidence only; deterministic SOP policy (decision.Route,
// internal/autonomy) retains final authority, and every failure routes to a human.
//
// The contract it implements is ratified at Gate 3 of PLAN-JEV-Wave-2-Hardening
// and recorded in docs/architecture/JEV-DECISION-CONTRACT.md (§13–§20). The bridge
// is provider-neutral: it names no provider, model, or transport, and adapter
// profile identity/version is a SOP-side concept (M3) — a profile version is
// stamped here, and the adapters add no wire-level version field.
package bridge

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// QuestionType is the closed set of provider-neutral question types a profile may
// declare. It mirrors the adapter profile's question vocabulary.
type QuestionType string

const (
	// TypeChoice asks for exactly one of the question's allowed choices.
	TypeChoice QuestionType = "choice"
	// TypeBoolean asks for a yes/no decision by its underlying probability.
	TypeBoolean QuestionType = "boolean"
	// TypeScore asks for a numeric score.
	TypeScore QuestionType = "score"
)

func (t QuestionType) known() bool {
	switch t {
	case TypeChoice, TypeBoolean, TypeScore:
		return true
	default:
		return false
	}
}

// Question is one decision in a batched profile request.
type Question struct {
	ID           string       `json:"id"`
	Type         QuestionType `json:"type"`
	Instructions string       `json:"instructions,omitempty"`
	Criteria     string       `json:"criteria,omitempty"`
	Choices      []string     `json:"choices,omitempty"`
}

// Request is a batched profile request: one or more questions against one state.
type Request struct {
	State     string            `json:"state"`
	Questions []Question        `json:"questions"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Answer is one normalized profile answer. Only the fields appropriate to Type
// are populated.
type Answer struct {
	Type          QuestionType       `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Probability   float64            `json:"probability,omitempty"`
	Score         float64            `json:"score,omitempty"`
}

// Usage carries provider-neutral token accounting.
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// Result is a batched profile result. It carries no contract version: the bridge
// stamps the recognized profile version (M3).
type Result struct {
	Provider string            `json:"provider"`
	Model    string            `json:"model,omitempty"`
	Answers  map[string]Answer `json:"answers"`
	Usage    Usage             `json:"usage"`
}

// ScoreBand maps an inclusive upper score bound to a canonical choice. Bands are
// evaluated by ascending Max; the first band whose Max is >= the score applies.
type ScoreBand struct {
	Max    float64
	Choice decision.Choice
}

// KindSpec is the deterministic mapping for one canonical decision kind: the
// profile question type to issue, and the capability-specific rules that reduce
// its answer to a canonical (Choice, optional Confidence).
type KindSpec struct {
	// Type is the profile question type to issue for this kind.
	Type QuestionType

	// CHOICE settings.
	ChoiceMap map[string]decision.Choice
	// ChosenProbAsConfidence lets the probability of the *chosen* native choice
	// stand in for confidence when the answer carries no explicit confidence. It
	// is opt-in per capability: it is never inferred.
	ChosenProbAsConfidence bool

	// BOOLEAN settings. A boolean is not a choice on its own: it becomes one only
	// through an explicit threshold policy.
	BooleanThreshold *float64
	TrueChoice       decision.Choice
	FalseChoice      decision.Choice

	// SCORE settings. A raw score is not a probability: it becomes a confidence
	// only through an explicit capability normalization.
	ScoreBands []ScoreBand
	ScoreRange *[2]float64 // {min,max}, normalized linearly into [0,1]
}

// Profile is a recognized, versioned adapter profile. Version is stamped by the
// bridge (M3); the adapters add no wire-level version field. Name is a
// correlation hint only — recognition is structural (see Registry.Build and
// Built.Interpret), never name-alone.
type Profile struct {
	Name       string
	Version    int
	Deprecated bool
	Kinds      map[string]KindSpec
}

func (p *Profile) spec(kind string) (KindSpec, bool) {
	if p == nil {
		return KindSpec{}, false
	}
	s, ok := p.Kinds[kind]
	return s, ok
}

// Registry is the set of profiles the bridge recognizes. Selection is
// deterministic: a canonical kind must be declared by exactly one profile.
type Registry struct {
	profiles []*Profile
}

// NewRegistry returns a registry over the given profiles, preserving order.
func NewRegistry(profiles ...*Profile) *Registry {
	return &Registry{profiles: profiles}
}

// Built is a profile request the bridge issued, plus the canonical context needed
// to interpret its result deterministically.
type Built struct {
	// Request is the profile wire request to send to the adapter.
	Request Request
	// Kind is the canonical decision kind (decision.Request.UseCase).
	Kind string
	// Allowed is the canonical allowed-choice set (may be empty).
	Allowed []decision.Choice

	profile *Profile
	spec    KindSpec
}

// Build translates a canonical request into a batched profile request. It selects
// the single profile that declares the request's kind, derives a deterministic
// question ID, and fails closed (never guessing a low-confidence answer) when no
// profile serves the kind or the profile is ambiguous.
func (r *Registry) Build(req decision.Request) (Built, error) {
	kind := strings.TrimSpace(req.UseCase)
	if kind == "" {
		return Built{}, fmt.Errorf("%w: request carries no use-case/kind", decision.ErrUnsupportedCapability)
	}

	var chosen *Profile
	var chosenSpec KindSpec
	matches := 0
	for _, p := range r.profiles {
		if s, ok := p.spec(kind); ok {
			matches++
			chosen, chosenSpec = p, s
		}
	}
	switch {
	case matches == 0:
		return Built{}, fmt.Errorf("%w: no recognized profile serves kind %q", decision.ErrUnsupportedCapability, kind)
	case matches > 1:
		return Built{}, fmt.Errorf("%w: kind %q is declared by multiple profiles (ambiguous)", decision.ErrInvalidResult, kind)
	}

	if !chosenSpec.Type.known() {
		return Built{}, fmt.Errorf("%w: profile %q declares an unknown question type for kind %q", decision.ErrInvalidResult, chosen.Name, kind)
	}
	if err := validateSpec(chosen.Name, kind, chosenSpec); err != nil {
		return Built{}, err
	}

	subject := strings.TrimSpace(req.Subject)
	q := Question{
		ID:   kind, // deterministic, unambiguous: one canonical decision per call
		Type: chosenSpec.Type,
	}
	if chosenSpec.Type == TypeChoice {
		q.Criteria = subject
		if q.Criteria == "" {
			q.Criteria = kind // the profile requires non-empty criteria
		}
		q.Choices = sortedChoiceKeys(chosenSpec.ChoiceMap)
	}

	return Built{
		Request: Request{
			State:     buildState(kind, subject, req.Signals),
			Questions: []Question{q},
		},
		Kind:    kind,
		Allowed: append([]decision.Choice(nil), req.Choices...),
		profile: chosen,
		spec:    chosenSpec,
	}, nil
}

// Interpret validates a profile result against the request the bridge issued,
// recognizes the profile structurally, and returns canonical SOP evidence. Every
// failure class fails closed: it returns a typed decision sentinel and no usable
// evidence, never an approval.
func (b Built) Interpret(res Result) (decision.Result, error) {
	// Recognition: the provider name is a correlation check, not the selection
	// key. Structure (below) is the actual key (§17, Gate 3 constraint 4).
	if strings.TrimSpace(res.Provider) == "" {
		return decision.Result{}, fmt.Errorf("%w: profile result has no provider", decision.ErrInvalidResult)
	}
	if res.Provider != b.profile.Name {
		return decision.Result{}, fmt.Errorf("%w: unknown profile %q", decision.ErrInvalidResult, res.Provider)
	}
	if len(res.Answers) == 0 {
		return decision.Result{}, fmt.Errorf("%w: profile result has no answers", decision.ErrIndeterminate)
	}
	// The bridge issues exactly one question per call; enforce that cardinality.
	if len(b.Request.Questions) != 1 {
		return decision.Result{}, fmt.Errorf("%w: bridge expects a single decision per call", decision.ErrInvalidResult)
	}
	q := b.Request.Questions[0]

	// Question identity: the answer set must exactly match the sent question set.
	if len(res.Answers) != 1 {
		return decision.Result{}, fmt.Errorf("%w: expected exactly one answer, got %d", decision.ErrInvalidResult, len(res.Answers))
	}
	ans, ok := res.Answers[q.ID]
	if !ok {
		for id := range res.Answers {
			return decision.Result{}, fmt.Errorf("%w: unexpected answer for question %q", decision.ErrInvalidResult, id)
		}
		return decision.Result{}, fmt.Errorf("%w: missing answer for question %q", decision.ErrIndeterminate, q.ID)
	}

	choice, confidence, err := b.reduce(q, ans)
	if err != nil {
		return decision.Result{}, err
	}

	if !choice.Known() {
		return decision.Result{}, fmt.Errorf("%w: profile produced unknown choice %q", decision.ErrInvalidResult, choice)
	}
	if len(b.Allowed) > 0 && !containsChoice(b.Allowed, choice) {
		return decision.Result{}, fmt.Errorf("%w: choice %q is outside the request's allowed set", decision.ErrInvalidResult, choice)
	}
	if confidence == nil {
		return decision.Result{}, fmt.Errorf("%w: profile gave no confidence for kind %q", decision.ErrIndeterminate, b.Kind)
	}
	if !validUnit(*confidence) {
		return decision.Result{}, fmt.Errorf("%w: confidence is not a finite value in [0,1]", decision.ErrInvalidResult)
	}

	out := decision.Result{
		ContractVersion: decision.ContractVersion,
		Status:          decision.StatusOK,
		Kind:            b.Kind,
		Choice:          choice,
		Confidence:      confidence,
		Diagnostics:     b.diagnostics(res),
	}
	// Self-check against the SOP-owned validator, so the bridge can never emit
	// evidence SOP would reject.
	probe := decision.Request{UseCase: b.Kind, Choices: b.Allowed}
	if err := probe.ValidateResult(out); err != nil {
		return decision.Result{}, err
	}
	return out, nil
}

// reduce maps one profile answer to a canonical (Choice, optional Confidence),
// applying the capability-specific rules of the kind's spec. It fails closed.
func (b Built) reduce(q Question, ans Answer) (decision.Choice, *float64, error) {
	if ans.Type != q.Type {
		return "", nil, fmt.Errorf("%w: answer type %q does not match question type %q", decision.ErrInvalidResult, ans.Type, q.Type)
	}
	switch b.spec.Type {
	case TypeChoice:
		return b.reduceChoice(ans)
	case TypeBoolean:
		return b.reduceBoolean(ans)
	case TypeScore:
		return b.reduceScore(ans)
	default:
		return "", nil, fmt.Errorf("%w: unsupported question type %q", decision.ErrInvalidResult, q.Type)
	}
}

func (b Built) reduceChoice(ans Answer) (decision.Choice, *float64, error) {
	native := strings.TrimSpace(ans.Choice)
	if native == "" {
		return "", nil, fmt.Errorf("%w: choice answer is empty", decision.ErrIndeterminate)
	}
	for name, p := range ans.Probabilities {
		if !validUnit(p) {
			return "", nil, fmt.Errorf("%w: probability for choice %q out of range [0,1]", decision.ErrInvalidResult, name)
		}
	}
	c, ok := b.spec.ChoiceMap[native]
	if !ok {
		return "", nil, fmt.Errorf("%w: choice %q is not in the profile's allowed-value table", decision.ErrIndeterminate, native)
	}
	if ans.Confidence != nil {
		if !validUnit(*ans.Confidence) {
			return "", nil, fmt.Errorf("%w: confidence is not a finite value in [0,1]", decision.ErrInvalidResult)
		}
		c2 := *ans.Confidence
		return c, &c2, nil
	}
	// Only an opt-in capability may read a probability as confidence, and only the
	// probability of the *chosen* outcome — never a different event's probability.
	if b.spec.ChosenProbAsConfidence {
		if p, has := ans.Probabilities[native]; has {
			if !validUnit(p) {
				return "", nil, fmt.Errorf("%w: probability for choice %q out of range [0,1]", decision.ErrInvalidResult, native)
			}
			c2 := p
			return c, &c2, nil
		}
	}
	return "", nil, fmt.Errorf("%w: choice answer carries no usable confidence", decision.ErrIndeterminate)
}

func (b Built) reduceBoolean(ans Answer) (decision.Choice, *float64, error) {
	if b.spec.BooleanThreshold == nil {
		return "", nil, fmt.Errorf("%w: no boolean threshold policy for the capability", decision.ErrIndeterminate)
	}
	p := ans.Probability
	if !validUnit(p) {
		return "", nil, fmt.Errorf("%w: boolean probability is not a finite value in [0,1]", decision.ErrInvalidResult)
	}
	if p >= *b.spec.BooleanThreshold {
		c := p
		return b.spec.TrueChoice, &c, nil
	}
	c := 1 - p
	return b.spec.FalseChoice, &c, nil
}

func (b Built) reduceScore(ans Answer) (decision.Choice, *float64, error) {
	if b.spec.ScoreRange == nil || len(b.spec.ScoreBands) == 0 {
		return "", nil, fmt.Errorf("%w: no score normalization for the capability", decision.ErrIndeterminate)
	}
	lo, hi := b.spec.ScoreRange[0], b.spec.ScoreRange[1]
	if hi <= lo {
		return "", nil, fmt.Errorf("%w: invalid score range configured", decision.ErrInvalidResult)
	}
	s := ans.Score
	if math.IsNaN(s) || math.IsInf(s, 0) || s < lo || s > hi {
		return "", nil, fmt.Errorf("%w: score is outside the configured range", decision.ErrInvalidResult)
	}
	var choice decision.Choice
	for _, band := range b.spec.ScoreBands {
		if s <= band.Max {
			choice = band.Choice
			break
		}
	}
	if choice == "" {
		return "", nil, fmt.Errorf("%w: score %v matches no band", decision.ErrIndeterminate, s)
	}
	c := (s - lo) / (hi - lo)
	return choice, &c, nil
}

// diagnostics records bounded, class-only profile metadata. It never copies
// provider error text, endpoint URLs, response bodies, tokens, or credentials.
func (b Built) diagnostics(res Result) map[string]string {
	d := map[string]string{
		"profile":         b.profile.Name,
		"profile_version": strconv.Itoa(b.profile.Version),
	}
	if m := strings.TrimSpace(res.Model); m != "" {
		d["model"] = m
	}
	if b.profile.Deprecated {
		d["deprecated"] = "true"
	}
	return d
}

// RequestJSON encodes the profile wire request. It is the data the transport
// (owned elsewhere) writes to the adapter.
func (b Built) RequestJSON() ([]byte, error) {
	return json.Marshal(b.Request)
}

// MapError maps a profile-side failure class to the canonical SOP sentinel. It is
// the class-only bridge of §18: it names the class, never the provider detail.
func MapError(kind string) error {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "unavailable", "provider_failure":
		return decision.ErrProviderFailure
	case "invalid_request", "malformed_response":
		return decision.ErrInvalidResult
	case "unsupported", "unsupported_capability":
		return decision.ErrUnsupportedCapability
	default:
		return decision.ErrIndeterminate
	}
}

// validateSpec rejects a profile whose kind spec is internally unusable, so a
// misconfigured profile fails at Build rather than silently mis-mapping an answer.
func validateSpec(name, kind string, s KindSpec) error {
	switch s.Type {
	case TypeChoice:
		if len(s.ChoiceMap) == 0 {
			return fmt.Errorf("%w: profile %q kind %q has an empty choice map", decision.ErrInvalidResult, name, kind)
		}
		for native, c := range s.ChoiceMap {
			if strings.TrimSpace(native) == "" || !c.Known() {
				return fmt.Errorf("%w: profile %q kind %q has an invalid choice mapping", decision.ErrInvalidResult, name, kind)
			}
		}
	case TypeBoolean:
		if s.BooleanThreshold == nil {
			return fmt.Errorf("%w: profile %q kind %q has no boolean threshold", decision.ErrInvalidResult, name, kind)
		}
		if !validUnit(*s.BooleanThreshold) {
			return fmt.Errorf("%w: profile %q kind %q has an out-of-range boolean threshold", decision.ErrInvalidResult, name, kind)
		}
		if !s.TrueChoice.Known() || !s.FalseChoice.Known() {
			return fmt.Errorf("%w: profile %q kind %q has an unknown boolean choice", decision.ErrInvalidResult, name, kind)
		}
	case TypeScore:
		if s.ScoreRange == nil || len(s.ScoreBands) == 0 {
			return fmt.Errorf("%w: profile %q kind %q has no score normalization", decision.ErrInvalidResult, name, kind)
		}
	}
	return nil
}

func buildState(kind, subject string, signals map[string]float64) string {
	var b strings.Builder
	b.WriteString(subject)
	if len(signals) > 0 {
		keys := make([]string, 0, len(signals))
		for k := range signals {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "\n%s=%g", k, signals[k])
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return kind
	}
	return s
}

func sortedChoiceKeys(m map[string]decision.Choice) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func containsChoice(allowed []decision.Choice, c decision.Choice) bool {
	for _, a := range allowed {
		if a == c {
			return true
		}
	}
	return false
}

func validUnit(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}
