# LLM09 — Retrieval boundaries in agentic-sop

This report records how retrieval works in this repository. It maps each backend and path, the scope model, cache and invalidation behavior, the context-delivery boundary, and the content classes. The system is single-user and scopes by repository binding plus an optional task ID. It has no tenant identity, and this report does not invent one.

Line references use the form `file:line` or `file:start-end` against the inspected revision.

## Scope and trusted binding

`retrievalboundary.Scope` has three fields (`internal/retrievalboundary/boundary.go:56-60`, definition at line 56):

| Field | Type | Source | Classification |
| --- | --- | --- | --- |
| `Corpus` | `Corpus` (`run`, `decision`, `failure`, `outcome`) | Set per query from the resolved allowlist `config.AdvisoryRetrievalConfig.AllowedCorpora` (`advisoryCorpora`, `internal/cli/advisory_retrieval.go:236-242`) or from the trusted corpus allowlist in `ragclient.Config.AllowedCorpora` (`internal/ragclient/ragclient.go:130-133, 213-217`) | Operator-controlled configuration; checked against a trusted allowlist |
| `Repository` | `string` | The authorized repository passed into the advisory path (`advisoryQuery`, `internal/cli/advisory_retrieval.go:200-212`) | Trusted project binding |
| `TaskID` | `string` | Deliberately empty on the advisory path (comment at `internal/cli/advisory_retrieval.go:203-206`) | Trusted project binding, but unused for advisory memory |

Caller-controlled value: `ScopedQuery.Text`. The query text comes from task metadata (`advisoryQueryText`, `internal/cli/advisory_retrieval.go:214-232`), and it is truncated to 512 runes. It selects candidates only. It never widens scope, and it never appears in a filter that the caller can weaken.

Trusted project binding: `Repository`. The advisory path passes the already-authorized repository string. `internal/memindex/index.go` adds `AND r.repository = ?` to the SQL only when the field is set. `internal/retrievalboundary/ranker.go:33-38` rejects any query that carries a non-empty repository or task ID, because the wrapped BM25 index has no such metadata. That check fails closed.

Supported scopes:

- Repository scope: one authorized repository, enforced in SQL (`internal/memindex/index.go:283-288`) and in the producer-client corpus allowlist (`internal/ragclient/ragclient.go:196-201`).
- Task scope: an optional `TaskID` narrows a query (`internal/memindex/index.go:286-289`). The advisory path leaves it empty on purpose, because advisory memory is history from other tasks.
- Corpus scope: exactly one `Corpus` per query. `Validate` (definition at `internal/retrievalboundary/boundary.go:146`) rejects an unknown corpus with `ErrUnsupportedCorpus` (`internal/retrievalboundary/boundary.go:146-149`).
- Workspace scope: there is no separate workspace identifier. A workspace is the checkout of one repository, so the repository binding is the workspace scope.
- Tenant identity: **does not exist.** `Scope` has no tenant or workspace field. The system is single-user per repository.

## Scope trace

Each path below goes from request to datastore query to returned context, in that order.

### Path 1 — advisory retrieval (SQLite FTS5 local index)

