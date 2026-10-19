package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/memindex"
	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/prompt"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

type fakeAdvisoryRetriever struct {
	res     retrievalboundary.Result
	err     error
	queries []retrievalboundary.ScopedQuery
}

func (f *fakeAdvisoryRetriever) Retrieve(_ context.Context, q retrievalboundary.ScopedQuery) (retrievalboundary.Result, error) {
	f.queries = append(f.queries, q)
	return f.res, f.err
}

func advisoryCfg(t *testing.T, enabled bool) config.Config {
	t.Helper()
	for _, k := range []string{
		config.EnvAdvisoryRetrievalEnabled,
		config.EnvAdvisoryRetrievalIndex,
		config.EnvAdvisoryRetrievalOutcomes,
		config.EnvAdvisoryRetrievalCorpora,
	} {
		t.Setenv(k, "")
	}
	return config.Config{
		Project: config.Project{Name: "agentic-sop"},
		AdvisoryRetrieval: config.AdvisoryRetrievalConfig{
			Enabled:        enabled,
			IndexPath:      filepath.Join(t.TempDir(), "index.db"),
			OutcomesDir:    filepath.Join(t.TempDir(), "outcomes"),
			AllowedCorpora: []string{"outcome", "failure"},
			TimeoutMS:      1000,
			MaxResults:     5,
			TokenBudget:    2000,
		},
	}
}

func specFixture() *taskfile.Spec {
	return &taskfile.Spec{ID: "RDM-003", Title: "Add bounded advisory retrieval", Description: "retrieve applicable history"}
}

func outcomeEvidence(id, text, status string) retrievalboundary.Evidence {
	return retrievalboundary.Evidence{
		RecordID:    id,
		ContentHash: "hash-" + id,
		Corpus:      retrievalboundary.CorpusOutcome,
		Citation:    "run/" + id,
		Text:        text,
		Status:      status,
		Tokens:      retrievalboundary.EstimateTokens(text),
	}
}

func TestAdvisoryDisabledRetrievesNothing(t *testing.T) {
	cfg := advisoryCfg(t, false)
	f := &fakeAdvisoryRetriever{res: retrievalboundary.Result{Evidence: []retrievalboundary.Evidence{outcomeEvidence("r1", "x", "success")}}}
	items, res := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), f, "agentic-sop", "")
	if len(items) != 0 || len(f.queries) != 0 {
		t.Fatalf("disabled mode retrieved: items=%d queries=%d", len(items), len(f.queries))
	}
	if res.Status != ragStatusSkipped || res.ReasonCode != ragReasonAdvisoryDisabled {
		t.Fatalf("result = %+v, want a disabled skip", res)
	}
}

func TestAdvisoryPlanningAddsUntrustedCitedItems(t *testing.T) {
	cfg := advisoryCfg(t, true)
	f := &fakeAdvisoryRetriever{res: retrievalboundary.Result{Evidence: []retrievalboundary.Evidence{
		outcomeEvidence("r1", "verified correction: retry the transient provider failure", "success"),
	}}}
	items, res := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), f, "agentic-sop", "")
	if res.Phase != AdvisoryPhasePlanning || res.Status != ragStatusSucceed {
		t.Fatalf("result = %+v", res)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	it := items[0]
	if it.Source != sopctx.SourceKnowledgeRAG || it.Identity != "run/r1" {
		t.Errorf("item source/identity = %v/%q", it.Source, it.Identity)
	}
	if !strings.Contains(it.Text, "retry the transient") {
		t.Errorf("item text lost the record body: %q", it.Text)
	}
	if !strings.Contains(it.Reason, "untrusted") || !strings.Contains(it.Reason, "never an approval") {
		t.Errorf("item reason = %q, want an untrusted, non-approval label", it.Reason)
	}
	if it.Priority != sopctx.PriorityKnowledgeRAG {
		t.Errorf("priority = %d", it.Priority)
	}
	// Scope is enforced on every query.
	if f.queries[0].Scope.Repository != "agentic-sop" || f.queries[0].Scope.TaskID != "" {
		t.Errorf("scope = %+v (a current-task filter would exclude historical evidence)", f.queries[0].Scope)
	}
}

func TestAdvisoryRecoveryUsesFailureContext(t *testing.T) {
	cfg := advisoryCfg(t, true)
	f := &fakeAdvisoryRetriever{}
	_, res := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), f, "agentic-sop", "syntax error at or near SELECT")
	if res.Phase != AdvisoryPhaseRecovery {
		t.Fatalf("phase = %q, want recovery", res.Phase)
	}
	if len(f.queries) == 0 || !strings.Contains(f.queries[0].Text, "syntax error at or near SELECT") {
		t.Fatalf("recovery query did not carry the failure context: %+v", f.queries)
	}
}

func TestAdvisoryFailureFallsBackWithReason(t *testing.T) {
	cfg := advisoryCfg(t, true)
	f := &fakeAdvisoryRetriever{err: retrievalboundary.ErrTimeout}
	items, res := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), f, "agentic-sop", "")
	if len(items) != 0 {
		t.Fatalf("failed retrieval produced items: %+v", items)
	}
	if res.Status != ragStatusFailed || res.ReasonCode != ragReasonAdvisoryTimeout {
		t.Fatalf("result = %+v, want a timeout reason", res)
	}
}

func TestAdvisoryDropsOutOfScopeAndApprovalRecords(t *testing.T) {
	cfg := advisoryCfg(t, true)
	approval := outcomeEvidence("r2", "approved", "success")
	approval.Citation = "approval=yes"
	other := outcomeEvidence("r3", "other corpus", "success")
	other.Corpus = retrievalboundary.CorpusRun
	f := &fakeAdvisoryRetriever{res: retrievalboundary.Result{Evidence: []retrievalboundary.Evidence{approval, other}}}
	items, _ := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), f, "agentic-sop", "")
	if len(items) != 0 {
		t.Fatalf("out-of-scope or approval evidence was mapped: %+v", items)
	}
}

