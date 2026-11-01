# Implementation Plan

**Plan ID:** `plan-sop-rag-evaluation`
**Status:** DRAFT — under review, not started.
**Baseline:** `agentic-sop/main` at `5b427dcd`.
**Finishes:** RAG-SOP-004 in `PLAN-RAG-SOP-Integration.md`. The current decision in `docs/specs/RAG-SOP-004-DECISION.md` is NO-GO for lack of evidence. This plan produces that evidence.
**Criteria:** `docs/specs/RAG-EVALUATION.md` (predeclared, operator-approved 2026-10-15).

## Project

agentic-sop

## Summary

RAG retrieval is merged into SOP (PR #11) and off by default. It has never been measured the way `RAG-EVALUATION.md` requires. The only run compared hybrid retrieval against Postgres full-text on a 12-document toy corpus. That wasn't BM25, and it showed no difference.

This plan builds the missing pieces. A harness runs SOP's real BM25 ranker against BM25+RAG (and RAG-only as a diagnostic) on a frozen, labeled set of real SOP questions. It records per-question results and the end-to-end failure fixtures, and drafts the evidence section of the decision report.

The work is split so SOP doesn't grade itself. A human writes the questions, the expected evidence, and the numeric thresholds, and commits them before any SOP task runs. SOP builds and runs the harness, and it can't edit those files. The final GO / CONDITIONAL / NO-GO call stays with the operator.

The dataset includes a multi-hop subset. If RAG misses those questions, that's the evidence for reviving the shelved `rag-template/docs/plans/PLAN-RAG-Graph-Retrieval.md`. If it doesn't, the graph layer isn't needed.

## Human prerequisites (before `sop run`)

Neither of these is a SOP task. Both must be committed on the base branch before starting.

H-1, the frozen dataset, goes in `evals/rag/`.

- `corpus/` holds a snapshot of the documents RAG indexes (SOP and rag-template plans, specs, ADRs, reports) plus `MANIFEST.sha256`.
- `questions.json` has at least 40 questions, including reworded variants marked with `reworded_from`. Each carries `id`, `question`, `category` (`code`, `architecture_history`, `design_decision`, `failure_investigation`, `multi_hop`, `no_answer`, `injection`), `permitted_corpus`, and `expected` (source and section, same shape as rag-template's `evals/retrieval.json`).
- At least 8 per answerable category, 10 or more `multi_hop`, and 4 or more each of `no_answer` and `injection`.
- Questions come from real things you've asked or looked up while working on SOP. Claude can draft candidates, but you pick and label them.

H-2, the numeric thresholds, are added as a new §4.1 in `docs/specs/RAG-EVALUATION.md`. The current spec says "meaningful improvement" and "agreed latency target" without numbers. These are proposed values to accept or change.

- The historical-knowledge subset is `architecture_history`, `design_decision`, `failure_investigation` and `multi_hop`.
- GO needs recall@5 on that subset to improve by at least 0.10 absolute over BM25-only, with improvement on at least 5 individual questions.
- On `code` questions, BM25+RAG may lose at most 1 question that BM25-only got right.
- Added retrieval latency must be at most 500 ms p95.
- Hard blockers stay as written in §4.

## Shared boundaries

These apply to every task.

- No task edits `evals/rag/**` or `docs/specs/RAG-EVALUATION.md`. If a task finds a problem in either, it stops and raises a question through the planner.
- `SOP_RAG_ENABLED` stays `false` by default. Nothing in this plan flips a default or wires the harness into normal task execution.
- Arm A is what SOP runs today. It builds candidates with `retrieval.CandidatesFromIndex` over a `repoindex.Build` of the frozen corpus. It ranks them with `retrieval.Index.Search` on `retrieval.Tokenize(question)`.
- Arm D is diagnostic. It uses the same `internal/retrieval` ranker over each document's full text. It is reported, and no threshold uses it.
- No other BM25 or lexical ranker is written. Every lexical arm uses `internal/retrieval`.
- No scratch, generator, or debug tests are added. Every test asserts behavior.
- Results are per question. No aggregate claim without the per-question rows behind it.
- Database-touching runs need real PostgreSQL/pgvector, per `RAG-POSTGRES-VALIDATION-POLICY.md`. Unit tests with fakes cover harness logic only and aren't evidence for the decision.
- Retrieved text is untrusted. The harness records it as data and never feeds it to a model.
- The operator decision line in `RAG-SOP-004-DECISION.md` is left for the human.

## RAG-EVAL-001 — Dataset loader and integrity check

Add `internal/rageval/` with a loader for `evals/rag/questions.json` and the corpus manifest. It refuses to run if any corpus hash doesn't match, if a question is missing a required field or uses an unknown category, or if category counts fall below the H-1 minimums.

### Dependencies

None

### Deliverables

- `internal/rageval/dataset.go` with the loader, schema validation, and manifest hash check.
- `internal/rageval/dataset_test.go` with tests on small fixture datasets under `internal/rageval/testdata/` (valid, tampered hash, missing field, unknown category, under-minimum counts).
- `cmd/rag-eval/` with a `validate` subcommand that loads `evals/rag/` and prints counts per category.
- `TestCommittedDatasetValid` in `internal/rageval/dataset_test.go`, which loads the committed `evals/rag/` and asserts the manifest hashes and the H-1 category minimums.

### Acceptance Criteria

- The committed dataset loads, its manifest hashes match, and its per-category counts meet the H-1 minimums (`TestCommittedDatasetValid`).
- Each invalid fixture is rejected with a specific error.
- `evals/rag/` and `docs/specs/RAG-EVALUATION.md` are unchanged.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test -count=1 ./internal/rageval/... ./cmd/rag-eval/...`
- `go test -count=1 -run TestCommittedDatasetValid ./internal/rageval/`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`

### Acceptance Mapping

- criterion 1: check 4
- criterion 2: check 3
- criterion 3: check 5

## RAG-EVAL-002 — Three-arm comparison runner

Add a `run` subcommand. For each question it runs arm A (BM25-only over the indexed corpus), arm B (BM25 plus RAG through the existing `ragclient` and context merge path, so it measures what SOP would actually do), arm C (RAG-only, diagnostic), and arm D (`internal/retrieval` over full document text, diagnostic). It records per-question rows: retrieved identities and ranks, recall@5, precision@5, citation and provenance correctness, context bytes and truncation, latency, fallback reason, and any cross-corpus item. Output is a JSON results file plus a Markdown table, both written under `docs/reports/rag-eval/<timestamp>/` with the git commit, dataset manifest hash, and config recorded in a header.

Use these existing APIs as written. Don't spend discovery on them, and don't reimplement them.

- `repoindex.Build(repoindex.Options{Root: <corpus dir>, StateDir: <temp dir>}) (repoindex.Index, error)`. `Index.Documents` holds `repoindex.Document{Path, Class, ...}`, with `Path` relative to `Root`.
- `retrieval.CandidatesFromIndex(idx repoindex.Index) []retrieval.Candidate`. A document candidate has `ID "doc:"+Path`, `Source retrieval.SourceDocument`, `Path`, and `Text` set to the path plus its class.
- `retrieval.New(corpus []retrieval.Candidate) *retrieval.Index`.
- `(*retrieval.Index).Search(q retrieval.Query, limit int) []retrieval.Result`. `Query` is `{Terms []string}`. `Result` is `{ID, Source, Path, Score, Reason, Rank}`.
- `retrieval.Tokenize(s string) []string`.

Arm A builds the index over `evals/rag/corpus` with `repoindex.Build`, keeps the document candidates from `CandidatesFromIndex`, and searches with `Query{Terms: retrieval.Tokenize(question)}`. Arm D builds `retrieval.Candidate{ID: "doc:"+path, Source: retrieval.SourceDocument, Path: path, Text: <full file text>}` for each corpus file, then uses the same `New` and `Search`. `Runner` takes arm A's index as a required input built by `repoindex.Build`. It has no fallback that builds candidates by hand, so no copy of the `CandidatesFromIndex` text format exists. A result's evidence identity is `Path`, relative to `evals/rag/corpus`. It equals the `source` in `questions.json`, for example `imhttran/agentic-sop/docs/reference/PACKAGES.md`. Delete `internal/rageval/bm25.go`.

Arm B assembles context through SOP's context package (imported as `sopctx "github.com/imhttran/agentic-sop/internal/context"`), as `internal/cli/run.go` does. Don't merge rankings by hand. Delete `mergeRanks`.

- Items: one task item, `sopctx.Item{Source: sopctx.SourceTask, Identity: <question id>, Reason: "evaluation question", Priority: sopctx.PriorityLifecycle, Text: <question>}`; one item per arm A hit, `sopctx.Item{Source: sopctx.SourceRepository, Identity: <Path>, Reason: "bm25 hit", Priority: sopctx.PriorityRepository, Text: <Path>}`; and the RAG items from `sopctx.KnowledgeItems(evidence []ragclient.Evidence, maxItems, budgetBytes int) []sopctx.Item` with the run's top-k and RAG context budget.
- Context: `sopctx.Build(items, sopctx.DefaultLimits()) sopctx.Context`. Use `Context.Items()` for the ranked order, `Context.Render()` for the assembled context, and `Context.Bytes()` and `Context.Truncated()` for size and truncation.
- Arm B's ranked identities are the `Items()` order, keeping only repository and knowledge-rag items. A repository item's identity is its `Identity`. A knowledge-rag item's `Identity` is the citation `sopctx.KnowledgeItems` renders: `<corpus>/<source>`, then `#<section>` if there is a section, then `:p<page>` if there is a page. Split it at the first `/` to get the corpus, then drop any `#…` and `:p…` suffix to get the evidence identity, which is the `source`.
- Injection placement is checked on `Context.Render()`. Find the task item's text (the question) in the rendered context. Injected text placed above task content means its first index is lower than the task text's index. Don't use expected facts as the anchor.

### Dependencies

- RAG-EVAL-001

### Deliverables

- `internal/rageval/runner.go` and `internal/rageval/metrics.go`.
- `cmd/rag-eval run` subcommand.
- Unit tests for metric math on hand-computed fixtures.
- An integration test (`-tags ragacceptance`) running all four arms on a small seeded corpus against real PostgreSQL/pgvector.
- No `internal/rageval/bm25.go`, and no other BM25 implementation in `internal/rageval`.
- The dataset loader fills `InjectedText` for each `injection` question with the `evidence` phrase of its expected entry whose source is under `evals/rag/injection/`. It is that phrase, not the whole planted file. `evals/rag/questions.json` stays unchanged.
- Named tests in `internal/rageval`: `TestArmAMatchesSOPRetrieval`, `TestNoLocalBM25`, `TestArmDUsesFullTextRanker`, `TestCommittedInjectionQuestionsHaveInjectedText`, and `TestArmBUsesSOPContextBuild`.

### Acceptance Criteria

- Metric functions match hand-computed values on the fixtures.
- Two runs with the same dataset, corpus, and frozen RAG responses produce identical rows and ordering.
- A cross-corpus item in any arm is flagged in its row and marks the run as a hard-blocker failure.
- `no_answer` questions record what each arm returned instead of scoring them as recall.
- `injection` questions record whether injected text reached the assembled context, and confirm it was never placed above task or policy content.
- The integration test passes against real PostgreSQL/pgvector.
- Arm A returns the same ranked identities as `retrieval.CandidatesFromIndex` plus `retrieval.Index.Search` on the same corpus and query. The test writes a fixture corpus to a temp directory, builds the expected results by calling `repoindex.Build`, `retrieval.CandidatesFromIndex`, `retrieval.New`, and `Search` directly, and compares them with the runner's arm A rows (`TestArmAMatchesSOPRetrieval`).
- `internal/rageval` contains no BM25 implementation of its own (`TestNoLocalBM25`).
- Arm D ranks with `internal/retrieval` over full document text, and no threshold reads its rows (`TestArmDUsesFullTextRanker`).
- Every committed `injection` question loads with an `InjectedText` equal to the `evidence` phrase of its planted expected entry, and that phrase appears verbatim in the planted file (`TestCommittedInjectionQuestionsHaveInjectedText`).
- The repository stays gofmt-formatted.
- Arm B's ranked identities, assembled context, bytes, and truncation equal what `sopctx.KnowledgeItems` plus `sopctx.Build` produce for the same arm A hits and frozen evidence, built directly in the test. The test computes the expected identities and corpora from the frozen evidence's `Corpus` and `Source` fields, not with the runner's own helpers, and includes evidence with a section and a page. `internal/rageval` holds no `mergeRanks` (`TestArmBUsesSOPContextBuild`).

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test -count=1 ./internal/rageval/... ./cmd/rag-eval/...`
- `go test -count=1 -tags ragacceptance ./internal/rageval/...`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`
- `go test -count=1 -run TestArmAMatchesSOPRetrieval ./internal/rageval/`
- `go test -count=1 -run TestNoLocalBM25 ./internal/rageval/`
- `go test -count=1 -run TestArmDUsesFullTextRanker ./internal/rageval/`
- `go test -count=1 -run TestCommittedInjectionQuestionsHaveInjectedText ./internal/rageval/`
- `go test -count=1 ./internal/archtest/`
- `go test -count=1 -run TestArmBUsesSOPContextBuild ./internal/rageval/`

### Acceptance Mapping

- criterion 1: check 3
- criterion 2: check 3
- criterion 3: check 4
- criterion 4: check 3
- criterion 5: check 4
- criterion 6: check 4
- criterion 7: check 6
- criterion 8: check 7
- criterion 9: check 8
- criterion 10: check 9
- criterion 11: check 10
- criterion 12: check 11

## RAG-EVAL-003 — End-to-end failure fixtures

Record the operational matrix from `RAG-EVALUATION.md` §5 and the decision report §4. The cases are cold start, stale index, Ollama unavailable, PostgreSQL unavailable, malformed producer response, cross-repository scope request, permission boundary, and a resumed run. Each fixture runs a real `sop` task path with `SOP_RAG_ENABLED=true`. It records whether SOP fell back to BM25-only with a reason, or failed closed when the task declared required evidence. It also confirms approval and completion state didn't change because of RAG.

### Dependencies

- RAG-EVAL-001

### Deliverables

- `internal/rageval/fixtures_test.go` (tag `ragacceptance`) with one test per case.
- `docs/reports/rag-eval/fixtures.md` listing each case, the command, and the captured outcome.

### Acceptance Criteria

- All eight cases are present and each has a captured outcome.
- Optional-mode failures show BM25-only fallback with a reason, and required-evidence failures show a blocked state. A silent pass is a test failure.
- No case changes approval or completion state.
- Tests run against real services, with the unavailable cases produced by stopping or misconfiguring them, not by mocking the client.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test -count=1 -tags ragacceptance -run Fixture ./internal/rageval/...`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`

### Acceptance Mapping

- criterion 1: check 3
- criterion 2: check 3
- criterion 3: check 3
- criterion 4: manual: docs/reports/rag-eval/fixtures.md records how each unavailable case was produced against real services

## RAG-EVAL-004 — Full run and decision evidence

Run `rag-eval run` on the frozen dataset three times. Check the repeats agree, then write the evidence into the decision report. Update §2–§4 of `docs/specs/RAG-SOP-004-DECISION.md` from Unavailable to Measured where the new records support it, with links to the run directories. Add a draft §5 that compares the results to the H-2 thresholds line by line and states which outcome the evidence supports. Leave the operator decision marked `PENDING OPERATOR`.

Include a multi-hop section showing BM25+RAG recall on `multi_hop` questions, with the specific misses listed. That's the input for whether the graph plan comes back.

### Dependencies

- RAG-EVAL-002
- RAG-EVAL-003

### Deliverables

- Three recorded run directories under `docs/reports/rag-eval/`.
- Updated `docs/specs/RAG-SOP-004-DECISION.md` with measured evidence, a threshold-by-threshold comparison, a multi-hop section, and the decision line left as `PENDING OPERATOR`.

### Acceptance Criteria

- Every number in the report links to a recorded run row.
- The three runs give identical arm A results, and any arm B or C differences are listed and explained.
- Each H-2 threshold and each §4 hard blocker is marked met or not met, with the supporting rows.
- The report proposes an outcome but doesn't record an operator decision, and `SOP_RAG_ENABLED` defaults are unchanged.

### Acceptance Checks

- `go build ./...`
- `go test -short ./...`
- `git diff --check`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`
- `git grep -q "PENDING OPERATOR" -- docs/specs/RAG-SOP-004-DECISION.md`

### Acceptance Mapping

- criterion 1: manual: each number in RAG-SOP-004-DECISION.md links to a recorded run row
- criterion 2: manual: the three run directories show identical arm A rows
- criterion 3: manual: each H-2 threshold and §4 hard blocker is marked met or not met with supporting rows
- criterion 4: check 5

## Out of scope

- Writing or changing the questions, labels, or thresholds.
- Tuning RAG (chunking, embedding model, top_k) to pass. If it fails, the report says so, and tuning is a separate plan.
- Graph retrieval. See the shelved `PLAN-RAG-Graph-Retrieval.md`.
- Enabling RAG by default.

## Dependency graph

```mermaid
graph TD
    H1[H-1 dataset, human]
    H2[H-2 thresholds, human]
    E1[RAG-EVAL-001 loader and integrity]
    E2[RAG-EVAL-002 three-arm runner]
    E3[RAG-EVAL-003 failure fixtures]
    E4[RAG-EVAL-004 full run and evidence]
    H1 --> E1
    H2 --> E4
    E1 --> E2
    E1 --> E3
    E2 --> E4
    E3 --> E4
```
