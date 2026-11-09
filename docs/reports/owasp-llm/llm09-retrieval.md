# LLM09 — Retrieval boundaries in agentic-sop

Status: **VERIFIED for the tested Linux scope; enabled-by-default claims are not made.**
Baseline: `5ac9b4ba6ba6d15b66f74aec19cffb8ef084a775` (`origin/main`), with the
RETRIEVE-001 inventory committed at `536057ab698034039dba11417b412c17441d521d`.
Tested platform: Linux `7.0.0-38-generic` x86_64, Go `1.27.1`,
`modernc.org/sqlite v1.59.0`.

This report is the RETRIEVE-001 deliverable, extended with the RETRIEVE-002
isolation/stale-record evidence and the RETRIEVE-003 rebuild evidence. The
RETRIEVE-001 inventory was accepted at report SHA-256
`b3c9880d2943d836559f4675200c6cb1697af8a6adc7697ab527e7618f1e63c4`; this revision
adds later evidence and therefore has a different hash. The accepted hash is
preserved in the SOP approval and acceptance-decision audits, not rewritten here.

## Scope and trusted binding

Retrieval is single-user and repository-scoped. There is no tenant or workspace
identity, and none is invented.

| Identity | Source | Classification |
| --- | --- | --- |
| `Scope.Corpus` | Resolved `advisory_retrieval.allowed_corpora` or `rag.allowed_corpora`; exactly one corpus per query | Operator-controlled allowlist; `retrievalboundary.Validate` rejects an unknown corpus (`ErrUnsupportedCorpus`) |
| `Scope.Repository` | `config.Project.Name`, passed by `internal/cli/run.go` into `advisoryKnowledgeForAttempt` | Trusted project binding |
| `Scope.TaskID` | Deliberately empty on the advisory path, because advisory memory is history from other tasks | Optional narrowing; unused for advisory memory |
| `ScopedQuery.Text` | Deterministically derived from task metadata and, in recovery, the recorded failure context; truncated to 512 runes | Caller-controlled; selects candidates only and never widens scope |

The advisory path now fails closed when the trusted repository is empty:
`retrieveAdvisory` returns `ADVISORY_INVALID_QUERY` and performs no query. This
closes the previously possible unscoped read at the exported advisory seam.
Production still supplies `cfg.Project.Name`; the guard is defense in depth.

The local backend applies corpus, eligibility, repository, and task predicates in
SQL before ranking (`internal/memindex/index.go`, `Search`). `mapAdvisoryResult`
rechecks both the evidence corpus and the evidence repository against the trusted
query scope before mapping, so an injected or faulty backend cannot return a
cross-repository record as advisory evidence. Evidence whose repository is empty
or different is dropped; a backend that cannot populate `Evidence.Repository`
therefore fails closed for scoped advisory retrieval.

The BM25 ranker adapter (`internal/retrievalboundary/ranker.go`) rejects a query
that carries a non-empty repository or task scope because the in-memory index has
no such metadata. It fails closed rather than returning evidence as if it matched
a narrower scope.

## Scope trace

### Advisory retrieval — local SQLite FTS5

1. Request: `advisoryKnowledgeForAttempt` skips advisory retrieval when the task
   declares `RequiredEvidence`; otherwise it calls `retrieveAdvisory`.
2. Disabled or invalid configuration returns no items.
3. Query construction: `advisoryQuery` builds one `ScopedQuery` per configured
   corpus with `Scope.Corpus`, `Scope.Repository`, `Scope.TaskID`, `Text`, and the
   configured bounds. `retrievalboundary.Validate` normalizes the limits.
4. Trusted-scope guard: an empty repository fails closed before any backend call.
5. Retriever selection: `openAdvisoryRetriever` opens the separate SQLite index and
   wraps it in `memindex.LocalRetriever` with a gated current-source snapshot.
6. Datastore query: `LocalRetriever.Retrieve` fetches up to the boundary ceilings
   through `Index.Search`.
