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
- Placement applies only to arm B, the only arm that assembles a SOP context. Arms A, C and D record whether injected text was retrieved, set placement to not applicable, and never raise the placement hard-blocker.
- If a knowledge-rag citation doesn't split back into the evidence's own `Corpus` and `Source`, the run fails with an error naming the citation.

The rest of this task is already implemented and passes. The one change left is in `internal/rageval/runner.go`: `Runner.Run` must return an error, not panic or return rows, when `Dataset` is nil or `Provider` is nil. Add `TestRunnerRejectsNilInputs` in `internal/rageval/runner_test.go`. Make that change first; don't re-verify the rest by hand, since validation checks it.

Each named test must exist. `go test -run <Name>` passes when no test matches, so every named test also has a `git grep --untracked` existence check. A missing test fails that check.

### Dependencies

- RAG-EVAL-001

### Deliverables

- `internal/rageval/runner.go` and `internal/rageval/metrics.go`.
- `cmd/rag-eval run` subcommand.
- Unit tests for metric math on hand-computed fixtures.
- The dataset loader fills `InjectedText` for each `injection` question with the `evidence` phrase of its expected entry whose source is under `evals/rag/injection/`. It is that phrase, not the whole planted file. `evals/rag/questions.json` stays unchanged.
- Named tests in `internal/rageval`: `TestArmAMatchesSOPRetrieval`, `TestNoLocalBM25`, `TestArmDUsesFullTextRanker`, `TestCommittedInjectionQuestionsHaveInjectedText`, and `TestArmBUsesSOPContextBuild`.

### Acceptance Criteria

- Metric functions match hand-computed values on the fixtures.
- Two runs with the same dataset, corpus, and frozen RAG responses produce identical rows and ordering.
- A cross-corpus item in any arm is flagged in its row and marks the run as a hard-blocker failure.
- `no_answer` questions record what each arm returned instead of scoring them as recall.
- `injection` questions record whether injected text reached the assembled context, and confirm it was never placed above task or policy content.
- Arm A returns the same ranked identities as `retrieval.CandidatesFromIndex` plus `retrieval.Index.Search` on the same corpus and query. The test writes a fixture corpus to a temp directory, builds the expected results by calling `repoindex.Build`, `retrieval.CandidatesFromIndex`, `retrieval.New`, and `Search` directly, and compares them with the runner's arm A rows (`TestArmAMatchesSOPRetrieval`).
- `internal/rageval` contains no BM25 implementation of its own (`TestNoLocalBM25`).
- Arm D ranks with `internal/retrieval` over full document text, and no threshold reads its rows (`TestArmDUsesFullTextRanker`).
- Every committed `injection` question loads with an `InjectedText` equal to the `evidence` phrase of its planted expected entry, and that phrase appears verbatim in the planted file (`TestCommittedInjectionQuestionsHaveInjectedText`).
- The repository stays gofmt-formatted.
- Arm B's ranked identities, assembled context, bytes, and truncation equal what `sopctx.KnowledgeItems` plus `sopctx.Build` produce for the same arm A hits and frozen evidence, built directly in the test. The test computes the expected identities and corpora from the frozen evidence's `Corpus` and `Source` fields, not with the runner's own helpers, and includes evidence with a section and a page. `internal/rageval` holds no `mergeRanks` (`TestArmBUsesSOPContextBuild`).
- Only arm B evaluates injection placement. An injection question whose planted phrase arm C retrieves records the phrase as retrieved, with placement not applicable, and doesn't make the run a hard-blocker (`TestInjectionPlacementOnlyArmB`).
- A knowledge-rag citation that doesn't split back into its evidence's own `Corpus` and `Source` makes `Runner.Run` return an error that names the citation. `splitCitation` or its caller returns that error; it is never silently ignored (`TestInvalidCitationFailsRun`).
- `Runner.Run` returns an error naming the missing input when `Dataset` or `Provider` is nil (`TestRunnerRejectsNilInputs`).

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test -count=1 ./internal/rageval/... ./cmd/rag-eval/...`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`
- `go test -count=1 -run TestArmAMatchesSOPRetrieval ./internal/rageval/`
- `go test -count=1 -run TestNoLocalBM25 ./internal/rageval/`
- `go test -count=1 -run TestArmDUsesFullTextRanker ./internal/rageval/`
- `go test -count=1 -run TestCommittedInjectionQuestionsHaveInjectedText ./internal/rageval/`
- `go test -count=1 ./internal/archtest/`
- `go test -count=1 -run TestArmBUsesSOPContextBuild ./internal/rageval/`
- `go test -count=1 -run TestInjectionPlacementOnlyArmB ./internal/rageval/`
- `go test -count=1 -run TestInvalidCitationFailsRun ./internal/rageval/`
- `git grep -q --untracked "func TestArmAMatchesSOPRetrieval" -- internal/rageval`
- `git grep -q --untracked "func TestNoLocalBM25" -- internal/rageval`
- `git grep -q --untracked "func TestArmDUsesFullTextRanker" -- internal/rageval`
- `git grep -q --untracked "func TestCommittedInjectionQuestionsHaveInjectedText" -- internal/rageval`
- `git grep -q --untracked "func TestArmBUsesSOPContextBuild" -- internal/rageval`
- `git grep -q --untracked "func TestInjectionPlacementOnlyArmB" -- internal/rageval`
- `git grep -q --untracked "func TestInvalidCitationFailsRun" -- internal/rageval`
- `go test -count=1 -run TestRunnerRejectsNilInputs ./internal/rageval/`
- `git grep -q --untracked "func TestRunnerRejectsNilInputs" -- internal/rageval`