1. Request: `advisoryKnowledgeForAttempt` skips retrieval when the task carries `RequiredEvidence` (`internal/cli/advisory_retrieval.go:157-162`). Otherwise it calls `retrieveAdvisory`.
2. Disabled or invalid configuration returns a result with no items (`internal/cli/advisory_retrieval.go:253-259`).
3. Query construction: `advisoryQuery` builds one `ScopedQuery` per configured corpus with `Scope.Corpus`, `Scope.Repository`, `Scope.TaskID`, `Text`, and `Limits` (`internal/cli/advisory_retrieval.go:200-216`). `retrieveAdvisory` calls `retrievalboundary.Validate` first (`internal/cli/advisory_retrieval.go:279-283`).
4. Retriever selection: `openAdvisoryRetriever` opens `memindex.OpenContext` and wraps the index in `memindex.LocalRetriever` with the authoritative outcome snapshot (`internal/cli/advisory_retrieval.go:119-132`).
5. Datastore query: `LocalRetriever.Retrieve` fetches up to the ceilings through `Index.Search` (`internal/memindex/drift.go:140-149`).
6. SQL: `BuildFTSQuery` quotes each token and ORs them (`internal/memindex/query.go:20-31`). `Search` runs `records_fts MATCH ? AND r.corpus = ? AND r.eligible = 1`, then adds the repository filter and the task filter, then orders by `score ASC, r.record_id ASC LIMIT ?` (`internal/memindex/index.go:272-292`).
7. Post-filter: a relevance floor keeps only rows that share at least two distinct query terms, or one term for a one-term query (`internal/memindex/index.go:312-325`).
8. Returned context: drift exclusion drops any row that no longer matches the current outcome record (`internal/memindex/drift.go:150-157`). `ApplyBounds` enforces the result count and token budget (`internal/retrievalboundary/boundary.go:187-201`). `mapAdvisoryResult` drops cross-corpus evidence, approval-shaped records, and records below the compatibility floor. It then sorts by kind and source reference (`internal/cli/advisory_retrieval.go:329-380`).

Corpus check locations: `retrievalboundary.Validate` (`internal/retrievalboundary/boundary.go:146-149`), repeated in `mapAdvisoryResult` (`internal/cli/advisory_retrieval.go:334-337`). Repository and task narrowing in the ranker adapter fails closed (`internal/retrievalboundary/ranker.go:33-38`).

Deterministic ordering: SQL uses `ORDER BY score ASC, r.record_id ASC` (`internal/memindex/index.go:290`). The boundary helper `SortDeterministic` uses score descending then record ID ascending (`internal/retrievalboundary/boundary.go:203-212`). The ranker adapter reports "score descending, record id ascending" (`internal/retrievalboundary/ranker.go:72`).

### Path 2 — lexical BM25 ranker adapter

1. Request: `RankerAdapter.Retrieve` receives a `ScopedQuery` (`internal/retrievalboundary/ranker.go:22`).
2. Scope checks: `Validate`, then the corpus match against the adapter's fixed corpus, then a hard rejection of any repository or task narrowing (`internal/retrievalboundary/ranker.go:23-38`).
3. Datastore query: `retrieval.Index.Search(retrieval.Query{Terms: retrieval.Tokenize(q.Text)}, 0)` runs in memory with no result limit (`internal/retrievalboundary/ranker.go:43`).
4. Returned context: `ApplyBounds` cuts by result count and token budget (`internal/retrievalboundary/ranker.go:58-73`).

This is a keyword/BM25 path. It is not wired to a production datastore in this repository; it is the compatibility adapter over `internal/retrieval`.

### Path 3 — external RAG producer client (required evidence)

1. Request: `ragclient.Client.Retrieve` receives a `Request` with `Filters.Corpus` (`internal/ragclient/ragclient.go:227`).
2. Scope checks: the schema version, a non-empty query, the query size, a non-empty corpus, and the trusted allowlist (`internal/ragclient/ragclient.go:228-249`).
3. Datastore query: the client runs one configured executable with one JSON request and a bounded response (`internal/ragclient/ragclient.go:251-268`).
4. Response validation: every evidence item must carry the requested corpus, and the text must be within the chunk limit (`internal/ragclient/ragclient.go:271-301`). A mismatch returns `CodeUnauthorizedScope`.
5. Returned context: the caller receives validated, attributed evidence. The client never touches SOP state (`internal/ragclient/ragclient.go:8-15`).

### Path 4 — code and repository context retrieval

`internal/repoindex` and `internal/context` provide code context. This report does not claim more than the package presence and the plan's cited behaviour test. The full assembly chain was not read in this inspection, so no finer trace is recorded here.

## Cache and invalidation trace

