package dqharness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

func fixtureCorpus() decisionquality.Corpus {
	return decisionquality.Corpus{
		Version: 1, Split: "tuning",
		Cases: []decisionquality.Case{{
			ID: "c1", Class: decisionquality.ClassPlanning,
			TaskContext: "plan the retry",
			PreDecisionEvidence: []decisionquality.EvidenceItem{
				{RecordID: "e1", Text: "retry the transient provider failure", ContentHash: "h1"},
			},
			Expected: decisionquality.ExpectedOutcome{Kind: "plan", Decision: "RETRY", Rationale: "evidence"},
		}},
	}
}

func TestParseDecisionRejectsUnknownOutput(t *testing.T) {
	if d, err := ParseDecision("RETRY", []string{"RETRY", "PROCEED"}); err != nil || d != "RETRY" {
		t.Fatalf("ParseDecision = %q, %v", d, err)
	}
	if _, err := ParseDecision("I think maybe retrying is fine", []string{"PROCEED"}); err == nil {
		t.Fatal("unknown output must be an explicit error, not a default")
	}
}

func TestPromptIsArmBlindExceptEvidence(t *testing.T) {
	c := fixtureCorpus().Cases[0]
	a := BuildPrompt(c, nil, []string{"RETRY"})
	b := BuildPrompt(c, c.PreDecisionEvidence, []string{"RETRY"})
	if !strings.Contains(a, "(none)") || !strings.Contains(b, "e1") {
		t.Fatalf("evidence block missing:\nA=%s\nB=%s", a, b)
	}
	// Everything before the evidence block must be identical.
	ha, hb := strings.Split(a, "Evidence available")[0], strings.Split(b, "Evidence available")[0]
	if ha != hb {
		t.Fatal("prompt differs across arms outside the evidence block")
	}
}

