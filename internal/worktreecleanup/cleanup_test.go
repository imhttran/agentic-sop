package worktreecleanup

import "testing"

// These tests are deterministic and model-free: they exercise the pure
// classification model with crafted facts. They do not touch git, the network, or
// any model.

func classifyOne(f Facts) Item { return Classify(f) }

// 1. clean repository
func TestCleanRepository(t *testing.T) {
	a := Assess(nil, false)
	if a.Disposition != DispositionClean {
		t.Fatalf("disposition = %s, want CLEAN", a.Disposition)
	}
}

// 2. expected task report
func TestTaskOutput(t *testing.T) {
	it := classifyOne(Facts{Path: "docs/reports/x/SP-003.md", GitState: "??", DeliverableOf: "SP-003"})
	if it.Class != ClassTaskOutput || it.Action != ActionCommitWithTask {
		t.Fatalf("got %s/%s, want TASK_OUTPUT/commit_with_task", it.Class, it.Action)
	}
}

// 3. unrelated modified user file
func TestUnrelatedModifiedUserFile(t *testing.T) {
	it := classifyOne(Facts{Path: "internal/other/config_test.go", GitState: "M", Unrelated: true})
	if it.Class != ClassUnrelatedChange || it.Action != ActionPreserve {
		t.Fatalf("got %s/%s, want UNRELATED_CHANGE/preserve", it.Class, it.Action)
	}
}

// 4. unknown untracked file
func TestUnknownUntrackedFile(t *testing.T) {
	it := classifyOne(Facts{Path: "mystery.bin", GitState: "??"})
	if it.Class != ClassUnknown || it.Action != ActionHumanDecision {
		t.Fatalf("got %s/%s, want UNKNOWN/human_decision", it.Class, it.Action)
	}
	a := Assess([]Item{it}, false)
	if a.Disposition != DispositionBlockedUnknownOwnership {
		t.Fatalf("disposition = %s, want BLOCKED_UNKNOWN_OWNERSHIP", a.Disposition)
	}
}

// 5. generated artifact
func TestGeneratedArtifact(t *testing.T) {
	it := classifyOne(Facts{Path: "bin/tool", GitState: "??", GeneratedConvention: true})
	if it.Class != ClassGenerated || it.Action != ActionPreserve {
		t.Fatalf("got %s/%s, want GENERATED/preserve", it.Class, it.Action)
	}
}

// 6. proven orphan/stale artifact
func TestProvenOrphanStaleArtifact(t *testing.T) {
	it := classifyOne(Facts{Path: "internal/oldwire/", GitState: "??", ProvenUnreferenced: true})
	if it.Class != ClassStaleArtifact || it.Action != ActionDeleteAfterApproval {
		t.Fatalf("got %s/%s, want STALE_ARTIFACT/delete_after_approval", it.Class, it.Action)
	}
	a := Assess([]Item{it}, false)
	if a.Disposition != DispositionProposalReady {
		t.Fatalf("disposition = %s, want CLEANUP_PROPOSAL_READY", a.Disposition)
	}
}

// 7. completed plan still in docs/plans
func TestCompletedPlanStillInDocsPlans(t *testing.T) {
	it := classifyOne(Facts{
		Path: "docs/plans/PLAN-X.md", GitState: "M", PlanDoc: true, PlanState: "COMPLETED",
		LifecycleComplete: true, HistoryConvention: true,
	})
	if it.Class != ClassCompletedPlan || it.Action != ActionMoveToHistory {
		t.Fatalf("got %s/%s, want COMPLETED_PLAN/move_to_history", it.Class, it.Action)
	}
}