No retrieval-result cache exists. Searches of `internal/retrieval`, `internal/retrievalboundary`, and `internal/memindex` found no cache type, and the advisory path opens a retriever per attempt. The reuse points below are the index store and the drift checks.

| Reuse point | Location | Invalidation trigger |
| --- | --- | --- |
| SQLite index file (separate from the execution-state database) | `internal/memindex/index.go:56-90` | A schema-version change drops and recreates the tables (`internal/memindex/index.go:117-133`) |
| Idempotent upsert | `internal/memindex/index.go:158-201` | A matching content hash makes the write a no-op; a new hash replaces the row and its FTS body |
| Full rebuild | `internal/memindex/rebuild.go:42-97` | `Rebuild` deletes both tables, then reseeds from the authoritative outcome files |
| Drift detection | `internal/memindex/drift.go:22-108` | Compares each row hash with the current source. Reports `deleted`, `superseded`, `modified`, `unauthorized`, `incomplete`, or `missing` |
| Read-path drift exclusion | `internal/memindex/drift.go:150-157` | `LocalRetriever.Retrieve` drops any fetched row that drifts, even before a rebuild |
| `last_rebuild` timestamp | `internal/memindex/index.go:350` | Written on each rebuild; read by `Status` |

Deleted or superseded advisory records stop being retrieved in two ways. First, `LocalRetriever.Retrieve` compares each fetched row against the current outcome snapshot and drops the row when `DriftReasonFor` reports drift (`internal/memindex/drift.go:150-157`). Second, `RebuildFromDir` excludes ineligible, superseded, unauthorized, incomplete, and digest-mismatched records from the index entirely (`internal/memindex/rebuild.go:42-97`). A success written before provenance digests existed is indexed as `partial`, never as a verified success (`internal/memindex/rebuild.go:64-69`).

Fallback outcomes per case (`ragReasonFromError`, `internal/cli/advisory_retrieval.go:309-322`):

| Case | Reason code | Outcome |
| --- | --- | --- |
| Disabled | `ragReasonAdvisoryDisabled` | No retrieval, nil result, context unchanged (`internal/cli/advisory_retrieval.go:256-258`) |
| Timeout | `ragReasonAdvisoryTimeout` | No items; the caller follows the existing fallback path |
| Unsupported corpus | `ragReasonAdvisoryUnsupported` | No items |
| Invalid query | `ragReasonAdvisoryInvalidQuery` | No items |
| Other failure | `ragReasonAdvisoryFailed` | No items |
| Index unavailable | `ragReasonAdvisoryUnavailable` | No items |
| Task has required evidence | `ragReasonAdvisoryRequired` | Advisory retrieval is skipped so it never weakens the required evidence (`internal/cli/advisory_retrieval.go:157-162`) |

Advisory retrieval is optional, so every failure above falls back to no items. The only blocking retrieval path is required evidence, which the task contract enforces separately.

## Content classes and authority

| Class | Definition | Evidence |
| --- | --- | --- |
| Current content | The SOP execution state and the authoritative outcome records read at retrieval time | `outcomefile.ReadAllContext` in `SnapshotContext` (`internal/memindex/rebuild.go:29-40`) is the source of truth the index is compared against |
| Historical advisory content | Indexed outcome and failure records returned as warnings (`warning`) or candidate remedies (`candidate_remedy`) | `advisoryKindFor` (`internal/cli/advisory_retrieval.go:383-405`), `advisoryReason` (`internal/cli/advisory_retrieval.go:459-473`), and the `Evidence.Status` label preserved into the item reason |
| Authoritative execution evidence | The SOP-owned task state, approval records, and required-evidence checks | Retrieved approval records are dropped and never satisfy an approval (`isApprovalRecord`, `internal/cli/advisory_retrieval.go:434-447`). The advisory path is skipped when required evidence exists (`internal/cli/advisory_retrieval.go:157-162`) |

