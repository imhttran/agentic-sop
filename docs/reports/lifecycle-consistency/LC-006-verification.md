# LC-006 — Verification and Behavior-Preservation Audit

**Status:** read-only audit; documentation only. **Scope:** verify that the
lifecycle-consistency hardening (LC-001…LC-005) holds, that existing successful
completion behavior is unchanged, and that RM-003's recorded artifacts are
byte-unchanged by this plan. **Verdict:** **GO**.

LC-006 creates exactly one file — this report — under `docs/reports/lifecycle-consistency/`.
It creates, modifies, or deletes no production file, no run artifact, and no
archive record.

> **Provenance note.** The preceding governed LC-006 run produced a draft that
> returned HOLD after reading (a) LC-003's **superseded** `FAILED` run artifact
> (`state.json`/`gate.json` from the retried first attempt) and (b) an
> **intermediate** `gofmt` failure recorded during LC-005's own fix cycle. This
> operator-completed version corrects those stale readings against the current,
> committed state and a freshly executed gate suite (§2, §5), and records the
> required per-item assessments, including the cross-process TOCTOU assessment
> (§7). No production file was changed to produce this report.

## 1. Verdict

**GO.** Every acceptance-gate criterion passes on the current tree, and the
hardening's invariants are enforced and covered by deterministic tests.