### Acceptance Mapping

- criterion 1: check 3
- criterion 2: check 3
- criterion 3: check 3
- criterion 4: check 3
- criterion 5: check 3
- criterion 6: check 5
- criterion 7: check 6
- criterion 8: check 7
- criterion 9: check 8
- criterion 10: check 9
- criterion 11: check 10
- criterion 12: check 11
- criterion 13: check 12
- criterion 14: check 20

## RAG-EVAL-002B — Fix the harness producer query

Fix one function in one file: `writeProducer` in `internal/rageval/ragacceptance_test.go`. Change nothing else.

Its producer script's SQL currently filters with `to_tsvector('simple', text) @@ plainto_tsquery('simple', '$query')`. That requires every query word, so the seeded questions match nothing. Replace that condition with an any-word match: split the query on spaces, and keep a row when its lowercased text contains at least one query word. Change `ORDER BY source` to `ORDER BY embedding <-> '[1,0,0]', source`. Keep the corpus filter unchanged.

The any-word SQL change is already in place. One bug remains in the same script: the loop line `printf '%%s' "$rows" | while IFS='|' read -r source section text; do` drops the last row, because `$(...)` strips the trailing newline and `read` returns false on a final line with no newline. With a single matching row, the producer returns nothing. Change that `printf '%%s'` to `printf '%%s\n'` (inside the Go format string, so the script runs `printf '%s\n'`). That fix is now in place too.

One more edit, in the same file, function `assertAcceptanceRows`. Its injection block reads the `Q-ACC-INJECT/` row for `ArmRAG` (arm C) and requires `InjectionBelowTask` to be set. That contradicts RAG-EVAL-002: only arm B evaluates placement. Change the block to:

- for the arm C row (`ArmRAG`): require `InjectionReached` to be non-nil and `InjectionBelowTask` to be nil;
- for the arm B row (`ArmHybrid`): require `InjectionReached` and `InjectionBelowTask` to be non-nil, and `*InjectionBelowTask` to be true.

That edit is now in place too.

Last edit, in the same file. `assertRealCrossCorpusBlocker` expects the real backend to return a foreign-corpus item. It never can: the producer filters by the requested corpus, and `ragclient` rejects evidence whose corpus differs from the request. Replace it with `assertRealCorpusIsolation`: keep the same setup (foreign corpus allowlisted, request corpus `acceptanceCorpus`), then require every retrieved item in every row to carry `acceptanceCorpus`, every row's `CrossCorpus` to be empty, and the run status not to be `StatusHardBlocker`. Update its call site. Keep `acceptanceCrossCorpusResult` and its assertions unchanged; that frozen case still proves a leaked item is flagged and hard-blocks.

Don't change the runner. These edits in `ragacceptance_test.go` are the whole task.

