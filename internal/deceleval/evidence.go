package deceleval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/failure"
)

// EvidenceFor maps a scenario descriptor to SOP's real deterministic
// classification Evidence. It is the descriptor-to-fixture mapping used by both
// the baseline (A) and, later, the advisory (B) runner, so both run the SAME
// classification path (failure.Classify). It defines no policy and calls no model.
//
// An unmappable descriptor returns an error, which the runner records as a
// FIXTURE FAILURE (missing evidence), never as a classification outcome.
func EvidenceFor(s Scenario) (failure.Evidence, error) {
	obs := strings.ToLower(s.Observable)
	switch s.Category {
	case "security_sensitive":
		return failure.Evidence{Source: "IMPLEMENT", SecurityBoundary: true}, nil
	case "governance_boundary":
		if strings.Contains(obs, "approval") || strings.Contains(obs, "bypass") {
			return failure.Evidence{Source: "IMPLEMENT", ApprovalRequired: true, ApprovalReason: "the change crosses an approval boundary"}, nil
		}
		return failure.Evidence{Source: "IMPLEMENT", DestructiveOperation: true}, nil
	case "ambiguous":
		if s.SeedKind == "NO_PROGRESS" {
			// A recorded no-progress stop: the harness made no repository progress.
			return failure.Evidence{Source: "IMPLEMENT", Err: errors.New("IMPLEMENT_NO_PROGRESS: no repository progress after retries")}, nil
		}
		return failure.Evidence{Source: "IMPLEMENT", RequirementsConflict: true}, nil
	case "missing_validation":
		if strings.Contains(obs, "no deterministic check") || strings.Contains(obs, "unverifiable") {
			return failure.Evidence{Source: "VALIDATE", RequirementsConflict: true}, nil
		}
		return failure.Evidence{Source: "VALIDATE", ValidationRequired: true, ValidationConfigured: false}, nil
	case "build_dependency":
		return failure.Evidence{Source: "VALIDATE", BuildFailed: true, Detail: "compile error"}, nil
	case "test_regression":
		if strings.Contains(obs, "flaky") || strings.Contains(obs, "intermittent") || strings.Contains(obs, "timing") {
			return failure.Evidence{Source: "VALIDATE", Err: errors.New("transient test timeout")}, nil
		}
		return failure.Evidence{Source: "VALIDATE", TestFailed: true, Detail: "a deterministic test failed"}, nil
	case "benign_impl":
		if s.SeedKind == "INCOMPLETE_IMPLEMENTATION" {
			return failure.Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "the implementation is unfinished"}}, nil
		}
		return failure.Evidence{Source: "IMPLEMENT", ChangeRequired: true, MutationObserved: false, NoChangesProduced: true}, nil
	case "provider_fault":
		return failure.Evidence{Source: "IMPLEMENT", Err: errors.New("provider unavailable: transient network error")}, nil
	default:
		return failure.Evidence{}, fmt.Errorf("deceleval: no evidence mapping for category %q", s.Category)
	}
}
