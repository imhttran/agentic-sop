package dqharness

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

const evalDir = "../../evals/decision-quality"

func TestKeywordRetrieverRespectsExclusionsAndBounds(t *testing.T) {
	c := decisionquality.Case{
		ID:          "c1",
		Class:       decisionquality.ClassRecovery,
		TaskContext: "retry the transient provider failure",
		PreDecisionEvidence: []decisionquality.EvidenceItem{
			{RecordID: "e1", Text: "retry the transient provider failure", ContentHash: "h1"},
			{RecordID: "e2", Text: "transient provider failure retry succeeded", ContentHash: "h2"},
			{RecordID: "e3", Text: "unrelated migration", ContentHash: "h3"},
		},
		EventualSolutionIDs: []string{"e2"},
	}
	// The harness asserts exclusions; the retriever itself must only ever see
	// pre-decision evidence, so e2 (declared as a solution) must be removed from the
	// case before retrieval in production. Here we verify the retriever returns only
	// indexed pre-decision records and honors a tight budget.
	c.PreDecisionEvidence = c.PreDecisionEvidence[:1]
	got, err := (KeywordRetriever{}).Retrieve(c, 64)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected a keyword match")
	}
	// A budget smaller than one record is a hard bound: nothing is returned.
	tight, err := (KeywordRetriever{}).Retrieve(c, 1)
	if err != nil {
		t.Fatalf("Retrieve tight: %v", err)
	}
	if len(tight) != 0 {
		t.Fatalf("a tight budget returned %d records; the bound must be hard", len(tight))
	}
	for _, ev := range got {
		if ev.RecordID == "e2" || ev.RecordID == "e3" {
			t.Fatalf("retriever returned non-indexed record %q", ev.RecordID)
		}
	}
}

func TestRunArmThreeRepeatsAndNoLeakage(t *testing.T) {
	c := decisionquality.Case{
		ID:          "c1",
		Class:       decisionquality.ClassPlanning,
		TaskContext: "plan the retry",
		PreDecisionEvidence: []decisionquality.EvidenceItem{
			{RecordID: "e1", Text: "retry the transient provider failure", ContentHash: "h1"},
		},
		EventualSolutionIDs: []string{"e2"},
		Expected:            decisionquality.ExpectedOutcome{Kind: "plan", Decision: "RETRY", Rationale: "evidence supports retry"},
	}
	corpus := decisionquality.Corpus{Version: 1, Split: "tuning", Cases: []decisionquality.Case{c}}
	h := Harness{
		Config:    RunConfig{Model: "m", EvidenceTokens: 2000},
		Rules:     decisionquality.FrozenRules(),
		Retriever: KeywordRetriever{},
		Decide: func(_ context.Context, cs decisionquality.Case, retrieved []decisionquality.EvidenceItem) (string, int, int, error) {
			for _, ev := range retrieved {
				if ev.RecordID == "e2" {
					t.Fatal("leaked the eventual solution into the prompt")
				}
			}
			return "RETRY", 10, 5, nil
		},
	}
	rec, err := h.RunArm(context.Background(), decisionquality.ArmB, corpus)
	if err != nil {
		t.Fatalf("RunArm: %v", err)
	}
	if len(rec.Results) != 3 {
		t.Fatalf("results = %d, want three repeats", len(rec.Results))
	}
	for _, r := range rec.Results {
		if !r.Correct {
			t.Fatalf("deterministic decision scored wrong: %+v", r)
		}
	}
	if rec.RetrievalMode == "" || rec.RetrievalMode == "none" {
		t.Fatalf("retrieval mode not recorded: %q", rec.RetrievalMode)
	}
}

