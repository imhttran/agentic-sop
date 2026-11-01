# RDM acceptance evidence — 2026-10-19 (revision 2)

**Type:** Operator-side verification record (non-normative).
**Scope:** `RDM-002` and `RDM-002B` acceptance criteria, review-finding disposition,
and the repository gates.
**Bound to:** `7540edc9bc2b3bb5a9c6cfe4f3799021b807e085` and the file hashes below.

## Source-input binding

| Item                     | Value                                                              |
| ------------------------ | ------------------------------------------------------------------ |
| Branch                   | `feat/rag-decision-memory`                                         |
| HEAD                     | `7540edc9bc2b3bb5a9c6cfe4f3799021b807e085`                         |
| Plan                     | `docs/plans/SOP-RAG-Decision-Memory-Plan.md`                       |
| Plan SHA-256             | `d15aa3484ca57b259a71bf2f86fa13a14b2b00911f9843391383867be1e23511` |
| SOP recorded source hash | `d15aa348…23511` (matches; `plan.meta.json`)                       |
| Plan ID                  | `sop-rag-decision-memory-plan`                                     |

The work is **uncommitted** by design: `human.approval_before_commit` is on.

### Checked files

| File                                                   | SHA-256                                                            |
| ------------------------------------------------------ | ------------------------------------------------------------------ |
| `internal/outcomefile/outcomefile.go`                  | `d342e1e636966e01c30a73484418a5cf0a9a99f10c274fb41a72a8bc0a51e8f3` |
| `internal/outcomefile/outcomefile_test.go`             | `7fd08c4f827dcd960b4eeea7310b69b4cf5bf9f4aa73432a46c70c8f7366e39c` |
| `internal/memindex/drift.go`                           | `72cb4ac668f47b923bfb0feb6aed1ca6c2db0dcfdd2886c934550e19e1c7aef4` |
| `internal/memindex/index.go`                           | `d7723afc1c9dfd85d980d0747bde370546c0f658627cac678803fee542b640a9` |
| `internal/memindex/memindex_test.go`                   | `1c81c0f5967cd86087e467694929ee19fa9680b8eb163f75d512b239165a309d` |
| `internal/memindex/query.go`                           | `d7a6e132017fdc28d91765b80bdc125907143f35c13285f8c779c3c370ea1fd1` |
| `internal/memindex/rebuild.go`                         | `39be485cf8914041242c691a2e22721a67f258d139dfdfcf166c2e429b0864c2` |
| `internal/retrievalboundary/boundary.go`               | `2f82ddead79c400ac0d8657302a7ad8b240b045fe5f996308450df29452b26bd` |
| `internal/retrievalboundary/errors.go`                 | `87e1666e4294f3e636968ad56305c0add50014dd1d4b8db143f3b1c2cdb4e626` |
| `internal/retrievalboundary/ranker.go`                 | `55da845756033e5b9fac2a01c56f3ba43952da3569801b7fb6dc33a27b941246` |
| `internal/retrievalboundary/retrievalboundary_test.go` | `41dd38cf7fe2b5f8aaa84ad38cbb570c1fdf3296b0ae1119b050f0582ae455f4` |

## Review-finding disposition

Findings reported by SOP's review on the first continuation, verified against the code.