7. SQL: `BuildFTSQuery` quotes each token and ORs them, so user text cannot break
   FTS syntax. `Search` applies `records_fts MATCH ? AND r.corpus = ? AND
   r.eligible = 1`, then the repository filter when set, then the task filter, and
   orders by `bm25` score and record ID.
8. Post-filter: a relevance floor keeps only rows sharing at least two distinct
   query terms (one for a one-term query).
9. Returned context: the gated current-source snapshot excludes records whose
   source artifact is deleted or whose digest no longer matches; drift exclusion
   then drops any fetched row that no longer matches the current outcome. The
   boundary applies the result-count and token budgets. `mapAdvisoryResult` drops
   cross-corpus, cross-repository, approval-shaped, and sub-floor records, then
   yields low-priority untrusted context items.

### BM25 code and repository context

1. Request: `internal/cli/run.go` sets `Retrieved: codeRetrievalItems(...)` in
   `sopctx.Inputs` when `context.code_retrieval` is enabled.
2. `codeRetrievalItems` calls `codeRetrievalHits`, which builds the Structural
   Repository Index with `repoindex.Build(repoindex.Options{Root: dir})`. `dir` is
   the project working directory; the index is workspace-bound and reads no other
   checkout.
3. `retrieval.New(retrieval.CodeCandidatesFromIndex(idx))` ranks symbols and file
   paths with deterministic BM25; `QueryFromEvidence` assembles the task ID,
   title, acceptance criteria, and failure context. The top five pointers become an
   advisory list of IDs and paths, never file contents.
4. The result is a `sopctx.Item` with `SourceRetrieval` and `PriorityGeneric`; the
   context engine appends it to the canonical items. It is advisory and is not an
   instruction, an approval, or authoritative execution evidence.
5. `sop retrieve` exposes the same model-free ranker over symbols, files, and
   documents. It is read-only and writes no artifact.

### BM25 ranker adapter

`RankerAdapter` adapts `retrieval.Index` to the bounded `Retriever` interface. It
checks the corpus, rejects repository/task narrowing it cannot honor, ranks the
full match set, and applies the result/token bounds. It is not wired to a
production datastore.

### External RAG producer client

1. Request: `ragclient.Client.Retrieve` sends one bounded `rag-retrieval/v1` request
   with the configured trusted corpus.
2. Scope checks: schema version, non-empty query, query size, non-empty corpus, and
   the operator allowlist. A corpus outside the allowlist is
   `UNAUTHORIZED_SCOPE`.
3. Datastore query: one configured executable, one JSON request, bounded response.
4. Response validation: strict JSON; every evidence item must carry the requested
   corpus; chunk and response sizes are bounded. A corpus mismatch rejects the whole
   response.
5. Returned context: validated, attributed, untrusted evidence. Required-evidence
   tasks use this path; optional tasks fall back to no RAG items on failure.

The RAG protocol has no repository identity. Its isolation unit is the corpus; this
report does not claim a project binding the protocol does not carry. Live producer,
PostgreSQL, embedding, and live-model behavior are **NOT_RUN**.

### Required evidence

`enforceRequiredEvidenceOnce` performs one attempt-scoped retrieval and validates
every declared `corpus:`/`source:` identity. A disabled RAG, invalid configuration,
timeout, unavailable producer, or missing identity becomes the existing
`REQUIRED_EVIDENCE_MISSING` block with no agent invocation. Optional RAG and optional
advisory retrieval never satisfy and never replace required evidence.

## Cache and invalidation trace

**No retrieval-result cache exists.** A repository search of
`internal/retrieval`, `internal/retrievalboundary`, `internal/memindex`,
`internal/ragclient`, `internal/repoindex`, and the advisory/code retrieval call
sites found no result-cache type or cache-key path. The prompt cache is a
model-response cache, not a retrieval cache, and is outside this scope. The
retrieval-result cache criterion is therefore **NOT_APPLICABLE**; the reused
surfaces below are a derived index and an attempt-scoped evidence handoff.

