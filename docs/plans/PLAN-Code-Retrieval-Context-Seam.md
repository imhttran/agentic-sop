# PLAN — Code Retrieval Context Seam (EV Option A)

Status: approved 2026-11-06 (Option A). Steps 1–4 implemented; step 5 awaits model-budget approval.

This is the "separate plan" that
[EV-005 §2.3](../reports/ev-context-run/EV-005-acceptance-criteria.md) asks for before
any live retrieval seam exists. It wires the existing BM25 code retrieval into run
context behind an opt-in flag, then measures it.

## 1. Why

SOP builds a structural code index (`repoindex`) and ranks it with BM25
(`retrieval`), but no run uses either. `implementContext` in
[run.go](../../internal/cli/run.go) gives the model only the plan, changed-file
paths, execution and recovery items, memory and RAG knowledge. The Context Engine
says it "performs no retrieval". So the agent starts each task without a ranked
list of the code that task most likely touches.

The offline evidence already says the ranking helps:

| E1 rule (EV-005 §1.3) | Threshold | Measured (EV-003) | Result |
| --- | --- | --- | --- |
| Δp@5 or ΔMRR | ≥ +0.050 or ≥ +0.100 | +0.111 / +0.255 | pass |
| Δr@5 | ≥ −0.020 | +0.370 | pass |
| Δbytes | ≤ +24 | −8 | pass |
| index build / search | ≤ 256 ms / ≤ 100 ms | 128 ms / 50 ms | pass |

By its own rule **E1 = GO**. Nobody recorded that, and nobody made the Option A/B
decision. This plan takes Option A.

## 2. Goal and non-goals

**Goal:** with `context.code_retrieval: true`, every implement/fix attempt gets the
top-k BM25 hits (symbols and file paths; no document bodies, see §8) for its task as
low-priority context. With the flag off, the context and prompt stay byte-identical to today.

**Non-goals (this plan):**
- Reading or chunking function bodies. Hits are pointers (ID + path); the agent opens the files itself.
- Embeddings or vector search. `vectoreval` already rejects vector retrieval unless it beats BM25.
- The multi-agent worker context (`orchestration.BuildWorkerContext`). Follow-up once this path is measured.
- Any change to routing, approval, retry, budget or verification semantics.

## 3. Design

All of it reuses existing code. No new package, no new dependency.

1. **Config:** add `CodeRetrieval bool \`yaml:"code_retrieval"\`` to
   `config.ContextEfficiency`, next to `decision_memory` and `handoff`. Default false.
2. **Query:** `retrieval.QueryFromEvidence(spec.ID, [title, criteria...], failureCtx)`.
   It existed unused. The title is always included: it is the strongest signal.
3. **Index:** `repoindex.Build(repoindex.Options{Root: dir})`, then
   `retrieval.New(retrieval.CodeCandidatesFromIndex(idx)).Search(q, k)`. The new
   `CodeCandidatesFromIndex` is `CandidatesFromIndex` minus documents, so it reads no
   Markdown from disk; `CandidatesFromIndex` itself is unchanged (EV-003 stays pinned).
   No git state is collected: Head/Dirty only feed the index identity, which ranking
   never reads. Built once per attempt so a fix attempt sees the code the previous
   attempt wrote.
4. **Item:** one `sopctx.Item` listing all hits, with a new source
   `SourceRetrieval` ("retrieval"), `Identity: "bm25"`, `Priority: PriorityGeneric`.
   It is not a file item, so it never consumes the `MaxFiles` ceiling (whose overflow
   drops every later item). At equal priority "retrieval" sorts after "memory", so
   curated decisions keep precedence; RAG knowledge stays strictly last. `k` = 5.
   Symbol hits print their file (`<symbol id> in <path>`); symbol IDs carry none.
