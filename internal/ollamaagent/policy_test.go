package ollamaagent

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// TestPolicyForIsCoherent asserts, table-driven, that every in-scope capability
// has a policy with an explicit ceiling, the expected read-only flag, and the
// expected tool surface. A change to any capability's MaxIterations, ReadOnly
// flag, or allowed tools without updating this table fails the test.
//
// PLAN and REVIEW use the derived totals planTotalTurns/reviewTotalTurns rather
// than re-deriving the sums here, so the test guards the totals the policy
// actually publishes instead of restating the expression they replaced.
func TestPolicyForIsCoherent(t *testing.T) {
	cases := []struct {
		cap        agent.Capability
		wantIter   int
		wantReadOn bool
		wantTools  []string
		forbid     []string
	}{
		{agent.Plan, planTotalTurns, true,
			[]string{toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles, toolharness.ToolGitStatus, toolharness.ToolGitDiff},
			[]string{toolharness.ToolWriteFile, toolharness.ToolCreateFile, toolharness.ToolRunCommand}},
		{agent.Review, reviewTotalTurns, true,
			[]string{toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles, toolharness.ToolGitStatus, toolharness.ToolGitDiff},
			[]string{toolharness.ToolWriteFile, toolharness.ToolCreateFile, toolharness.ToolRunCommand}},
		{agent.DesignTests, maxIterationsDesignTests, true,
			[]string{toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles, toolharness.ToolGitStatus, toolharness.ToolGitDiff},
			[]string{toolharness.ToolWriteFile, toolharness.ToolCreateFile, toolharness.ToolRunCommand}},
		{agent.DiagnoseFailure, maxIterationsDiagnose, true,
			[]string{toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles, toolharness.ToolRunCommand, toolharness.ToolGitStatus, toolharness.ToolGitDiff},
			[]string{toolharness.ToolWriteFile, toolharness.ToolCreateFile}},
		{agent.Implement, maxIterationsImplement, false,
			toolharness.Tools(), nil},
		{agent.Fix, maxIterationsFix, false,
			toolharness.Tools(), nil},
	}
	for _, tc := range cases {
		t.Run(string(tc.cap), func(t *testing.T) {
			p := PolicyFor(tc.cap)
			if p.MaxIterations != tc.wantIter {
				t.Errorf("MaxIterations = %d, want %d", p.MaxIterations, tc.wantIter)
			}
			if p.ReadOnly != tc.wantReadOn {
				t.Errorf("ReadOnly = %t, want %t", p.ReadOnly, tc.wantReadOn)
			}
			for _, tool := range tc.wantTools {
				if !p.Allows(tool) {
					t.Errorf("should allow %s", tool)
				}
			}
			for _, tool := range tc.forbid {
				if p.Allows(tool) {
					t.Errorf("must not allow %s", tool)
				}
			}
		})
	}

	// The phased capabilities carry explicit finalize bounds; every other
	// capability leaves them zero, since finalizeEligible is only reached for
	// IMPLEMENT/FIX.
	impl := PolicyFor(agent.Implement)
	if impl.FinalizeAfter != implementFinalizeAfter || impl.LateStageAfter != implementLateStageAfter ||
		impl.CompletionWindow != implementCompletionWindow || impl.ForceFinalizeAfter != implementForceFinalizeAfter ||
		impl.FinalizeTurns != implementFinalizeTurns {
		t.Errorf("IMPLEMENT finalize bounds = %+v, want the implement* constants", impl)
	}
	fix := PolicyFor(agent.Fix)
	if fix.FinalizeAfter != implementFinalizeAfter || fix.LateStageAfter != implementLateStageAfter ||
		fix.CompletionWindow != implementCompletionWindow || fix.ForceFinalizeAfter != fixForceFinalizeAfter ||
		fix.FinalizeTurns != fixFinalizeTurns {
		t.Errorf("FIX finalize bounds = %+v, want fix* force/finalize-turns and implement* soft bounds", fix)
	}
}

// TestUnknownCapabilityGetsConservativeReadOnlyDefault asserts a capability not
// named in PolicyFor cannot accidentally gain write access.
func TestUnknownCapabilityGetsConservativeReadOnlyDefault(t *testing.T) {
	p := PolicyFor(agent.Capability("NOT_A_REAL_CAPABILITY"))
	if p.MaxIterations != maxIterationsDefault {
		t.Errorf("MaxIterations = %d, want %d", p.MaxIterations, maxIterationsDefault)
	}
	if !p.ReadOnly {
		t.Error("the default policy must be read-only")
	}
	for _, tool := range []string{toolharness.ToolWriteFile, toolharness.ToolCreateFile, toolharness.ToolRunCommand} {
		if p.Allows(tool) {
			t.Errorf("the default policy must not allow %s", tool)
		}
	}
}