| Reuse point | Scope | Invalidation / validation |
| --- | --- | --- |
| SQLite FTS5 index | Separate database file from execution state; per configured workspace | Schema-version change drops/recreates tables; upsert is idempotent by content hash; rebuild reseeds from authoritative outcome files |
| Read-path current-source check | Same repository/corpus scope as the SQL query | `SnapshotGatedContext` applies `GateWithSourcesAndDigestsContext` with `FileSourceDigestContext`; a deleted or modified source artifact is excluded before a manual rebuild, and the configured retrieval deadline can cancel the check; `LocalRetriever` then applies drift exclusion |
| Rebuild | One outcomes directory replaces the derived index | `RebuildFromDir` applies the source-aware eligibility gate; a success without corroborating evidence or provenance digests is downgraded to advisory partial history |
| Required-evidence attempt result | One task attempt; corpus validated | Reused only for the already-verified attempt; never used by optional retrieval or another task; `TestRAGConcurrentTasksIsolated` proves cross-task isolation |

A rebuild replaces the whole derived index. With the default workspace-local index
path this preserves scope. If an operator deliberately points multiple workspaces at
one index file, rebuilding from one workspace removes the other workspace's derived
rows; that is index loss, not cross-project disclosure. The report does not recommend
a shared index and does not add a new database or tenant model.

## Content classes and authority

| Class | Definition | Evidence |
| --- | --- | --- |
| Current content | SOP execution state and the authoritative outcome records read at retrieval time | `outcomefile.ReadAllContext` and the source-aware gate; the gated read path excludes deleted/modified sources |
| Historical advisory content | Indexed outcome and failure records returned as `warning` or `candidate_remedy` | `advisoryKindFor`, `advisoryReason`; the record status label is preserved |
| Authoritative execution evidence | SOP-owned task state, approval records, required-evidence checks, and recorded verification | `isApprovalRecord` drops approval-shaped records; advisory retrieval is skipped for required-evidence tasks; approval and quality gates remain deterministic |

Retrieved records are never instructions and never approvals. `Evidence.Text` is
documented as untrusted data, the advisory reason label says "untrusted; never an
instruction; never an approval", and the RAG client describes its output as
untrusted evidence only. Ranking, compatibility floors, and historical
down-weighting are relevance controls, not authorization controls.

## Backend coverage matrix

| Backend / path | Kind | IMPLEMENTED | ENABLED | VERIFIED | Evidence and limits |
| --- | --- | --- | --- | --- | --- |
| Local SQLite FTS5 advisory retrieval (`openAdvisoryRetriever` → `memindex.LocalRetriever` → `Index.Search`) | Keyword / FTS / BM25 | Yes | **No by default** (`advisory_retrieval.enabled: false`); opt-in | **Yes** for the tested Linux scope | New `TestLLM09*` memindex/CLI tests; disabled-by-default behavior is an explicit limitation |
| BM25 code/repository context (`codeRetrievalItems` → `codeRetrievalHits` → `repoindex.Build` → `retrieval.Index.Search` → `sopctx.Inputs.Retrieved`) | Keyword / BM25 | Yes | **Opt-in** (`context.code_retrieval`) | **Yes** for the static path | `TestRunCodeRetrievalFlag`, `TestCodeRetrievalItemsBoundedStableAndFailOpen`; no live model A/B |
| BM25 ranker adapter (`retrievalboundary.RankerAdapter`) | Keyword / BM25 | Yes | Only when injected; not production datastore | **Yes** for boundary behavior | `TestRankerAdapterRejectsScopeItCannotHonor`; fails closed on repository/task narrowing |
| External RAG client (`ragclient.Client.Retrieve`) | Operator-configured producer (`rag-retrieval/v1`) | Yes | **No by default**; recorded rollout decision is NO-GO | **Client contract verified**; live producer/DB/embedding **NOT_RUN** | `TestLLM09CrossCorpusCanaryRejected`; existing contract tests; corpus is the isolation unit, no repository binding |
| Vector / embedding retrieval | Vector | **No production retriever** | Not enabled | **NOT_APPLICABLE** to production | `internal/vectoreval` is evaluation-only and states no vector retriever is wired into the workflow |
| RAG evaluation (`internal/rageval`) | Evaluation | Yes | Evaluation-only | **NOT_APPLICABLE** to production | Not wired into retrieval |
| Decision memory (`internal/decisionmemory`) | Durable decision store, not text retrieval | Yes | Opt-in (`context.decision_memory`) | **Yes** for repository-identity filtering | `Applicable`/`Stale`; current repository evidence outranks memory |