| Acceptance criterion                                                                                                                          | Result                      |
| --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------- |
| `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, `go test -race ./...`, documentation link check, `git diff --check` all pass | **PASS** (§3)               |
| Before/after evidence: clean-plan completion still archives `COMPLETE`, preserves task history                                                | **PASS** (§4)               |
| RM-003 `approval.json`, `approval-history.json`, archive byte-unchanged                                                                       | **PASS** (§5)               |
| Every changed spec statement consistent with implemented/tested behavior                                                                      | **PASS** (§8)               |
| Report states verified surfaces and a GO/HOLD verdict                                                                                         | **PASS** (§9, this section) |

## 2. Stage status and invariant ownership

All eight stages are `LOCAL_DONE` in the active plan
(`plan-sop-lifecycle-consistency-hardening`, state `ACTIVE`).

| Stage  | Invariant            | Persisted status | Recorded gate           | Notes                                                                                                                                                                                                                                                                                                          |
| ------ | -------------------- | ---------------- | ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| LC-001 | contract (read-only) | LOCAL_DONE       | PASS                    | committed `b5cfbe1`                                                                                                                                                                                                                                                                                            |
| LC-002 | I1                   | LOCAL_DONE       | PASS                    | committed `b64ce75`                                                                                                                                                                                                                                                                                            |
| LC-003 | I2                   | LOCAL_DONE       | — (external completion) | committed `2d6a91a`; `external-completion.json` records completion at `2d6a91a`, verification `PASS`. Its `state.json`/`gate.json` still hold the **retried first attempt's** `FAILED`/`CONTINUE` record and are superseded by the external completion. This is the artifact the governed audit draft misread. |
| LC-004 | I4                   | LOCAL_DONE       | PASS                    | committed `5a0edad`                                                                                                                                                                                                                                                                                            |
| LC-005 | I5                   | LOCAL_DONE       | PASS                    | committed `7eb451e`                                                                                                                                                                                                                                                                                            |
| LC-006 | audit                | LOCAL_DONE       | PASS                    | this report (uncommitted)                                                                                                                                                                                                                                                                                      |
| LC-007 | supersession         | LOCAL_DONE       | PASS                    | committed `d9db079`                                                                                                                                                                                                                                                                                            |
| LC-008 | graph ordering       | LOCAL_DONE       | PASS                    | committed `d6ef044`                                                                                                                                                                                                                                                                                            |

## 3. Acceptance gate suite (freshly executed)

Executed on the current tree (HEAD `7eb451e`, plus this uncommitted report).

| Gate                     | Command                             | Result                       |
| ------------------------ | ----------------------------------- | ---------------------------- |
| Formatting               | `gofmt -l .`                        | **PASS** (empty)             |
| Static analysis          | `go vet ./...`                      | **PASS**                     |
| Build                    | `go build ./...`                    | **PASS**                     |
| Unit tests               | `go test -count=1 ./...`            | **PASS** (all packages)      |
| Race tests               | `go test -race -count=1 ./...`      | **PASS** (all packages)      |
| Documentation link check | `scripts/checks/check-doc-links.sh` | **PASS** (`broken links: 0`) |
| Diff hygiene             | `git diff --check`                  | **PASS**                     |

The `gofmt` gate is clean. The earlier `LINT FAIL` line in
`.agent-sdlc/runs/LC-005/validation.json` is an intermediate fix-cycle record; the
final LC-005 validation and this audit both record it clean.

## 4. Behavior preservation — clean-plan completion still archives `COMPLETE`

- **Before (LC-001 §8.1):** `sop plan complete` gated only on `AllSatisfied`; a
  clean, fully-satisfied plan archived `COMPLETE`.
- **After (LC-004):** `Complete` additionally reuses the shared closure predicate
  (`closureBlocker`), refusing an unresolved work item, an active-plan `PENDING`
  approval, or missing verification — the clean, fully-satisfied-and-verified plan
  is unchanged.
- **Evidence:** `internal/planflow/complete_test.go`
  `TestCompleteArchivesFullySatisfiedPlan` (archives `COMPLETE`, task records
  preserved, plan released from ACTIVE) and `internal/cli/plan_complete_test.go`
  `TestRunPlanCompleteArchivesSatisfiedPlan` pass deterministically; the refusal
  path leaves the plan ACTIVE with tasks intact.

## 5. Historical evidence immutability (byte-unchanged)

Artifacts are under `.agent-sdlc/`, which no commit and no stage of this plan
writes; their hashes are identical at every checkpoint in this engagement
(pre-hardening baseline, each stage preflight, and post-LC-006).

RM-003 (`.agent-sdlc/runs/RM-003/`), directory aggregate
`ed6f2d5105d4ca6bae8809f6d7bc8c6e02952bcab6e1ec6f1a441ac6fa4d38ff`:

| Artifact                   | SHA-256                                                            |
| -------------------------- | ------------------------------------------------------------------ |
| `approval.json`            | `5040f9291170ed10d6e105c6509ffe41db0e3a0943c2d4d66ee2770d64f8e7e5` |
| `approval-history.json`    | `dcd0e94d4b93dcdfe756ab8edc1d887c73488453e9c340d7e9dc67c230f693b3` |
| `external-completion.json` | `d2ede404388ec00be81c8eb29d8a701028704c7d80526e3bc23429e789dd1b3e` |

RM-003's plan archive (`.agent-sdlc/archive/plan-run-metrics-and-observability/`),
directory aggregate `6bcc83f2839ae2165584a1e6fb5e69fd7ae317dd9670b265a33ba745b99868fb`:

| Artifact         | SHA-256                                                            |
| ---------------- | ------------------------------------------------------------------ |
| `lifecycle.json` | `8ad567f2eee80f60598e68d8a723756cd87d28f991902477eb685b739fe777d0` |
| `plan.json`      | `151e36b7cef359154dd25da6b94f64eaa82dadc25699e8826ae63697498ba8b3` |
| `plan.meta.json` | `a351e6456d2906283f4e058e5bf53225d7f2910ffd08bf31935332545ae02978` |
| `tasks.json`     | `7f4caca0c4b580627aeb4e161626618ef614cf6212b41e7e69ff8b7e6f95ae5d` |

LC-003 (`.agent-sdlc/runs/LC-003/`), directory aggregate
`d36839bb6c407aa8eea8daca1c417d897d8a84f525735631dfdcd7a3ac1f4990` — unchanged.

**Stale disposition (verified):** RM-003's `approval.json` is `status: PENDING`,
`kind: NEEDS_HUMAN`, `task_id: RM-003`, and its task was externally completed
(`external-completion.json` present). Under the LC-001 contract it is classified
**`stale`** (active ∧ satisfied) — never rewritten or deleted by this plan.

## 6. Required verification items

| #   | Item                                         | Result                                | Evidence                                                                                                                                                                                                                    |
| --- | -------------------------------------------- | ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Shared closure predicates                    | **PASS**                              | `closureBlocker` (`internal/planflow/historicalize.go`) is the single predicate; `Complete` reuses it (`planflow.go`), and readiness derives from the same sources.                                                         |
| 2   | Pending approval applicability               | **PASS**                              | Active-plan `PENDING` blocks regardless of satisfaction; unrelated-plan `PENDING` ignored. LC-005 table rows + `TestApprovalStatesUnsatisfiedPendingBlocks`.                                                                |
| 3   | SUPERSEDED semantics                         | **PASS**                              | `ApprovalSuperseded` is a distinct status; `Resolved()` includes it; `decide()` refuses a superseded head. LC-005 `superseded…` row; LC-007 tests.                                                                          |
| 4   | Historical approval preservation             | **PASS**                              | Append-only history; original request preserved verbatim (LC-007 tests); RM-003 byte-unchanged (§5).                                                                                                                        |
| 5   | complete vs historicalize behavior           | **PASS**                              | LC-005 drives both the `complete` and `historicalize` closures per row and asserts agreement; distinct operations preserved (only the shared gate changed).                                                                 |
| 6   | Verification evidence requirements           | **PASS**                              | A satisfied task needs a `PASSED` run, an external-completion record, or a not-required record; `ExecutionDone` is exempt. LC-004 `TestCompleteRefusesMissingVerification`, `TestCompleteExecutionDoneNeedsNoVerification`. |
| 7   | External completion compatibility            | **PASS**                              | External completion resolves a pending head; no `PENDING` head remains for the completed task (LC-002 tests; LC-005 `externally-completed…` row).                                                                           |
| 8   | Reconciliation ordering and rollback         | **PASS**                              | `ReplaceGraph` writes rows before edges; transactional rollback preserved (LC-008 `graph_test.go`).                                                                                                                         |
| 9   | Operator-facing readiness and error messages | **PASS**                              | `closureBlocker` returns a reason naming the offending task ids; `Complete` wraps it as `ErrHistoricalizationIneligible`; CLI refusals assert the `approval`/`verification` reasons.                                        |
| 10  | Cross-process TOCTOU risk in `Complete`      | **PASS (with documented limitation)** | See §7.                                                                                                                                                                                                                     |

## 7. Item 10 — cross-process TOCTOU in `Complete`

**Existing protection.**

- `historicalize` performs an **explicit pre-mutation re-read**: it recomputes
  `historicalizationFingerprint(meta, tasks)` and compares it to the readiness
  fingerprint (and, when supplied, `ExpectedFingerprint`), then re-runs
  `historicalizationStillEligible` before mutating. A change between readiness and
  mutation is refused with `ErrStaleHistoricalizationState`
  (`TestHistoricalizeStaleStateRefused`, `TestHistoricalizeDetectsStateChangeBetweenReads`).
- `complete` evaluates the shared `closureBlocker` **immediately before**
  `archivePlan`, with no intervening user code, so a state change that appears
  _after a prior readiness evaluation_ is caught because `Complete` re-reads fresh
  state rather than trusting an earlier verdict
  (`TestCompleteRechecksClosureBeforeArchiving`).

**Remaining race window.** Between `closureBlocker`’s read and `archivePlan`’s
write there is no _in-process_ scheduling point, so there is no in-process race.
A truly concurrent **external process** could still write an approval/verification
artifact in that window; `complete` has no `ExpectedFingerprint`/re-read seam to
detect it. This is not deterministically testable without adding a production test
hook.

**Does the plan permit this?** Yes. The plan's Safety Invariants and Closure
Invariants require closure to **fail closed** (never archive an open gate or
missing verification) and to make the two closers agree; they do **not** require
cross-process atomicity. SOP's model is single-operator, single-writer, and
`complete` is an operator-invoked close. The single adjacent check satisfies the
"recheck immediately before mutation" intent by construction.

**Recommendation.** No production change is made here (out of LC-004/006 scope).
If cross-process atomicity is later desired, it should be a **separate, scoped
task** (e.g., give `complete` the same `ExpectedFingerprint` + pre-mutation re-read
that `historicalize` already has). Until then, this limitation is documented, not
claimed as solved.

## 8. Spec-consistency findings

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md`
  (committed `2ee57e3`) records `resolved(A) := APPROVED ∨ DECLINED ∨ SUPERSEDED`
  and the `unresolved(A,T) := active(A)` closure predicate; both match the
  implemented and tested behavior (LC-003/LC-004/LC-007). **Consistent.**