| #   | Severity | Finding                                                | Class         | Evidence                                                                                                                                                                                                                    |
| --- | -------- | ------------------------------------------------------ | ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | HIGH     | `drift.go` nil-`Current` returns stale evidence        | **CONFIRMED** | `Retrieve` skipped exclusion when `Current == nil`. Now fails closed with `InvalidQueryError`; covered by `TestLocalRetrieverRequiresCurrent`.                                                                              |
| 2   | HIGH     | `DetectDrift` classification diverges from eligibility | **CONFIRMED** | Drift checked only superseded/authorized/hash and ignored incomplete or status/evidence ineligibility. Now `DriftReasonFor` mirrors `Eligible()` and adds `DriftIncomplete`; covered by `TestDriftReasonForClassification`. |
| 3   | MEDIUM   | Drift loads the whole table and over-fetches per query | **CONFIRMED** | `Retrieve` called `HashRows` (entire index) plus a full source scan for every query. Now classifies only the fetched candidate rows; the candidate fetch stays capped at the ceiling.                                       |
| 4   | MEDIUM   | `ApplyBounds` can exceed the token budget              | **CONFIRMED** | The first candidate was always included. The budget is now a hard bound; covered by `TestApplyBoundsRejectsItemOverBudget`.                                                                                                 |
| 5   | MEDIUM   | `Upsert` existence-check race                          | **REJECTED**  | The pre-check is an optimization only; the write is a transaction with `INSERT … ON CONFLICT DO UPDATE`, SQLite serializes writers, and the operation is idempotent. No corruption path.                                    |
| 6   | MEDIUM   | `Rebuild` drops `eligible=false` IDs                   | **REJECTED**  | The plan requires indexing only complete, eligible records; dropping them is the contract, and `skipped` is reported. Retrieval could not return them either way.                                                           |
| 7   | LOW      | `BuildFTSQuery` exact-phrase semantics                 | **REJECTED**  | Each token is deliberately quoted and ANDed; punctuation and FTS operators cannot break syntax. Covered by `TestSearchHandlesExactErrorsTaskIDsAndPunctuation` and `TestBuildFTSQueryQuotesUserText`.                       |
| 8   | LOW      | `Status.SchemaVersion` is not read from the database   | **REJECTED**  | `Open` rejects a mismatched stored version, so the reported constant equals the stored value by construction.                                                                                                               |
| 9   | LOW      | `wrapSQL` classifies errors by substring               | **CONFIRMED** | String matching can misclassify. Now uses `sqlite.Error.Code()` (busy/locked → timeout, corrupt/not-a-db → corrupt) with a message fallback; covered by `TestCorruptIndexIsTyped`.                                          |
| 10  | LOW      | `outcomefile` does not sync the directory              | **CONFIRMED** | The rename was not durable. Added a best-effort directory sync after rename.                                                                                                                                                |
| 11  | LOW      | `outcomefile` accepts duplicate record IDs             | **CONFIRMED** | `ReadAll` silently overwrote a duplicate. It now keeps the first and rejects the later file explicitly; covered by `TestDuplicateRecordIDIsRejected`.                                                                       |
| 12  | LOW      | `RankerAdapter` stamps scope it cannot filter          | **CONFIRMED** | The wrapped ranker has no repository/task metadata, so the adapter now fails closed for those scopes; covered by `TestRankerAdapterRejectsScopeItCannotHonor`.                                                              |
| 13  | HIGH     | `IngestRun`/`Write` hash responsibility is split       | **CONFIRMED** | An ingested record had an empty `ContentHash` and was ineligible before `Write`. `IngestRun` now computes it; covered by `TestIngestedRecordIsImmediatelyEligible`.                                                         |

Confirmed: 9. Rejected: 4.

## Historical verdicts (preserved, not waived, not rewritten)

| Task       | Recorded verdict | Recorded lifecycle            | Note                                          |
| ---------- | ---------------- | ----------------------------- | --------------------------------------------- |
| `RDM-001`  | `PARTIAL`        | `LOCAL_DONE`                  | recorded before an acceptance mapping existed |
| `RDM-002`  | `PARTIAL`        | `LOCAL_DONE`                  | recorded before an acceptance mapping existed |
| `RDM-002B` | `PARTIAL`        | `LOCAL_DONE` (task `BLOCKED`) | stopped at the PostgreSQL-validation gate     |

These verdicts are historical evidence. They were not waived, re-derived, or edited.

After the review repairs, `RDM-002B` was re-run in `verify-first` mode and recorded a
**new `VERIFIED`** outcome with zero agent calls. That is a fresh verified result from
the deterministic gates; the earlier `PARTIAL` attempt remains in the run history and
was not rewritten.

## RDM-003 partial work (not verified)

`RDM-003` stopped at `FIX_NO_PROGRESS` after leaving uncommitted, partially integrated
work: `internal/cli/advisory_retrieval.go`, `internal/config/advisory_retrieval.go`,
and additive fields in `internal/cli/rag_verifier.go` and `internal/config/config.go`.
The tree builds and the full suite passes after correcting two compile errors, but the
advisory retriever has **no call site and no producer adapter**, so RDM-003's behavior
is not implemented or verified. These files are outside the acceptance scope above.

## RDM-002 acceptance results

| #   | Check                                                           | Result |
| --- | --------------------------------------------------------------- | ------ |
| 1   | `go test ./internal/outcomefile -run TestIngestionIsIdempotent` | PASS   |
| 2   | `… -run TestStatusesRemainDistinct`                             | PASS   |
| 3   | `… -run TestGatePassCannotConvertPartialToSuccess`              | PASS   |
| 4   | `… -run TestMissingEvidenceIsExplicit`                          | PASS   |
| 5   | `… -run TestSourceDeletionSupersessionAndAccessInvalidate`      | PASS   |
| 6   | `… -run TestRedactionAndProvenance`                             | PASS   |

## RDM-002B acceptance results

