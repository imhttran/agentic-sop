// Package worktreecleanup implements SOP's deterministic, model-free classification
// model for the /sop-cleanup skill (skills/sop-cleanup).
//
// It is pure: given the observable facts an inspector collects read-only, it
// assigns each dirty/relevant path exactly one primary classification, a proposed
// action, and a confidence, then derives a single assessment disposition. It never
// mutates the repository, never calls a model, and never decides to delete anything:
// a destructive action is only ever *proposed* and requires explicit human approval.
//
// The skill (skills/sop-cleanup/SKILL.md) documents this model; this package is its
// executable, testable form.
package worktreecleanup

// Class is the single primary classification of one worktree item.
type Class string

const (
	ClassTaskOutput           Class = "TASK_OUTPUT"
	ClassActiveWork           Class = "ACTIVE_WORK"
	ClassUserOwned            Class = "USER_OWNED"
	ClassHistoricalEvidence   Class = "HISTORICAL_EVIDENCE"
	ClassSOPLifecycleEvidence Class = "SOP_LIFECYCLE_EVIDENCE"
	ClassGenerated            Class = "GENERATED"
	ClassStaleArtifact        Class = "STALE_ARTIFACT"
	ClassCompletedPlan        Class = "COMPLETED_PLAN"
	ClassActivePlan           Class = "ACTIVE_PLAN"
	ClassNewPlan              Class = "NEW_PLAN"
	ClassUnrelatedChange      Class = "UNRELATED_CHANGE"
	ClassUnknown              Class = "UNKNOWN"
)

// Action is the proposed disposition of one item. Only delete_after_approval and
// move_to_history are destructive, and each requires explicit human approval.
type Action string

const (
	ActionPreserve            Action = "preserve"
	ActionCommitWithTask      Action = "commit_with_task"
	ActionDeleteAfterApproval Action = "delete_after_approval"
	ActionMoveToHistory       Action = "move_to_history"
	ActionHumanDecision       Action = "human_decision"
)

// Confidence records how strong the evidence behind a classification is.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Disposition is the single result of an assessment.
type Disposition string

const (
	DispositionClean                    Disposition = "CLEAN"
	DispositionProposalReady            Disposition = "CLEANUP_PROPOSAL_READY"
	DispositionApprovalRequired         Disposition = "CLEANUP_APPROVAL_REQUIRED"
	DispositionBlockedUnknownOwnership  Disposition = "BLOCKED_UNKNOWN_OWNERSHIP"
	DispositionBlockedLifecycleConflict Disposition = "BLOCKED_LIFECYCLE_CONFLICT"
)

// Facts are the observable facts about one dirty/relevant path, gathered read-only
// by the inspector. Every field is evidence the inspector can establish without
// guessing; classification is a pure function of them.
type Facts struct {
	Path     string
	GitState string // e.g. "M", "??", "A "

	// SOP lifecycle evidence (protected).
	LifecycleEvidence bool

	// In-progress work.
	ActiveWork bool

	// Plan documents.
	PlanDoc           bool
	PlanState         string // "ACTIVE" | "NEW" | "COMPLETED" | ""
	LifecycleComplete bool
	HistoryConvention bool
	AlreadyInHistory  bool

	// Task output.
	DeliverableOf string
	TaskReport    bool

	// Prior, deliberately kept reports/history.
	HistoricalEvidence bool

	// Provenance.
	GeneratedConvention bool
	Unrelated           bool
	ProvenUnreferenced  bool
	KnownOwner          bool
}

// Item is one classified worktree item.
type Item struct {
	Path       string     `json:"path"`
	GitState   string     `json:"git_state"`
	Class      Class      `json:"class"`
	Evidence   string     `json:"evidence"`
	Action     Action     `json:"action"`
	Confidence Confidence `json:"confidence"`
}

