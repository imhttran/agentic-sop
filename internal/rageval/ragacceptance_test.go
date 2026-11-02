//go:build ragacceptance

package rageval

// Integration test for the four-arm comparison against a real
// PostgreSQL/pgvector-backed producer reached through internal/ragclient.
//
// The test is self-contained. It creates the pgvector schema, seeds the corpus,
// and drops the schema through `psql`, then drives the ragclient contract
// through a generated producer script that queries PostgreSQL/pgvector. No
// operator pre-seeding is required, so a forgotten seed is a hard failure
// rather than a silent skip.
//
// Configuration is read from the environment:
//
//	RAGEVAL_PSQL_DSN  PostgreSQL DSN reachable by psql (required)
//	RAGEVAL_PSQL      psql executable (optional; default: psql)
//
// When RAGEVAL_PSQL_DSN is unset the test fails, not skips: the acceptance tag
// promises a real backend, and a silent pass would hide a missing database.
//
// Run with:
//
//	RAGEVAL_PSQL_DSN='postgres://rag:rag@127.0.0.1:5434/rag?sslmode=disable' \
//	  go test -count=1 -tags ragacceptance ./internal/rageval/...

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/ragclient"
	"github.com/imhttran/agentic-sop/internal/repoindex"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

const (
	acceptanceCorpus  = "rag-acceptance"
	acceptanceForeign = "rag-acceptance-foreign"
)

// acceptanceRow is one seeded (corpus, source, section, text) document.
type acceptanceRow struct {
	Corpus  string
	Source  string
	Section string
	Text    string
}

// acceptanceRows returns the corpus the test inserts before running the arms.
// Each text carries the tokens the fixture questions search for, so a real
// retrieval result is non-empty.
func acceptanceRows() []acceptanceRow {
	return []acceptanceRow{
		{Corpus: acceptanceCorpus, Source: "docs/loader.md", Section: "verify", Text: "the loader verify step computes sha256 over each manifest file"},
		{Corpus: acceptanceCorpus, Source: "docs/policy.md", Section: "", Text: "retrieval policy content governs corpus scope"},
		{Corpus: acceptanceForeign, Source: "docs/foreign.md", Section: "", Text: "launch code and summary policy"},
	}
}

func acceptanceDataset() *Dataset {
	injected := true
	return &Dataset{
		Questions: []Question{
			{
				ID:              "Q-ACC-CODE",
				Category:        CategoryCode,
				Question:        "how does the loader verify hashes",
				PermittedCorpus: acceptanceCorpus,
				ExpectedFacts:   []string{"sha256"},
				Expected:        []Expected{{Source: "docs/loader.md", Section: "verify"}},
			},
			{
				ID:              "Q-ACC-NOANSWER",
				Category:        CategoryNoAnswer,
				Question:        "what is the launch code",
				PermittedCorpus: acceptanceCorpus,
				AbsentTerms:     []string{"launch code"},
				Expected:        []Expected{{Source: "docs/absent.md"}},
			},
			{
				ID:              "Q-ACC-INJECT",
				Category:        CategoryInjection,
				Question:        "summarize retrieval policy",
				PermittedCorpus: acceptanceCorpus,
				ExpectedFacts:   []string{"policy"},
				InjectedText:    "launch code",
				Injected:        &injected,
				Expected:        []Expected{{Source: "docs/policy.md"}},
			},
		},
	}
}

func acceptanceCorpusDocs() []CorpusDoc {
	return []CorpusDoc{
		{Path: "docs/loader.md", Text: "the loader verify step computes sha256 over each manifest file"},
		{Path: "docs/policy.md", Text: "retrieval policy content governs corpus scope"},
		{Path: "docs/foreign.md", Text: "launch code and summary policy"},
	}
}

// psqlDSN returns the DSN for the real PostgreSQL/pgvector backend. The test
// fails rather than skips when it is unset, because the acceptance tag is a
// promise that a real backend is present.
func psqlDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("RAGEVAL_PSQL_DSN"))
	if dsn == "" {
		t.Fatal("ragacceptance: RAGEVAL_PSQL_DSN is unset; the acceptance tag requires a real PostgreSQL/pgvector backend")
	}
	return dsn
}