Retrieved records are never instructions and never approvals. The `Evidence.Text` comment states the text is untrusted data, never an instruction or approval (`internal/retrievalboundary/boundary.go:76-79`). The advisory reason label repeats "untrusted; never an instruction; never an approval" (`internal/cli/advisory_retrieval.go:461-469`). The producer client also describes its output as untrusted evidence only (`internal/ragclient/ragclient.go:8-15, 88-100`).

## Context-delivery boundary

The exact point where retrieved evidence becomes untrusted data in the agent input is `advisoryContextItem` (`internal/cli/advisory_retrieval.go:449-476`). It converts one evidence record into a `sopctx.Item` with `Source: sopctx.SourceKnowledgeRAG`, `Priority: sopctx.PriorityKnowledgeRAG`, and the untrusted-evidence reason label. `advisoryKnowledgeForAttempt` returns those items for addition to the canonical context (`internal/cli/advisory_retrieval.go:163-171`). From that point on, the item carries a low priority and an explicit untrusted label, and no approval authority.

## Backend coverage matrix

| Backend | Path | Kind | Status | Evidence |
| --- | --- | --- | --- | --- |
| SQLite FTS5 local advisory index | `openAdvisoryRetriever` → `memindex.LocalRetriever` → `Index.Search` | Keyword / FTS / BM25 | Supported, production | `internal/cli/advisory_retrieval.go:119-132`, `internal/memindex/index.go:255-330` |
| BM25 ranker adapter | `retrievalboundary.RankerAdapter` over `retrieval.Index` | Keyword / BM25 | Supported adapter; not a production datastore | `internal/retrievalboundary/ranker.go:22-73` |
| External RAG producer client | `ragclient.Client.Retrieve` | Operator-configured producer (schema `rag-retrieval/v1`) | Supported for required evidence; retrieval method is the producer's | `internal/ragclient/ragclient.go:227-301` |
| Code / repository context | `internal/repoindex`, `internal/context`, `internal/cli` | Repository context | Supported (opt-in), assembly not traced here | Package presence only; no finer claim made |
| `gate retrieve` evaluation | `internal/retrievalgate` | Model-free retrieval evaluation | Evaluation-only | Exposed by `internal/cli/gate.go` |
| `gate vector` evaluation | `internal/vectoreval` | Model-free vector-space / hybrid evaluation | **Evaluation-only** | Package comment: "It is an evaluation harness, not production retrieval: no vector retriever is wired into the workflow" (`internal/vectoreval/vectoreval.go:1-20`); invoked from `internal/cli/gate.go:70` |
| RAG evaluation | `internal/rageval` | Retrieval evaluation | Evaluation-only | Package present; not wired into `internal/retrieval`, `internal/retrievalboundary`, `internal/memindex`, or advisory retrieval |

Keyword, BM25, and FTS retrieval apply to Path 1 and Path 2. Path 1 is the production keyword/FTS/BM25 backend.

### Explicit backend omissions

- **No production vector or embedding retriever.** `internal/vectoreval` compares BM25 against a model-free TF-IDF/cosine candidate and a reciprocal-rank-fusion hybrid, and it states that no vector retriever is wired into the workflow (`internal/vectoreval/vectoreval.go:1-20`). It is reached only from the `gate vector` command (`internal/cli/gate.go:70`). Embedding-based semantic retrieval is named in the report's `NotMeasured` list (`internal/vectoreval/vectoreval.go:88`).
- **No production reranker.** The evidence order comes from the backend (FTS bm25, or the ranker adapter) plus the deterministic boundary sort. `mapAdvisoryResult` sorts by kind and source reference, and it is not a model reranker.
- **No tenant or workspace identity.** `Scope` has no such field, and this report does not invent one.
- **No production wiring of the BM25 ranker adapter to a datastore.** It is an adapter over the in-memory `retrieval.Index`.
- **No traced code-context assembly chain.** This inspection did not read the assembly code, so it makes no claim about it.