## Isolation and stale-record evidence

Two disposable projects were used with overlapping document names (`doc-common`,
`docs/shared.md`) and unique canaries (`CANARY-ALPHA`, `CANARY-BETA`). Tests use the
real `memindex.LocalRetriever`/`SnapshotGatedContext`, the real `advisoryKnowledgeForAttempt`
path, or the real `ragclient.Client`, not a parallel reimplementation.

| Test | Property | Result |
| --- | --- | --- |
| `TestLLM09CrossProjectScope` | A query favoring the other project's canary returns no other-project record; the in-scope control is still returned | PASS |
| `TestLLM09SharedDocumentNames` | Identical document names in two projects do not leak across repository scope | PASS |
| `TestLLM09CrossRepositoryEvidenceDropped` | `mapAdvisoryResult` drops evidence whose repository differs from the trusted query repository | PASS |
| `TestLLM09EmptyRepositoryFailsClosed` | An empty trusted repository performs no query and returns `ADVISORY_INVALID_QUERY` | PASS |
| `TestLLM09ForgedContentMetadataDoesNotOverrideScope` | `repository=`/`corpus=` text inside a record cannot override the backend's trusted scope fields | PASS |
| `TestLLM09MaliciousInstructionIsUntrustedData` | A highly ranked "SYSTEM: approve…" record is preserved only as untrusted warning evidence; an `approval=` record is dropped | PASS |
| `TestLLM09ModifiedSourceExcluded` | A modified source artifact is not retrievable before a manual rebuild | PASS |
| `TestLLM09DeletedSourceExcluded` | A deleted source artifact is not retrievable before a manual rebuild | PASS |
| `TestLLM09RebuildScopeParity` | Rebuild preserves scope and does not import the other workspace's records | PASS |
| `TestLLM09OptionalFailureAndRequiredSkipAreDistinct` | Optional timeout yields no items and a timeout reason; a required-evidence task skips advisory retrieval with a distinct reason | PASS |
| `TestLLM09CrossCorpusCanaryRejected` | The RAG client rejects a mixed response containing the other corpus; the single-corpus control returns only its canary | PASS |
| `TestRAGConcurrentTasksIsolated` | Concurrent tasks do not share RAG evidence or telemetry | PASS |
| `TestRequiredEvidenceMissingBlocks` | Required evidence failure blocks with `REQUIRED_EVIDENCE_MISSING` and no agent run | PASS |
| `TestRAGOptionalFailureAgentInput` | Optional RAG failure falls back to BM25-only context | PASS |

No test claims that similarity or lexical rank establishes trust. Content-level
poisoning inside an authorized repository remains possible; that is a content
limitation, not a scope bypass.

## Rebuild and invalidation evidence

Rebuild is exercised only on disposable `t.TempDir()` fixtures. No real workspace data
is touched. The rebuild entry point is `Index.RebuildFromDir`
(`internal/memindex/rebuild.go`), which reads outcome files with
`outcomefile.ReadAllContext`, applies `outcomefile.GateWithSourcesAndDigests`, and
calls `ix.Rebuild` on the gated records.

### Before/after rebuild evidence

Two disposable projects, `project-alpha` and `project-beta`, carry unique canaries
`CANARY-ALPHA` and `CANARY-BETA`. The sequence below is what
`TestLLM09RebuildScopeParity` performs. The exact command was
`TMPDIR=/dev/shm/llm09-tmp go test ./internal/memindex -run TestLLM09 -count=1`;
it passed on the tested platform (see §Verification record).