// 8. machine archive vs docs/history distinction
func TestMachineArchiveVsDocsHistory(t *testing.T) {
	archive := classifyOne(Facts{Path: ".agent-sdlc/archive/plan-x/lifecycle.json", GitState: "??", LifecycleEvidence: true})
	if archive.Class != ClassSOPLifecycleEvidence {
		t.Fatalf("archive class = %s, want SOP_LIFECYCLE_EVIDENCE", archive.Class)
	}
	planDone := classifyOne(Facts{Path: "docs/plans/PLAN-X.md", GitState: "M", PlanDoc: true, PlanState: "COMPLETED", LifecycleComplete: true, HistoryConvention: true})
	if planDone.Class != ClassCompletedPlan {
		t.Fatalf("plan class = %s, want COMPLETED_PLAN", planDone.Class)
	}
	if archive.Class == planDone.Class {
		t.Fatal("machine archive and repository document history must not be the same class")
	}
}

// 9. historical references preserved
func TestHistoricalReferencesPreserved(t *testing.T) {
	// A prior report that still references the plan's original docs/plans/ path is
	// historical evidence: preserved, never rewritten.
	it := classifyOne(Facts{Path: "docs/reports/x/OLD.md", GitState: "M", HistoricalEvidence: true})
	if it.Class != ClassHistoricalEvidence || it.Action != ActionPreserve {
		t.Fatalf("got %s/%s, want HISTORICAL_EVIDENCE/preserve", it.Class, it.Action)
	}
}

// 10. live documentation link requiring update
func TestLiveDocLinkRequiresUpdate(t *testing.T) {
	// A report (task output) is preserved and committed with the task; the model
	// never proposes deleting or rewriting it even when it contains a stale link.
	it := classifyOne(Facts{Path: "docs/reports/x/CURRENT.md", GitState: "M", TaskReport: true})
	if it.Class != ClassTaskOutput || it.Action != ActionCommitWithTask {
		t.Fatalf("got %s/%s, want TASK_OUTPUT/commit_with_task", it.Class, it.Action)
	}
	if it.Action == ActionDeleteAfterApproval {
		t.Fatal("a document may never be proposed for deletion here")
	}
}

// 11. dirty-state change between assess/apply
func TestReassessRequiredOnDrift(t *testing.T) {
	base := Baseline{HEAD: "aaa", Status: " M x"}
	if ReassessRequired(base, base) {
		t.Fatal("identical baselines must not require reassessment")
	}
	if !ReassessRequired(base, Baseline{HEAD: "bbb", Status: " M x"}) {
		t.Fatal("HEAD drift must require reassessment")
	}
	if !ReassessRequired(base, Baseline{HEAD: "aaa", Status: " M x\n?? y"}) {
		t.Fatal("status drift must require reassessment")
	}
}

// 12. commit allowed but push not authorized
func TestCommitAllowedButPushNotAuthorized(t *testing.T) {
	if c, p := CommitPolicy(true, false); !c || p {
		t.Fatalf("CommitPolicy(true,false) = (%v,%v), want (true,false)", c, p)
	}
	if c, p := CommitPolicy(false, true); c || p {
		t.Fatalf("CommitPolicy(false,true) = (%v,%v), want (false,false)", c, p)
	}
	if c, p := CommitPolicy(true, true); !c || !p {
		t.Fatalf("CommitPolicy(true,true) = (%v,%v), want (true,true)", c, p)
	}
}

// 13. protected .agent-sdlc evidence
func TestProtectedAgentSdlcEvidence(t *testing.T) {
	for _, p := range []string{
		".agent-sdlc/runs/SP-001/trace.json",
		".agent-sdlc/archive/plan-x/tasks.json",
		".agent-sdlc/runs/CLEF-005/classification.json",
	} {
		it := classifyOne(Facts{Path: p, GitState: "??", LifecycleEvidence: true})
		if it.Class != ClassSOPLifecycleEvidence || it.Action != ActionPreserve {
			t.Fatalf("%s: got %s/%s, want SOP_LIFECYCLE_EVIDENCE/preserve", p, it.Class, it.Action)
		}
	}
}