// TestHardCeilingsAndSoftThresholdsAreDistinct asserts the two kinds of bound are
// kept distinct and ordered as the lifecycle engines require. It encodes the
// invariant that soft thresholds steer a healthy run while hard ceilings stop one
// that would not stop: the force-finalize ceiling sits above every soft threshold,
// and the late-stage threshold sits above the finalize threshold.
func TestHardCeilingsAndSoftThresholdsAreDistinct(t *testing.T) {
	hardCeilings := []struct {
		name string
		val  int
	}{
		{"maxIterationsImplement", maxIterationsImplement},
		{"maxIterationsFix", maxIterationsFix},
		{"maxIterationsDesignTests", maxIterationsDesignTests},
		{"maxIterationsDiagnose", maxIterationsDiagnose},
		{"maxIterationsDefault", maxIterationsDefault},
		{"implementForceFinalizeAfter", implementForceFinalizeAfter},
		{"implementFinalizeTurns", implementFinalizeTurns},
	}
	for _, c := range hardCeilings {
		if c.val <= 0 {
			t.Errorf("hard ceiling %s = %d, want positive", c.name, c.val)
		}
	}

	softThresholds := []struct {
		name string
		val  int
	}{
		{"planDiscoveryTurns", planDiscoveryTurns},
		{"planSynthesisTurns", planSynthesisTurns},
		{"reviewInspectTurns", reviewInspectTurns},
		{"reviewInspectNudgeAfter", reviewInspectNudgeAfter},
		{"reviewSynthesizeTurns", reviewSynthesizeTurns},
		{"implementNudgeAfter", implementNudgeAfter},
		{"implementNowAfter", implementNowAfter},
		{"implementClosingAfter", implementClosingAfter},
		{"implementFinalizeAfter", implementFinalizeAfter},
		{"implementCompletionWindow", implementCompletionWindow},
		{"implementLateStageAfter", implementLateStageAfter},
	}
	for _, c := range softThresholds {
		if c.val <= 0 {
			t.Errorf("soft threshold %s = %d, want positive", c.name, c.val)
		}
	}

	// Soft thresholds must sit below the hard ceilings that would stop the run,
	// and the soft thresholds must be ordered so a healthy run is steered before
	// the ceiling is ever reached.
	if implementForceFinalizeAfter <= implementLateStageAfter {
		t.Errorf("implementForceFinalizeAfter = %d must sit above implementLateStageAfter = %d", implementForceFinalizeAfter, implementLateStageAfter)
	}
	if implementForceFinalizeAfter >= maxIterationsImplement {
		t.Errorf("implementForceFinalizeAfter = %d must sit below maxIterationsImplement = %d", implementForceFinalizeAfter, maxIterationsImplement)
	}
	if implementNudgeAfter >= implementNowAfter {
		t.Errorf("implementNudgeAfter = %d must sit below implementNowAfter = %d", implementNudgeAfter, implementNowAfter)
	}
	if implementNowAfter >= implementFinalizeAfter {
		t.Errorf("implementNowAfter = %d must sit below implementFinalizeAfter = %d", implementNowAfter, implementFinalizeAfter)
	}
	// The closing steering must land after the standard implement-now steering and
	// before the unmutated run is finalized, so it always precedes the cutoff.
	if implementClosingAfter <= implementNowAfter {
		t.Errorf("implementClosingAfter = %d must sit above implementNowAfter = %d", implementClosingAfter, implementNowAfter)
	}
	if implementClosingAfter >= implementLateStageAfter {
		t.Errorf("implementClosingAfter = %d must sit below implementLateStageAfter = %d", implementClosingAfter, implementLateStageAfter)
	}
	if implementFinalizeAfter >= implementLateStageAfter {
		t.Errorf("implementFinalizeAfter = %d must sit below implementLateStageAfter = %d", implementFinalizeAfter, implementLateStageAfter)
	}
	if reviewInspectNudgeAfter >= reviewInspectTurns {
		t.Errorf("reviewInspectNudgeAfter = %d must sit below reviewInspectTurns = %d", reviewInspectNudgeAfter, reviewInspectTurns)
	}
}