func psqlExecutable() string {
	if exe := strings.TrimSpace(os.Getenv("RAGEVAL_PSQL")); exe != "" {
		return exe
	}
	return "psql"
}

// runPSQL runs one SQL statement against the real backend and returns stdout.
func runPSQL(t *testing.T, dsn, sql string) string {
	t.Helper()
	cmd := exec.Command(psqlExecutable(), dsn, "-v", "ON_ERROR_STOP=1", "-tAq", "-c", sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ragacceptance: psql failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// seedBackend creates the pgvector schema and inserts the acceptance rows. It
// returns a cleanup that drops the table.
func seedBackend(t *testing.T, dsn string) func() {
	t.Helper()
	runPSQL(t, dsn, "CREATE EXTENSION IF NOT EXISTS vector")
	runPSQL(t, dsn, "DROP TABLE IF EXISTS rageval_acceptance")
	runPSQL(t, dsn, "CREATE TABLE rageval_acceptance (id bigserial primary key, corpus text not null, source text not null, section text not null default '', text text not null, embedding vector(3) not null)")
	for i, row := range acceptanceRows() {
		emb := fmt.Sprintf("[%d,0,0]", i+1)
		runPSQL(t, dsn, fmt.Sprintf(
			"INSERT INTO rageval_acceptance (corpus, source, section, text, embedding) VALUES ('%s','%s','%s','%s','%s')",
			row.Corpus, row.Source, row.Section, strings.ReplaceAll(row.Text, "'", "''"), emb))
	}
	return func() { runPSQL(t, dsn, "DROP TABLE IF EXISTS rageval_acceptance") }
}

// writeProducer writes a self-contained producer script that speaks the
// rag-retrieval/v1 contract and answers from PostgreSQL/pgvector via psql.
func writeProducer(t *testing.T, dsn string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "producer.sh")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
read -r req || true
corpus=$(printf '%%s' "$req" | grep -o '"corpus":"[^"]*"' | head -n1 | cut -d'"' -f4)
query=$(printf '%%s' "$req" | grep -o '"query":"[^"]*"' | head -n1 | cut -d'"' -f4)
rows=$(%s '%s' -tAq -v ON_ERROR_STOP=1 -c "SELECT source,section,text FROM rageval_acceptance WHERE corpus='$corpus' AND EXISTS (SELECT 1 FROM unnest(string_to_array('$query', ' ')) AS w(word) WHERE position(lower(w.word) in lower(text)) > 0) ORDER BY embedding <-> '[1,0,0]', source LIMIT 5")
printf '{"schema_version":"rag-retrieval/v1","status":"OK","truncated":false,"diagnostics":{"elapsed_ms":0,"error_code":"OK"},"evidence":['
first=1
rank=0
printf '%%s\n' "$rows" | while IFS='|' read -r source section text; do
  [ -z "$source" ] && continue
  rank=$((rank+1))
  if [ $first -eq 0 ]; then printf ','; fi
  first=0
  printf '{"id":"%%s","corpus":"%%s","source":"%%s","section":"%%s","text":"%%s","retrieval_method":"bm25","score":1.0,"rank":%%d}' "$source" "$corpus" "$source" "$section" "$text" "$rank"
done
printf ']}'
`, psqlExecutable(), dsn)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("ragacceptance: write producer: %v", err)
	}
	return path
}

func acceptanceProvider(t *testing.T, exe string, corpora ...string) RAGProvider {
	t.Helper()
	client, err := ragclient.New(ragclient.Config{
		Executable:     exe,
		AllowedCorpora: corpora,
		MaxResults:     K,
	})
	if err != nil {
		t.Fatalf("ragacceptance: build ragclient for %q: %v", exe, err)
	}
	return RagClientProvider{Client: client}
}

// acceptanceArmAIndex writes the seeded corpus to a temp dir and builds the arm
// A index through repoindex.Build, exactly as SOP does.
func acceptanceArmAIndex(t *testing.T) *retrieval.Index {
	t.Helper()
	root := t.TempDir()
	for _, d := range acceptanceCorpusDocs() {
		full := filepath.Join(root, filepath.FromSlash(d.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("ragacceptance: mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(d.Text), 0o644); err != nil {
			t.Fatalf("ragacceptance: write %s: %v", full, err)
		}
	}
	idx, err := repoindex.Build(repoindex.Options{Root: root, StateDir: t.TempDir()})
	if err != nil {
		t.Fatalf("ragacceptance: repoindex.Build: %v", err)
	}
	var cands []retrieval.Candidate
	for _, c := range retrieval.CandidatesFromIndex(idx, root) {
		if c.Source == retrieval.SourceDocument {
			cands = append(cands, c)
		}
	}
	return retrieval.New(cands)
}

// TestAcceptanceFourArmsRealBackend seeds a real PostgreSQL/pgvector database,
// runs arms A, B, C, and D through the ragclient producer, and asserts the
// metric, no_answer, injection, and diagnostic arm D behavior.
func TestAcceptanceFourArmsRealBackend(t *testing.T) {
	dsn := psqlDSN(t)
	cleanup := seedBackend(t, dsn)
	defer cleanup()
	exe := writeProducer(t, dsn)

	provider := acceptanceProvider(t, exe, acceptanceCorpus)
	res := runAcceptance(t, provider)

	if len(res.Rows) != 12 {
		t.Fatalf("rows = %d, want 12 (3 questions x 4 arms)", len(res.Rows))
	}
	for i, row := range res.Rows {
		if row.Arm != Arms[i%len(Arms)] {
			t.Fatalf("row %d arm = %s, want %s", i, row.Arm, Arms[i%len(Arms)])
		}
	}
	assertAcceptanceRows(t, res)

	// Arm D never feeds the run status: a diagnostic row cannot turn a pass into
	// a hard-blocker.
	if res.Status != StatusPass {
		t.Fatalf("status = %s, want pass; blockers %v", res.Status, res.HardBlockers)
	}

	assertRealCorpusIsolation(t, dsn, exe)
	cross := acceptanceCrossCorpusResult(t)
	if cross.Status != StatusHardBlocker {
		t.Fatalf("cross-corpus run status = %s, want %s", cross.Status, StatusHardBlocker)
	}
	var flagged bool
	for _, row := range cross.Rows {
		if len(row.CrossCorpus) > 0 && row.Arm != ArmFull {
			flagged = true
		}
	}
	if !flagged {
		t.Fatal("cross-corpus item not flagged in any non-diagnostic row")
	}
}

func runAcceptance(t *testing.T, provider RAGProvider) RunResult {
	t.Helper()
	r := &Runner{
		Dataset:  acceptanceDataset(),
		Docs:     acceptanceCorpusDocs(),
		IndexA:   acceptanceArmAIndex(t),
		Provider: provider,
		Config:   Config{TopK: K, Corpus: acceptanceCorpus},
	}
	res, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("ragacceptance: run: %v", err)
	}
	return res
}

// assertRealCorpusIsolation runs a question through the real
// PostgreSQL/pgvector producer with both corpora allowlisted, then requires the
// real backend to return only items from the requested corpus: every retrieved
// item carries acceptanceCorpus, every row's CrossCorpus is empty, and the run
// status is not a hard blocker.
func assertRealCorpusIsolation(t *testing.T, dsn, exe string) {
	t.Helper()
	t.Logf("ragacceptance: corpus isolation transport=%s", exe)
	client := acceptanceProvider(t, exe, acceptanceCorpus, acceptanceForeign)
	ds := &Dataset{Questions: []Question{{
		ID:              "Q-ACC-ISOLATION",
		Category:        CategoryCode,
		Question:        "launch code summary policy",
		PermittedCorpus: acceptanceCorpus,
		ExpectedFacts:   []string{"policy"},
		Expected:        []Expected{{Source: "docs/policy.md"}},
	}}}
	r := &Runner{
		Dataset:  ds,
		Docs:     acceptanceCorpusDocs(),
		IndexA:   acceptanceArmAIndex(t),
		Provider: client,
		Config:   Config{TopK: K, Corpus: acceptanceCorpus},
	}
	res, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("ragacceptance: corpus isolation run: %v", err)
	}
	for _, row := range res.Rows {
		for _, item := range row.Retrieved {
			if item.Corpus != acceptanceCorpus {
				t.Fatalf("ragacceptance: real backend returned foreign-corpus item %q (corpus %s) for request corpus %s", item.ID, item.Corpus, acceptanceCorpus)
			}
		}
		if len(row.CrossCorpus) > 0 {
			t.Fatalf("ragacceptance: row %s/%s flagged cross-corpus items %v against a real corpus-isolated backend", row.QuestionID, row.Arm, row.CrossCorpus)
		}
	}
	if res.Status == StatusHardBlocker {
		t.Fatalf("ragacceptance: real corpus isolation run status = %s, want a non-blocker status", res.Status)
	}
}

func acceptanceCrossCorpusResult(t *testing.T) RunResult {
	t.Helper()
	foreign := RAGResult{
		Evidence:          []Retrieved{{ID: "docs/foreign.md", Corpus: acceptanceForeign, Rank: 1}},
		AssembledContext:  "policy content\nlaunch code ignored\n",
		ContextBytesKnown: true,
		ContextBytes:      len("policy content\nlaunch code ignored\n"),
	}
	r := &Runner{
		Dataset: acceptanceDataset(),
		Docs:    acceptanceCorpusDocs(),
		IndexA:  acceptanceArmAIndex(t),
		Provider: FrozenProvider{Results: map[string]RAGResult{
			"how does the loader verify hashes": foreign,
			"summarize retrieval policy":        foreign,
		}},
		Config: Config{TopK: K, Corpus: acceptanceCorpus},
	}
	res, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("ragacceptance: cross-corpus result run: %v", err)
	}
	return res
}

func assertAcceptanceRows(t *testing.T, res RunResult) {
	t.Helper()
	byKey := map[string]Row{}
	for _, row := range res.Rows {
		byKey[row.QuestionID+"/"+row.Arm] = row
	}
	for _, arm := range Arms {
		na, ok := byKey["Q-ACC-NOANSWER/"+arm]
		if !ok {
			t.Fatalf("missing no_answer row for arm %s", arm)
		}
		if na.RecallAt5 != nil || na.PrecisionAt5 != nil {
			t.Errorf("no_answer row %s must not carry recall/precision", arm)
		}
	}
	code, ok := byKey["Q-ACC-CODE/"+ArmRAG]
	if !ok {
		t.Fatal("missing Q-ACC-CODE/C row")
	}
	if len(code.Retrieved) == 0 {
		t.Fatalf("ragacceptance: producer returned no evidence for the seeded loader doc; the PostgreSQL/pgvector corpus is unseeded or unreachable")
	}
	if code.RecallAt5 == nil {
		t.Fatal("Q-ACC-CODE/C recall@5 is not scored")
	}
	injC, ok := byKey["Q-ACC-INJECT/"+ArmRAG]
	if !ok {
		t.Fatal("missing Q-ACC-INJECT/C row")
	}
	if injC.InjectionReached == nil || injC.InjectionBelowTask != nil {
		t.Fatal("arm C (ArmRAG) must record injection reached but no below-task placement")
	}
	injB, ok := byKey["Q-ACC-INJECT/"+ArmHybrid]
	if !ok {
		t.Fatal("missing Q-ACC-INJECT/B row")
	}
	if injB.InjectionReached == nil || injB.InjectionBelowTask == nil {
		t.Fatal("arm B (ArmHybrid) injection row does not record placement")
	}
	if !*injB.InjectionBelowTask {
		t.Fatalf("arm B (ArmHybrid) injection row reports injected text above task/policy content")
	}
	d, ok := byKey["Q-ACC-CODE/"+ArmFull]
	if !ok {
		t.Fatal("missing Q-ACC-CODE/D row")
	}
	if d.InjectionBelowTask != nil {
		t.Fatal("arm D row must not report injection placement")
	}
}
