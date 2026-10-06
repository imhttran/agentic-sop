package cli

import "testing"

// TestEndToEndSkillEvaluation reuses the canonical command fixtures instead of
// implementing a skill-side scheduler, recovery loop, router, or approval policy.
// The natural-language adapter's contract is checked in internal/skill; these
// scenarios prove the SOP behavior it must delegate to and report faithfully.
func TestEndToEndSkillEvaluation(t *testing.T) {
	scenarios := []struct {
		name   string
		checks []func(*testing.T)
	}{
		{"01_new_repository", []func(*testing.T){TestRunNamedPlan}},
		{"02_partial_plan", []func(*testing.T){TestRunPlainRunResumesActivePlan, TestRunResumesActiveTaskInsteadOfStartingAnother}},
		{"03_recoverable_block", []func(*testing.T){TestRecoveryContinuesThroughPlan, TestRecoveryDoesNotRepeatSameTask}},
		{"04_unrecoverable_block", []func(*testing.T){TestRecoveryFailureStopsInvocation, TestAutomaticRecoveryDoesNotBypassDependencies, TestRunRetryBudgetExhausted}},
		{"05_approval_required", []func(*testing.T){TestRunActiveApprovalRequestStillBlocks, TestApproveDoesNotBypassGates}},
		{"06_needs_human", []func(*testing.T){TestRunWaitingForHumanStageStaysHuman, TestNeedsHumanClassificationWithoutRequestIsNotApproval}},
		{"07_false_implementation_completion", []func(*testing.T){TestRunImplementClaimsChangesButNone, TestRunVerifiedAlreadySatisfied}},
		{"08_validation_and_bounded_fix", []func(*testing.T){TestRunFixLoopRepairsFailingValidation, TestRunFixLoopExhausted}},
		{"09_configured_local_fallback", []func(*testing.T){TestRoutingSmallFallsBackToCloudWhenLocalUnavailable}},
		{"10_completed_plan", []func(*testing.T){TestRunSamePlanContinuesAfterCompletion, TestRecoverySkipsCompletedTasksAfterSuccess}},
		{"11_resume_and_reconcile", []func(*testing.T){TestRunResumesInterruptedActiveTask, TestRunReconcileChangedExecutedStops, TestRunReconcileListChangedIsReadOnly, TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory}},
		{"12_provider_failure", []func(*testing.T){TestLocalFallbackIsNotGenerationRecovery, TestApplyTaskRoutingFailsClosedWithoutFactory}},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			for i, check := range scenario.checks {
				t.Run(string(rune('A'+i)), check)
			}
		})
	}
}
