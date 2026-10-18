// Bounded no-progress recovery (SOP-004).
//
// This file classifies a failed attempt, refuses blind retries of an unchanged
// failure, records why each recovery happened and what changed in the approach,
// picks a routing-respecting model fallback, and produces one actionable blocker
// report when recovery is exhausted. Like Decide it is pure: no I/O, no provider
// calls, no state transitions. Durable state (the previous outcome signature, the
// continuation count) is read by the caller (runpkg.Run.ReadAttempt,
// continuation.Consume) and passed in.
//
// # Classification
//
// Classify maps AttemptEvidence to exactly one AttemptClass, in this precedence:
//
//  1. AuthorizationFailure     DeniedTools > 0, or Kind APPROVAL_REQUIRED /
//     SECURITY_BOUNDARY
//  2. MissingPrerequisite      MissingPrerequisite names an absent runtime, tool,
//     or artifact
//  3. ImplementationNoProgress Kind NO_PROGRESS, a discovery-only attempt, or no
//     repository change
//  4. Productive               a repository change was observed
//
// Evidence inputs: failure kind and disposition (internal/failure), denied-tool
// count (toolharness OutcomeDenied), the missing prerequisite, whether the
// repository changed, the discovery-only flag, and the outcome signature. The
// classes are a recovery-local closed set built on internal/failure; no new
// failure.Kind is added.
//
// # Capability inventory
//
//   - typed failure vocabulary: EXISTS (internal/failure)
//   - deterministic recovery decision and class ladder: EXISTS (Decide, NextClass)
//   - unchanged-failure detection: PARTIAL in internal/cli/drive.go; the pure gate
//     is Recover here
//   - authorization / missing-prerequisite classification: MISSING before; Classify
//   - discovery-only detection: MISSING before; Classify
//   - recovery reason and material change record: MISSING before; Recovery
//   - model fallback: EXISTS (model.Resolve, Result.LocalFallback,
//     adaptiveroute.Route, router.Decide); Fallback consumes those outputs
//   - continuation limit persistence: PARTIAL in runpkg.Run continuations.txt;
//     the budget check is continuation.Consume
//   - single blocker report on exhaustion: MISSING before; Exhausted and Blockers
//
// CLI wiring lives in internal/cli/recovery_wiring.go and recoverTask: the seam
// records the recovery journal, refuses an unchanged/discovery-only retry, and
// applies the terminal no-progress block. This package remains pure.

package recovery

import (
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
)

// AttemptClass is the closed classification of a failed attempt.
type AttemptClass string

const (
	AuthorizationFailure     AttemptClass = "authorization_failure"
	MissingPrerequisite      AttemptClass = "missing_prerequisite"
	ImplementationNoProgress AttemptClass = "implementation_no_progress"
	Productive               AttemptClass = "productive"
)

// Valid reports whether c is a known attempt class.
func (c AttemptClass) Valid() bool {
	switch c {
	case AuthorizationFailure, MissingPrerequisite, ImplementationNoProgress, Productive:
		return true
	default:
		return false
	}
}

// Fixed reason phrases for no-progress recovery.
const (
	ReasonAuthorization       = "authorization failure; a permission must be granted or adjusted"
	ReasonMissingPrerequisite = "a required prerequisite is missing; it must be supplied"
	ReasonUnchangedFailure    = "the failure is unchanged since the previous attempt; it is not retried"
	ReasonNoMaterialChange    = "no material change in approach; the attempt is not retried"
	ReasonContinuationLimit   = "maximum continuations reached; a human decision is required"
	ReasonRoutingInactive     = "model routing is not configured; no fallback is selected"
	ReasonOverrideKept        = "an explicit operator class override is kept; no fallback is selected"
	ReasonCloudRefused        = "local-to-cloud fallback is not permitted"
	ReasonNoFallbackNeeded    = "productive work keeps its model"
	ReasonFallbackClass       = "no progress on this class; the next class is selected"
	ReasonFallbackLocal       = "local runtime unavailable; the configured cloud fallback is selected"
)

// Fixed material-change phrases. A recovery with an empty Change is refused.
const (
	ChangeClass         = "the model class changes"
	ChangeStrategy      = "the strategy is replanned on the same class"
	ChangeProgress      = "the repository changed since the previous attempt"
	ChangeNewEvidence   = "the failure signature changed since the previous attempt"
	ChangeLocalFallback = "the configured cloud fallback replaces the unavailable local runtime"
)

// AttemptEvidence is the typed evidence an attempt is classified from.
type AttemptEvidence struct {
	Kind        failure.Kind
	Disposition failure.Disposition
	// DeniedTools counts tool calls the harness denied (toolharness OutcomeDenied).
	DeniedTools int
	// MissingPrerequisite names an absent runtime, tool, or artifact.
	MissingPrerequisite string
	// RepoChanged reports whether the attempt changed the repository.
	RepoChanged bool
	// DiscoveryOnly reports read/search activity with no repository change.
	DiscoveryOnly bool
	// Signature is the attempt's outcome signature (runpkg.Run.RecordAttempt).
	Signature string
}