This is a harness integration test of the `ragclient` wiring against real PostgreSQL/pgvector, not a retrieval-quality test. It reads `RAGEVAL_PSQL_DSN` and fails, not skips, when it is unset. Run it with `RAGEVAL_PSQL_DSN=postgres://rag:rag@127.0.0.1:5434/rag?sslmode=disable` set.

### Dependencies

- RAG-EVAL-002

### Deliverables

- `internal/rageval/ragacceptance_test.go` with the corrected `writeProducer` query.

### Acceptance Criteria

- The harness integration test passes against real PostgreSQL/pgvector, and every seeded question that has expected evidence gets at least one evidence item from the producer.
- `evals/rag/` and `docs/specs/RAG-EVALUATION.md` are unchanged.

### Acceptance Checks

- `go build ./...`
- `go vet -tags ragacceptance ./internal/rageval/...`
- `go test -count=1 -tags ragacceptance ./internal/rageval/...`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`

### Acceptance Mapping

- criterion 1: check 3
- criterion 2: check 4

## RAG-EVAL-003 — End-to-end failure fixtures

Record the operational matrix from `RAG-EVALUATION.md` §5 and the decision report §4 at the RAG layer of SOP's real run path. Each fixture runs SOP's `run` command in-process through the existing `internal/cli` test harness, with the real `ragclient` executing a producer. Only the agent is faked. The tests prove what SOP does when the producer fails: an optional task falls back to BM25-only and records why, and a task with required evidence blocks.

Write the tests in a new file, `internal/cli/rag_fixtures_test.go`, with the build tag `ragacceptance` and `package cli`. Delete `internal/rageval/fixtures_test.go`; it drove the `sop` binary with guessed configuration and is replaced by this file. Use these existing helpers from `internal/cli` as written. Don't reimplement them:

- `ragReqEnv(t)` sets `SOP_RAG_ENABLED`, `SOP_RAG_EXECUTABLE`, `SOP_RAG_CORPUS` and `SOP_RAG_ALLOWED_CORPORA` (corpus `kb`). After calling it, point the producer at your script with `t.Setenv(config.EnvRAGExecutable, path)`.
- Optional-RAG task: `dir := threeStageGraph(t)`, then an agent built exactly as in `TestRAGTelemetryOptionalFallback` in `rag_required_evidence_test.go` (a `ragCapturingAgent` wrapping `fakeCapabilityAgent` with `maxTasksPlanJSON`), then `runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--max-tasks", "1")`. Read the outcome with `readRAGTelemetry(t, dir, "S001")`.
- Required-evidence task: `dir := requiredEvidenceGraph(t)`, `a := newReqAgent()`, then the same `runInjectedCLI` call. A block means `len(a.inputs) == 0` and the output names `REQUIRED_EVIDENCE_MISSING`.
- Write each producer as a `#!/bin/sh` script in `t.TempDir()` with mode 0755. It reads one request line with `read -r req || true`, then prints one `rag-retrieval/v1` JSON response or exits non-zero.

The eight cases, each its own test named `TestRAGFixture<Case>`:

- `ColdStart`: producer returns status OK with no evidence. Optional task: exit OK, `used_in_context` false.
- `StaleIndex`: producer returns status ERROR with error_code STALE_INDEX. Optional task: exit OK, telemetry status failed, reason_code OPTIONAL_FALLBACK, `used_in_context` false.
- `EmbeddingProviderUnavailable`: producer returns status ERROR with error_code UNAVAILABLE, standing in for Ollama being down. Optional task: as `StaleIndex`.
- `PostgresUnavailable`: the producer really runs `psql` against `postgres://rag:rag@127.0.0.1:1/rag?sslmode=disable`, a port with nothing listening, and exits with psql's status. Optional task: as `StaleIndex`.
- `MalformedProducerResponse`: producer prints `not json`. Optional task: as `StaleIndex`.
- `CrossRepositoryScope`: producer returns one evidence item whose corpus is `other`. Optional task: as `StaleIndex`, and no `other` item reaches the agent's prompt.
- `RequiredEvidenceBlocks`: producer returns status ERROR with error_code UNAVAILABLE. Required-evidence task: blocked, agent not invoked.
- `ResumedRun`: the `StaleIndex` setup run twice in the same `dir`. Both runs record the same telemetry status and reason_code.