// Classify assigns one classification, proposed action, and confidence to a path
// from its facts. The rules are ordered by precedence: protected lifecycle
// evidence wins; a proven-orphan delete is only ever *proposed*.
func Classify(f Facts) Item {
	it := Item{Path: f.Path, GitState: f.GitState}
	switch {
	case f.LifecycleEvidence:
		it.Class, it.Action, it.Confidence = ClassSOPLifecycleEvidence, ActionPreserve, ConfidenceHigh
		it.Evidence = "SOP run/archive/lifecycle evidence (protected); never delete or rewrite"

	case f.ActiveWork:
		it.Class, it.Action, it.Confidence = ClassActiveWork, ActionPreserve, ConfidenceHigh
		it.Evidence = "in-progress work; preserve"

	case f.PlanDoc && f.PlanState == "ACTIVE":
		it.Class, it.Action, it.Confidence = ClassActivePlan, ActionPreserve, ConfidenceHigh
		it.Evidence = "active SOP plan document; preserve"

	case f.PlanDoc && f.PlanState == "NEW":
		it.Class, it.Action, it.Confidence = ClassNewPlan, ActionPreserve, ConfidenceHigh
		it.Evidence = "planned (not yet active) SOP plan document; preserve"

	case f.PlanDoc && f.PlanState == "COMPLETED":
		it.Class, it.Confidence = ClassCompletedPlan, ConfidenceHigh
		switch {
		case f.AlreadyInHistory:
			it.Action = ActionPreserve
			it.Evidence = "completed plan already under the repository history convention; preserve"
		case f.LifecycleComplete && f.HistoryConvention:
			it.Action = ActionMoveToHistory
			it.Evidence = "machine lifecycle COMPLETE; no active association; docs/history convention present"
		default:
			it.Action = ActionPreserve
			it.Evidence = "completed plan; lifecycle/history precondition unmet; preserve"
		}

	case f.DeliverableOf != "":
		it.Class, it.Action, it.Confidence = ClassTaskOutput, ActionCommitWithTask, ConfidenceHigh
		it.Evidence = "SOP task deliverable of " + f.DeliverableOf

	case f.TaskReport:
		it.Class, it.Action, it.Confidence = ClassTaskOutput, ActionCommitWithTask, ConfidenceHigh
		it.Evidence = "SOP task report/deliverable"

	case f.HistoricalEvidence:
		it.Class, it.Action, it.Confidence = ClassHistoricalEvidence, ActionPreserve, ConfidenceHigh
		it.Evidence = "prior evidence kept for the record; preserve, do not rewrite"

	case f.GeneratedConvention:
		it.Class, it.Action, it.Confidence = ClassGenerated, ActionPreserve, ConfidenceMedium
		it.Evidence = "matches a known generated-file convention (gitignore-eligible)"

	case f.ProvenUnreferenced:
		it.Class, it.Action, it.Confidence = ClassStaleArtifact, ActionDeleteAfterApproval, ConfidenceMedium
		it.Evidence = "proven unreferenced: no imports, no plan/task ownership; delete only after approval"

	case f.Unrelated:
		it.Class, it.Action, it.Confidence = ClassUnrelatedChange, ActionPreserve, ConfidenceHigh
		it.Evidence = "unrelated to SOP scope; operator-owned; preserve"

	case f.KnownOwner:
		it.Class, it.Action, it.Confidence = ClassUserOwned, ActionPreserve, ConfidenceMedium
		it.Evidence = "known non-SOP owner; preserve"

	default:
		it.Class, it.Action, it.Confidence = ClassUnknown, ActionHumanDecision, ConfidenceLow
		it.Evidence = "ownership not established; never delete automatically"
	}
	return it
}

// Assessment is the result of classifying a set of items.
type Assessment struct {
	Items       []Item      `json:"items"`
	Disposition Disposition `json:"disposition"`
}

// Assess derives the single assessment disposition. A lifecycle conflict dominates;
// an UNKNOWN item blocks on ownership; a proposed destructive action requires
// approval; otherwise the presence of items yields a ready proposal and no items is
// CLEAN.
func Assess(items []Item, lifecycleConflict bool) Assessment {
	a := Assessment{Items: items}
	switch {
	case lifecycleConflict:
		a.Disposition = DispositionBlockedLifecycleConflict
	case hasClass(items, ClassUnknown):
		a.Disposition = DispositionBlockedUnknownOwnership
	case requiresApproval(items):
		a.Disposition = DispositionApprovalRequired
	case len(items) > 0:
		a.Disposition = DispositionProposalReady
	default:
		a.Disposition = DispositionClean
	}
	return a
}

func hasClass(items []Item, c Class) bool {
	for _, it := range items {
		if it.Class == c {
			return true
		}
	}
	return false
}

func requiresApproval(items []Item) bool {
	for _, it := range items {
		if it.Action == ActionDeleteAfterApproval || it.Action == ActionMoveToHistory {
			return true
		}
	}
	return false
}

// Baseline is the assessment-time identity used to detect drift before APPLY.
type Baseline struct {
	HEAD   string
	Status string
}

// ReassessRequired reports whether the repository changed materially since the
// assessment: an apply step must stop and reassess rather than act on stale facts.
func ReassessRequired(baseline, current Baseline) bool {
	return baseline.HEAD != current.HEAD || baseline.Status != current.Status
}

// CommitPolicy encodes the boundary that approval to commit never implies approval to
// push: a push requires both an approved commit and an explicit approved push.
func CommitPolicy(commitApproved, pushApproved bool) (commit, push bool) {
	return commitApproved, commitApproved && pushApproved
}
