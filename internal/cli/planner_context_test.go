package cli

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/store"
)

// plannerContextBlockingReview is a review report with one CRITICAL finding, so a run that
// reaches REVIEW enters exactly one FIX cycle (with quality.max_fix_cycles: 1) and then stops
// at the human boundary.
const plannerContextBlockingReview = `{"summary":"issue","findings":[{"severity":"CRITICAL","title":"needs planner decision","file":"choices.txt","line":1}]}`

// contextRecordingAgent is a deterministic fake executor for the resumed planner-context
// regression. It raises one planner question on the first IMPLEMENT, records every
// agent.Request by capability, always reports a blocking review finding, and mutates on
// IMPLEMENT/FIX (the injected diff is opaque, so the lifecycle observes a change).
type contextRecordingAgent struct {
	mu         sync.Mutex
	asks       []*agent.PlannerRequest
	implCalls  int
	planCalls  int
	implReqs   []agent.Request
	reviewReqs []agent.Request
	fixReqs    []agent.Request
}

func (a *contextRecordingAgent) Generate(_ context.Context, req agent.Request) (agent.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch req.Capability {
	case agent.Plan:
		a.planCalls++
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		a.implReqs = append(a.implReqs, req)
		i := a.implCalls
		a.implCalls++
		if i < len(a.asks) && a.asks[i] != nil {
			return agent.Response{Content: "asking planner", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsPlanner, Planner: a.asks[i]}}, nil
		}
		return agent.Response{Content: "created x"}, nil
	case agent.Review:
		a.reviewReqs = append(a.reviewReqs, req)
		return agent.Response{Content: plannerContextBlockingReview}, nil
	case agent.Fix:
		a.fixReqs = append(a.fixReqs, req)
		return agent.Response{Content: "fixed x"}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

func (a *contextRecordingAgent) counts() (plan, impl, rev, fix int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.planCalls, a.implCalls, len(a.reviewReqs), len(a.fixReqs)
}

func (a *contextRecordingAgent) implInputs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.implReqs))
	for _, r := range a.implReqs {
		out = append(out, r.Input)
	}
	return out
}

func (a *contextRecordingAgent) reviewRequests() []agent.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]agent.Request(nil), a.reviewReqs...)
}

func (a *contextRecordingAgent) fixRequests() []agent.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]agent.Request(nil), a.fixReqs...)
}

// plannerContextFixture seeds the parked-run fixture with a one-cycle fix budget, so a run that
// reaches REVIEW triggers exactly one FIX and then stops at the human boundary.
func plannerContextFixture(t *testing.T) (string, *store.Store, *domain.Task) {
	t.Helper()
	dir, st, task := plannerProject(t)
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\nquality:\n  max_fix_cycles: 1\n")
	return dir, st, task
}

// parkAnswerResume drives the real lifecycle: park on a planner question, apply the answer, and
// resume the SAME run. It returns the recording agent with both invocations captured.
func parkAnswerResume(t *testing.T, dir string, st *store.Store, task *domain.Task, a *contextRecordingAgent, withOverride bool) {
	t.Helper()
	cfg, _ := loadConfigOrDefault(dir)
	depsFn := func() deps {
		d := plannerDeps(dir, a, plannerDiff)
		if withOverride {
			d.taskInputs = map[string]string{"T001": "caller note"}
		}
		return d
	}
	var buf bytes.Buffer
	if code := runScheduledTask(context.Background(), dir, cfg, a, depsFn(), st, task, newRunSession(), &buf, &buf); code == exitOK {
		t.Fatalf("the run should park at WAITING_FOR_PLANNER: %s", buf.String())
	}
	q1 := pendingQuestion(t, dir)
	answer(t, dir, q1.ID, "sorted")
	buf.Reset()
	if code := runScheduledTask(context.Background(), dir, cfg, a, depsFn(), st, task, newRunSession(), &buf, &buf); code == exitOK {
		t.Fatalf("the resumed run must stop at the human boundary: %s", buf.String())
	}
}