// Classify maps attempt evidence to exactly one class (see the precedence above).
func Classify(ev AttemptEvidence) AttemptClass {
	switch {
	case ev.DeniedTools > 0 || ev.Kind == failure.ApprovalRequired || ev.Kind == failure.SecurityBoundary:
		return AuthorizationFailure
	case ev.MissingPrerequisite != "":
		return MissingPrerequisite
	case ev.Kind == failure.NoProgress || ev.DiscoveryOnly || !ev.RepoChanged:
		return ImplementationNoProgress
	default:
		return Productive
	}
}

// Budget is one used/max counter.
type Budget struct {
	Used int `json:"used"`
	Max  int `json:"max"`
}

// Remaining reports whether the budget allows another use.
func (b Budget) Remaining() bool { return b.Used < b.Max }

// Budgets are the recovery counters a blocker report shows.
type Budgets struct {
	Escalations   Budget `json:"escalations"`
	Replans       Budget `json:"replans"`
	Continuations Budget `json:"continuations"`
}

// Recovery records one recovery step: why it happened and what materially
// changed in the approach.
type Recovery struct {
	Attempt   int          `json:"attempt"`
	Class     AttemptClass `json:"class"`
	Action    Action       `json:"action"`
	FromClass model.Class  `json:"from_class,omitempty"`
	ToClass   model.Class  `json:"to_class,omitempty"`
	Reason    string       `json:"reason"`
	Change    string       `json:"change,omitempty"`
}

// Recover decides whether a failed attempt may be retried. It reuses Decide for
// the base evidence, then applies the no-progress rules: authorization failures
// and missing prerequisites go to a human; productive work continues while the
// continuation budget lasts; an unchanged or discovery-only failure is never
// retried on the same approach. Every retry_same/replan/escalate outcome carries
// a non-empty Reason and Change.
func Recover(p Policy, ev AttemptEvidence, prevSignature string, base Evidence, continuations Budget) (Decision, Recovery) {
	class := Classify(ev)
	d := Decision{FromClass: base.Class, Attempt: base.Attempt}
	change := ""

	switch class {
	case AuthorizationFailure:
		d.Action, d.Reason = ActionHuman, ReasonAuthorization
	case MissingPrerequisite:
		d.Action, d.Reason = ActionHuman, ReasonMissingPrerequisite
	case Productive:
		// Productive work still passes through Decide first: a safety, approval,
		// plan-invalid, or unclassified condition outranks the productive shortcut.
		if d = Decide(p, base); d.Action == ActionHuman || d.Action == ActionBlock {
			break
		}
		if continuations.Remaining() {
			d.Action, d.ToClass, d.Reason, change = ActionRetrySame, base.Class, ReasonContinuation, ChangeProgress
		} else {
			d.Action, d.Reason = ActionHuman, ReasonContinuationLimit
		}
	default:
		d = Decide(p, base)
		unchanged := prevSignature != "" && ev.Signature == prevSignature
		if ev.DiscoveryOnly {
			// A discovery-only attempt made no repository change. Retrying it is not a
			// material change in approach, even when no previous signature exists or
			// escalation/replan is disabled.
			d.Action, d.ToClass, d.Reason = ActionHuman, "", ReasonNoMaterialChange
			break
		}
		switch d.Action {
		case ActionEscalate:
			change = ChangeClass
		case ActionReplan:
			change = ChangeStrategy
		case ActionRetrySame:
			// Retrying the same approach is only allowed on new evidence.
			if unchanged {
				d.Action, d.ToClass, d.Reason = ActionHuman, "", ReasonUnchangedFailure
			} else {
				change = ChangeNewEvidence
			}
		case ActionNone:
			// The caller's existing path would retry; an unchanged failure must not.
			if unchanged {
				d.Action, d.Reason = ActionHuman, ReasonUnchangedFailure
			}
		}
	}

	return d, Recovery{
		Attempt: d.Attempt, Class: class, Action: d.Action,
		FromClass: d.FromClass, ToClass: d.ToClass, Reason: d.Reason, Change: change,
	}
}

