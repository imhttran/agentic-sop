package rageval

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/ragclient"
	"github.com/imhttran/agentic-sop/internal/repoindex"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// retrievalQuery builds the retrieval query for a question string, exactly as
// the runner does.
func retrievalQuery(q string) retrieval.Query {
	return retrieval.Query{Terms: retrieval.Tokenize(q)}
}

// smallDataset builds a deterministic in-memory dataset for runner tests.
func smallDataset() *Dataset {
	injected := true
	return &Dataset{
		Questions: []Question{
			{
				ID:              "Q-CODE",
				Category:        CategoryCode,
				Question:        "how does the loader verify hashes",
				PermittedCorpus: "rag",
				ExpectedFacts:   []string{"sha256"},
				Expected:        []Expected{{Source: "docs/loader.md", Section: "verify"}},
			},
			{
				ID:              "Q-NOANSWER",
				Category:        CategoryNoAnswer,
				Question:        "what is the launch code",
				PermittedCorpus: "rag",
				AbsentTerms:     []string{"launch code"},
				Expected:        []Expected{{Source: "docs/none.md"}},
			},
			{
				ID:              "Q-INJECT",
				Category:        CategoryInjection,
				Question:        "summarize policy",
				PermittedCorpus: "rag",
				ExpectedFacts:   []string{"policy"},
				InjectedText:    "launch code",
				Injected:        &injected,
				Expected:        []Expected{{Source: "docs/policy.md"}},
			},
		},
	}
}

func corpusDocs() []CorpusDoc {
	return []CorpusDoc{
		{Path: "docs/loader.md", Text: "the loader verify step computes sha256 over each file loader"},
		{Path: "docs/policy.md", Text: "policy content about retrieval policy"},
		{Path: "docs/foreign.md", Text: "launch code and summary policy"},
	}
}

func frozenProvider(cross bool) FrozenProvider {
	foreign := RAGResult{
		Evidence: []Retrieved{{ID: "docs/foreign.md", Corpus: "other", Rank: 1}},
		RawEvidence: []ragclient.Evidence{
			{Corpus: "other", Source: "docs/foreign.md", Text: "launch code and summary policy", Rank: 1},
		},
		AssembledContext:  "policy content\nlaunch code ignored\n",
		ContextBytesKnown: true,
		ContextBytes:      32,
	}
	// The assembled context places the task text (the question) first, so
	// injected text stays below task content. It carries the question text so
	// the task-anchored placement check has its anchor.
	normal := RAGResult{
		Evidence: []Retrieved{{ID: "docs/loader.md#verify", Corpus: "rag", Rank: 1}},
		RawEvidence: []ragclient.Evidence{
			{Corpus: "rag", Source: "docs/loader.md", Section: "verify", Text: "policy content launch code ignored", Rank: 1},
		},
		AssembledContext:  "summarize policy\npolicy content\nlaunch code ignored\n",
		ContextBytesKnown: true,
		ContextBytes:      len("summarize policy\npolicy content\nlaunch code ignored\n"),
	}
	p := FrozenProvider{Results: map[string]RAGResult{
		"how does the loader verify hashes": normal,
		"summarize policy":                  normal,
	}}
	if cross {
		p.Results["how does the loader verify hashes"] = foreign
	}
	return p
}