Update `docs/reports/rag-eval/fixtures.md`: for each case, say whether the failure is scripted or real, and give the asserted outcome.

### Dependencies

- RAG-EVAL-001
- RAG-EVAL-002B

### Deliverables

- `internal/cli/rag_fixtures_test.go` (tag `ragacceptance`) with the eight `TestRAGFixture<Case>` tests.
- `docs/reports/rag-eval/fixtures.md` listing each case, how it was produced, and its asserted outcome.

### Acceptance Criteria

- All eight `TestRAGFixture` tests exist and pass.
- Optional-task failure cases exit OK with telemetry reason_code OPTIONAL_FALLBACK and `used_in_context` false. A silent pass with retrieved evidence in context fails the test.
- The required-evidence case blocks with `REQUIRED_EVIDENCE_MISSING`, and the agent is not invoked.
- `PostgresUnavailable` runs a real `psql` against an unreachable address, not a canned exit code.
- `docs/reports/rag-eval/fixtures.md` states for each case whether the failure is scripted or real.
- `evals/rag/` and `docs/specs/RAG-EVALUATION.md` are unchanged.

### Acceptance Checks

- `go build ./...`
- `go vet -tags ragacceptance ./internal/cli/... ./internal/rageval/...`
- `go test -count=1 -tags ragacceptance -run TestRAGFixture ./internal/cli/`
- `git grep -q --untracked "func TestRAGFixtureColdStart" -- internal/cli`
- `git grep -q --untracked "func TestRAGFixturePostgresUnavailable" -- internal/cli`
- `git grep -q --untracked "func TestRAGFixtureRequiredEvidenceBlocks" -- internal/cli`
- `git grep -q --untracked "func TestRAGFixtureResumedRun" -- internal/cli`
- `git grep -q --untracked "127.0.0.1:1/rag" -- internal/cli/rag_fixtures_test.go`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`

### Acceptance Mapping

- criterion 1: check 3
- criterion 2: check 3
- criterion 3: check 3
- criterion 4: check 8
- criterion 5: manual: docs/reports/rag-eval/fixtures.md marks each case scripted or real
- criterion 6: check 9

## RAG-EVAL-004A — Make `rag-eval run` refuse stubs and record the producer

Change `cmd/rag-eval/main.go` and `internal/rageval/report.go` only. The `run` subcommand must never fall back to a stub, and every report must name the real producer it measured.

- In `ragProvider`, when `RAG_EVAL_RAG_EXECUTABLE` is empty, return an error instead of the empty `FrozenProvider`. The `run` subcommand then exits non-zero and writes no report directory.
- The `run` subcommand also requires the environment variables `RAG_EVAL_PRODUCER_COMMIT` and `RAG_EVAL_EMBED_MODEL`. If either is empty, it exits non-zero and writes no report directory.
- Add an optional `-excluded` flag naming a file with one corpus source per line, the documents the producer could not ingest.
- Add a `Producer` field to `rageval.Header` with `Executable`, `Commit`, `EmbedModel`, and `ExcludedSources []string`, filled from the above. `WriteReports` writes it in `results.json` and as header lines in `results.md`.
- Change the `-corpus` flag default from `rag` to `sop-engineering-kb`.

Add `cmd/rag-eval/main_test.go` with two tests. `TestRunRequiresProducer` sets `RAG_EVAL_RAG_EXECUTABLE` to empty with `t.Setenv`, runs the `run` subcommand's function with `-out` pointing at `t.TempDir()`, and asserts a non-zero result and an empty output directory. `TestHeaderRecordsProducer` writes a report through `rageval.WriteReports` with a `Producer` set, and asserts that `results.json` contains the commit, the embedding model, and every excluded source. If `main` calls `os.Exit` directly, move the subcommand body into a function that returns an exit code so the test can call it.

### Dependencies

- RAG-EVAL-002B
- RAG-EVAL-003

### Deliverables

- `cmd/rag-eval/main.go` with the refusal, the required variables, the `-excluded` flag, and the new corpus default.
- `cmd/rag-eval/main_test.go` with `TestRunRequiresProducer` and `TestHeaderRecordsProducer`.
- `internal/rageval/report.go` with the `Producer` header field.

### Acceptance Criteria

- `rag-eval run` with no `RAG_EVAL_RAG_EXECUTABLE` exits non-zero and writes no report (`TestRunRequiresProducer`).
- A written report records the producer commit, embedding model, and excluded sources (`TestHeaderRecordsProducer`).
- Existing `internal/rageval` tests still pass.
- `evals/rag/` and `docs/specs/RAG-EVALUATION.md` are unchanged.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test -count=1 ./internal/rageval/... ./cmd/rag-eval/...`
- `go test -count=1 -run TestRunRequiresProducer ./cmd/rag-eval/`
- `go test -count=1 -run TestHeaderRecordsProducer ./cmd/rag-eval/`
- `git grep -q --untracked "func TestRunRequiresProducer" -- cmd/rag-eval`
- `git grep -q --untracked "func TestHeaderRecordsProducer" -- cmd/rag-eval`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`

### Acceptance Mapping

- criterion 1: check 4
- criterion 2: check 5
- criterion 3: check 3
- criterion 4: check 8

## RAG-EVAL-004C — Score retrieval at document level

The first measurement runs scored every lexical arm at zero. `expectedIDs` in `internal/rageval/runner.go` builds expected identities as `source#section`. Arms A, B and D return file-level identities (`source` only), so they can never match. Arm C returns the producer's `source#section`, but the producer's section names are its own `##` chunks, not the labeled headings. Recall must compare like with like.