// Fallback selects a model fallback from existing routing outputs. It never
// re-derives routing: the class order comes from NextClass and the cloud
// candidate from res.LocalFallback. It refuses (ok=false) after an authorization
// failure, when routing is inactive, when an operator override is set, for
// productive work, past the escalation budget, and for a local-to-cloud move
// that allowCloud forbids. It never selects a class below baseline. A class
// fallback names only the class; the caller resolves the concrete model through
// model.Resolve.
func Fallback(p Policy, class AttemptClass, res model.Result, baseline, override model.Class, allowCloud bool, escalations int) (model.Selection, Recovery, bool) {
	cur := res.Selection.Class
	rec := Recovery{Class: class, Action: ActionHuman, FromClass: cur}
	refuse := func(reason string) (model.Selection, Recovery, bool) {
		rec.Reason = reason
		return model.Selection{}, rec, false
	}

	switch {
	case class == AuthorizationFailure:
		return refuse(ReasonAuthorization)
	case !res.Active:
		return refuse(ReasonRoutingInactive)
	case override != "":
		return refuse(ReasonOverrideKept)
	case class == Productive:
		return refuse(ReasonNoFallbackNeeded)
	}

	if class == MissingPrerequisite {
		if res.LocalFallback == nil {
			return refuse(ReasonMissingPrerequisite)
		}
		if !allowCloud {
			return refuse(ReasonCloudRefused)
		}
		rec.Action, rec.ToClass, rec.Reason, rec.Change = ActionRetrySame, res.LocalFallback.Class, ReasonFallbackLocal, ChangeLocalFallback
		return *res.LocalFallback, rec, true
	}

	next, ok := NextClass(cur)
	if !ok {
		return refuse(ReasonNoLargerClass)
	}
	if baseline != "" && !onLadder(baseline) {
		// A baseline outside the class ladder cannot become a fallback class.
		return refuse(ReasonNoLargerClass)
	}
	if !p.Enabled || escalations >= p.MaxEscalations {
		return refuse(ReasonEscalationLimit)
	}
	if !atLeast(next, baseline) {
		next = baseline
	}
	rec.Action, rec.ToClass, rec.Reason, rec.Change = ActionEscalate, next, ReasonFallbackClass, ChangeClass
	return model.Selection{Class: next, Source: res.Selection.Source, Fallback: true, Reason: ReasonFallbackClass}, rec, true
}

// onLadder reports whether c is one of the known model classes. An empty class is
// not on the ladder (and is handled separately by the caller).
func onLadder(c model.Class) bool {
	switch c {
	case model.ClassSmall, model.ClassMedium, model.ClassLarge:
		return true
	default:
		return false
	}
}

// atLeast reports whether c is base or above it on the NextClass ladder. An
// empty base imposes no floor.
func atLeast(c, base model.Class) bool {
	for k := base; k != ""; k, _ = NextClass(k) {
		if k == c {
			return true
		}
	}
	return base == ""
}

// BlockerReport is the single actionable report produced when recovery is
// exhausted or refused.
type BlockerReport struct {
	Task       string       `json:"task"`
	Class      AttemptClass `json:"class"`
	LastReason string       `json:"last_reason"`
	History    []Recovery   `json:"history"`
	Budgets    Budgets      `json:"budgets"`
	NextAction string       `json:"next_action"`
}

// Fixed next operator action per class.
const (
	NextGrantPermission     = "grant or adjust the denied permission, then retry the task"
	NextSupplyPrerequisite  = "supply the missing prerequisite, then retry the task"
	NextChangeApproach      = "change the task scope, capability, or model, then retry the task"
	NextReviewContinuations = "review the partial work, then raise max_continuations or split the task"
)

// Exhausted builds the blocker report for task. Consecutive duplicate history
// entries (the same recovery recorded twice by repeated exhaustion events)
// collapse into one.
func Exhausted(task string, history []Recovery, class AttemptClass, b Budgets) BlockerReport {
	r := BlockerReport{Task: task, Class: class, Budgets: b, History: []Recovery{}}
	for _, h := range history {
		if n := len(r.History); n > 0 && r.History[n-1] == h {
			continue
		}
		r.History = append(r.History, h)
	}
	if n := len(r.History); n > 0 {
		r.LastReason = r.History[n-1].Reason
	}
	switch class {
	case AuthorizationFailure:
		r.NextAction = NextGrantPermission
		if r.LastReason == "" {
			r.LastReason = ReasonAuthorization
		}
	case MissingPrerequisite:
		r.NextAction = NextSupplyPrerequisite
		if r.LastReason == "" {
			r.LastReason = ReasonMissingPrerequisite
		}
	case Productive:
		r.NextAction = NextReviewContinuations
		if r.LastReason == "" {
			r.LastReason = ReasonContinuationLimit
		}
	default:
		r.NextAction = NextChangeApproach
		if r.LastReason == "" {
			r.LastReason = ReasonNoMaterialChange
		}
	}
	return r
}

// Blockers holds at most one blocker report per task, so duplicate exhaustion
// events for the same task collapse into the first report.
type Blockers map[string]BlockerReport

// Record stores r unless task already has a report; it reports whether r was
// stored.
func (bs Blockers) Record(r BlockerReport) bool {
	if _, ok := bs[r.Task]; ok {
		return false
	}
	bs[r.Task] = r
	return true
}
