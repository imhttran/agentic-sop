# EV-003 — Retrieval-Enabled Evaluation Using Existing Capabilities

E1 offline, model-free evaluation of the existing `repoindex` (CTX-002),
`retrieval` (CTX-003), and `retrievalgate` (CTX-004) facilities over the EV-002
corpus, comparing the **baseline** repository-evidence arm against the **retrieval**
(BM25) arm on precision@k, recall@k, MRR, and context bytes, and recording measured
retrieval overhead.

**No production behavior change is made and no new dependency is added.** The only
additions are this report and a test-only, model-free evaluation harness
(`internal/retrievalgate/ev003_live_test.go`). No runtime path is changed; no
`.agent-sdlc` state is written.

## 1. Scope and authority

- **Corpus / baseline protocol:** `docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md`.
- **Facilities used (existing, unmodified):**
  - `repoindex.Build` (`internal/repoindex/index.go`) — structural index (CTX-002).
  - `retrieval.New` / `retrieval.Index.Search` (`internal/retrieval/retrieval.go`) — deterministic BM25 ranking (CTX-003).
  - `retrieval.CandidatesFromIndex` (`internal/retrieval/retrieval.go`) — live corpus from the index.
  - `retrievalgate.Measure` (`internal/retrievalgate/gate.go`) — the shared CTX-004 metric (precision@k, recall@k, MRR, bytes).
  - `retrievalgate.baselineOrder` (`internal/retrievalgate/gate.go`) — the unranked, stable-ID-ordered baseline.
- **Harness:** `internal/retrievalgate/ev003_live_test.go`, `TestEV003LiveEvaluation`
  (test-only; never compiled into any binary). Reproduces this report's numbers via
  `go test ./internal/retrievalgate/ -run TestEV003LiveEvaluation -v`.
- **Precedent for live-index + BM25 usage:** `internal/cli/retrieve.go` (`runRetrieve`).

## 2. Evaluation contract (as executed)

| Element | Definition |
| --- | --- |
| Baseline arm | Unranked lexical matches in stable ID order (`retrievalgate.baselineOrder`): every candidate sharing a query token, sorted with `sort.Strings`. No score, no ranking. |
| Retrieval arm | `retrieval.New(candidates).Search(Query{Terms}, k)` — BM25, descending score, ties broken by ascending candidate ID. |
| Metric | `retrievalgate.Measure(ids, relevant, k, candidates)` — identical code for both arms. |
| `k` | 5. |
| Ground truth | Labelled relevant IDs in the `retrieval.Candidate` vocabulary: symbol IDs, `file:<path>`, `doc:<path>`. |
| Corpus | Nine live-repository cases, one per Phase 8 §14 category (see §4). |

## 3. Determinism and order-independence

The evaluation is **model-free** (no provider, no network, no token counting) and
**deterministic**: identical inputs yield an identical report.

Determinism is established by construction and **asserted by the harness**:

1. **Sorted collections.** `repoindex.Build` fully sorts every collection
   (`Symbols`, `Imports`, `Documents`; `index.go`), and `retrieval.CandidatesFromIndex`
   sorts the candidate corpus by candidate ID. `baselineOrder` sorts its output with
   `sort.Strings`.
2. **Tie-break by stable identity.** BM25 ties break by ascending candidate ID
   (`retrieval.go`, `Search`), so no map or slice iteration order reaches the result.
3. **Re-run identity.** The harness runs the metric twice over identical inputs and
   fails (`t.Fatalf`) if any per-case metric row differs (`ev003Key` comparison).
4. **Order independence.** The harness reverses the candidate corpus, re-runs both
   arms, and fails (`t.Fatalf`) if any per-case metric row differs. This is the
   filesystem-/iteration-order invariance check required by acceptance.

`TestEV003LiveEvaluation` passes (`go test ./internal/retrievalgate/ -run
TestEV003LiveEvaluation -v`), so the re-run identity, order-independence, and CTX-003
field assertions all hold.

## 4. Per-case results (baseline vs retrieval)

`k = 5`. Every retrieval result exposes its **source**, **score**, **reason**, and
**rank** per CTX-003 (`retrieval.Result`: `ID`, `Source`, `Path`, `Score`, `Reason`,
`Rank`). The metric rows below are produced by the shared `retrievalgate.Measure` and
transcribed verbatim from the harness run over the live `agentic-sop` structural index
(6510 candidates).

| # | Case (`Category`) | base p@5 | base r@5 | base mrr | base bytes | cand p@5 | cand r@5 | cand mrr | cand bytes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | `single_file` | 0.000 | 0.000 | 0.000 | 249 | 0.200 | 1.000 | 1.000 | 291 |
| 2 | `multi_file` | 0.000 | 0.000 | 0.000 | 221 | 0.200 | 0.333 | 0.333 | 253 |
| 3 | `existing_test_discovery` | 0.000 | 0.000 | 0.000 | 228 | 0.000 | 0.000 | 0.000 | 184 |
| 4 | `interface_implementation` | 0.000 | 0.000 | 0.000 | 221 | 0.200 | 1.000 | 0.333 | 212 |
| 5 | `configuration_change` | 0.200 | 1.000 | 0.250 | 252 | 0.000 | 0.000 | 0.000 | 323 |
| 6 | `documentation_and_code` | 0.200 | 0.500 | 0.200 | 304 | 0.200 | 0.500 | 0.500 | 251 |
| 7 | `bug_fix` | 0.000 | 0.000 | 0.000 | 272 | 0.200 | 1.000 | 0.500 | 173 |
| 8 | `cross_package_dependency` | 0.000 | 0.000 | 0.000 | 221 | 0.400 | 1.000 | 0.333 | 212 |
| 9 | `replan_after_failure` | 0.200 | 1.000 | 0.500 | 250 | 0.200 | 1.000 | 0.250 | 246 |