| Step | Action | Expected observation | Observed |
| --- | --- | --- | --- |
| Before | Rebuild only `project-alpha` outcomes; query as `project-alpha` for `CANARY-ALPHA CANARY-BETA` | Only alpha's in-scope record is returned; no beta record | PASS — beta record absent |
| After | Add `project-beta` outcome to the same outcomes directory; rebuild `RebuildFromDir` again on the alpha workspace | Alpha's record remains; beta's record still invisible because the trusted repository is alpha | PASS — rebuild does not import beta |
| Determinism | Rebuild the same frozen outcome directory twice | Same in-scope record set both times | PASS — identical result set |

The rebuild preserves scope because the trusted `Scope.Repository` filter is applied
at query time (`Index.Search`), independent of which rows the derived index holds. A
rebuild does not widen the trusted repository; it cannot surface another workspace's
records as in-scope evidence.

- `TestLLM09RebuildScopeParity` writes two projects' outcomes, rebuilds the derived
  index, and proves the query for project A returns only A while the project B record
  remains invisible. Rebuilding does not import the other workspace's evidence into
  the queried scope.
- `TestReindexProvenanceBeforeAndAfterRebuild` (Plan 05) proves a modified source is
  excluded on rebuild, a deleted run directory is never returned, and valid
  historical records remain usable.
- `TestLLM09ModifiedSourceExcluded` and `TestLLM09DeletedSourceExcluded` prove the
  read path now applies the same source-aware gate before a rebuild, so obsolete
  records are not used as current evidence merely because the index was not rebuilt.
- Rebuild removes only derived index rows; authoritative outcome files and user
  indexes are not deleted. A shared index file is an operator configuration, not a
  supported cross-project store; rebuilding it from one workspace drops the other
  workspace's derived rows as described above.

## Historical down-weighting

`internal/retrieval` applies `HistoricalWeight = 0.5` to document candidates whose
repository authority is historical. This is a relevance adjustment: the record still
appears, lower than current evidence for the same terms. It is not an authorization
control and does not mark a record verified. Code retrieval uses
`CodeCandidatesFromIndex`, which excludes document candidates, so the code path does
not apply the historical document weight. `TestCandidatesFromIndexFullTextAndHistory`
asserts the weight.

## Required-evidence timeout behavior

| Path | Timeout / failure | Required evidence? | Observed behavior |
| --- | --- | --- | --- |
| RAG producer | Request deadline or unavailable producer | Yes | `retrieveAttemptEvidence` returns the error; `enforceRequiredEvidenceOnce` records `REQUIRED_EVIDENCE_MISSING` and blocks before any agent call. No fallback to optional RAG or BM25 |
| RAG producer | Request deadline or unavailable producer | No | `optionalRAGKnowledge` returns nil; telemetry records `OPTIONAL_FALLBACK`; the task proceeds with the remaining deterministic context |
| Advisory local index | Time budget / invalid query / unavailable index | No | No items; a bounded `ADVISORY_TIMEOUT`, `ADVISORY_INVALID_QUERY`, or failure reason; the existing fallback path continues |
| Advisory local index | Task declares required evidence | N/A | Advisory retrieval is skipped entirely with `ADVISORY_REQUIRED_EVIDENCE`; it cannot replace or weaken required evidence |

## Criterion-to-evidence mapping

The four RETRIEVE-003 acceptance criteria are mapped to the evidence and the
IMPLEMENTED / ENABLED / VERIFIED dimension of each. No criterion, gate, or scope is
added beyond this list.