func TestLimitsStopCleanlyAndResumeWithoutRepeating(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "checkpoint.jsonl")
	corpus := fixtureCorpus()
	rules := decisionquality.FrozenRules()
	decisions := DecisionSet(corpus)
	calls := 0
	decide := func(_ context.Context, c decisionquality.Case, _ []decisionquality.EvidenceItem) (string, int, int, error) {
		calls++
		return c.Expected.Decision, 10, 5, nil
	}
	opts := RunOptions{
		Corpus: corpus, Rules: rules, Decisions: decisions,
		Config:  RunConfig{Model: "m", PromptHash: "p", EvidenceTokens: 10, PromptTemplate: "t"},
		Binding: Binding{Corpus: "c", Protocol: "p", Rubric: "r", Harness: HarnessRevision, Model: "m", Sampling: "s"},
		DecideA: decide, DecideB: decide, Retriever: KeywordRetriever{},
		Limits:         Limits{MaxDecisions: 2, MaxRequests: 100, MaxRetries: 5, MaxElapsed: time.Minute, MaxTokens: 10000},
		CheckpointPath: cp,
	}
	first, err := RunComparison(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.Stopped == "" {
		t.Fatal("the call limit did not stop the run")
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 before the limit", calls)
	}
	before := len(first.A.Results) + len(first.B.Results)
	usageFile := filepath.Join(dir, "usage.json")
	var st UsageState
	if data, rerr := os.ReadFile(usageFile); rerr == nil {
		if uerr := json.Unmarshal(data, &st); uerr != nil {
			t.Fatal(uerr)
		}
	}
	if st.Decisions != 2 || st.Requests != 2 || st.Tokens == 0 {
		t.Fatalf("persisted usage = %+v, want 2 decisions and 2 requests", st)
	}

	opts.Limits.MaxDecisions = 100
	second, err := RunComparison(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Stopped != "" {
		t.Fatalf("resume stopped: %s", second.Stopped)
	}
	// Three repeats per arm, two arms: six decisions total. Two already ran, so resume
	// must issue exactly four more.
	if calls != 6 {
		t.Fatalf("calls after resume = %d, want 6 (no decision repeated; %d results before)", calls, before)
	}
	if data, rerr := os.ReadFile(usageFile); rerr == nil {
		if uerr := json.Unmarshal(data, &st); uerr != nil {
			t.Fatal(uerr)
		}
	}
	if st.Decisions != 6 || st.Requests != 6 {
		t.Fatalf("usage after resume = %+v, want 6 decisions and 6 requests", st)
	}
	if err := VerifyRun(corpus, rules, second.A, second.B); err != nil {
		t.Fatalf("VerifyRun: %v", err)
	}
}

func TestInFlightRequestIsNotLostOnResume(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.json")
	b := Binding{Corpus: "c", Protocol: "p", Rubric: "r", Harness: HarnessRevision, Model: "m", Sampling: "s"}
	st := UsageState{Binding: b, Decisions: 2, Requests: 5, Retries: 1, Tokens: 100, ActiveElapsedMS: 2500, InFlight: true}
	if err := SaveUsage(path, st); err != nil {
		t.Fatal(err)
	}
	got, err := LoadUsage(path, b)
	if err != nil {
		t.Fatal(err)
	}
	if got.InFlight {
		t.Fatal("in-flight state was not cleared")
	}
	if got.Requests != 5 {
		t.Fatalf("requests = %d, want the in-flight request still counted", got.Requests)
	}
	if got.UncertainRequests != 1 {
		t.Fatalf("uncertain requests = %d, want 1", got.UncertainRequests)
	}
	if got.ActiveElapsedMS != 2500 {
		t.Fatalf("elapsed = %d, want the persisted active time", got.ActiveElapsedMS)
	}
	other := b
	other.Model = "different"
	if _, err := LoadUsage(path, other); err == nil {
		t.Fatal("usage from a different binding was accepted")
	}
}

func TestCheckpointBindingRejectsIncompatibleResume(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "cp.jsonl")
	b := Binding{Corpus: "c", Protocol: "p", Rubric: "r", Harness: HarnessRevision, Model: "m", Sampling: "s"}
	if err := ensureBinding(cp, b); err != nil {
		t.Fatal(err)
	}
	if err := appendCheckpoint(cp, CheckpointEntry{Arm: "A", CaseID: "c1", Repeat: 1, Result: decisionquality.CaseResult{CaseID: "c1", Repeat: 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCheckpoint(cp, b); err != nil {
		t.Fatalf("identical binding rejected: %v", err)
	}
	other := b
	other.Model = "different"
	if _, err := LoadCheckpoint(cp, other); err == nil {
		t.Fatal("a checkpoint from a different model was accepted")
	}

	legacy := filepath.Join(dir, "legacy.jsonl")
	if err := os.WriteFile(legacy, []byte(`{"arm":"A","case_id":"c1","repeat":1,"result":{"case_id":"c1","repeat":1}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCheckpoint(legacy, b); err == nil {
		t.Fatal("a checkpoint without a binding header was accepted")
	}
}

func TestVerifyRunDetectsMissingCoverageAndUnmatchedConditions(t *testing.T) {
	corpus := fixtureCorpus()
	rules := decisionquality.FrozenRules()
	a := decisionquality.RunRecord{Arm: "A", Model: "m", PromptHash: "p", EvidenceTokens: 10}
	b := decisionquality.RunRecord{Arm: "B", Model: "m", PromptHash: "p", EvidenceTokens: 10}
	if err := VerifyRun(corpus, rules, a, b); err == nil {
		t.Fatal("an empty run must not verify")
	}
	for rep := 1; rep <= rules.RepeatsPerCase; rep++ {
		a.Results = append(a.Results, decisionquality.CaseResult{Arm: "A", CaseID: "c1", Repeat: rep, Correct: true})
		b.Results = append(b.Results, decisionquality.CaseResult{Arm: "B", CaseID: "c1", Repeat: rep, Correct: true})
	}
	if err := VerifyRun(corpus, rules, a, b); err != nil {
		t.Fatalf("complete matched run failed: %v", err)
	}
	b.PromptHash = "other"
	if err := VerifyRun(corpus, rules, a, b); err == nil {
		t.Fatal("unmatched prompt hash must fail verification")
	}
}

func TestReplayBlockerNamesMissingFixtures(t *testing.T) {
	c := decisionquality.Corpus{Cases: []decisionquality.Case{{ID: "x", Executable: true}}}
	got := ReplayBlocker(c)
	if !strings.Contains(got, "no repository fixture") && !strings.Contains(got, "no isolation") {
		t.Fatalf("blocker = %q", got)
	}
}

func TestModelDecisionClassifiesErrors(t *testing.T) {
	c := fixtureCorpus().Cases[0]
	d := ModelDecision(func(context.Context, string, string) (string, error) {
		return "", errors.New("provider down")
	}, "m", []string{"RETRY"})
	if _, _, _, err := d(context.Background(), c, nil); err == nil {
		t.Fatal("provider error was not propagated")
	}
	d = ModelDecision(func(context.Context, string, string) (string, error) {
		return "garbage", nil
	}, "m", []string{"RETRY"})
	if _, _, _, err := d(context.Background(), c, nil); err == nil {
		t.Fatal("unparseable output was not an error")
	}
}
