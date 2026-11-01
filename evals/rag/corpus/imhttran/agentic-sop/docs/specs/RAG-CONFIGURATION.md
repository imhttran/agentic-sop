# RAG Retrieval — Configuration and Feature Flags

**Type:** Configuration specification (RAG-SOP-001 deliverable).
**Status:** APPROVED (architecture) — operator-approved 2026-10-15 for RAG-SOP-002 implementation (documentation record; not a SOP lifecycle approval).
**Plan:** [`docs/plans/PLAN-RAG-SOP-Integration.md`](../plans/PLAN-RAG-SOP-Integration.md).
**Related:** [contract](RAG-RETRIEVAL-CONTRACT.md), [threat model](RAG-SECURITY-THREAT-MODEL.md).

## 1. Precedence

Values resolve with SOP's existing precedence: **environment > project config
(`.agent-sdlc/config.yaml`) > built-in default**. An unparseable value is an actionable
error (never silently ignored). Names and defaults are proposals for RAG-SOP-002/003.

## 2. Keys (proposed)

| Env var | Config key | Default | Meaning |
| --- | --- | --- | --- |
| `SOP_RAG_ENABLED` | `rag.enabled` | `false` | Master opt-in. Off ⇒ byte-identical baseline; RAG code path not exercised. |
| `SOP_RAG_BACKEND` | `rag.backend` | `external` | Only `external` (separate process/endpoint) is in scope. |
| `SOP_RAG_TRANSPORT` | `rag.transport` | `exec` | Spike transport: `exec` (chosen) or `http` (deferred). |
| `SOP_RAG_EXECUTABLE` | `rag.executable` | _(none)_ | Absolute path to the retrieval-only entry point (exec transport). Validated; never shell-interpolated. |
| `SOP_RAG_ENDPOINT` | `rag.endpoint` | `http://127.0.0.1:PORT` | Loopback endpoint (http transport only). |
| `SOP_RAG_TIMEOUT_MS` | `rag.timeout_ms` | `3000` | Per-request deadline. |
| `SOP_RAG_MAX_RESULTS` | `rag.max_results` | `5` | `top_k` cap. |
| `SOP_RAG_CONTEXT_BUDGET_BYTES` | `rag.context_budget_bytes` | `4096` | RAG item byte budget in the Context Engine. |
| `SOP_RAG_MAX_QUERY_BYTES` | `rag.max_query_bytes` | `4096` | Query ceiling. |
| `SOP_RAG_MAX_RESPONSE_BYTES` | `rag.max_response_bytes` | `262144` | Whole-response ceiling. |
| `SOP_RAG_MAX_CHUNK_BYTES` | `rag.max_chunk_bytes` | `16384` | Per-chunk ceiling. |
| `SOP_RAG_ALLOWED_CORPORA` | `rag.allowed_corpora` | `[]` | **Trusted corpus allowlist** — the isolation unit (D2). Empty ⇒ retrieval impossible (fail closed). |
| `SOP_RAG_CORPUS` | `rag.corpus` | _(none)_ | Default corpus for a request when the task does not name one; MUST be a member of `SOP_RAG_ALLOWED_CORPORA`. |

## 3. Semantics

- **Disabled by default.** With `SOP_RAG_ENABLED=false` (or `rag.enabled: false`) the
  retrieval path is never invoked; BM25 behaviour, approvals, validation, and lifecycle are
  byte-identical to today.
- **Opt-in only on explicit paths.** Even when enabled, RAG is invoked only from the
  context-build seam for configured task/prompt paths — not on every cheap operation.
- **Corpus allowlisting.** `SOP_RAG_ALLOWED_CORPORA` is the operator-controlled, **trusted**
  allowlist that scopes retrieval: a request for a corpus outside it is rejected
  (`UNAUTHORIZED_SCOPE`) and never widens scope. The `rag-template` side validates against its
  own allowlist too; either rejection fails closed. An empty allowlist makes retrieval
  impossible. Corpus filtering is applied in keyword and vector retrieval as a **pre-filter**
  (before ranking and limits), and legacy NULL-corpus rows are never returned.
- **Credentials are not SOP's.** DB/provider credentials stay in `rag-template`'s
  environment; SOP config holds only the executable/endpoint and allowlists.
- **Bytes, not tokens.** All budgets are bytes, aligned with the Context Engine's
  provider-independent accounting (see `internal/context`).

## 4. Baseline-compatibility contract

A verification must show, for frozen cases:
- `SOP_RAG_ENABLED=false` ⇒ unchanged retrieval/context and identical lifecycle.
- `SOP_RAG_ENABLED=true` ⇒ added, cited, low-priority evidence within existing limits, with
  deterministic ordering; repeat runs with frozen input/response construct identical
  context.

## 5. Open decisions

- Config block name (`rag:`) and key names require operator sign-off before RAG-SOP-002/003.
- Whether `required_evidence` is declared per task file, per plan stage, or per config
  entry (proposed: per task, an allowlist of corpus/source identities).