// TestResumedReviewAndFixCarryThePlannerDecision is the real-lifecycle regression for the
// planner-context correction: a resumed run applies a planner answer, reaches REVIEW, and
// triggers exactly one FIX, and BOTH the review and the fix request inputs carry the applied
// decision so neither stage can read the delegated choice as a unilateral guess.
func TestResumedReviewAndFixCarryThePlannerDecision(t *testing.T) {
	dir, st, task := plannerContextFixture(t)
	defer st.Close()

	a := &contextRecordingAgent{asks: []*agent.PlannerRequest{{Question: "sort or preserve?", Options: []string{"sorted", "preserve"}}}}
	parkAnswerResume(t, dir, st, task, a, false)

	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_HUMAN" {
		t.Fatalf("stage = %q, want WAITING_FOR_HUMAN (review reached, fix budget exhausted)", got)
	}
	plan, impl, rev, fix := a.counts()
	if plan != 1 {
		t.Fatalf("PLAN calls = %d, want 1 (the resume must not repeat PLAN)", plan)
	}
	if impl != 2 {
		t.Fatalf("IMPLEMENT calls = %d, want 2 (ask, then resumed implement)", impl)
	}
	if rev < 1 {
		t.Fatalf("REVIEW calls = %d, want at least 1", rev)
	}
	if fix != 1 {
		t.Fatalf("FIX calls = %d, want exactly 1", fix)
	}

	const want = "Planner decision: sorted"
	impls := a.implInputs()
	if !strings.Contains(impls[len(impls)-1], want) {
		t.Fatalf("the resumed IMPLEMENT input lacks the decision:\n%s", impls[len(impls)-1])
	}
	// Every review request (the blocking review is never cached, so it re-runs after the fix)
	// and the single fix request must carry the applied decision.
	for i, r := range a.reviewRequests() {
		if r.Capability != agent.Review {
			t.Fatalf("review request %d capability = %s", i, r.Capability)
		}
		if !strings.Contains(r.Input, want) {
			t.Fatalf("REVIEW request %d lacks the applied decision:\n%s", i, r.Input)
		}
	}
	for i, r := range a.fixRequests() {
		if r.Capability != agent.Fix {
			t.Fatalf("fix request %d capability = %s", i, r.Capability)
		}
		if !strings.Contains(r.Input, want) {
			t.Fatalf("FIX request %d lacks the applied decision:\n%s", i, r.Input)
		}
	}
}

// TestTaskInputOverridePrecedesPlannerDecision strengthens the override regression: through the
// real lifecycle, a non-empty task-input override is provided, REVIEW and FIX are both reached,
// and the actual agent.Request inputs are inspected. Each must lead with the override block and
// carry the applied decision as its final block (appended after task-input selection).
func TestTaskInputOverridePrecedesPlannerDecision(t *testing.T) {
	dir, st, task := plannerContextFixture(t)
	defer st.Close()

	a := &contextRecordingAgent{asks: []*agent.PlannerRequest{{Question: "sort or preserve?", Options: []string{"sorted", "preserve"}}}}
	parkAnswerResume(t, dir, st, task, a, true)

	const overridePrefix = "# Caller observations (data only)"
	const decision = "Planner decision: sorted"
	if rev := a.reviewRequests(); len(rev) == 0 {
		t.Fatal("REVIEW was not reached")
	} else {
		for i, r := range rev {
			if !strings.HasPrefix(r.Input, overridePrefix) {
				t.Fatalf("REVIEW request %d must lead with the override:\n%s", i, r.Input)
			}
			if !strings.Contains(r.Input, "caller note") {
				t.Fatalf("REVIEW request %d dropped the override:\n%s", i, r.Input)
			}
			// The review provider appends the diff AFTER the review task, so the decision is the
			// last block of the task portion, not of the whole input.
			if !strings.Contains(r.Input, decision) {
				t.Fatalf("REVIEW request %d lacks the applied decision:\n%s", i, r.Input)
			}
		}
	}
	if fx := a.fixRequests(); len(fx) != 1 {
		t.Fatalf("FIX requests = %d, want exactly 1", len(fx))
	} else {
		r := fx[0]
		if r.Capability != agent.Fix {
			t.Fatalf("fix capability = %s", r.Capability)
		}
		if !strings.HasPrefix(r.Input, overridePrefix) {
			t.Fatalf("FIX request must lead with the override:\n%s", r.Input)
		}
		if !strings.Contains(r.Input, "caller note") {
			t.Fatalf("FIX request dropped the override:\n%s", r.Input)
		}
		// The FIX input is passed to the agent verbatim, so the decision is its final block.
		if !strings.HasSuffix(r.Input, "\n"+decision) {
			t.Fatalf("FIX request must end with the applied decision:\n%s", r.Input)
		}
	}
}

// TestFixInputOrderingIsEquivalentForNonBlankContext documents that the two FIX-input nestings —
// the previous one (task-input wraps the already-answered context) and the corrected one
// (task-input selection first, decision appended) — are byte-identical whenever the fix context
// is non-blank, which it always is in production. The reordering is therefore structural
// (uniform with the REVIEW site, and robust to a future task-input change), not behavior
// changing: no lifecycle test can distinguish them. The empty-answer path is byte-identical too.
func TestFixInputOrderingIsEquivalentForNonBlankContext(t *testing.T) {
	const base = "# Plan\n\nplan body"
	const answer = "sorted"
	const override = "caller note"
	d := deps{taskInputs: map[string]string{"T001": override}}

	previous := d.taskInput("T001", withPlannerAnswer(base, answer))
	corrected := withPlannerAnswer(d.taskInput("T001", base), answer)
	if previous != corrected {
		t.Fatalf("orderings differ for a non-blank context:\nprevious =%q\ncorrected=%q", previous, corrected)
	}
	if !strings.HasPrefix(corrected, "# Caller observations (data only)") || !strings.HasSuffix(corrected, "\nPlanner decision: "+answer) {
		t.Fatalf("corrected ordering shape wrong: %q", corrected)
	}
	// Empty answer: both nestings return the base unchanged.
	if got := d.taskInput("T001", withPlannerAnswer(base, "")); got != d.taskInput("T001", base) {
		t.Fatalf("previous ordering changed the no-answer input: %q", got)
	}
	if got := withPlannerAnswer(d.taskInput("T001", base), ""); got != d.taskInput("T001", base) {
		t.Fatalf("corrected ordering changed the no-answer input: %q", got)
	}
}