Make recall, precision and citation correctness compare documents, not sections:

- In `expectedIDs`, return each expected `Source` once, without `#section`. Keep the order stable and drop duplicates.
- Add `func documentID(id string) string` in `internal/rageval/metrics.go` that returns the part before the first `#`, or the whole string when there is none.
- In `RankedIDs`, map each item's `ID` through `documentID` and keep the first occurrence of each document, so the top five are five distinct documents.
- Don't change `Retrieved.ID` values or the row's `retrieved` list. Rows still record what each arm returned, sections included.

Add `TestRecallIsDocumentLevel` in `internal/rageval/metrics_test.go`. It must show that an arm returning `a.md` and another returning `a.md#Some Section` both get recall 1.0 against an expected entry with source `a.md` and any section, and that two sections of the same file count as one item in the top five.

### Dependencies

- RAG-EVAL-004A

### Deliverables

- `internal/rageval/metrics.go` with `documentID` and document-level `RankedIDs`.
- `internal/rageval/runner.go` with document-level `expectedIDs`.
- `TestRecallIsDocumentLevel` in `internal/rageval/metrics_test.go`.

### Acceptance Criteria

- Recall, precision and citation correctness compare documents, not sections (`TestRecallIsDocumentLevel`).
- Existing `internal/rageval` and `cmd/rag-eval` tests still pass, updated only where they assumed section-level matching.
- `evals/rag/` and `docs/specs/RAG-EVALUATION.md` are unchanged.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test -count=1 ./internal/rageval/... ./cmd/rag-eval/...`
- `go test -count=1 -run TestRecallIsDocumentLevel ./internal/rageval/`
- `git grep -q --untracked "func TestRecallIsDocumentLevel" -- internal/rageval`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`

### Acceptance Mapping

- criterion 1: check 4
- criterion 2: check 3
- criterion 3: check 6

## RAG-EVAL-004D — Summarize recorded runs, with a post-hoc full-context score

Add a deterministic `summarize` subcommand to `cmd/rag-eval`. It reads recorded runs and computes every number the decision report uses, so no number is worked out by hand. It reads only files. It never runs a producer, a model, or a database.

Put the computation in a new file, `internal/rageval/summary.go`. Use these existing functions as written: `Load(dir string) (*Dataset, error)` in `dataset.go`, `RankedIDs(items []Retrieved, k int) []string` and `documentID(id string) string` in `metrics.go`, and `expectedIDs(q Question) []string` in `runner.go`.