- `docs/specs/HUMAN-APPROVAL.md` and `docs/specs/PLAN-HISTORICALIZATION.md` were
  **not modified** by this plan; no documented gate is weaker than its
  implementation. **No clarification required.**
- No second or contradictory invariant mapping remains; the LC-001 §5/§8.2 mapping
  is the sole one.

## 9. Verified surfaces

Approval boundary (`internal/approval/approval.go` supersede/decide/applicable);
historicalization readiness + closure (`internal/planflow/historicalize.go`);
completion closure (`internal/planflow/planflow.go` `Complete`); graph persistence
(`internal/store/sqlite.go` `ReplaceGraph`); CLI closure surfaces
(`internal/cli/plan.go`, `internal/cli/approval.go`, `internal/cli/approval_supersede.go`);
and the deterministic suites (`internal/planflow/approval_states_regression_test.go`,
`internal/planflow/complete_test.go`, `internal/cli/plan_complete_test.go`,
`internal/planflow/historicalize_scope_test.go`, `internal/store/graph_test.go`,
`internal/cli/approval_supersede_test.go`).

## 10. Outstanding requirements and recommended disposition

- **Approvals:** none pending (`sop approvals` empty). No human gate is open.
- **Not performed (out of scope for this task):** the LC-006 report is **not
  committed**, and the plan is **not** completed or historicalized.
- **Recommended disposition:** with all gates green, all eight stages
  `LOCAL_DONE`, and historical evidence byte-unchanged, the plan is **eligible**
  for a final close (`sop plan complete` or `sop plan historicalize`). Doing so
  requires explicit human authorization, and the LC-006 report should be committed
  first (its own separate authorization). The item-10 external-process TOCTOU
  limitation (§7) is acceptable under the plan's safety invariants; a stronger
  guarantee, if wanted, should be filed as a separate scoped task.