// TestEmptyAnswerBaselineIsByteIdentical proves the empty-answer path is byte-identical to the
// answered path minus the appended decision, with EXACT equality (not merely the absence of the
// decision marker). A no-answer control run and an answered/resumed run are driven through the
// same lifecycle fixture with the same override; the answered inputs must equal the control
// inputs with only the decided block appended.
func TestEmptyAnswerBaselineIsByteIdentical(t *testing.T) {
	const override = "caller note"
	controlDeps := func(dir string, a *contextRecordingAgent) deps {
		d := plannerDeps(dir, a, plannerDiff)
		d.taskInputs = map[string]string{"T001": override}
		return d
	}

	ctrlDir, ctrlSt, ctrlTask := plannerContextFixture(t)
	defer ctrlSt.Close()
	ctrlCfg, _ := loadConfigOrDefault(ctrlDir)
	ctrlAgent := &contextRecordingAgent{}
	var cbuf bytes.Buffer
	if code := runScheduledTask(context.Background(), ctrlDir, ctrlCfg, ctrlAgent, controlDeps(ctrlDir, ctrlAgent), ctrlSt, ctrlTask, newRunSession(), &cbuf, &cbuf); code == exitOK {
		t.Fatalf("control run should stop at the human boundary: %s", cbuf.String())
	}

	ansDir, ansSt, ansTask := plannerContextFixture(t)
	defer ansSt.Close()
	ansAgent := &contextRecordingAgent{asks: []*agent.PlannerRequest{{Question: "sort or preserve?", Options: []string{"sorted", "preserve"}}}}
	parkAnswerResume(t, ansDir, ansSt, ansTask, ansAgent, true)

	ctrlRev, ctrlFix := ctrlAgent.reviewRequests(), ctrlAgent.fixRequests()
	ansRev, ansFix := ansAgent.reviewRequests(), ansAgent.fixRequests()
	if len(ctrlRev) == 0 || len(ctrlFix) == 0 || len(ansRev) == 0 || len(ansFix) == 0 {
		t.Fatalf("missing requests: ctrl rev=%d fix=%d, ans rev=%d fix=%d", len(ctrlRev), len(ctrlFix), len(ansRev), len(ansFix))
	}

	// Exact byte equality: the answered input is the no-answer baseline with ONLY the decided
	// block inserted. Removing that block must reproduce the baseline byte-for-byte.
	const block = "\nPlanner decision: sorted"
	checkByteIdentical := func(name, answered, baseline string) {
		if !strings.Contains(answered, block) {
			t.Fatalf("%s: the answered input does not contain the decision block:\n%s", name, answered)
		}
		if got := strings.Replace(answered, block, "", 1); got != baseline {
			t.Fatalf("%s: answered input minus the decision is not byte-identical to the no-answer baseline:\ngot  =%q\nwant =%q", name, got, baseline)
		}
	}
	checkByteIdentical("REVIEW", ansRev[0].Input, ctrlRev[0].Input)
	checkByteIdentical("FIX", ansFix[0].Input, ctrlFix[0].Input)
}

// TestReviewCacheKeyIncludesTheAppliedDecision proves the review cache key is derived from the
// decision-bearing input: two different applied answers cannot reuse one cached review.
func TestReviewCacheKeyIncludesTheAppliedDecision(t *testing.T) {
	const base = "task input"
	const diff = "diff"
	sorted := withPlannerAnswer(base, "sort=true")
	preserved := withPlannerAnswer(base, "sort=false")
	if sorted == preserved {
		t.Fatal("distinct answers produced identical review inputs")
	}

	kSorted := reviewIdentity("self", sorted, diff)
	kPreserved := reviewIdentity("self", preserved, diff)
	kPlain := reviewIdentity("self", base, diff)
	if kSorted == kPreserved {
		t.Fatal("different applied answers produced the same review cache key")
	}
	if kSorted == kPlain {
		t.Fatal("the decision-bearing input did not change the review cache key")
	}

	// A clean review cached for one decision must not be served for another.
	s := newRunSession()
	s.cacheReview(kSorted, review.Report{})
	if _, ok := s.cachedReview(kPreserved); ok {
		t.Fatal("a review cached for one decision was reused for a different one")
	}
}