| # | Criterion | IMPLEMENTED | ENABLED | VERIFIED | Evidence |
| --- | --- | --- | --- | --- | --- |
| 1 | Rebuild preserves scope and does not silently import another workspace | Yes — `Index.RebuildFromDir` gates records with `GateWithSourcesAndDigests`; `Index.Search` filters by `Scope.Repository` | Rebuild path is used by `memindex.LocalRetriever`; advisory retrieval itself is opt-in | Yes for the tested scope | `TestLLM09RebuildScopeParity`; §Before/after rebuild evidence |
| 2 | Historical down-weighting is relevance behavior, not an authorization control | Yes — `HistoricalWeight = 0.5` in `internal/retrieval/retrieval.go` | Enabled on the BM25 document path only | Yes for the tested scope | `TestCandidatesFromIndexFullTextAndHistory`; §Historical down-weighting |
| 3 | Focused tests and required checks pass; PR lists backend versions and exact verified paths | Yes — repository-required checks recorded | `go build`/`go vet`/`go test` | Yes | §Verification record; backend matrix |
| 4 | Report links Plan 05 provenance evidence and Plan 07 evidence-authority boundaries | Yes — handoff section | N/A (documentation) | Yes | §Dependency handoff; `internal/memindex/provenance_test.go` |

## Verified references

The following repository paths were inspected for this revision. Each is a concrete,
verified location, not a placeholder.

- `internal/memindex/rebuild.go` — `RebuildFromDir` (rebuild) and `SnapshotGatedContext` (gated read path).
- `internal/memindex/llm09_test.go` — `TestLLM09RebuildScopeParity` and the other `TestLLM09*` scope tests.
- `internal/memindex/provenance_test.go` — `TestReindexProvenanceBeforeAndAfterRebuild` (Plan 05 provenance evidence).
- `internal/retrieval/retrieval.go` — `HistoricalWeight = 0.5` and `CandidatesFromIndex`.
- `internal/retrieval/retrieval_test.go` — `TestCandidatesFromIndexFullTextAndHistory`.
- `internal/outcomefile` — `ReadAllContext`, `GateWithSourcesAndDigests`, `FileSourceExists`, `FileSourceDigest`.

## Verification record

Platform and versions:

- Linux `7.0.0-38-generic` x86_64.
- Go `1.27.1`.
- `modernc.org/sqlite v1.59.0` (SQLite FTS5 is used by the local advisory index).
- SOP CLI `sop dev`; recorded binary revision `82ad60bc9316e58c5969db9631191403582f6ba6`,
  binary SHA-256 `f1830d1e6744d3de60ada7b55f010bc1a21202bc57efd0723fdea08c974a188e`
  (development build, `modified: true`).
- Base commit `5ac9b4ba6ba6d15b66f74aec19cffb8ef084a775`; RETRIEVE-001 commit
  `536057ab698034039dba11417b412c17441d521d`.

