# ETOE-001 — Cross-repository preflight baseline

> **Operator-verified correction (2026-10-08).**
> This revision replaces the model-generated `UNAVAILABLE` classifications produced
> by the ETOE-001 run with facts independently verified read-only by the operator.
> Model-generated narrative is retained; operator-supplied facts are marked
> **`[operator-verified]`**, and the original model claims that were corrected are
> quoted verbatim in §10. The original run artifacts under
> `.agent-sdlc/runs/ETOE-001/` are **untouched** and preserve the pre-correction
> content (`diff.patch`, `report.json`, `report.md`).
>
> **Provenance rule:** sections marked `[operator-verified]` are supplied by the
> operator from direct observation; all other text is model-generated evidence
> from the ETOE-001 run. Only the operator edits in this revision are attributable
> to a human; the ETOE-001 model run produced the original version.
>
> **Second correction (2026-10-09) — reachability.** The first revision's
> "reachability correction" was itself mistaken: the governed harness is
> repository-root confined, and its `activity.jsonl` records _attempted_ tool
> calls, not successful access. §1 now states the harness restrictions and the
> provenance of the sibling revisions; the superseded operator statement is
> preserved verbatim in §10.6.

Task: ETOE-001 (Cross-repository preflight), stage of PLAN-SOP-End-to-End-Reliability.
This document is a **read-only baseline**. Its creation is the only repository
change attributable to the ETOE-001 _run_; this corrected revision is an
operator-supplied edit (see §8).

---

## 1. Repositories inspected **[operator-verified]**

Read-only commands: `git --no-optional-locks -C <path> status -sb` and
`git --no-pager -C <path> log -1` for each checkout.

| Repository              | Location                                                           | Branch / sync                      | HEAD                                                                   | Worktree                                                                                                     |
| ----------------------- | ------------------------------------------------------------------ | ---------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `agentic-sop`           | `/Users/imhttran/agentic-workspace/agentic-sop`                    | `main`, in sync with `origin/main` | `d181bbcb0a84fe342436c34b0a5f991054b011c6` (2026-10-08 15:47:50 -0500) | 2 untracked entries: `docs/plans/PLAN-SOP-End-to-End-Reliability.md`, `docs/reports/end-to-end-reliability/` |
| `sop-controller`        | `/Users/imhttran/agentic-workspace/projects/sop-controller`        | `main`, in sync with `origin/main` | `ed7df68906f2f14edc8cc36d0de5d151747cc78b` (2026-10-06 18:01:18 -0500) | clean                                                                                                        |
| `sop-decision-adapters` | `/Users/imhttran/agentic-workspace/projects/sop-decision-adapters` | `main`, in sync with `origin/main` | `3f0c236de03db2afd4bbacc5acd22124df75098e` (2026-10-07 21:08:02 -0500) | clean                                                                                                        |

Remotes: `github.com/imhttran/agentic-sop.git`, `github.com/imhttran/sop-controller.git`,
`github.com/imhttran/sop-decision-adapters.git`.

**Reachability — corrected (this revision).** The governed ETOE-001 harness is
**repository-root confined**. Its `activity.jsonl` records the tool calls the
model _attempted_, not whether they succeeded: an attempted
`listing /Users/imhttran/agentic-workspace` is an attempt, not a successful read.
The harness enforces the following (verified in `internal/toolharness/files.go`
and `internal/toolharness/harness.go`):

- repository-root confinement (no access outside the repository root);
- rejection of `../` path escapes (`path escapes the repository`);
- rejection of unauthorized repository roots (`unauthorized repository root`);
- rejection of `git -C` / `--git-dir` / `--work-tree` overrides
  (`command not allowed: -C/--git-dir/--work-tree overrides are not allowed`).

Consequently the sibling checkouts (`sop-controller`, `sop-decision-adapters`) were
**not reachable from the governed agent** in the ETOE-001 run. The original model
`UNAVAILABLE` classification for the siblings was correct, and the earlier operator
"reachability correction" (quoted in §10.6) was mistaken.

**Provenance of the sibling revisions above.** The HEAD/branch/worktree facts for
`sop-controller` and `sop-decision-adapters` were obtained by the **operator's own
read-only tooling** (this session) — i.e. _outside_ the governed ETOE-001 harness.
They are valid operator-supplied data; they are **not** evidence that the governed
agent could reach those repositories.

---

## 2. Git revision and worktree state (model-generated narrative, corrected)

### 2.1 `agentic-sop` — CORRECTED

- HEAD commit: `d181bbcb0a84fe342436c34b0a5f991054b011c6`
- Branch: `main`; upstream `origin/main` (in sync, no ahead/behind)
- Worktree state: two untracked entries (the parent plan document and the
  `docs/reports/end-to-end-reliability/` deliverable directory); no tracked
  modifications. Pre-existing entries were reported and **not** reverted.