| #   | Check                                                                                          | Result |
| --- | ---------------------------------------------------------------------------------------------- | ------ |
| 1   | `go test ./internal/memindex -run TestLocalIngestAndRetrieveWithoutPostgres`                   | PASS   |
| 2   | `… -run TestLocalIndexHasNoPostgresDependency`                                                 | PASS   |
| 3   | `… -run TestSearchHandlesExactErrorsTaskIDsAndPunctuation`                                     | PASS   |
| 4   | `… -run TestInterruptedIngestionPreservesRecords`                                              | PASS   |
| 5   | `… -run TestRebuildIsDeterministic`                                                            | PASS   |
| 6   | `… -run TestStaleEvidenceIsNotReturned`                                                        | PASS   |
| 7   | `… -run TestConcurrentReadersAndWriter`                                                        | PASS   |
| 8   | `… -run TestIndexNeverCreatesExecutionStateTables`                                             | PASS   |
| 9   | `go test ./internal/quality -run TestPostgresIntegrationRequiredMatchesDatabaseSensitivePaths` | PASS   |

## Repository gates

| Gate  | Command                  | Result        |
| ----- | ------------------------ | ------------- |
| Build | `go build ./...`         | PASS          |
| Vet   | `go vet ./...`           | PASS          |
| Tests | `go test ./... -count=1` | PASS (exit 0) |

## Reproduction

```bash
git rev-parse HEAD
shasum -a 256 docs/plans/SOP-RAG-Decision-Memory-Plan.md
go test ./internal/outcomefile -count=1
go test ./internal/memindex -count=1
go test ./internal/retrievalboundary -count=1
go build ./... && go vet ./... && go test ./... -count=1
```

## Scope

These are current verification results for the working tree at the bound source
inputs. They do not rewrite SOP's recorded verdicts, and they do not authorize a
commit, a merge, or an approval.

## RDM-003 — completed and verified

Integration point: `implementContext` (the per-attempt canonical context builder),
planning and recovery phases, with the local SQLite FTS5 index and the authoritative
outcome files used for drift exclusion. No subprocess protocol.

| File | SHA-256 |
| --- | --- |
| `internal/cli/advisory_retrieval.go` | `2cd7caa17e4c35769f69e1b8ec8282fc2ab436853283d3edfc76104d90c31dc5` |
| `internal/cli/advisory_retrieval_test.go` | `d77c45d97dbc9599046c164c84bd6d98c75d394e748ec6448d7787254534b944` |
| `internal/config/advisory_retrieval.go` | `928ed2f37b2b31bd11a71abeddff858c50c76f0444a8813c892d32ff6cdda5f0` |
| `internal/cli/rag_verifier.go` | `af3c4e30b7e10e5ced7249f37a0ec3a0cc90354c5a80e3f71052b827b00524de` |
| `internal/cli/run.go` | `3a737d2ae0131468869570836ad310ac6056d2d6360faf5e7683de3a035eed39` |
| `internal/memindex/index.go` | `ae4d83215d6a3be52b1b5004bb1fd6495b2df2df161f1776d23438eb0e83bbe4` |
| `internal/retrievalboundary/boundary.go` | `aa3a4af450232735961d8aee5eb50790683546ffe21aa08daee14f703a001561` |

A real design bug found and fixed during integration: the query filtered by the
**current** task ID, which excluded all historical evidence. The scope now filters by
corpus and repository only; task context stays in the query text.

SOP recorded `RDM-003` as **VERIFIED** under `verify-first` with zero agent calls and all
six mapped acceptance criteria met.

## RDM-004 — corpus audit: insufficient evidence

### Corpus status

Revision 1 is preserved unchanged. `REVISION-2.md` and `REVISION-2-MANIFEST.json` label
it **UNUSABLE for held-out evaluation** (`usable_for_evaluation: false`); they are an
audit record, not corrected inputs. `CORPUS-AUDIT.md` records the audit
(`REVISION-2.sha256`: `f08b4f2e…6f7a2`, `c33375b3…5076`, `3aa5ae37…dc0e`).

### Audit result

Searched 309 run directories (203 `task.md`, 202 `report.json`, 190 `validation.json`)
and git history. **0 of 12 executable cases recovered.** No run records a starting
repository revision; keyword matches were incidental code text or unrelated reports, and
were not treated as identity.

Affected: `dq-rec-001`..`dq-rec-010`, `dq-neg-003`, `dq-neg-007` — each missing starting
revision, task input, verification command, and provenance.

### Evaluation target

Decision quality (token equality) is measurable today. Executable recovery success is
not: it needs the model's own action applied before verification, and the current output
is a token only. A canned repair is never scored as model success. The target must be
declared explicitly; the 10/10/10 scope cannot be preserved from source-backed runs.

### Accounting

Usage is now durable: `usage.json` persists decisions, requests, retries, tokens, and
active elapsed time across resume, with the same binding as the checkpoint. A request is
counted before it starts (`in_flight`), so an interrupted request survives and is reported
as uncertain. Limits apply to the persisted totals.

### Result

No cloud calls, no setup expansion, no semantic retrieval, no measured report. RDM-004
remains `PLANNED`; `--verify-only` correctly fails. Insufficient evidence is the finding.

