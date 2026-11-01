# RAG-SOP-003 — Phase 4B: Runtime Context Integration

## Authorization and scope

Implement **Phase 4B only** in `~/agentic-workspace/agentic-sop` on branch `feat/rag-sop-integration`.

The prior work (Packages 1, 2A, 2B and Phase 3.1–3.3 and Phase 4A) is uncommitted. **Preserve all existing changes.** Do not repeat Phase 4A or rewrite its results. Do not reconcile or execute the active SOP plan. Do not commit, push, merge, deploy, or modify `rag-template`.

## Goal

Connect the already-implemented `context.KnowledgeItems` and `ragclient` to the **real** SOP task execution context, while keeping BM25, task governance, and approval behavior unchanged. Keep RAG disabled by default. Use a single task-attempt-scoped retrieval result when both required-evidence verification and prompt context need it.

## Step 1 — Trace the actual execution seam

Inspect `internal/cli/run.go`, `internal/cli/drive.go`, `internal/cli/handoff_context.go`, `internal/prompt/compile.go`, `internal/orchestration/context_routing.go`, `internal/cli/rag_verifier.go`, `internal/cli/required_evidence.go`, `internal/context/rag.go`, and the relevant tests. Trace a real task from `runGraph` through required-evidence enforcement, `context.Build`, prompt compilation, and agent dispatch. Document the exact seam used for the change. Do not add a test-only wrapper or an unused code path.

## Step 2 — Task-attempt-scoped retrieval

1. Add a small attempt-scoped result carrying validated evidence, retrieval status, and safe diagnostic metadata. Avoid package-level mutable caches or global state.
2. When RAG is enabled, perform at most one retrieval per task attempt where possible; share its validated result between required-evidence verification and context construction. Preserve the existing fail-closed behavior until verification succeeds.
3. Keep corpus and source authorization enforced by `ragclient`; recheck returned provenance, schema and caller scope. Never broaden the corpus to meet a requirement.
4. Enforce configured deadline, result limit, response size, and cancellation. Prevent cross-task/cross-attempt leakage, including concurrent and retried tasks.
5. Do not run retrieval when RAG is disabled or when execution is outside the explicitly configured task path.
6. If a task has required evidence, a retrieval failure or unmet requirement must produce the existing `REQUIRED_EVIDENCE_MISSING` BLOCKED state, with durable missing identities. It must not continue with optional fallback.
7. Tasks without required evidence may use the result for optional context, but an optional error must leave the task on the BM25-only path with a structured, content-free fallback reason.

## Step 3 — Construct the real context and prompt

1. Convert verified, allowed evidence using the existing `context.KnowledgeItems` at `SourceKnowledgeRAG`, priority 6.
2. Connect these items to the canonical `context.Build` and actual agent prompt compiler. Prove they reach the real task prompt.
3. Apply `SOP_RAG_TOP_K` and `SOP_RAG_CONTEXT_BUDGET` in bytes, together with the **shared** SOP context budget. Higher-priority items (0–5) must remain selected when total budget is tight. Do not compare BM25 and RAG raw scores.
4. Preserve BM25 retrieval as-is. With `SOP_RAG_ENABLED=false`, the final context **and full compiled prompt** must be byte-identical to the baseline, and the RAG process must not start.
5. Preserve readable, stable corpus/source/section/page citations and retrieval method. Reject missing or mismatched required provenance. Treat retrieved text as untrusted: delimit it at the prompt boundary as quoted/reference material, not policy or tool instructions. The deterministic governance layer must remain the sole authority for approvals and state transitions.
6. Correct the Phase-4A deduplication edge case if needed: distinct un-hashed chunks from the *same corpus, source, and section* must not collapse. Prefer a stable chunk identifier where the contract supplies one; otherwise include a deterministic text fingerprint in the dedup key. Keep exact duplicates collapsed and preserve deterministic order.
7. Emit safe telemetry for attempted/used/failed, bytes, selected evidence identities or hashes, filtered count, duration, and fallback reason. Never log full retrieved content by default.

## Step 4 — Verify with focused tests

Write meaningful tests that exercise the actual production call chain, including:

- RAG disabled: byte-identical context and compiled prompt; zero producer invocations.
- RAG enabled: cited knowledge reaches the real task agent prompt.
- Existing BM25 results unchanged; priority 6 does not evict priorities 0–5 under a shared tight budget.
- Deterministic ordering, UTF-8-safe byte ceilings, count limits, and stable provenance.
- Different un-hashed chunks under the same source and section both survive; exact duplicates do not.
- Required evidence satisfied; missing / unauthorized / cross-corpus / timeout blocks before the agent starts and retains the durable reason and identities.
- Optional timeout, malformed result, and unavailable producer fall back to BM25 with a structured reason.
- One retrieval shared across verification and context; concurrent tasks and retries do not share evidence improperly.
- Retrieved prompt-injection instructions remain untrusted reference content; approvals and lifecycle transitions stay deterministic.

Run:

```bash
go test -count=1 ./internal/context/... ./internal/cli/... ./internal/ragclient/... ./internal/prompt/...
go build ./...
go vet ./...
make test-fast
```

Run applicable architecture tests. If a real-producer integration environment is available, execute the existing tagged acceptance test using an **isolated** test DB/schema. If prerequisites are absent, report **NOT RUN**, never PASS or a silent skip. Do not touch the active `.agent-sdlc` database.

## Step 5 — Report and stop

Report changed files, the exact real execution seam, single-retrieval mechanism, disabled-mode byte-identity evidence, budget/priority behavior, failure and fallback tests, prompt trust boundary, executed test results, and remaining risks. Preserve all existing uncommitted changes. Stop after Phase 4B; do **not** start Package 4 or RAG-SOP-004, and do not change SOP task states.

**Success:** A real SOP task prompt receives bounded, provenance-rich RAG knowledge only when enabled; required evidence remains fail-closed; optional failures preserve BM25; disabled behavior is byte-identical; all runnable tests pass.