// TestTuningSmoke validates the harness end to end on the tuning split only. It is a
// mechanics test, not an evaluation result: it never touches the held-out corpus.
func TestTuningSmoke(t *testing.T) {
	corpus, err := decisionquality.LoadCorpus(filepath.Join(evalDir, "corpus-tuning.json"))
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Split != "tuning" {
		t.Fatalf("split = %q, want tuning", corpus.Split)
	}
	decide := func(_ context.Context, c decisionquality.Case, _ []decisionquality.EvidenceItem) (string, int, int, error) {
		return c.Expected.Decision, 20, 10, nil
	}
	h := Harness{
		Config:    RunConfig{Model: "smoke", EvidenceTokens: 2000},
		Rules:     decisionquality.FrozenRules(),
		Decide:    decide,
		Retriever: KeywordRetriever{},
		Sandbox:   passSandbox{},
	}
	a, err := h.RunArm(context.Background(), decisionquality.ArmA, *corpus)
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.RunArm(context.Background(), decisionquality.ArmB, *corpus)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Results) != 3*len(corpus.Cases) || len(b.Results) != 3*len(corpus.Cases) {
		t.Fatalf("results A=%d B=%d, want %d", len(a.Results), len(b.Results), 3*len(corpus.Cases))
	}
	sa, sb := decisionquality.Summarize(a), decisionquality.Summarize(b)
	paired := decisionquality.PairedRun(a, b, 7)
	report := decisionquality.RenderReport(decisionquality.ReportInput{
		Corpus: *corpus, Rules: decisionquality.FrozenRules(),
		A: sa, B: sb, Paired: paired, Gate: decisionquality.EvaluateGate(decisionquality.FrozenRules(), sa, sb, paired),
		Decision: decisionquality.Decision(sa.Primary >= 0, false, false),
	})
	if !strings.Contains(report, "Paired comparison") {
		t.Fatalf("report is not a report:\n%s", report)
	}
}

// TestPilotTuningSmoke runs the planning-only pilot harness on the tuning split with a
// fixture decider. It proves the mechanics; it makes no model call and is not a result.
func TestPilotTuningSmoke(t *testing.T) {
	corpus, err := decisionquality.LoadCorpus(filepath.Join("../../evals/decision-memory-pilot", "corpus-tuning.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := decisionquality.ValidatePilot(*corpus); err != nil {
		t.Fatal(err)
	}
	decide := func(_ context.Context, c decisionquality.Case, _ []decisionquality.EvidenceItem) (string, int, int, error) {
		return c.Expected.Decision, 20, 8, nil
	}
	h := Harness{
		Config:    RunConfig{Model: "fixture", PromptHash: "p", EvidenceTokens: 2000, PromptTemplate: "t"},
		Rules:     decisionquality.FrozenRules(),
		Decide:    decide,
		Retriever: KeywordRetriever{},
	}
	a, err := h.RunArm(context.Background(), decisionquality.ArmA, *corpus)
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.RunArm(context.Background(), decisionquality.ArmB, *corpus)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Results) != 3*len(corpus.Cases) || len(b.Results) != 3*len(corpus.Cases) {
		t.Fatalf("results A=%d B=%d, want %d", len(a.Results), len(b.Results), 3*len(corpus.Cases))
	}
}

// TestPilotLexicalThresholdCompliance proves the corrected query returns records that meet
// the declared lexical floor and rejects records below it. It does NOT judge semantic
// relevance: no independent relevance adjudication exists. It runs on the tuning split
// only; held-out labels never shape the query.
func TestPilotLexicalThresholdCompliance(t *testing.T) {
	corpus, err := decisionquality.LoadCorpus(filepath.Join("../../evals/decision-memory-pilot", "corpus-tuning.json"))
	if err != nil {
		t.Fatal(err)
	}
	retriever := KeywordRetriever{}
	retrieved, rejected, violations := 0, 0, 0
	for _, c := range corpus.Cases {
		got, err := retriever.Retrieve(c, DefaultEvidenceTokens)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]decisionquality.EvidenceItem{}
		for _, ev := range c.PreDecisionEvidence {
			byID[ev.RecordID] = ev
		}
		kept := map[string]bool{}
		queryTerms := retrievalboundary.Tokenize(c.TaskContext)
		share := func(text string) int {
			set := map[string]bool{}
			for _, tok := range retrievalboundary.Tokenize(text) {
				set[tok] = true
			}
			n := 0
			seen := map[string]bool{}
			for _, tok := range queryTerms {
				if set[tok] && !seen[tok] {
					seen[tok] = true
					n++
				}
			}
			return n
		}
		for _, ev := range got {
			kept[ev.RecordID] = true
			if share(ev.Text) >= 2 {
				retrieved++
			} else {
				violations++
			}
		}
		for id, ev := range byID {
			if !kept[id] && share(ev.Text) < 2 {
				rejected++
			}
		}
	}
	if violations != 0 {
		t.Fatalf("%d retrieved items share fewer than two query terms", violations)
	}
	if retrieved == 0 {
		t.Fatal("the corrected query returned no threshold-compliant evidence")
	}
	if rejected == 0 {
		t.Fatal("no below-floor evidence was rejected")
	}
	t.Logf("threshold_compliant=%d below_floor_rejected=%d (lexical only; semantic relevance not judged)", retrieved, rejected)
}

type passSandbox struct{}

func (passSandbox) Replay(context.Context, decisionquality.Case, string) (bool, error) {
	return true, nil
}