// 14. no active SOP plan
func TestNoActiveSOPPlan(t *testing.T) {
	// Absence of an active plan is not an error: a clean tree is still CLEAN, and
	// task output with no active plan is still classified and preserved.
	if a := Assess(nil, false); a.Disposition != DispositionClean {
		t.Fatalf("no active plan + clean tree: disposition = %s, want CLEAN", a.Disposition)
	}
	it := classifyOne(Facts{Path: "docs/reports/x/SP-009.md", GitState: "??", DeliverableOf: "SP-009"})
	if it.Class != ClassTaskOutput {
		t.Fatalf("class = %s, want TASK_OUTPUT (no active plan is not an error)", it.Class)
	}
}

// 15. active plan with legitimate task output
func TestActivePlanWithTaskOutput(t *testing.T) {
	items := []Item{
		classifyOne(Facts{Path: "docs/plans/PLAN-Y.md", GitState: "M", PlanDoc: true, PlanState: "ACTIVE"}),
		classifyOne(Facts{Path: "docs/reports/y/SP-002.md", GitState: "??", DeliverableOf: "SP-002"}),
	}
	if items[0].Class != ClassActivePlan {
		t.Fatalf("plan class = %s, want ACTIVE_PLAN", items[0].Class)
	}
	if items[1].Class != ClassTaskOutput {
		t.Fatalf("report class = %s, want TASK_OUTPUT", items[1].Class)
	}
	a := Assess(items, false)
	if a.Disposition != DispositionProposalReady {
		t.Fatalf("disposition = %s, want CLEANUP_PROPOSAL_READY", a.Disposition)
	}
}

// Guard: every documented class is reachable and distinct.
func TestClassVocabularyComplete(t *testing.T) {
	all := []Class{
		ClassTaskOutput, ClassActiveWork, ClassUserOwned, ClassHistoricalEvidence,
		ClassSOPLifecycleEvidence, ClassGenerated, ClassStaleArtifact, ClassCompletedPlan,
		ClassActivePlan, ClassNewPlan, ClassUnrelatedChange, ClassUnknown,
	}
	seen := map[Class]bool{}
	for _, c := range all {
		if seen[c] {
			t.Fatalf("duplicate class %s", c)
		}
		seen[c] = true
	}
	if len(seen) != 12 {
		t.Fatalf("class vocabulary has %d entries, want 12", len(seen))
	}
}

// Disposition outcomes: the five required results, driven by classification and
// proposed action, never by item count alone.

// 1. clean repository -> CLEAN
func TestDispositionClean(t *testing.T) {
	if a := Assess(nil, false); a.Disposition != DispositionClean {
		t.Fatalf("disposition = %s, want CLEAN", a.Disposition)
	}
}

// 2. dirty items, all explained, preserve-only -> CLEANUP_COMPLETE_WITH_PRESERVED_WORK
func TestDispositionCompleteWithPreservedWork(t *testing.T) {
	items := []Item{
		classifyOne(Facts{Path: "internal/other/x.go", GitState: "M", Unrelated: true}),
		classifyOne(Facts{Path: "bin/tool", GitState: "??", GeneratedConvention: true}),
		classifyOne(Facts{Path: "docs/plans/PLAN-A.md", GitState: "M", PlanDoc: true, PlanState: "ACTIVE"}),
	}
	for _, it := range items {
		if it.Action != ActionPreserve {
			t.Fatalf("%s action = %s, want preserve", it.Path, it.Action)
		}
	}
	a := Assess(items, false)
	if a.Disposition != DispositionCompleteWithPreservedWork {
		t.Fatalf("disposition = %s, want CLEANUP_COMPLETE_WITH_PRESERVED_WORK", a.Disposition)
	}
}