5. **Wiring:** `codeRetrievalItems(cfg, spec, failureCtx, rn, d)` in
   [retrieve.go](../../internal/cli/retrieve.go), next to `sop retrieve`, fills a new
   `Retrieved []Item` field on `sopctx.Inputs`, which `FromInputs` adds last. That is
   the `Retrieved` seam EV-005 §2.3 names.
6. **Failure:** any error gives zero items. Code retrieval is advisory and never blocks.
7. **Telemetry:** each attempt writes `code-retrieval.json` in the run directory
   (`status` ok/empty/error, `reason`, `hits`, `duration_ms`). `SourceRetrieval` is in
   the Context Engine's source order, so `runtrace.ContextInfo.Sources` counts it.

## 4. Steps

| # | Step | Done when |
| --- | --- | --- |
| 1 | Record E1 = GO and Option A in EV-005 and fix the PACKAGES.md claim that "every run uses" `retrieval` | Docs match the code |
| 2 | Config flag + `Inputs.Retrieved` + `codeRetrievalItems` + wiring (§3) | `go build ./... && go test ./... && go vet ./...` clean |
| 3 | Tests: flag off gives a byte-identical context (golden); flag on adds ≤ 5 `PriorityGeneric` items in stable order; an index error adds zero items and does not fail the attempt | Three tests in `internal/cli` |
| 4 | Code-task retrieval measurement: 11 task-phrased queries labelled with `.go` files, over the live repo index (test-only, like EV-003; the frozen RAG dataset is unchanged) | `go test ./internal/retrievalgate/ -run TestCodeTaskRetrieval -v` reports hit@k and MRR (§8) |
| 5 | E2 live A/B per EV-005 §2: N ≥ 30 runs per arm, flag off vs on | GO / HOLD / NO-GO recorded |
| 6 | Only if E2 = GO: consider default-on, then the follow-ups in §6 | Separate decision |

Steps 1–4 need no model budget. Step 5 does.

## 5. Approvals needed

- **Architecture (now):** Option A, an opt-in seam, off by default. Approving this plan is that decision.
- **Model budget (before step 5):** 60+ live `sop run` executions over the EV-002 corpus.
- **Spec owner (step 2):** the new context input is documented in `docs/reference/CONFIGURATION.md`; the Context Engine's "performs no retrieval" comment is updated. `docs/specs/PROMPT-EXECUTION.md` does not enumerate context sources, so it is unchanged.

## 6. Follow-ups (not in this plan)

- Split camelCase identifiers in the tokenizer, restrict file candidates to source files, and add doc comments to symbol candidates. Measure each with the step 4 harness (§8); re-pin EV-003 if `CandidatesFromIndex` changes.
- Callers and references (via `golang.org/x/tools`/`gopls`) for impact analysis in plan and review stages.
- Code retrieval for the orchestration worker context.

## 7. Open questions

1. Should hits also feed `sop plan` / the planner, or only implement/fix attempts? This plan does only attempts.
2. Is `k = 5` right? On the step 4 cases hit@3 = hit@5 = 0.545 and hit@10 = 0.636, so 5 costs little and 10 adds one case.

## 8. Step 4 results (2026-11-06)

EV-003's queries were hand-picked identifier tokens. With queries phrased like real
task titles, ranking every candidate (the EV-003 arm) mostly returns Markdown, much
of it duplicate copies under `evals/rag/corpus/` and `.sop-evidence/`:

| Arm (11 cases) | hit@3 | hit@5 | hit@10 | MRR@10 |
| --- | --- | --- | --- | --- |
| all candidates | 0.091 | 0.091 | 0.273 | 0.065 |
| code only (what the seam sends) | 0.545 | 0.545 | 0.636 | 0.511 |

So the seam ranks symbols and file paths only. The remaining misses rank non-Go
artifacts (JSON evidence, report files) or miss because the tokenizer does not split
camelCase (`AddWidget` never matches "widget"). Both are §6 follow-ups and are
measurable with the same harness: restrict file candidates to source files, split
camelCase identifiers, add doc comments to symbol text.
