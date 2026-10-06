// Package adaptiveroute is the CTX-011 Adaptive Routing policy: evidence-driven model
// class selection that optimizes toward the smallest capable model.
//
// The harness chooses the class; a model never chooses itself. The policy is a pure,
// deterministic function of the deterministic router's baseline class and accumulated
// evaluation evidence (observed success or failure of a capability at a class). It never
// uses an opaque or learned model. Every decision exposes a fixed reason phrase and a
// deterministic summary of the evidence that supported it.
//
// An explicit operator override always wins, preserving the existing override and
// fallback semantics. The policy never downgrades below the router's baseline: it only
// escalates when the observed evidence shows the current class is not capable enough.
package adaptiveroute

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/model"
)

// Defaults for the evidence gate. A class is only judged when it has at least
// DefaultMinSample observations, and it is considered capable when its observed success
// rate is at least DefaultThreshold.
const (
	DefaultMinSample = 3
	DefaultThreshold = 0.75
)

// Fixed, deterministic reason phrases. They are never model-generated prose, so an
// adaptive decision stays auditable.
const (
	ReasonOverride           = "explicit model-class override"
	ReasonNoEvidence         = "no evaluation evidence for the baseline class; baseline class retained"
	ReasonBaselineSufficient = "baseline class met the success threshold; smallest capable class retained"
	ReasonEscalated          = "baseline class fell below the success threshold; escalated toward a capable class"
	ReasonExhausted          = "no class met the success threshold; largest class selected"
)

// Outcome is one observed evaluation outcome: whether a capability succeeded at a model
// class.
type Outcome struct {
	Class      model.Class
	Capability agent.Capability
	Success    bool
}

// Input is an evidence-driven routing request. Every field is either a deterministic
// router result, an operator value, or observed evidence.
type Input struct {
	// Baseline is the deterministic router's class (router.Decide). It bounds the
	// policy from below.
	Baseline model.Class
	// Capability scopes the evidence to comparable work.
	Capability agent.Capability
	// Override is an explicit operator model-class override; it wins when valid.
	Override model.Class
	// Evidence is the accumulated evaluation outcomes.
	Evidence []Outcome
	// MinSample and Threshold tune the evidence gate; zero values select the defaults.
	MinSample int
	Threshold float64
}

// Decision is the explainable routing outcome.
type Decision struct {
	// Class is the selected model class.
	Class model.Class
	// Baseline is the deterministic router's class, before adaptation.
	Baseline model.Class
	// Reason is the fixed phrase explaining the selection.
	Reason string
	// Evidence is a deterministic, human-readable summary of the evidence used.
	Evidence string
}

// Route selects a model class from the baseline and accumulated evidence. It is pure and
// deterministic: identical inputs yield an identical decision.
func Route(in Input) Decision {
	baseline := in.Baseline
	if !baseline.Valid() {
		baseline = model.ClassMedium
	}

	if in.Override.Valid() {
		return Decision{Class: in.Override, Baseline: baseline, Reason: ReasonOverride, Evidence: "override=" + string(in.Override)}
	}

	minSample := in.MinSample
	if minSample <= 0 {
		minSample = DefaultMinSample
	}
	threshold := in.Threshold
	if threshold <= 0 {
		threshold = DefaultThreshold
	}

	stats := tally(in.Evidence, in.Capability)

	class := baseline
	for {
		st := stats[class]
		if st.n < minSample || rate(st) >= threshold {
			break
		}
		next := upgrade(class)
		if next == class {
			break
		}
		class = next
	}

	st := stats[class]
	dec := Decision{Class: class, Baseline: baseline, Evidence: summarize(baseline, stats, minSample, threshold)}
	switch {
	case class == baseline && st.n < minSample:
		dec.Reason = ReasonNoEvidence
	case class == baseline && rate(st) >= threshold:
		dec.Reason = ReasonBaselineSufficient
	case class == baseline:
		// The baseline is already the largest class and is measured below the
		// threshold, so no class can be upgraded to.
		dec.Reason = ReasonExhausted
	case st.n >= minSample && rate(st) < threshold:
		// Escalated to the largest class, which is measured below the threshold.
		dec.Reason = ReasonExhausted
	default:
		dec.Reason = ReasonEscalated
	}
	return dec
}

// classStat is the observed sample count and success count for a class.
type classStat struct {
	n       int
	success int
}

// tally counts successes and samples per class for the given capability. Outcomes for
// other capabilities are ignored, so evidence is only used for comparable work.
func tally(outcomes []Outcome, capability agent.Capability) map[model.Class]classStat {
	stats := map[model.Class]classStat{}
	for _, o := range outcomes {
		if o.Capability != capability || !o.Class.Valid() {
			continue
		}
		st := stats[o.Class]
		st.n++
		if o.Success {
			st.success++
		}
		stats[o.Class] = st
	}
	return stats
}

// rate is the observed success rate, 0 when there is no evidence.
func rate(st classStat) float64 {
	if st.n == 0 {
		return 0
	}
	return float64(st.success) / float64(st.n)
}

// upgrade returns the next larger class, or the class itself at LARGE.
func upgrade(c model.Class) model.Class {
	switch c {
	case model.ClassSmall:
		return model.ClassMedium
	case model.ClassMedium:
		return model.ClassLarge
	default:
		return model.ClassLarge
	}
}

// summarize renders a deterministic, human-readable evidence summary. Classes are
// listed in the routing table's stable order, so the text never depends on map order.
func summarize(baseline model.Class, stats map[model.Class]classStat, minSample int, threshold float64) string {
	var parts []string
	for _, c := range model.Classes {
		st := stats[c]
		parts = append(parts, fmt.Sprintf("%s=%d/%d (%.2f)", c, st.success, st.n, rate(st)))
	}
	return fmt.Sprintf("baseline=%s; %s; threshold=%.2f; min_sample=%d", baseline, strings.Join(parts, "; "), threshold, minSample)
}