Focused commands and observed results (run in the isolated worktree; `TMPDIR` on
tmpfs avoids the slow SQLite fsync that otherwise exceeds Go's per-package timeout):

```sh
TMPDIR=/dev/shm/llm09-tmp go test ./internal/memindex -run TestLLM09 -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/cli -run TestLLM09 -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/ragclient -run TestLLM09 -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/cli -run 'TestRunCodeRetrievalFlag|TestCodeRetrievalItemsBoundedStableAndFailOpen' -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/cli -run TestRAGConcurrentTasksIsolated -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/cli -run TestRequiredEvidenceMissingBlocks -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/cli -run TestRAGOptionalFailureAgentInput -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/retrieval -run TestCandidatesFromIndexFullTextAndHistory -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/retrievalboundary -run 'TestRankerAdapterRejectsScopeItCannotHonor|TestValidateRejectsUnboundedOrInvalidLimits' -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/outcomefile -run TestIngestProvenance -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/outcomefile -run TestLLM09 -count=1
# ok
TMPDIR=/dev/shm/llm09-tmp go test ./internal/memindex -run 'TestReindexProvenance|TestStaleEvidenceIsNotReturned' -count=1
# ok
```

Repository-required checks (run after the final code and report revision):

```sh
GOFLAGS=-p=1 TMPDIR=/dev/shm/llm09-tmp go build ./...          # exit 0
GOFLAGS=-p=1 TMPDIR=/dev/shm/llm09-tmp go vet ./...            # exit 0
GOFLAGS=-p=1 TMPDIR=/dev/shm/llm09-tmp go test ./... -count=1  # exit 0; 0 failures
```

The `TMPDIR`/`GOFLAGS` settings are execution-environment settings for this
filesystem. The repository's configured validation command remains `go test ./...`;
the PR records the same result. The security scanner is report-only and recorded
`INDETERMINATE` in this environment; it is not presented as a security pass.

Review follow-up: the Codex review P2 on `SnapshotGatedContext` (the read-path gate
ignored the retrieval deadline while hashing source artifacts) was fixed with
`outcomefile.GateWithSourcesAndDigestsContext` and `FileSourceDigestContext`.
`TestLLM09GateContextCanceled` and `TestLLM09FileSourceDigestContextCanceled` pass.

## Dependency handoff

- **Plan 05 — data/model poisoning.** Plan source:
  [`PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md`](../../history/plans/PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md).
  Merged in PR #268 with follow-ups in PR #274. The reused contract is `outcomefile`
  (`IngestRunFor`, content identity, eligibility, `GateWithSourcesAndDigests`) and
  `memindex.RebuildFromDir`. POISON-002 is implemented; POISON-001/003 remain
  `NOT_RUN`. This plan reuses the provenance gate and does not re-author ingestion.
- **Plan 03 — sensitive information disclosure.** Plan source:
  [`PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md`](../../plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md).
  Merged static tracing in PR #266. Its report records the retrieval
  corpus/repository/task scope and the untrusted-evidence boundary. Live extraction
  remains `NOT_RUN`; this plan does not claim live DLP.
- **Plan 07 — misinformation.** Plan source:
  [`PLAN-07-SOP-LLM07-Misinformation.md`](../../plans/PLAN-07-SOP-LLM07-Misinformation.md).
  `NOT_RUN` (plan `PROPOSED`). Plan 07 owns the evidence-authority boundaries; this
  plan consumes the baseline authority (SOP outcome eligibility, recorded
  verification, content hash, and the required-evidence contract) and adds no new
  authority model.
- **Lifecycle resume issue.** The RETRIEVE-001 manual acceptance gate required an
  explicit operator decision. The normal resume path re-entered RETRIEVE-001 and
  failed in the planning agent with an invalid-plan JSON error. The operator chose
  the supported external-completion path; completion provenance is recorded as
  `external`, commit `536057ab698034039dba11417b412c17441d521d`, validation `PASS`.
  RETRIEVE-002 also exhausted its bounded model recovery in repeated NO_PROGRESS
  after the implementation was committed; it was external-completed at commit
  `ca4c800e8d45b097078b03ca7a0fbf889492befc`, validation `PASS`. Fixing the
  resume/planning behavior is outside this task.

## Limitations

- The local advisory backend is implemented but **disabled by default**. Its tests
  are deterministic and run through the real retrieval interfaces; they are not live
  model evidence.
- RAG is disabled by default and the recorded rollout decision is NO-GO. Live
  producer, PostgreSQL, embedding-provider, and live-model results are **NOT_RUN**.
  The RAG protocol isolates by corpus; it carries no project/repository identity.
- No production vector or embedding retriever exists. `internal/vectoreval` is an
  evaluation harness only, so vector/embedding coverage is **NOT_APPLICABLE** to
  production and is not claimed as a pass.
- The code-context trace is static. No live model A/B, task-success, or usefulness
  measurement is claimed.
- Similarity and lexical scores do not establish trust. Content-level poisoning
  inside an authorized repository remains possible; no semantic immunity is claimed.
- A shared derived index file is not a supported multi-workspace store. Rebuilding
  from one workspace replaces the derived index and drops the other workspace's rows;
  the default is a workspace-local index.
- `SnapshotGatedContext` stats and hashes source artifacts on each retrieval. The gate
  and digest reads are context-aware, so the configured retrieval deadline can cancel
  them; the cost is bounded by the outcome-file set and has not been
  performance-benchmarked.
- Strict repository matching in `mapAdvisoryResult` requires a backend to populate
  `Evidence.Repository`. The local backend and ranker adapter do; an uninformed
  backend yields no advisory items rather than unscoped evidence.
- The security scan is report-only and `INDETERMINATE` in this environment (scanner
  adapters unavailable/failed). It is not evidence of a clean security posture.