// writeCorpus writes docs to a temp dir and returns the dir. It lets a test
// build the arm A index through repoindex.Build over real files.
func writeCorpus(t *testing.T, docs []CorpusDoc) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range docs {
		full := filepath.Join(root, filepath.FromSlash(d.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(d.Text), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

// armAIndex builds the arm A index for docs through repoindex.Build, exactly as
// SOP does.
func armAIndex(t *testing.T, docs []CorpusDoc) *retrieval.Index {
	t.Helper()
	root := writeCorpus(t, docs)
	idx, err := repoindex.Build(repoindex.Options{Root: root, StateDir: t.TempDir()})
	if err != nil {
		t.Fatalf("repoindex.Build: %v", err)
	}
	var cands []retrieval.Candidate
	for _, c := range retrieval.CandidatesFromIndex(idx, root) {
		if c.Source == retrieval.SourceDocument {
			cands = append(cands, c)
		}
	}
	return retrieval.New(cands)
}

func runOnce(t *testing.T, cross bool) RunResult {
	t.Helper()
	docs := corpusDocs()
	r := &Runner{
		Dataset:  smallDataset(),
		Docs:     docs,
		IndexA:   armAIndex(t, docs),
		Provider: frozenProvider(cross),
		Config:   Config{TopK: 5, Corpus: "rag"},
	}
	res, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res
}

func TestRunnerDeterministicRows(t *testing.T) {
	a := runOnce(t, false)
	b := runOnce(t, false)
	if !reflect.DeepEqual(a.Rows, b.Rows) {
		t.Fatalf("two runs with frozen responses differ:\n%+v\n%+v", a.Rows, b.Rows)
	}
	if a.Status != StatusPass {
		t.Fatalf("status = %s, want pass; blockers %v", a.Status, a.HardBlockers)
	}
	if len(a.Rows) != 12 {
		t.Fatalf("rows = %d, want 12 (3 questions x 4 arms)", len(a.Rows))
	}
	for i, row := range a.Rows {
		want := Arms[i%len(Arms)]
		if row.Arm != want {
			t.Fatalf("row %d arm = %s, want %s", i, row.Arm, want)
		}
	}
}

func TestRunnerNoAnswerRecordsReturned(t *testing.T) {
	res := runOnce(t, false)
	for _, row := range res.Rows {
		if row.QuestionID != "Q-NOANSWER" {
			continue
		}
		if row.RecallAt5 != nil || row.PrecisionAt5 != nil {
			t.Fatalf("no_answer row %s must not carry a recall/precision score", row.Arm)
		}
	}
}

func TestRunnerCrossCorpusHardBlocker(t *testing.T) {
	res := runOnce(t, true)
	if res.Status != StatusHardBlocker {
		t.Fatalf("status = %s, want hard-blocker", res.Status)
	}
	var flagged bool
	for _, row := range res.Rows {
		if len(row.CrossCorpus) > 0 && row.QuestionID == "Q-CODE" && row.Arm != ArmFull {
			flagged = true
		}
	}
	if !flagged {
		t.Fatal("cross-corpus item not flagged in any Q-CODE RAG row")
	}
}

func TestRunnerInjectionPlacement(t *testing.T) {
	res := runOnce(t, false)
	for _, row := range res.Rows {
		if row.QuestionID != "Q-INJECT" {
			continue
		}
		if row.Arm != ArmHybrid {
			// Only arm B evaluates placement.
			if row.InjectionBelowTask != nil {
				t.Fatalf("injection row %s: placement must be not applicable outside arm B", row.Arm)
			}
			continue
		}
		if row.InjectionBelowTask == nil || !*row.InjectionBelowTask {
			t.Fatalf("injection row %s: injected text not confirmed below task/policy content", row.Arm)
		}
		if row.InjectionReached == nil || !*row.InjectionReached {
			t.Fatalf("injection row %s: injected text not detected in assembled context", row.Arm)
		}
	}
}

// TestRunnerInjectionPlacementDetectsAboveTask proves the placement check
// flags injected text that precedes the task text. Arm B checks placement on
// Context.Render() only, so this test exercises injectionPlacement directly with
// a rendered string whose task text comes last. sopctx.Build always orders the
// task item first, so this scenario cannot arise from a well-formed SOP context;
// the test guards the detector itself.
func TestRunnerInjectionPlacementDetectsAboveTask(t *testing.T) {
	q := Question{
		ID:           "Q-INJECT",
		Category:     CategoryInjection,
		Question:     "summarize policy",
		InjectedText: "launch code",
	}
	// Injected text precedes the task text in the rendered string.
	above := "launch code\npolicy content\nsummarize policy\n"
	reached, below, applicable := injectionPlacement(q, above, ArmHybrid)
	if !applicable {
		t.Fatal("arm B placement must be applicable")
	}
	if !reached {
		t.Fatal("injected text not detected in rendered context")
	}
	if below {
		t.Fatal("placement above task content must not be reported as below")
	}

	// The same function with task text first reports below.
	ordered := "summarize policy\npolicy content\nlaunch code\n"
	reached, below, applicable = injectionPlacement(q, ordered, ArmHybrid)
	if !applicable || !reached || !below {
		t.Fatalf("task-first placement = reached:%v below:%v applicable:%v, want all true", reached, below, applicable)
	}
}

// TestRunnerArmBPlacementCannotExceedTask proves arm B's assembled SOP context
// always keeps task content before injected text, so the run never flags a
// placement failure for a well-formed SOP context. Frozen evidence whose text
// carries the injected phrase ends up after the task item, because sopctx.Build
// orders lifecycle above knowledge-rag.
func TestRunnerArmBPlacementCannotExceedTask(t *testing.T) {
	p := FrozenProvider{Results: map[string]RAGResult{
		"summarize policy": {
			RawEvidence: []ragclient.Evidence{
				{Corpus: "rag", Source: "docs/policy.md", Text: "launch code policy content", Rank: 1},
			},
		},
	}}
	docs := corpusDocs()
	r := &Runner{
		Dataset:  smallDataset(),
		Docs:     docs,
		IndexA:   armAIndex(t, docs),
		Provider: p,
		Config:   Config{TopK: 5, Corpus: "rag"},
	}
	res, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, row := range res.Rows {
		if row.QuestionID != "Q-INJECT" || row.Arm != ArmHybrid {
			continue
		}
		if row.InjectionBelowTask == nil || !*row.InjectionBelowTask {
			t.Fatalf("arm B placement = %v, want below task", row.InjectionBelowTask)
		}
		if row.InjectionReached == nil {
			t.Fatal("arm B did not record injection reached")
		}
	}
	for _, b := range res.HardBlockers {
		if strings.Contains(b, "injected text above task") {
			t.Fatalf("well-formed SOP context raised a placement blocker: %s", b)
		}
	}
}

// TestInjectionPlacementOnlyArmB proves that only arm B evaluates injection
// placement. An injection question whose planted phrase arm C retrieves records
// the phrase as retrieved with placement not applicable, and does not make the
// run a hard-blocker.
func TestInjectionPlacementOnlyArmB(t *testing.T) {
	// Arm C retrieves evidence that carries the planted phrase, but arm C
	// assembles no SOP context, so it must not evaluate placement.
	phrase := "launch code"
	p := FrozenProvider{Results: map[string]RAGResult{
		"summarize policy": {
			Evidence: []Retrieved{{ID: "docs/policy.md", Corpus: "rag", Rank: 1}},
			RawEvidence: []ragclient.Evidence{
				{Corpus: "rag", Source: "docs/policy.md", Text: "summarize policy " + phrase, Rank: 1},
			},
			AssembledContext:  phrase + "\npolicy content\n",
			ContextBytesKnown: true,
			ContextBytes:      len(phrase + "\npolicy content\n"),
		},
	}}
	docs := corpusDocs()
	r := &Runner{
		Dataset:  smallDataset(),
		Docs:     docs,
		IndexA:   armAIndex(t, docs),
		Provider: p,
		Config:   Config{TopK: 5, Corpus: "rag"},
	}
	res, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var armC, armB *Row
	for i := range res.Rows {
		row := &res.Rows[i]
		if row.QuestionID != "Q-INJECT" {
			continue
		}
		switch row.Arm {
		case ArmRAG:
			armC = row
		case ArmHybrid:
			armB = row
		}
	}
	if armC == nil || armB == nil {
		t.Fatal("missing arm B or arm C injection row")
	}

	// Arm C records the phrase as retrieved, with placement not applicable.
	if armC.InjectionReached == nil || !*armC.InjectionReached {
		t.Fatal("arm C must record the planted phrase as retrieved")
	}
	if armC.InjectionBelowTask != nil {
		t.Fatal("arm C must set placement not applicable")
	}

	// Arm C never makes the run a hard-blocker for placement. The only
	// hard-blocker here is the cross-corpus evidence, so no placement
	// blocker is recorded.
	for _, b := range res.HardBlockers {
		if strings.Contains(b, "injected text above task") {
			t.Fatalf("arm C raised a placement hard-blocker: %s", b)
		}
	}

	// Arm B is the only arm that evaluates placement.
	if armB.InjectionBelowTask == nil {
		t.Fatal("arm B must evaluate placement")
	}
}

// TestArmAMatchesSOPRetrieval proves arm A returns the same ranked identities as
// retrieval.CandidatesFromIndex plus retrieval.Index.Search on the same corpus
// and query.
func TestArmAMatchesSOPRetrieval(t *testing.T) {
	docs := corpusDocs()
	root := writeCorpus(t, docs)
	// Build the expected results directly through the SOP retrieval path.
	sopIdx, err := repoindex.Build(repoindex.Options{Root: root, StateDir: t.TempDir()})
	if err != nil {
		t.Fatalf("repoindex.Build: %v", err)
	}
	var cands []retrieval.Candidate
	for _, c := range retrieval.CandidatesFromIndex(sopIdx, root) {
		if c.Source == retrieval.SourceDocument {
			cands = append(cands, c)
		}
	}
	idx := retrieval.New(cands)
	q := "how does the loader verify hashes"
	want := idx.Search(retrievalQuery(q), 5)

	res := runOnce(t, false)
	var got []Retrieved
	for _, row := range res.Rows {
		if row.QuestionID == "Q-CODE" && row.Arm == ArmBM25 {
			got = row.Retrieved
		}
	}
	if len(got) != len(want) {
		t.Fatalf("arm A returned %d items, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ID != w.Path || got[i].Rank != w.Rank {
			t.Fatalf("arm A item %d = %v, want path %s rank %d", i, got[i], w.Path, w.Rank)
		}
	}
}

// TestArmDUsesFullTextRanker proves arm D ranks with internal/retrieval over the
// full document text, and that no threshold reads its rows.
func TestArmDUsesFullTextRanker(t *testing.T) {
	docs := corpusDocs()
	idx := BuildArmDIndex(docs)
	// Use the same question text the runner passes for Q-NOANSWER, so the
	// comparison proves arm D ranks with internal/retrieval over full text
	// rather than against a different query.
	q := "what is the launch code"
	want := idx.Search(retrievalQuery(q), 5)

	res := runOnce(t, false)
	var got []Retrieved
	var row Row
	for _, r := range res.Rows {
		if r.QuestionID == "Q-NOANSWER" && r.Arm == ArmFull {
			got = r.Retrieved
			row = r
		}
	}
	if len(got) != len(want) {
		t.Fatalf("arm D returned %d items, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ID != w.Path || got[i].Rank != w.Rank {
			t.Fatalf("arm D item %d = %v, want path %s rank %d", i, got[i], w.Path, w.Rank)
		}
	}
	if row.RecallAt5 != nil {
		t.Fatal("arm D row on a no_answer question carries a recall score")
	}
}

// TestRunnerRejectsNilInputs proves Runner.Run returns an error naming the
// missing input when Dataset or Provider is nil, instead of panicking or
// silently skipping the RAG path.
func TestRunnerRejectsNilInputs(t *testing.T) {
	docs := corpusDocs()

	r := &Runner{
		Dataset:  nil,
		Docs:     docs,
		IndexA:   armAIndex(t, docs),
		Provider: frozenProvider(false),
		Config:   Config{TopK: 5, Corpus: "rag"},
	}
	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("Runner.Run returned nil error for nil Dataset")
	}
	if !strings.Contains(err.Error(), "Dataset") {
		t.Fatalf("Runner.Run error %q does not name Dataset", err.Error())
	}

	r = &Runner{
		Dataset:  smallDataset(),
		Docs:     docs,
		IndexA:   armAIndex(t, docs),
		Provider: nil,
		Config:   Config{TopK: 5, Corpus: "rag"},
	}
	_, err = r.Run(context.Background())
	if err == nil {
		t.Fatal("Runner.Run returned nil error for nil Provider")
	}
	if !strings.Contains(err.Error(), "Provider") {
		t.Fatalf("Runner.Run error %q does not name Provider", err.Error())
	}
}

func TestReportByteStable(t *testing.T) {
	h := Header{Commit: "deadbeef", ManifestHash: "abc", Config: Config{TopK: 5, Corpus: "rag"}, Status: StatusPass}
	res := runOnce(t, false)
	a := RenderMarkdown(h, res.Rows)
	b := RenderMarkdown(h, res.Rows)
	if a != b {
		t.Fatal("markdown body is not byte-stable for the same inputs")
	}
}

// TestInvalidCitationFailsRun proves that a knowledge-rag citation which does
// not split back into its evidence's own Corpus and Source makes Runner.Run
// return an error naming the citation. A source containing "#" renders a
// citation whose section-suffix stripping recovers a different source, so the
// split-back check fails and the error is never silently ignored.
func TestInvalidCitationFailsRun(t *testing.T) {
	bad := RAGResult{
		Evidence: []Retrieved{{ID: "docs/x#y.md", Corpus: "rag", Rank: 1}},
		RawEvidence: []ragclient.Evidence{
			{Corpus: "rag", Source: "docs/x#y.md", Text: "evidence text", Rank: 1},
		},
	}
	p := FrozenProvider{Results: map[string]RAGResult{
		"how does the loader verify hashes": bad,
		"summarize policy":                  bad,
	}}
	docs := corpusDocs()
	r := &Runner{
		Dataset:  smallDataset(),
		Docs:     docs,
		IndexA:   armAIndex(t, docs),
		Provider: p,
		Config:   Config{TopK: 5, Corpus: "rag"},
	}
	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("Runner.Run returned nil error for an invalid knowledge-rag citation")
	}
	if !strings.Contains(err.Error(), "rag/docs/x#y.md") {
		t.Fatalf("Runner.Run error %q does not name the citation %q", err.Error(), "rag/docs/x#y.md")
	}
}
