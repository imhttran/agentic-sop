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
	if a.Disposition != DispositionApprovalRequired {
		t.Fatalf("disposition = %s, want CLEANUP_APPROVAL_REQUIRED", a.Disposition)
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