// TestCompletionPoliciesMatchEngines asserts each lifecycle capability's stated
// completion policy matches the phases the engines implement: PLAN and REVIEW run
// the two-phase engine (planTwoPhase / reviewTwoPhase), and IMPLEMENT and FIX run
// the phased engine with the same phase set.
func TestCompletionPoliciesMatchEngines(t *testing.T) {
	// PLAN: DISCOVERY → SYNTHESIS.
	if planTwoPhase.discoverLabel != "DISCOVERY" || planTwoPhase.synthLabel != "SYNTHESIS" {
		t.Errorf("PLAN labels = %q/%q, want DISCOVERY/SYNTHESIS", planTwoPhase.discoverLabel, planTwoPhase.synthLabel)
	}
	if planTwoPhase.discoveryTurns != planDiscoveryTurns || planTwoPhase.synthesisTurns != planSynthesisTurns {
		t.Error("PLAN phase bounds must come from the centralized policy")
	}
	if planTwoPhase.nudgeAfter != 0 {
		t.Errorf("PLAN nudgeAfter = %d, want 0 (no soft nudge)", planTwoPhase.nudgeAfter)
	}

	// REVIEW: INSPECT → SYNTHESIZE with a soft wrap-up nudge.
	if reviewTwoPhase.discoverLabel != "INSPECT" || reviewTwoPhase.synthLabel != "SYNTHESIZE" {
		t.Errorf("REVIEW labels = %q/%q, want INSPECT/SYNTHESIZE", reviewTwoPhase.discoverLabel, reviewTwoPhase.synthLabel)
	}
	if reviewTwoPhase.discoveryTurns != reviewInspectTurns || reviewTwoPhase.synthesisTurns != reviewSynthesizeTurns {
		t.Error("REVIEW phase bounds must come from the centralized policy")
	}
	if reviewTwoPhase.nudgeAfter != reviewInspectNudgeAfter {
		t.Errorf("REVIEW nudgeAfter = %d, want %d", reviewTwoPhase.nudgeAfter, reviewInspectNudgeAfter)
	}

	// IMPLEMENT/FIX: DISCOVER → CHANGE → FINALIZE.
	if implDiscover.label() != "DISCOVER" || implChange.label() != "CHANGE" || implFinalize.label() != "FINALIZE" {
		t.Errorf("phased engine labels = %q/%q/%q, want DISCOVER/CHANGE/FINALIZE",
			implDiscover.label(), implChange.label(), implFinalize.label())
	}
	if newExecutionState().phase != implDiscover {
		t.Error("a fresh execution state must start in DISCOVER")
	}

	// A soft threshold alone must never finalize an unmutated run: crossing the
	// finalize threshold without a mutation does not finalize, and the run keeps
	// going until the late-stage decision point.
	implPolicy := PolicyFor(agent.Implement)
	unmutated := newExecutionState()
	unmutated.counters.interactions = implementFinalizeAfter + 1
	if unmutated.finalizeEligible(1, implPolicy) {
		t.Error("an unmutated run must not finalize on a count alone before the late stage")
	}
	unmutated.counters.interactions = implementFinalizeAfter + 1
	if unmutated.finalizeEligible(implementForceFinalizeAfter, implPolicy) {
		t.Error("an unmutated run must not force-finalize even past the force ceiling")
	}
	unmutated.counters.interactions = implementLateStageAfter
	if !unmutated.finalizeEligible(1, implPolicy) {
		t.Error("an unmutated run must finalize at the late-stage decision point")
	}

	// A run still writing must not be finalized mid-change: a recent mutation
	// keeps it inside the completion window even after the finalize threshold. The
	// force-finalization ceiling still applies, however — it reserves the final
	// turns so a model that will not stop writing cannot consume the whole budget.
	stillWriting := newExecutionState()
	stillWriting.observeMutation()
	stillWriting.counters.interactions = implementFinalizeAfter + 1
	stillWriting.counters.sinceMutation = implementCompletionWindow - 1
	if stillWriting.finalizeEligible(1, implPolicy) {
		t.Error("a run still writing must not be finalized mid-change")
	}
	if !stillWriting.finalizeEligible(implementForceFinalizeAfter, implPolicy) {
		t.Error("the force-finalization ceiling must reserve the final turns even for a run that is still writing")
	}

	// A run that mutated and then stopped writing may finalize at the finalize
	// threshold, once the completion window has elapsed.
	stoppedWriting := newExecutionState()
	stoppedWriting.observeMutation()
	stoppedWriting.counters.interactions = implementFinalizeAfter
	stoppedWriting.counters.sinceMutation = implementCompletionWindow
	if !stoppedWriting.finalizeEligible(1, implPolicy) {
		t.Error("a run that stopped writing must finalize at the finalize threshold")
	}
}