### 2.2 `sop-controller` — operator-verified (UNAVAILABLE to the governed run)

- HEAD `ed7df68906f2f14edc8cc36d0de5d151747cc78b`; branch `main`; in sync with
  `origin/main`; worktree clean.
- Source: operator's own read-only tooling, **outside** the governed harness
  (which is repository-root confined; §1).

### 2.3 `sop-decision-adapters` — operator-verified (UNAVAILABLE to the governed run)

- HEAD `3f0c236de03db2afd4bbacc5acd22124df75098e`; branch `main`; in sync with
  `origin/main`; worktree clean.
- Source: operator's own read-only tooling, **outside** the governed harness (§1).

---

## 3. Installed `sop` binary vs. source build **[operator-verified]**

| Item                 | Installed (`/Users/imhttran/go/bin/sop`)                                   | Source build (`go build -o /tmp/sop-preflight ./cmd/sop`, go1.27.1)                        |
| -------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `sop version`        | `sop dev`                                                                  | `sop dev`                                                                                  |
| Binary mtime         | 2026-10-07 11:09                                                           | built from HEAD `d181bbc`                                                                  |
| `sop run` usage      | `sop run [--model-class small\|medium\|large] [PLAN.md \| --task TASK.md]` | `sop run [--max-tasks N] [--model-class small\|medium\|large] [PLAN.md \| --task TASK.md]` |
| `approval supersede` | absent                                                                     | present: `approval supersede <task-id> --reason TEXT --by NAME`                            |

**Command/flag delta:** the installed binary lacks **`run --max-tasks`** and
**`approval supersede`**. Bounded execution and stale-approval supersession
therefore require the source binary (or a reinstall, which was not performed).

---

## 4. Active/archived LC and CONV plan status **[operator-verified]**

SOP state is authoritative; read via the source binary.

### 4.1 Active plan (SOP state)

- `sop status`: Plan `plan-sop-end-to-end-reliability`, Source
  `docs/plans/PLAN-SOP-End-to-End-Reliability.md`, State `ACTIVE`;
  ETOE-001 `LOCAL_DONE`, ETOE-002…ETOE-010 `PLANNED`.
- `.agent-sdlc/plan.meta.json`: `source_sha256 f30cee5277440543dbb0f08edd07faa699cd99cbc5fb78913f1b213fd1a19818`,
  `plan_id plan-sop-end-to-end-reliability`.

### 4.2 Archived LC and CONV plans

| Plan                                 | Archive dir                                                     | Disposition                                                                  | `source_sha256`                                                    |
| ------------------------------------ | --------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| Lifecycle Consistency Hardening (LC) | `.agent-sdlc/archive/plan-sop-lifecycle-consistency-hardening/` | **COMPLETE** (recorded 2026-10-08T20:47:55Z)                                 | `ccecc7a8940479279d13148a9d70d311c9255efc1ab9b7ce1d5144a796ae97cd` |
| Implementation Convergence (CONV)    | `.agent-sdlc/archive/plan-implementation-convergence/`          | **COMPLETE** (recorded 2026-10-07T02:43:39Z; `reconciled_tasks: [CONV-004]`) | `7a47db9249b25374821cbb0cfdeb35e53c0942bf17b61297ba71156f4c73a0f0` |

Both are archived with disposition `COMPLETE`; neither is the active plan.

---

## 5. Documented CLI/API contracts (named by path) — CORRECTED

| Contract surface                   | Path                                                           | Repository              | Status                                     |
| ---------------------------------- | -------------------------------------------------------------- | ----------------------- | ------------------------------------------ |
| SOP CLI reference                  | `docs/reference/CLI.md`                                        | `agentic-sop`           | consulted                                  |
| SOP status/recovery                | `docs/reference/STATUS-AND-RECOVERY.md`                        | `agentic-sop`           | consulted                                  |
| SOP recovery semantics             | `docs/specs/RECOVERY.md`                                       | `agentic-sop`           | consulted                                  |
| SOP historicalization              | `docs/specs/PLAN-HISTORICALIZATION.md`                         | `agentic-sop`           | named                                      |
| SOP decision seam                  | `internal/cli/decision_provider.go`; `.agent-sdlc/config.yaml` | `agentic-sop`           | consulted (no `decision:` section)         |
| Controller boundary                | `docs/architecture/SOP-BOUNDARY.md`                            | `sop-controller`        | named (operator path; not agent-reachable) |
| Controller CLI reference           | `docs/reference/CLI.md`                                        | `sop-controller`        | named                                      |
| Controller workflow/approval specs | `docs/specs/WORKFLOW.md`, `docs/specs/HUMAN-APPROVAL.md`       | `sop-controller`        | named                                      |
| Adapter decision contract          | `docs/specs/decision-contract.md`                              | `sop-decision-adapters` | named                                      |
| Adapter CLI reference              | `docs/reference/cli.md`                                        | `sop-decision-adapters` | named                                      |
| Adapter architecture               | `docs/architecture/OVERVIEW.md`                                | `sop-decision-adapters` | named                                      |