The top retrieval result per case (CTX-003 fields) is:

| # | top ID | source | score | rank | reason |
| --- | --- | --- | --- | --- | --- |
| 1 | `file:internal/repoindex/index.go` | `file` | 14.257646 | 1 | `lexical match: index, repoindex` |
| 2 | `method:.../internal/retrieval.Index.Search` | `symbol` | 18.286791 | 1 | `lexical match: index, retrieval, search` |
| 3 | `file:internal/retrievalgate/gate.go` | `file` | 15.128327 | 1 | `lexical match: gate, retrievalgate` |
| 4 | `method:.../internal/retrieval.Index.Search` | `symbol` | 18.286791 | 1 | `lexical match: index, retrieval, search` |
| 5 | `function:.../internal/retrievalgate.Measure` | `symbol` | 12.966257 | 1 | `lexical match: measure, retrievalgate` |
| 6 | `file:docs/reports/ev-context-run/EV-003-retrieval-evaluation.md` | `file` | 15.947890 | 1 | `lexical match: context, evaluation, retrieval` |
| 7 | `file:internal/retrievalgate/ev003_live_test.go` | `file` | 6.927380 | 1 | `lexical match: retrievalgate` |
| 8 | `struct:.../internal/retrieval.Index` | `symbol` | 11.478081 | 1 | `lexical match: index, retrieval` |
| 9 | `function:.../internal/retrievalgate.Measure` | `symbol` | 12.966257 | 1 | `lexical match: measure, retrievalgate` |

## 5. Aggregate results (baseline vs retrieval)

Aggregation is the same deterministic mean the gate applies (`Counts.mean`), over the
nine cases:

| Arm | p@5 | r@5 | mrr | bytes_mean |
| --- | --- | --- | --- | --- |
| Baseline | 0.067 | 0.278 | 0.106 | 246 |
| Retrieval (BM25) | 0.178 | 0.648 | 0.361 | 238 |

The retrieval arm improves precision@5 (0.067 → 0.178), recall@5 (0.278 → 0.648), and
MRR (0.106 → 0.361), while slightly reducing mean context bytes (246 → 238).

## 6. Retrieval overhead (live corpus, measured)

The harness measures two quantities separately with `time.Since`:

| Overhead (live corpus, 6510 candidates) | Measurement point |
| --- | --- |
| **Index build time** | around `repoindex.Build(repoindex.Options{Root})` |
| **Search time** | around the full baseline + BM25 run over all nine cases |

Representative measured values from the harness run:

```text
EV003 index_build_ms=128 search_ms=50 candidates=6510 cases=9
```

These are wall-clock measurements of existing calls and are inherently
machine/load-dependent; the metric outputs (precision/recall/MRR/bytes) are the
deterministic values asserted by the harness. No production instrumentation was added.

## 7. Metrics: measured vs not measured

| Required metric | Measured here? | Source |
| --- | --- | --- |
| Retrieval quality — precision@k | **Measured** | `retrievalgate.Measure.Precision` |
| Retrieval quality — recall@k | **Measured** | `retrievalgate.Measure.Recall` |
| Retrieval quality — MRR | **Measured** | `retrievalgate.Measure.MRR` |
| Context size — bytes | **Measured** | `retrievalgate.Measure.Bytes` |
| Retrieval overhead — index build time | **Measured** | harness `time.Since(repoindex.Build)` |
| Retrieval overhead — search time | **Measured** | harness `time.Since(run over corpus)` |
| Task success / verification success | **Not measured** | requires running a model; `retrievalgate.Report.NotMeasured` |
| Latency (end-to-end execution) | **Not measured** | requires execution; live-path `internal/perf` |
| Token consumption | **Not measured** | provider-dependent, not normalized; informational at most, never policy (Phase 8 §26) |

This matches the measured/not-measured split the existing gate reports in
`retrievalgate.Report.NotMeasured`, so the EV-003 evaluation and the built-in
`sop gate retrieve` agree.

## 8. Scope discipline

- **No production behavior change.** No runtime path, lifecycle, approval, retry,
  replan, budget, or verification semantics were touched. The only added Go file is a
  `_test.go` file, which is not compiled into any binary.
- **No new dependency.** The harness imports only in-repo packages
  (`repoindex`, `retrieval`) plus the standard library (`fmt`, `path/filepath`,
  `testing`, `time`). `go.mod` is unchanged.
- **No `.agent-sdlc` state** is created, modified, or deleted.
- Pre-existing user-owned working-tree changes are preserved unchanged.

## 9. Verification record (EV-003-S4)

| Check | How performed | Result |
| --- | --- | --- |
| Two identical-input runs yield identical metric content | Harness re-runs both arms and compares every per-case row (`t.Fatalf` on mismatch) | PASS — `TestEV003LiveEvaluation` |
| Shuffled iteration order yields identical rankings | Harness reverses the candidate corpus and re-compares every per-case row | PASS — `TestEV003LiveEvaluation` |
| Every result exposes source, score, reason, rank | Harness asserts non-empty `ID`, `Source`, `Reason` and `Rank >= 1` for every emitted result | PASS — `TestEV003LiveEvaluation` |
| No production behavior change | Only `_test.go` added; `go build ./...`, `go test ./...`, `go vet ./...` clean | PASS |
| No new dependency | `go.mod` unchanged; imports are in-repo + stdlib | Inspected |
| Measured/not-measured statement matches output | §7 table mirrors harness output and `retrievalgate.Report.NotMeasured` | Inspected |

Required validations: `go build ./...`, `go test ./...`, `go vet ./...` — all pass.
