package runtrace

import "strings"

// Progress signals classify evidence SOP already owns into objectively-backed
// forms of progress. They are observation-only: nothing in this file feeds a
// stale counter, a retry/block/replan decision, a budget, a human boundary, or a
// termination reason. Activity (a tool call, a command, a read, a repeated
// inspection, a model narration) is NOT progress; only evidence a producer has
// already substantiated is a signal.

// SignalType is the closed category of a progress signal.
type SignalType string

const (
	// DiscoverySignal: a first-seen, informative inspection.
	DiscoverySignal SignalType = "DISCOVERY"
	// MutationSignal: a verified, invocation-attributed repository change.
	MutationSignal SignalType = "REPOSITORY_MUTATION"
	// VerificationSignal: a deterministic check that executed and passed.
	VerificationSignal SignalType = "VERIFICATION"
	// TransitionSignal: an observed lifecycle stage advancement.
	TransitionSignal SignalType = "STATE_TRANSITION"
)

// Signal subtypes. They are conceptual and intentionally coarse.
const (
	SubtypeNovelInspection   = "novel_inspection"
	SubtypeChangedFile       = "changed_file"
	SubtypeValidationPassed  = "validation_passed"
	SubtypeLifecycleAdvanced = "lifecycle_advanced"
)

// ProgressSignal is one observed, objectively-backed form of progress. Evidence
// is a concise reference (a path, a command, a phase), never raw output.
type ProgressSignal struct {
	Sequence int `json:"sequence"`
	// Iteration is the trace iteration the signal derives from, when applicable.
	Iteration int        `json:"iteration,omitempty"`
	Phase     string     `json:"phase,omitempty"`
	Type      SignalType `json:"type"`
	Subtype   string     `json:"subtype,omitempty"`
	Evidence  string     `json:"evidence,omitempty"`
}

// ProgressCounts summarizes the signals by category.
type ProgressCounts struct {
	Discovery           int `json:"discovery"`
	RepositoryMutations int `json:"repository_mutations"`
	Verification        int `json:"verification"`
	StateTransitions    int `json:"state_transitions"`
}

// transitionPhases are the lifecycle stage names that represent an advancement.
// VALIDATE is deliberately excluded: its activity stage is shared with an agent
// running a validation command (activity, not a lifecycle transition), so it
// cannot be told apart from the stream; validation progress is carried by the
// VERIFICATION signals instead.
var transitionPhases = map[string]bool{
	"PLAN": true, "IMPLEMENT": true, "REVIEW": true, "FIX": true,
	"QUALITY": true, "COMPLETE": true, "FAILED": true, "BLOCKED": true,
}

// SignalDiscoveryNovel and signalMutationVerified mirror the producer-set markers
// on the activity stream.
const (
	signalDiscoveryNovel   = "discovery.novel"
	signalMutationVerified = "mutation.verified"
)

// classifyProgress derives the progress signals from the run evidence. Discovery
// and transition signals come from the observed iterations; mutation signals come
// from the harness-verified mutations when present, else from the
// invocation-attributed changed files; verification signals come from the
// validation results.
func classifyProgress(in Inputs) []ProgressSignal {
	var out []ProgressSignal
	seenDiscovery := make(map[string]bool)
	verifiedMutation := false

	for _, it := range in.Iterations {
		switch {
		case it.Signal == signalDiscoveryNovel:
			// The producer already decided novelty; dedupe identical identities
			// defensively so a repeated marker cannot multiply signals.
			id := it.Action + "\x00" + it.Observation
			if seenDiscovery[id] {
				continue
			}
			seenDiscovery[id] = true
			out = append(out, ProgressSignal{
				Iteration: it.Sequence, Phase: it.Phase,
				Type: DiscoverySignal, Subtype: SubtypeNovelInspection, Evidence: it.Observation,
			})
		case it.Signal == signalMutationVerified:
			verifiedMutation = true
			out = append(out, ProgressSignal{
				Iteration: it.Sequence, Phase: it.Phase,
				Type: MutationSignal, Subtype: SubtypeChangedFile, Evidence: it.Observation,
			})
		case transitionPhases[it.Phase]:
			out = append(out, ProgressSignal{
				Iteration: it.Sequence, Phase: it.Phase,
				Type: TransitionSignal, Subtype: SubtypeLifecycleAdvanced, Evidence: it.Phase,
			})
		}
	}

	// When the agent did not report a per-mutation marker (for example a command
	// agent), the invocation-attributed changed files are the mutation evidence.
	if !verifiedMutation {
		for _, f := range in.ChangedFiles {
			out = append(out, ProgressSignal{Type: MutationSignal, Subtype: SubtypeChangedFile, Evidence: f})
		}
	}

	for _, v := range in.Verification {
		if strings.EqualFold(v.Status, "PASS") {
			out = append(out, ProgressSignal{Type: VerificationSignal, Subtype: SubtypeValidationPassed, Evidence: v.Command})
		}
	}

	for i := range out {
		out[i].Sequence = i + 1
	}
	return out
}

// summarize counts the signals by category.
func summarize(ps []ProgressSignal) ProgressCounts {
	var c ProgressCounts
	for _, p := range ps {
		switch p.Type {
		case DiscoverySignal:
			c.Discovery++
		case MutationSignal:
			c.RepositoryMutations++
		case VerificationSignal:
			c.Verification++
		case TransitionSignal:
			c.StateTransitions++
		}
	}
	return c
}