func TestAdvisoryPartialOutcomeIsNotARemedy(t *testing.T) {
	cfg := advisoryCfg(t, true)
	f := &fakeAdvisoryRetriever{res: retrievalboundary.Result{Evidence: []retrievalboundary.Evidence{
		outcomeEvidence("r1", "partial work", "partial"),
	}}}
	items, _ := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), f, "agentic-sop", "")
	if len(items) != 0 {
		t.Fatalf("a partial outcome was presented as a candidate remedy: %+v", items)
	}
}

func TestAdvisoryRespectsConfiguredBounds(t *testing.T) {
	cfg := advisoryCfg(t, true)
	cfg.AdvisoryRetrieval.MaxResults = 1
	f := &fakeAdvisoryRetriever{res: retrievalboundary.Result{Evidence: []retrievalboundary.Evidence{
		outcomeEvidence("r1", "correction one", "success"),
		outcomeEvidence("r2", "correction two", "success"),
	}}}
	items, _ := advisoryKnowledgeForAttempt(context.Background(), cfg, specFixture(), f, "agentic-sop", "")
	if len(items) != 1 {
		t.Fatalf("items = %d, want the configured bound of 1", len(items))
	}
}

func TestAdvisorySkipsTasksWithRequiredEvidence(t *testing.T) {
	cfg := advisoryCfg(t, true)
	spec := specFixture()
	spec.RequiredEvidence = []string{"corpus:outcome"}
	f := &fakeAdvisoryRetriever{}
	items, res := advisoryKnowledgeForAttempt(context.Background(), cfg, spec, f, "agentic-sop", "")
	if len(items) != 0 || len(f.queries) != 0 {
		t.Fatalf("required-evidence task ran advisory retrieval: items=%d queries=%d", len(items), len(f.queries))
	}
	if res.Status != ragStatusSkipped || res.ReasonCode != ragReasonAdvisoryRequired {
		t.Fatalf("result = %+v, want a required-evidence skip", res)
	}
}

// TestAdvisoryLocalIndexEndToEnd proves the production local path: authoritative outcome
// files, a rebuildable SQLite FTS index, and drift-checked bounded retrieval.
func TestAdvisoryLocalIndexEndToEnd(t *testing.T) {
	dir := t.TempDir()
	outcomes := filepath.Join(dir, "outcomes")
	if err := os.MkdirAll(outcomes, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := outcomefile.OutcomeRecord{
		RecordID:     "RDM-002-attempt-2",
		Corpus:       retrievalboundary.CorpusOutcome,
		Repository:   "agentic-sop",
		TaskID:       "RDM-002",
		Status:       outcomefile.StatusSuccess,
		Complete:     true,
		Authorized:   true,
		EvidenceRefs: []string{"run/RDM-002/report.json"},
		Citation:     "run/RDM-002",
		Text:         "bounded advisory retrieval history: verified correction, retry the transient provider failure",
	}
	if _, _, err := outcomefile.Write(outcomes, rec); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "index.db")
	ix, err := memindex.Open(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.RebuildFromDir(context.Background(), outcomes); err != nil {
		t.Fatal(err)
	}
	_ = ix.Close()

	cfg := advisoryCfg(t, true)
	cfg.AdvisoryRetrieval.IndexPath = indexPath
	cfg.AdvisoryRetrieval.OutcomesDir = outcomes
	cfg.AdvisoryRetrieval.AllowedCorpora = []string{"outcome"}

	items, res := advisoryKnowledgeForAttempt(context.Background(), cfg, &taskfile.Spec{
		ID:          "RDM-003",
		Title:       "bounded advisory retrieval history",
		Description: "verified correction retry transient provider failure",
	}, nil, "agentic-sop", "")
	if res.Status != ragStatusSucceed || len(items) != 1 {
		t.Fatalf("local retrieval = %+v, items=%d", res, len(items))
	}
	if !strings.Contains(items[0].Text, "retry the transient") {
		t.Fatalf("local retrieval lost the record body: %q", items[0].Text)
	}
}

func TestUpdateAdvisoryInclusionOnlyAfterCompilation(t *testing.T) {
	dir := t.TempDir()
	rn, err := runpkg.New(dir, "RDM-003")
	if err != nil {
		t.Fatal(err)
	}
	writeRAGTelemetry(rn, ragTelemetry{TaskID: "RDM-003", Attempt: 1})
	evidence := prompt.ItemHeader(sopctx.SourceKnowledgeRAG, "run/r1") + "\nbody"
	updateAdvisoryInclusion(rn, dir, "RDM-003", evidence, []string{"run/r1", "run/missing"})

	raw, err := os.ReadFile(filepath.Join(dir, ".agent-sdlc", "runs", "RDM-003", "rag.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"run/r1"`) {
		t.Fatalf("included reference missing from telemetry: %s", raw)
	}
	if strings.Contains(string(raw), `"run/missing"`) {
		t.Fatalf("a reference absent from the prompt was reported included: %s", raw)
	}
}

func TestOpenAdvisoryRetrieverRequiresConfig(t *testing.T) {
	_, closeFn, err := openAdvisoryRetriever(config.AdvisoryRetrievalConfig{})
	if err == nil {
		closeFn()
		t.Fatal("openAdvisoryRetriever accepted an empty configuration")
	}
	if !errors.Is(err, retrievalboundary.ErrInvalidQuery) && !strings.Contains(err.Error(), "empty index path") {
		t.Fatalf("err = %v, want an explicit invalid-path error", err)
	}
}
