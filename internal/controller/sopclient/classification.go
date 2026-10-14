package sopclient

import "strings"

// Display-only map from SOP's classification Kind to a coarse category for
// styling. It never reclassifies or reads the disposition. Kinds this map does
// not know are CategoryUnknown, never guessed.

// Category is a display-only grouping of SOP's classification Kind.
type Category string

const (
	// CategoryProvider: the model, transport, or provider call failed.
	CategoryProvider Category = "PROVIDER"
	// CategoryDeterministic: tests, validation, build, or review will keep failing
	// until the code changes.
	CategoryDeterministic Category = "DETERMINISTIC"
	// CategoryBudget: tool/iteration/retry budget exhausted; not a human boundary.
	CategoryBudget Category = "BUDGET"
	// CategoryHuman: SOP classified the failure as needing a human.
	CategoryHuman Category = "HUMAN"
	// CategoryUnknown: Kind absent or unrecognized.
	CategoryUnknown Category = "UNKNOWN"
)

// Kind values observed in SOP's artifacts; agentic-sop owns the full set.
const (
	// Provider / transient infrastructure kinds.
	KindTransientProvider = "TRANSIENT_PROVIDER"
	KindProviderError     = "PROVIDER_ERROR"
	KindModelError        = "MODEL_ERROR"

	// Deterministic code/validation kinds.
	KindTestFailure              = "TEST_FAILURE"
	KindValidationFailure        = "VALIDATION_FAILURE"
	KindBuildFailure             = "BUILD_FAILURE"
	KindIncompleteImplementation = "INCOMPLETE_IMPLEMENTATION"

	// Tool/iteration-budget exhaustion kinds.
	KindToolBudgetExhausted      = "TOOL_BUDGET_EXHAUSTED"
	KindIterationBudgetExhausted = "ITERATION_BUDGET_EXHAUSTED"
	KindRetryBudgetExhausted     = "RETRY_BUDGET_EXHAUSTED"

	// Human-boundary kinds.
	KindAmbiguousContract = "AMBIGUOUS_CONTRACT"
	KindHumanRequired     = "HUMAN_REQUIRED"

	KindReplanRequired = "REPLAN_REQUIRED"
)

// CategoryOf maps a Kind to a Category; empty or unknown is CategoryUnknown.
func CategoryOf(kind string) Category {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case KindTransientProvider, KindProviderError, KindModelError:
		return CategoryProvider
	case KindTestFailure, KindValidationFailure, KindBuildFailure, KindIncompleteImplementation:
		return CategoryDeterministic
	case KindToolBudgetExhausted, KindIterationBudgetExhausted, KindRetryBudgetExhausted:
		return CategoryBudget
	case KindAmbiguousContract, KindHumanRequired:
		return CategoryHuman
	default:
		return CategoryUnknown
	}
}

// Category maps c.Kind; nil is unknown. The disposition is never consulted.
func (c *Classification) Category() Category {
	if c == nil {
		return CategoryUnknown
	}
	return CategoryOf(c.Kind)
}

// Label renders a Category; unknown reads "Unknown", never a pass.
func (c Category) Label() string {
	switch c {
	case CategoryProvider:
		return "Provider"
	case CategoryDeterministic:
		return "Code or validation"
	case CategoryBudget:
		return "Budget exhausted"
	case CategoryHuman:
		return "Human"
	default:
		return "Unknown"
	}
}