- Add `func RecallAtK(expected []string, items []Retrieved, k int) (float64, bool)` in `metrics.go`. It works like `RecallAt5`, with depth `k`, and is document level. `RecallAt5` becomes `RecallAtK(expected, items, K)`.
- The subcommand is `rag-eval summarize -dir evals/rag -runs <dir>,<dir>,<dir> -control <dir> -out <dir>`. Each run directory holds a `results.json` written by `rag-eval run`. Read `rows` from it. Each row has `question_id`, `category`, `arm`, `retrieved` (a list of objects with `ID`, `Rank` and `Corpus`), `recall_at_5` and `latency_ms`. `latency_ms` is a decimal string or `unavailable`.
- Subsets: `historical` is the categories `architecture_history`, `design_decision`, `failure_investigation` and `multi_hop`. `code` is the `code` category. `multi_hop` is that category alone. `reworded` is every question with a non-empty `RewordedFrom`. Leave out questions with no expected sources.
- For each run, compute mean `recall_at_5` per arm (A, B, C, D) per subset.
- §4.1 checks, from the first run: `value` is mean B minus mean A on `historical`; `breadth` is the number of `historical` questions where B's recall@5 is higher than A's; `code_regressions` is the number of `code` questions where B's recall@5 is lower than A's; `latency_p95_ms` is the 95th percentile of arm B's numeric `latency_ms` (nearest rank). Compare each with `docs/specs/RAG-EVALUATION.md` §4.1: value at least 0.10, breadth at least 5, code regressions at most 1, p95 at most 500 ms. Also report `status` from each run's header.
- `arm_a_identical`: true when every run has the same arm A `(question_id, retrieved IDs in order)` list.
- Post-hoc full-context score, from the first run: for each arm B row, `RecallAtK(expectedIDs(q), row.retrieved, len(row.retrieved))`. Report its mean per subset, the mean number of distinct documents in arm B's context, and how many `historical` questions improve or regress against arm A's recall@5.
- Controls, from the `-control` run (recorded with `-top-k 10`): mean `RecallAtK(expectedIDs(q), row.retrieved, 10)` for arms A and D, per subset.
- Write `summary.json` and `summary.md` into `-out`. Every number names the run directory it came from. The post-hoc section and the controls are headed `POST-HOC: not part of the §4.1 decision rule`.

Add `TestSummarizeFixture` in `internal/rageval/summary_test.go`. It builds two identical primary runs and one control run in `t.TempDir()`, as `results.json` files over questions from the `testdata/valid` dataset, with hand-computed expected values. It asserts the §4.1 numbers, `arm_a_identical`, the post-hoc mean, the control means, and that `summary.md` contains the `POST-HOC` heading. Add `TestRecallAtK` in `metrics_test.go`, covering depth 5, depth 10, and two sections of one document counting once.

### Dependencies

- RAG-EVAL-004C

### Deliverables

- `internal/rageval/summary.go` with the summary computation.
- `RecallAtK` in `internal/rageval/metrics.go`.
- The `summarize` subcommand in `cmd/rag-eval/main.go`.
- `TestSummarizeFixture` in `internal/rageval/summary_test.go` and `TestRecallAtK` in `internal/rageval/metrics_test.go`.

### Acceptance Criteria

