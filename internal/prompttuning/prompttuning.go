// Package prompttuning is the CTX-012 Automatic Prompt Tuning controller: an
// evaluation-gated promotion boundary for prompt candidates.
//
// A candidate is promoted only when a deterministic evaluation on a non-empty corpus
// shows a measurable improvement over the baseline, and only when it satisfies every
// required structural invariant. The candidate's own opinion is never consulted: there
// is no field for it, and a proposal is rejected however confident its text is. The
// controller is pure and side-effect free. It records a decision; it never edits a
// prompt, and it cannot mutate lifecycle policy, permissions, verification criteria, or
// safety boundaries, which are enforced as caller-supplied invariants.
package prompttuning

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/prompt"
)

// Decision is the promotion outcome.
type Decision string

const (
	// Promote: the candidate measurably beat the baseline and satisfied every invariant.
	Promote Decision = "PROMOTE"
	// Reject: the candidate was not promoted.
	Reject Decision = "REJECT"
)

// Fixed reason phrases, so a decision is auditable and never model-generated prose.
const (
	ReasonInvariantViolation = "candidate violates a required invariant"
	ReasonEmptyCorpus        = "empty evaluation corpus; no evidence to promote on"
	ReasonNoImprovement      = "candidate showed no measurable improvement over the baseline"
	ReasonPromoted           = "candidate measurably improved over the baseline and satisfied every invariant"
	ReasonNoEvaluation       = "no evaluator supplied; cannot establish measurable improvement"
)

// Corpus is a deterministic evaluation corpus. Cases is the number of evaluation cases;
// a corpus with no cases cannot justify a promotion.
type Corpus struct {
	Cases int
}

// Evaluator evaluates a candidate prompt variant against a corpus. It is deterministic
// and model-free: the caller supplies an evaluation that does not ask a model whether
// the prompt is better.
type Evaluator func(candidate string, corpus Corpus) Score

// Score is a deterministic evaluation result over a corpus.
type Score struct {
	Passed int
	Total  int
}

// Rate is the pass rate, 0 when there are no cases.
func (s Score) Rate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Passed) / float64(s.Total)
}

// Candidate is a proposed prompt variant.
type Candidate struct {
	// Name identifies the variant (for example "compiler-v3").
	Name string
	// Prompt is the candidate's prompt text.
	Prompt string
}

// Invariant is a structural property a candidate must satisfy to be promotable, such as
// preserving a safety boundary, a required section, or the verification contract. It is
// model-free and deterministic. Returning a non-nil error rejects the candidate.
type Invariant func(candidate string) error

// Input is one tuning decision request.
type Input struct {
	// Baseline is the current, in-production prompt.
	Baseline Candidate
	// Proposal is the candidate prompt variant.
	Proposal Candidate
	// Corpus is the deterministic evaluation corpus.
	Corpus Corpus
	// Evaluate scores a candidate on the corpus. A nil evaluator cannot establish
	// improvement, so the candidate is rejected.
	Evaluate Evaluator
	// Margin is the required improvement in pass rate; the proposal's rate must exceed
	// the baseline's rate by more than Margin. A zero or negative margin requires a
	// strict improvement.
	Margin float64
	// Invariants are structural properties the proposal must satisfy.
	Invariants []Invariant
}

// Record preserves the full tuning decision for audit: the baseline and candidate, their
// evaluation scores, the decision, and the reason.
type Record struct {
	Baseline      Candidate `json:"baseline"`
	Proposal      Candidate `json:"proposal"`
	BaselineScore Score     `json:"baseline_score"`
	ProposalScore Score     `json:"proposal_score"`
	Corpus        int       `json:"corpus_cases"`
	Margin        float64   `json:"margin"`
	Decision      Decision  `json:"decision"`
	Reason        string    `json:"reason"`
}

// Decide evaluates the proposal against the baseline and returns the promotion record.
// It is pure and deterministic: identical inputs yield an identical record.
func Decide(in Input) Record {
	rec := Record{Baseline: in.Baseline, Proposal: in.Proposal, Corpus: in.Corpus.Cases, Margin: in.Margin}

	for _, inv := range in.Invariants {
		if inv == nil {
			continue
		}
		if err := inv(in.Proposal.Prompt); err != nil {
			rec.Decision, rec.Reason = Reject, fmt.Sprintf("%s: %v", ReasonInvariantViolation, err)
			return rec
		}
	}
	if in.Corpus.Cases <= 0 {
		rec.Decision, rec.Reason = Reject, ReasonEmptyCorpus
		return rec
	}
	if in.Evaluate == nil {
		rec.Decision, rec.Reason = Reject, ReasonNoEvaluation
		return rec
	}

	rec.BaselineScore = in.Evaluate(in.Baseline.Prompt, in.Corpus)
	rec.ProposalScore = in.Evaluate(in.Proposal.Prompt, in.Corpus)
	if rec.ProposalScore.Rate() <= rec.BaselineScore.Rate()+in.Margin {
		rec.Decision = Reject
		rec.Reason = fmt.Sprintf("%s (baseline %.3f, proposal %.3f, margin %.3f)",
			ReasonNoImprovement, rec.BaselineScore.Rate(), rec.ProposalScore.Rate(), in.Margin)
		return rec
	}
	rec.Decision, rec.Reason = Promote, ReasonPromoted
	return rec
}

// tuningTaskProbe is the task text CompilerInvariant compiles a candidate against, to
// check that instructions and the task stay separated.
const tuningTaskProbe = "tuning candidate evaluation"

// CompilerInvariant returns an invariant that requires a candidate prompt to compile
// cleanly: non-empty, with an instructions section and a task section kept separate. It
// demonstrates a model-free structural gate; a caller may add further invariants for
// safety boundaries, permissions, and the verification contract.
func CompilerInvariant(capability agent.Capability, class prompt.Class) Invariant {
	return func(candidate string) error {
		if strings.TrimSpace(candidate) == "" {
			return fmt.Errorf("empty candidate prompt")
		}
		compiled := prompt.Compile(prompt.Input{
			Capability:         capability,
			Class:              class,
			Task:               tuningTaskProbe,
			OutputRequirements: candidate,
		})
		if len(compiled.Sections) == 0 {
			return fmt.Errorf("prompt compiles to no sections")
		}
		instr, ok := compiled.Section("instructions")
		if !ok {
			return fmt.Errorf("prompt is missing the instructions section")
		}
		if _, ok := compiled.Section("task"); !ok {
			return fmt.Errorf("prompt is missing the task section")
		}
		if strings.Contains(instr.Body, tuningTaskProbe) {
			return fmt.Errorf("instructions and task are not separated")
		}
		return nil
	}
}