## Planning-only decision-memory pilot — measured (reconciled)

### Corpus accounting

| Stage | Count |
| --- | --- |
| Directory entries scanned at build time | 309 |
| Eligible | 62 |
| Excluded: no recorded classification | 247 |
| Corpus cases | 62 = 24 held-out + 38 tuning |
| Current run directories | 317 (8 post-build, all without classification) |

All 62 eligible run IDs appear in the corpus and no other case was added. Held-out:
10 planning, 14 negative; dispositions `NEEDS_HUMAN` 8, `BLOCK` 6, `CONTINUE` 8,
`AUTO_FIX` 2. Tuning and held-out families are disjoint.

### Freeze before execution

`PROTOCOL.sha256` / `CORPUS.sha256` were written at 15:34:06; `run-A.json` / `usage.json`
at 15:36:48. The checkpoint and usage bindings record the same corpus, protocol, rubric,
harness revision, model, and sampling. The gates were frozen before any model call.

### Metric derivation

| Metric | Derivation |
| --- | --- |
| Accuracy | correct repeats / evaluated repeats x 100, per arm |
| Paired difference | per-case mean over 3 repeats, then mean of (B - A) |
| Uncertainty | 2,000-resample bootstrap clustered by case |
| Tokens | estimated (chars/4) prompt+output per decision, summed |
| Safety counters | **unavailable** — never observed |
| False escalations | **unavailable** — predictions not persisted in this run |
| Cost | **unavailable** — no pricing configured |

### Arm-B retrieval did not occur

Deterministic reconstruction without a model call (`TestPilotRetrievalCoverage`):
**0 of 24 held-out and 0 of 38 tuning cases retrieved any evidence.** Identical token
totals (32,222) confirm both arms produced identical prompts. Arm B was inert, so the
+5.56 pp difference is model noise and cannot be attributed to retrieval.

### Result and decision

Arm A 31.94%, arm B 37.50%, paired +5.56 pp, case-clustered 95% interval
[-6.94, 20.83] pp. 144 decisions, 144 requests, 0 retries, 64,444 tokens, 60 s active.
**NO-GO**: the gain is below the 10 pp threshold, the interval includes zero, and the
retrieval manipulation was inert. Memory remains off by default.

### Interpretation

The label is the recorded SOP disposition. Scoring therefore measures **agreement with
historical classifications**, not independently validated decision quality, and says
nothing about executable recovery.

### Skip evidence

`RDM-004/004B/002C/005/006` are `NOT_REQUIRED` with `CORPUS-AUDIT.md` recorded as evidence.
That record cannot be rewritten: `sop task skip` rejects an already `NOT_REQUIRED` task.
`CLOSEOUT.md` supersedes it as the consolidated evidence; no execution state was edited.

## Post-run audit and retrieval repair

The original pilot's query joined all task-context tokens with an implicit **AND**, so arm
B retrieved no evidence (0 of 24 held-out, 0 of 38 tuning). The identical estimated token
totals (32,222) confirm both arms produced identical prompts; the measured +5.56 pp is
model noise, not a retrieval effect.

**Repair (development evidence only, tuning-tested):**

- `BuildFTSQuery` now joins quoted tokens with `OR`; bm25 ranks the candidates.
- `memindex.Search` enforces a deterministic relevance floor of two shared query terms
  (one for a one-term query); records below the floor are rejected.

Deterministic audit, no model calls:

| Split | Cases with evidence | Threshold-compliant items | Below floor | Below-floor rejected |
| --- | --- | --- | --- | --- |
| Tuning | 24 of 38 | 57 | 0 | 110 |
| Held-out (audit only) | 8 of 24 | 25 | 0 | 63 |

These counts measure **lexical threshold compliance**, not independently judged relevance: no human or model adjudication of relevance exists, so retrieval quality remains unverified. Prompt equality for the original run is **unverified** (prompts were not persisted; equal estimated token totals do not prove byte equality). Retrieved IDs and actual prompt inclusion are recorded in
`run/audit/retrieval-coverage.json` and `tuning-audit/audit/retrieval-coverage.json`.

**Report corrections:** the metric is labelled *classification accuracy (agreement
with recorded dispositions)*; tokens are labelled *estimated*; safety and false-escalation
metrics are marked unavailable. The coverage gate is applied to **disposition labels**:
24 cases and 4 labels pass, but `AUTO_FIX` has 2 cases (< 3), so the gate **fails**. The
sample was not enlarged. The original RDM-004 10/10/10 gate does not apply (recovery = 0).

**Not re-run.** The corrected query has not been evaluated with a model; `REVISION-2.md`
records it as development evidence and requires a new frozen held-out revision before any
benefit claim. Memory remains disabled by default.