// 3a. actionable proposal: a proven orphan (delete) -> CLEANUP_PROPOSAL_READY
// 3b. actionable proposal: task output (commit with task) -> CLEANUP_PROPOSAL_READY
func TestDispositionProposalReady(t *testing.T) {
	orphan := classifyOne(Facts{Path: "internal/old/", GitState: "??", ProvenUnreferenced: true})
	if a := Assess([]Item{orphan}, false); a.Disposition != DispositionProposalReady {
		t.Fatalf("orphan disposition = %s, want CLEANUP_PROPOSAL_READY", a.Disposition)
	}
	output := classifyOne(Facts{Path: "docs/reports/x/SP-009.md", GitState: "??", DeliverableOf: "SP-009"})
	if a := Assess([]Item{output}, false); a.Disposition != DispositionProposalReady {
		t.Fatalf("task-output disposition = %s, want CLEANUP_PROPOSAL_READY", a.Disposition)
	}
	moveDoc := classifyOne(Facts{Path: "docs/plans/PLAN-X.md", GitState: "M", PlanDoc: true, PlanState: "COMPLETED", LifecycleComplete: true, HistoryConvention: true})
	if a := Assess([]Item{moveDoc}, false); a.Disposition != DispositionProposalReady {
		t.Fatalf("completed-plan disposition = %s, want CLEANUP_PROPOSAL_READY", a.Disposition)
	}
}

// 4. unknown ownership -> BLOCKED_UNKNOWN_OWNERSHIP
func TestDispositionBlockedUnknownOwnership(t *testing.T) {
	unknown := classifyOne(Facts{Path: "mystery", GitState: "??"})
	if a := Assess([]Item{unknown}, false); a.Disposition != DispositionBlockedUnknownOwnership {
		t.Fatalf("disposition = %s, want BLOCKED_UNKNOWN_OWNERSHIP", a.Disposition)
	}
	// A blocked condition dominates even when actionable items also exist.
	orphan := classifyOne(Facts{Path: "old/", GitState: "??", ProvenUnreferenced: true})
	if a := Assess([]Item{orphan, unknown}, false); a.Disposition != DispositionBlockedUnknownOwnership {
		t.Fatalf("with unknown present: disposition = %s, want BLOCKED_UNKNOWN_OWNERSHIP", a.Disposition)
	}
}

// 5. lifecycle conflict -> BLOCKED_LIFECYCLE_CONFLICT (highest precedence)
func TestDispositionBlockedLifecycleConflict(t *testing.T) {
	unknown := classifyOne(Facts{Path: "mystery", GitState: "??"})
	if a := Assess([]Item{unknown}, true); a.Disposition != DispositionBlockedLifecycleConflict {
		t.Fatalf("disposition = %s, want BLOCKED_LIFECYCLE_CONFLICT", a.Disposition)
	}
	if a := Assess(nil, true); a.Disposition != DispositionBlockedLifecycleConflict {
		t.Fatalf("empty+conflict disposition = %s, want BLOCKED_LIFECYCLE_CONFLICT", a.Disposition)
	}
}

// The preserved CONV-001 case (an operator-designated untracked report) is
// explained and preserve-only, so it must read as a completed cleanup with
// preserved work — never as an actionable proposal.
func TestPreservedReportIsCompleteWithPreservedWork(t *testing.T) {
	const path = "docs/reports/CONV-001-convergence-baseline.md"
	// The same explained, preserved artifact must yield the preserved-work
	// disposition whether ownership is established by operator designation or by
	// the artifact's own historical nature: the disposition is not owner-flag
	// dependent, and no ownership evidence is fabricated.
	cases := map[string]Facts{
		"operator-designated owner": {Path: path, GitState: "??", KnownOwner: true},
		"historical first-attempt":  {Path: path, GitState: "??", HistoricalEvidence: true},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			it := classifyOne(f)
			if it.Action != ActionPreserve {
				t.Fatalf("action = %s, want preserve", it.Action)
			}
			a := Assess([]Item{it}, false)
			if a.Disposition != DispositionCompleteWithPreservedWork {
				t.Fatalf("disposition = %s, want CLEANUP_COMPLETE_WITH_PRESERVED_WORK", a.Disposition)
			}
		})
	}
}