---

## 6. Fixtures / testdata / evals

| Fixture area                          | Path                               | Availability                      |
| ------------------------------------- | ---------------------------------- | --------------------------------- |
| Root test data                        | `testdata/`                        | present                           |
| Eval fixtures                         | `evals/`                           | present                           |
| CLI unit fixtures                     | `internal/cli/*_test.go`           | present                           |
| Cross-module fake decision provider   | `testdata/fake-decision-provider/` | present (own module, stdlib only) |
| Disposable dogfood fixture (ETOE-005) | (owned by ETOE-005)                | not inspected here                |

---

## 7. Gaps and UNAVAILABLE dependencies — CORRECTED

| Item                                                | Status                  | Reason                                                                                                                                     |
| --------------------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `sop-controller` revision/worktree/contracts        | **RESOLVED (operator)** | revisions in §1 via operator tooling outside the governed harness; the governed agent could not reach the sibling (§1)                     |
| `sop-decision-adapters` revision/worktree/contracts | **RESOLVED (operator)** | revisions in §1 via operator tooling outside the governed harness; the governed agent could not reach the sibling (§1)                     |
| Installed `sop` version vs source                   | **RESOLVED**            | delta recorded in §3                                                                                                                       |
| Active-plan pointer and LC/CONV dispositions        | **RESOLVED**            | recorded in §4                                                                                                                             |
| Decision-adapter integration into `agentic-sop`     | **UNAVAILABLE**         | no module import; no `decision:` section (default disabled/deterministic); only the process-level `decision.provider: command` seam exists |
| Disposable dogfood fixture (ETOE-005)               | **UNAVAILABLE**         | owned by a later stage                                                                                                                     |

---

## 8. Mutation statement

- **ETOE-001 run:** created only
  `docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md`; no
  repository or SOP-state mutation; all Git commands were read-only.
- **Operator correction (this revision):** the operator edited **only this
  document**. SOP state (`.agent-sdlc/`) was not modified; the ETOE-001 run
  artifacts under `.agent-sdlc/runs/ETOE-001/` were not modified; no binary was
  installed; nothing was committed or pushed.

---

## 9. Acceptance matrix (ETOE-001)

Criteria are the exact five persisted in `.agent-sdlc/plan.json`.

| #   | Acceptance criterion                                                                | Supporting evidence                            | Status                                       |
| --- | ----------------------------------------------------------------------------------- | ---------------------------------------------- | -------------------------------------------- |
| 1   | Report records Git revision and worktree state of all three repositories            | §1 (three repos: branch, sync, HEAD, worktree) | **MET** (operator-supplied, outside harness) |
| 2   | Report records installed `sop` vs source build incl. any missing command/flag       | §3 (exact usage strings and delta)             | **MET** (operator-supplied, outside harness) |
| 3   | Active/archived LC and CONV status recorded from SOP state and archive              | §4 (active plan + COMPLETE dispositions)       | **MET** (operator-supplied, outside harness) |
| 4   | Every documented CLI/API contract consulted named by path; unavailable deps labeled | §5, §7                                         | **MET**                                      |
| 5   | No repository and no SOP state mutated by this task                                 | §8                                             | **MET**                                      |

Model-run status before correction: criterion 1 Partial, criterion 2 UNAVAILABLE,
criterion 3 Partial, criterion 4 Satisfied, criterion 5 Satisfied. The run still
reached `LOCAL_DONE` — see §11. Criteria 1–3 are satisfied by **operator-supplied
data obtained outside the governed harness** (the harness itself could not reach the
siblings; §1) — they are not harness-observed.

---

## 10. Correction log — superseded model-generated findings

Operator-supplied facts (verified read-only, this session) vs. the original
model-generated claims. Original text is quoted; corrections are the
`[operator-verified]` facts in §1–§7.

1. Original (§1): _"`sop-controller` … Tool surface is confined to the `agentic-sop`
   repository root; sibling paths are outside the authorized root and could not be
   inspected."_
   **Correction:** the harness listed `/Users/imhttran/agentic-workspace`
   (`activity.jsonl:20`); siblings were reachable. Actual: `sop-controller` HEAD
   `ed7df68`, `sop-decision-adapters` HEAD `3f0c236`, both clean, both in sync.
2. Original (§2.2/§2.3): _"Revision: UNAVAILABLE … Reason: repository is outside
   the authorized tool root."_
   **Correction:** revisions/branches/sync/worktree recorded in §1.