- The summary computes the §4.1 checks, `arm_a_identical`, the post-hoc full-context score and the controls exactly as specified (`TestSummarizeFixture`).
- `RecallAtK` is document level at any depth, and `RecallAt5` equals `RecallAtK` at depth 5 (`TestRecallAtK`).
- `summary.md` labels the post-hoc section and controls as outside the §4.1 rule.
- Existing `internal/rageval` and `cmd/rag-eval` tests still pass.
- `evals/rag/` and `docs/specs/RAG-EVALUATION.md` are unchanged.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `go test -count=1 ./internal/rageval/... ./cmd/rag-eval/...`
- `go test -count=1 -run TestSummarizeFixture ./internal/rageval/`
- `go test -count=1 -run TestRecallAtK ./internal/rageval/`
- `git grep -q --untracked "func TestSummarizeFixture" -- internal/rageval`
- `git grep -q --untracked "func TestRecallAtK" -- internal/rageval`
- `git grep -q --untracked "POST-HOC: not part of" -- internal/rageval/summary.go`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`

### Acceptance Mapping

- criterion 1: check 4
- criterion 2: check 5
- criterion 3: check 8
- criterion 4: check 3
- criterion 5: check 9

## RAG-EVAL-004B — Decision evidence from the recorded runs

The operator runs everything this task reads, outside SOP, after RAG-EVAL-004D. Don't run `rag-eval`, `go run`, or the producer yourself. Work only from these committed files:

- the three primary runs under `docs/reports/rag-eval/` whose header names producer commit `acd1fb911ac8bdd4f425b61df16a495f4a5047c9`, embedding model `nomic-embed-text`, and `TopK` 5;
- one control run under `docs/reports/rag-eval/` with the same producer and `TopK` 10;
- `docs/reports/rag-eval/summary/summary.md` and `summary.json`, written by `rag-eval summarize` from those runs.

If any of these is missing, stop and raise a question. Take every number from `summary.json`. Don't compute numbers yourself.

Update `docs/specs/RAG-SOP-004-DECISION.md`:

- In §2–§4, change Unavailable to Measured where the summary supports it, with links to the run directories and the summary.
- Add a draft §5 that compares the §4.1 checks line by line, as computed in the summary, and states which outcome the rule gives. Explain why arm B's top five equals arm A's: SOP ranks repository items above RAG items.
- Add a separate section headed `POST-HOC: not part of the §4.1 decision rule`. Report arm B's full-context score next to the depth-10 controls for arms A and D, the mean number of documents in arm B's context, and the improved and regressed counts. State plainly that this metric was chosen after seeing results, that arm B uses more context slots than arm A's top five, and that the depth-10 controls exist to make the comparison fair.
- Include multi-hop and reworded-question results from the summary, and list the multi-hop misses by question ID.
- State that the producer could not ingest the 37 excluded documents (none is expected evidence), and that arms A and D still index them.
- Leave the operator decision marked `PENDING OPERATOR`. Don't recommend GO or NO-GO beyond what the §4.1 rule gives.

### Dependencies

- RAG-EVAL-004D

### Deliverables

- Updated `docs/specs/RAG-SOP-004-DECISION.md` with measured evidence, the §4.1 comparison, the post-hoc section, multi-hop and reworded results, the exclusion note, and the decision line left as `PENDING OPERATOR`.

### Acceptance Criteria

- Every number in the report matches `summary.json` and links to the summary or a run directory.
- Each §4.1 threshold and each §4 hard blocker is marked met or not met, with supporting numbers.
- The post-hoc section is labeled as outside the §4.1 rule, reports the depth-10 controls next to arm B's full-context score, and states that the metric was chosen after seeing results.
- The report records no operator decision, and `SOP_RAG_ENABLED` defaults are unchanged.
- The report names the producer commit, the embedding model, and the 37 exclusions.

### Acceptance Checks

- `go build ./...`
- `go test -short ./...`
- `git diff --check`
- `git diff --quiet main -- evals/rag docs/specs/RAG-EVALUATION.md`
- `git grep -q "PENDING OPERATOR" -- docs/specs/RAG-SOP-004-DECISION.md`
- `git grep -q --untracked "POST-HOC: not part of the" -- docs/specs/RAG-SOP-004-DECISION.md`
- `git grep -q --untracked "acd1fb911ac8bdd4f425b61df16a495f4a5047c9" -- docs/specs/RAG-SOP-004-DECISION.md`

### Acceptance Mapping

- criterion 1: manual: every number in RAG-SOP-004-DECISION.md matches summary.json
- criterion 2: manual: each 4.1 threshold and 4 hard blocker is marked met or not met with supporting numbers
- criterion 3: check 6
- criterion 4: check 5
- criterion 5: check 7

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
    E2[RAG-EVAL-002 four-arm runner]
    E2B[RAG-EVAL-002B harness producer fix]
    E3[RAG-EVAL-003 failure fixtures]
    E4A[RAG-EVAL-004A refuse stubs, record producer]
    R[three measurement runs, operator]
    E4B[RAG-EVAL-004B decision evidence]
    H1 --> E1
    E1 --> E2
    E2 --> E2B
    E2B --> E3
    E2B --> E4A
    E3 --> E4A
    E4A --> E4C[RAG-EVAL-004C document-level scoring]
    E4C --> E4D[RAG-EVAL-004D summarize runs]
    E4D --> R
    R --> E4B
    H2 --> E4B
```