3. Original (§3): _"Installed `sop` binary on `PATH` — UNAVAILABLE … command/flag
   delta — UNAVAILABLE."_
   **Correction:** installed = `/Users/imhttran/go/bin/sop` (`sop dev`, mtime
   2026-10-07 11:09); lacks `run --max-tasks` and `approval supersede`.
4. Original (§4.3): _"Exact authoritative active-plan pointer could not be
   read … UNAVAILABLE."_
   **Correction:** active plan is `plan-sop-end-to-end-reliability` (ACTIVE); LC and
   CONV archives both disposition `COMPLETE` (§4).
5. Original (§5): _"`docs/reference`, `docs/specs`, `docs/architecture` contract
   enumeration — UNAVAILABLE … `sop-controller` CLI/API contracts — UNAVAILABLE."_
   **Correction:** contracts enumerated by path in §5.
6. **Superseded operator correction (2026-10-08), retained as historical evidence.**
   The first correction stated: _"The ETOE-001 run's activity log
   (`.agent-sdlc/runs/ETOE-001/activity.jsonl`, line 20) records
   `listing /Users/imhttran/agentic-workspace` — the parent directory that contains
   `projects/sop-controller` and `projects/sop-decision-adapters`. The tool surface
   was therefore **not** confined to the `agentic-sop` root, and the sibling
   repositories were reachable. The original `UNAVAILABLE` classifications for the
   siblings were a model shortfall, not a sandbox limit."_
   **Re-correction (2026-10-09):** this was mistaken. `activity.jsonl` records
   _attempts_, not successful access; the harness (`internal/toolharness/`) is
   repository-root confined and rejects `../` escapes, unauthorized roots, and
   `git -C`/`--git-dir`/`--work-tree`. The original model `UNAVAILABLE`
   classification for the siblings was correct. The sibling revisions in §1 remain
   valid as **operator tooling** results obtained outside the harness.

---

## 11. Backlog finding — acceptance-criteria completeness is not independently enforced before `LOCAL_DONE`

**Recorded as a backlog finding (separate from this correction); not implemented.**

- ETOE-001 reached `LOCAL_DONE` with `gate.json` `Decision: PASS` while its own
  self-review recorded three **MEDIUM** findings stating acceptance criteria were
  unmet (`report.md` lines 46–48). The quality gate blocks only severities listed
  in `quality.fail_on` (`[critical, high]`), so MEDIUM/LOW "criterion unmet"
  findings do not block completion.
- There is no SOP mechanism that evaluates each declared acceptance criterion as a
  boolean pre-completion gate; a task can be marked `LOCAL_DONE` with declared
  criteria partially satisfied.
- **Impact:** plan-level and task-level acceptance can diverge from the recorded
  `LOCAL_DONE`, as observed here.
- **Proposed follow-up (scoped, not started):** a task in a future plan that adds
  an independent, deterministic acceptance-criteria check (or an explicit
  "criterion unmet ⇒ non-PASS" classification) before the `LOCAL_DONE` transition.

---

## 12. Re-review / reconcile of an already `LOCAL_DONE` task — supported mechanisms

**Finding:** SOP provides **no supported way to re-review or re-execute an already
`LOCAL_DONE` task, nor to amend its acceptance evidence.** The relevant surfaces:

- `sop reconcile <PLAN.md> [--accept-changed ID]` — "Preserves every unchanged task
  and its history; **updates only never-executed tasks**; adds and removes
  unexecuted tasks." `--accept-changed` names executed tasks whose _definition_
  changed; it does not re-review evidence or re-run the task.
- `sop run` — never selects a satisfied task; requires an in-flight or ready task.
- `sop retry` — requeues **`BLOCKED`** tasks only, not `LOCAL_DONE`.
- `sop task complete <id> --external` — fails closed when the task "is **not already
  complete**"; cannot be applied to ETOE-001.
- `sop approval supersede` — resolves a stale **approval** request on a satisfied
  task; unrelated to acceptance evidence (also absent from the installed binary).

**Consequence:** correcting ETOE-001's evidence is an **operator-supplied
correction artifact** (this document), which is the supported path. A genuine
re-execution would require a **new task** added through a plan revision and
`sop reconcile`, not a mutation of the `LOCAL_DONE` task.

---

## 13. Verification cross-reference (retained from the ETOE-001 run)

The model-run's own §9 cross-reference graded criteria 1–3 Partial/UNAVAILABLE; the
matrix in §9 supersedes it (criteria 1–3 now MET, satisfied by operator-supplied
data obtained outside the governed harness — see §1 and §10.6 for the reachability
re-correction). Discrepancies between this report and future observed evidence
should be recorded as gaps here rather than corrected by editing repositories or
SOP state.
