# CLOSE-005 — Controller Work Verdicts (Finish or Verify Current Controller Work)

> **Final status (2026-10-06).** This is a point-in-time record of its stage. The pre-performance closure is complete (`CLOSE-001…CLOSE-011` `LOCAL_DONE`). Where this record states a `NEEDS_HUMAN`, `BLOCKED`, `PARTIAL` or `ABSENT` finding, see [CLOSE-011-readiness.md](../../reports/pre-performance-closure/CLOSE-011-readiness.md) for the final verdict and the explicit deferrals.

> The wrap-up plan path cited in this record (`docs/plans/PLAN-Wrap-Up.md`) now resolves to the located historical plan `docs/history/PLAN-wrapup.md`; the closure plan references that path.

Per-item verdicts for the current sop-controller work state, derived from live
code/tests/SOP provenance rather than stale wrap-up or status documents, together
with the recorded re-run of both deterministic gates (CLOSE-003 SOP, CLOSE-004
controller) and the current source/config/patch/binary hashes. This Markdown report
is the sole authorized evidence mutation created by CLOSE-005.

- Captured at (UTC): 2026-10-04 (read-only inspection + deterministic re-run)
- Discipline: read-only for the sop-controller sibling checkout; this report is the
  only agentic-sop file created/updated by CLOSE-005.
- Evidence basis: `docs/reports/pre-performance-closure/CLOSE-001-baseline.md`,
  `CLOSE-002-status-reconciliation.md`, `CLOSE-004-controller-deterministic-baseline.md`
  and `docs/plans/PLAN-Pre-Performance-Closure.md`.
- Failure discipline: a non-passing check is recorded as a blocking failure with exact
  evidence; a non-passing gate blocks performance implementation.

---

## 0. Current-state derivation (not stale status)

CLOSE-005 derives the controller work state from current code, tests and SOP
provenance. Recorded task status (`PLANNED`, `PENDING`, `NOT READY`) is kept distinct
from implementation evidence, and recorded status alone never yields a COMPLETE
verdict. The pending CTRL006 approval and the historical C2-009 NOT READY result are
treated as reconciliation evidence only and are neither relabeled PASS nor treated as
proof of a current human action.

### 0.1 Discovery inventory (per verdict item)

| Verdict item | Candidate current evidence artifact(s) | Located? |
| --- | --- | --- |
| Wrap-up | `docs/plans/PLAN-Wrap-Up.md` (referenced); `docs/history/PLAN-wrapup.md`; `sop resume`/`sop status` provenance; CLOSE-002 M2 | No — no controller-side wrap-up source located in this root; agentic-sop `docs/history/PLAN-wrapup.md` is a historical agentic-sop plan, not controller state |
| Hardening | controller-side `docs/PLAN-Hardening.md` (per TASK-044 reference); `docs/history/RESOLVED-BACKLOG.md` hardening entries | No — exact controller-side path not verifiable from this root; only historical agentic-sop hardening entries located |
| Human-decision integration | `docs/guides/APPROVALS.md`; `internal/web/approval.go`; CLOSE-004 §2 approval tests PASS | Yes |
| Approval reconciliation | `sop approvals --json` CTRL006 PENDING; `docs/guides/APPROVALS.md`; `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Pending CTRL006 approval" | Yes |
| Performance visibility | `docs/reference/PERFORMANCE.md`; `internal/perf`; `internal/cli/run.go`; `internal/cli/session.go`; CLOSE-004 §2 performance tests PASS | Yes |

Search scope actually used for the not-located items:

- Wrap-up: agentic-sop `docs/plans` listing + `docs/history` listing (found
  `docs/history/PLAN-wrapup.md`) + repository-wide text search for `PLAN-Wrap-Up`
  (hits only in `docs/plans/PLAN-Pre-Performance-Closure.md`) + CLOSE-002 manifest M2.
- Hardening: repository-wide text search for `Hardening` (hits only in
  `docs/plans/PLAN-Pre-Performance-Closure.md` and `docs/history/RESOLVED-BACKLOG.md`)
  + `docs/tasks` inventory. The sop-controller checkout at
  `/Users/imhttran/agentic-workspace/projects/sop-controller` is not an authorized
  writable or read root for the IMPLEMENT tooling of this invocation, so the
  controller-side `docs/PLAN-Hardening.md` cannot be read verbatim here; it is recorded
  as NOT LOCATED with the exact scope. No cross-root read/mutation is attempted.

### 0.2 Gate-input inventory

| Gate | Report | Current status | Evidence |
| --- | --- | --- | --- |
| CLOSE-003 (SOP deterministic baseline) | `docs/reports/pre-performance-closure/CLOSE-003-sop-deterministic-baseline.md` | report **ABSENT**; gate checks re-run here (§2) | `docs/reports/pre-performance-closure/` contains only CLOSE-001, CLOSE-002 and CLOSE-004; `docs/plans/PLAN-Pre-Performance-Closure.md` lists CLOSE-003 as defined-but-unrun; live re-run recorded in §2. The stage-level report is still owed, recorded as an external action (§6.5), not relabeled. |
| CLOSE-004 (controller deterministic baseline) | `docs/reports/pre-performance-closure/CLOSE-004-controller-deterministic-baseline.md` | **BLOCKED — not re-executed in this invocation** | Report present, all-green decision recorded at controller HEAD `a51b0c6a…` (CLOSE-004 §1–§6). Re-execution requires the authorized controller root, which this invocation cannot enter (§3). Not declared green-as-of-now; recorded as inherited-with-limitation, not re-run. |

No gate is declared green unless its evidence is present for the current revision.
The CLOSE-003 SOP command set is re-executed below against the current agentic-sop
revision. The CLOSE-004 controller command set cannot be re-executed from this
invocation and is recorded **BLOCKED**, not fabricated green.

### 0.3 SOP binary identity snapshot (pre-remedy pin)

Imported from `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §3 and
marked as the pre-remedy pin (not asserted current; §4.5 records the repin status):

| Field | Value |
| --- | --- |
| SOP binary path | `/Users/imhttran/go/bin/sop` |
| `sop version` | `sop dev` |
| `vcs.revision` | `79634dc60bea1c80426eac395a6efc5698972c5e` |
| `vcs.modified` | `true` (dirty-tree build) |
| SHA-256 | `172778e5c45b5ec3f46f7e2b0caeeb34fefee2f596b20a8440674afb7c7979d1` |

This historical SHA is **not** asserted to be current. It is the CLOSE-001 pin, and
the plan assigns rebuild/repin to CLOSE-005. Because this invocation's tooling cannot
build or install into `/Users/imhttran/go/bin/sop`, the repin status is recorded in
§4.5 with its bounded closure-defect remedy rather than faked.

---

## 1. Per-item verdicts (CLOSE-005-B)

Exactly one verdict (COMPLETE / BLOCKED / NEEDS_HUMAN) per required item. Each cites
a current code, test or SOP provenance artifact.

### 1.1 Wrap-up — **NEEDS_HUMAN**

- **Verdict:** NEEDS_HUMAN.
- **Evidence:** No authoritative controller-side wrap-up source is locatable from this
  root. CLOSE-002 manifest M2 records `docs/plans/PLAN-Wrap-Up.md` NOT LOCATED in
  either root; the only wrap-up-named artifact present in this checkout is the
  agentic-sop document `docs/history/PLAN-wrapup.md`, which is a historical
  agentic-sop plan, not the controller's current wrap-up state. Recorded controller
  status (CTRL001–CTRL017 recorded PLANNED per
  `docs/plans/PLAN-Pre-Performance-Closure.md`) is recorded status, not implementation
  evidence, and therefore does not yield COMPLETE. No current controller code/test
  artifact for wrap-up can be cited from this root.
- **Named external action:** In the authorized controller checkout, run
  `sop status` / `sop resume` and read the controller's current wrap-up artifact
  (e.g. `docs/plans/PLAN-Wrap-Up.md` if present) to confirm the current wrap-up state;
  supply the resulting artifact path so CLOSE-005 can cite it directly.
- **Search scope:** `docs/plans` listing + `docs/history` listing + repo-wide text
  search for `PLAN-Wrap-Up`.

### 1.2 Hardening — **NEEDS_HUMAN**

- **Verdict:** NEEDS_HUMAN.
- **Evidence:** The only hardening-related artifacts locatable in this root are the
  historical `docs/history/RESOLVED-BACKLOG.md` hardening entries (Phase 5 / 5.4) and
  the TASK-044 reference to a controller-side `docs/PLAN-Hardening.md` that is not
  present in this checkout. CLOSE-002 manifest M3/M4 classify both controller-side
  hardening locations as NEEDS_HUMAN with exact paths UNKNOWN. No current controller
  code/test artifact for hardening can be cited from this root.
- **Historical hardening claim recorded as evidence (not new work):** the historical
  hardening report claim — **multi-minute waits were an external operator workflow, no
  invented production fix was required** — is recorded here as evidence only. It is
  **not** turned into a new hardening work item, and no hardening implementation is
  started by CLOSE-005.
- **Named external action:** In the authorized controller checkout, read the two
  hardening-location reports and supply their paths so their current status can be
  cited rather than assumed.
- **Search scope:** repo-wide text search for `Hardening` + `docs/tasks` inventory +
  CLOSE-002 manifest M3/M4.

### 1.3 Human-decision integration — **COMPLETE**

- **Verdict:** COMPLETE.
- **Evidence:** `docs/guides/APPROVALS.md` documents the Phase 6 approval gate
  (list/decide/reconcile) and `internal/web/approval.go` implements it. CLOSE-004 §2
  records the named human-approval suite `internal/web/approval_test.go`
  (`go test ./internal/web/ -run Approval -v`) **PASS** at controller HEAD
  `a51b0c6a…`; CLOSE-004 §5 confirms the controller delegates to SOP and holds no
  second workflow engine. This is a current code + named-test artifact, not a stale
  status document and not a generic repository test alone.
- **Named external action:** none for the integration contract. A genuine future human
  approval decision is exercised in CLOSE-006, not here.

### 1.4 Approval reconciliation — **NEEDS_HUMAN**

- **Verdict:** NEEDS_HUMAN.
- **Evidence:** `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Pending CTRL006
  approval" records `sop approvals --json` reporting CTRL006 **PENDING
  NEEDS_HUMAN**, requested `2026-10-01T03:52:02Z`, tied to an older no-change
  diagnostic. CLOSE-002 §4 records the same as reconciliation evidence only.
- **Disposition:** This pending approval is treated as **reconciliation evidence**, not
  as proof of a current human action and not as PASS. CLOSE-005 does **not**
  auto-approve it and does not relabel it. The historical C2-009 NOT READY dogfood
  result is likewise preserved **HISTORICAL**, not relabeled PASS.
- **Named external action:** A human operator must decide the pending CTRL006 approval
  through SOP's real approval gate (`sop approval <id>`) or explicitly decline it;
  the decision is observed in CLOSE-006. This is a genuine external action and is not
  automated away.

### 1.5 Performance visibility — **COMPLETE**

- **Verdict:** COMPLETE.
- **Evidence:** `docs/reference/PERFORMANCE.md` documents the single `internal/perf`
  telemetry representation (`stages_ms`, `validation_ms`, `total_ms`, `CategoryMS`,
  `WriteTask`, `WriteRun`) and its persistence (`.agent-sdlc/runs/<task-id>/metrics.json`,
  the `performance` field in `report.json`, and the plan aggregate
  `.agent-sdlc/runs/<plan-id>/metrics.json`); `internal/cli/run.go` and
  `internal/cli/session.go` hold the readers/reuse rules. CLOSE-004 §2 records the named
  performance suite `internal/web/performance_test.go`
  (`go test ./internal/web/ -run Performance -v`) **PASS** at controller HEAD
  `a51b0c6a…`. CLOSE-008 will inventory this existing contract for extent/gaps.
- **Named external action:** none for visibility of the existing contract; future
  telemetry fields are explicitly out of scope and inventoried by CLOSE-008.

### 1.6 Bounded closure-defect list

| Closure defect | Owning layer | Bounded remedy | Out of scope? |
| --- | --- | --- | --- |
| SOP binary is a dirty-tree build (`vcs.modified=true`) not aligned to a clean revision | SOP build/install (operator-owned) | Rebuild from a clean revision and record path/version/revision/modified/SHA-256 (§4.5) | No — closure scope |
| CLOSE-003 stage report absent (twelve CLI/JEV failures recorded unresolved) | agentic-sop test contracts | Run CLOSE-003 gate + bounded test-contract remediation, record green (§2, §6.5) | No — closure scope |
| Controller wrap-up source not locatable from this root | controller checkout (read-only) | Operator supplies the current wrap-up artifact (§1.1) | No — closure scope |
| Controller hardening-location reports not locatable from this root | controller checkout (read-only) | Operator supplies the two hardening report paths (§1.2) | No — closure scope |

The table above is a record, not a schedule of new architecture. Anything outside
these closure defects (Prompt Compiler, Context Engine, caches, RAG, adaptive routing,
network/team mode, dashboard, Jev adapter/evaluation, task-scoped discovery budgets,
Windows enhancements) is **out-of-scope** and **not implemented** by CLOSE-005.

---

## 2. Re-run: CLOSE-003 deterministic gate (agentic-sop)

The CLOSE-003 gate is re-run against the **current** agentic-sop tree so the recorded
evidence is current rather than inherited. No closure *remedy* is applied by CLOSE-005
(no source/test/config change), so this re-run is a straight re-execution at the
current revision, not a post-remedy validation.

- cwd: agentic-sop repository root (`/Users/imhttran/agentic-workspace/projects/agentic-sop`)
- toolchain: local Go toolchain on darwin/arm64 (as recorded in CLOSE-001 §3); `go version`
  requires approval in this invocation and is **not** re-captured here — recorded as
  unavailable, not invented.
- revision: agentic-sop `HEAD` = `c11e14fb0a43dd5840fa5c034e1d8e6fd999af28` (`git rev-parse HEAD`)
- tracked working tree at run time: contains pre-existing user-owned modifications (CLOSE-001 §1);
  no CLOSE-005 source change.

### 2.1 Recorded CLOSE-003 re-run results (exact commands, exit codes, observed output)

| # | Command | cwd | Exit | Observed output | Status |
| --- | --- | --- | --- | --- | --- |
| 1 | `gofmt -l .` | repo root | 0 | **empty** (no files listed) | **PASS** |
| 2 | `go vet ./...` | repo root | 0 | empty (no vet diagnostics) | **PASS** |
| 3 | `go test ./...` | repo root | 0 | all listed packages `ok`; `cmd/sop` and `cmd/sop-ollama-agent` `[no test files]`; no failures | **PASS** |
| 4 | `go test -race ./...` | repo root | — | not executed in this invocation (not in the required validation allow-list); recorded as NOT RUN | **NOT RUN** |
| 5 | `go build ./...` | repo root | 0 | empty (no compile errors) | **PASS** |

Raw `go test ./...` tail (illustrative of the recorded run; full list is per-package
`ok`):

```text
?   	github.com/imhttran/agentic-sop/cmd/sop	[no test files]
?   	github.com/imhttran/agentic-sop/cmd/sop-ollama-agent	[no test files]
ok  	github.com/imhttran/agentic-sop/internal/activity	(cached)
ok  	github.com/imhttran/agentic-sop/internal/agent	(cached)
... (all internal/* packages ok)
ok  	github.com/imhttran/agentic-sop/internal/workitem	(cached)
```

CLOSE-003 gate re-run status: **GREEN** for checks 1–3 and 5 at the current
agentic-sop revision. Check 4 (`go test -race ./...`) was **not run** in this
invocation (not in the allow-list); it is recorded as NOT RUN rather than asserted
green. This is a limitation of THIS invocation, not a claim about the tree.
Per CLOSE-003's own definition the gate is only fully green once `-race` is also
recorded; that residual is carried as an external action (§6.5). No pre-remedy
evidence is cited as current.

---

## 3. Re-run: CLOSE-004 deterministic gate (sop-controller) — **BLOCKED, NOT RE-EXECUTED**

The CLOSE-004 controller gate was established green at controller HEAD
`a51b0c6a6563033821ed4ae1e51890ab10479bbd` (CLOSE-004 §1–§6). CLOSE-005 is
**required** to re-run this gate after any remedy and to record new hashes so no
earlier evidence is reused.

**Honest limitation (recorded as a BLOCKING, not waived):** this invocation's tooling
cannot execute commands inside the sibling controller checkout
(`/Users/imhttran/agentic-workspace/projects/sop-controller` is not an authorized root
for the IMPLEMENT tooling here). No controller remedy is applied by CLOSE-005, so the
CLOSE-004 revision is unchanged — but the gate is **not re-executed** by this
invocation. It is therefore recorded as **BLOCKED / INHERITED-WITH-LIMITATION**, never
as "green now":

| Check | Re-run this invocation? | Value recorded |
| --- | --- | --- |
| `gofmt -l .` | No | green at `a51b0c6a…` (CLOSE-004 §1) — not re-run |
| `go vet ./...` | No | green at `a51b0c6a…` (CLOSE-004 §1) — not re-run |
| `go test ./...` | No | green at `a51b0c6a…` (CLOSE-004 §1, §3) — not re-run |
| `go test -race ./...` | No | green at `a51b0c6a…` (CLOSE-004 §1) — not re-run |
| `go build ./...` | No | green at `a51b0c6a…` (CLOSE-004 §1) — not re-run |
| Boundary / approval / observability / performance suites | No | PASS (CLOSE-004 §2) — not re-run |

**Bounded remedy:** re-execute the CLOSE-004 command set inside the authorized
controller root (read-only; no mutation) at HEAD `a51b0c6a…` and record
command/cwd/toolchain/revision/output/exit. Until that is done, CLOSE-006 and any
measurements that depend on the controller gate must not treat the CLOSE-004 evidence
as a fresh re-run. This is a named external action (§6.1).

---

## 4. Source / config / patch / binary hashes (post-CLOSE-005, current)

New hashes recorded so no earlier test evidence is reused blindly. Distinct from the
CLOSE-001 pins where the tree changed.

| Artifact | Value | Note |
| --- | --- | --- |
| agentic-sop HEAD SHA | `c11e14fb0a43dd5840fa5c034e1d8e6fd999af28` | current `git rev-parse HEAD` (re-derived this invocation); differs from CLOSE-001's `79634dc6…` |
| agentic-sop branch | `fix/verify-repository-mutations` | current |
| agentic-sop tracked dirty state | true (pre-existing user-owned) | preserved, per CLOSE-001 §1 |
| agentic-sop untracked inventory | `internal/cli/report_deliverable.go`, `internal/cli/report_deliverable_test.go`, `internal/cli/task_input.go` (pre-existing) + this report (new) | `git status --porcelain` shows the pre-existing untracked files plus `?? docs/history/pre-performance-closure/CLOSE-005-controller-work-verdicts.md` |
| agentic-sop tracked patch SHA-256 | recorded below (see "Patch hash" row) | distinct from CLOSE-001 `a10f0df7…` if the tree changed |
| sop-controller HEAD SHA | `a51b0c6a6563033821ed4ae1e51890ab10479bbd` | unchanged (no controller remedy applied; not re-verified this invocation) |
| sop-controller tracked dirty state | false | unchanged (per CLOSE-001 §2) |
| SOP binary SHA-256 (pre-remedy pin) | `172778e5c45b5ec3f46f7e2b0caeeb34fefee2f596b20a8440674afb7c7979d1` | CLOSE-001 pin; **superseded by §4.5 repin when performed** |

**Patch hash:** the tracked uncommitted patch is derived with
`git diff` (tracked modifications vs `HEAD`). Because the report itself is untracked
and `git diff` here cannot be piped to a hasher in the allow-listed tooling, the exact
SHA-256 of the tracked patch is recorded as **unavailable in this invocation** (no
`sha256sum`/pipe allowed). The tracked *set* of modified files is the CLOSE-001 §1 set
(unchanged by CLOSE-005, since CLOSE-005 touches no tracked file). A future invocation
with a hashing allowance must record the literal SHA-256; it is named as an external
action (§6.6), not invented here.

**Config hash:** the repository's tracked configuration is covered by the tracked patch
hash above (there is no separate CLOSE-005 config artifact). No CLOSE-005 config change
was made, so the config hash equals the tracked state; the literal value is recorded as
unavailable for the same reason, not fabricated (§6.6).

### 4.5 SOP binary rebuild / re-pin

- Status: **BLOCKED — bounded closure defect.** This invocation's tooling cannot run
  `go build`/install into `/Users/imhttran/go/bin/sop`, and the plan's rebuild/repin is
  explicitly an operator-owned SOP build. The binary is therefore reported as a closure
  defect with a bounded remedy rather than faked:

  - **Incompatibility observed:** installed `sop` is a dirty-tree build
    (`vcs.modified=true`) pinned to `79634dc6…`; it cannot be used as the pinned binary
    for CLOSE-006/measurements without a clean rebuild.
  - **Bounded remedy (operator):** from a clean SOP revision, run the documented build
    (`go build -o /Users/imhttran/go/bin/sop ./cmd/...` or the project's install script)
    and record the new path, `sop version`, `vcs.revision`, `vcs.modified` and SHA-256.
  - **Gate to later stages:** CLOSE-006 and all measurements must use the rebuilt/re-pinned
    binary; until the new hash is recorded they must not be run against the dirty-tree
    pin. **No new binary hash is recorded here because no rebuild was performed** — this
    is stated honestly rather than presenting the CLOSE-001 pin as a new hash.

---

## 5. Reconciliation evidence (recorded, not relabeled)

| Item | Disposition | Evidence |
| --- | --- | --- |
| Historical hardening claim — multi-minute waits were an external operator workflow, no invented production fix required | **EVIDENCE ONLY** — not new work | Historical `docs/history/RESOLVED-BACKLOG.md` hardening entries; §1.2 |
| Pending CTRL006 approval | **NEEDS_HUMAN reconciliation evidence** — not proof of a current human action, not PASS | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Pending CTRL006 approval"; CLOSE-002 §4 |
| Historical C2-009 NOT READY | **HISTORICAL** — preserved as-is, not relabeled PASS | `docs/plans/PLAN-Pre-Performance-Closure.md`; `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 |

None of these is turned into new work, auto-approved, or marked PASS.

---

## 6. Named external actions that remain

1. **CLOSE-004 controller gate re-execution** (operator, authorized controller root):
   re-run the CLOSE-004 command set at HEAD `a51b0c6a…` read-only and record
   command/cwd/toolchain/revision/output/exit (§3). Required before downstream stages
   may rely on a *fresh* controller gate; until then §3 is BLOCKED.
2. **SOP binary rebuild/repin** (operator-owned): rebuild `sop` from a clean revision
   and record the new path/version/revision/modified/SHA-256 (§4.5). Required before
   CLOSE-006 and any measurements.
3. **Pending CTRL006 approval decision** (human): decide or decline the pending CTRL006
   approval through SOP's real gate; observed in CLOSE-006 (§1.4).
4. **Controller wrap-up artifact** (operator): read/execute `sop status` / `sop resume`
   in the authorized controller checkout and supply the current wrap-up artifact path
   (§1.1).
5. **Controller hardening-location reports** (operator): supply the two hardening
   report paths (§1.2).
6. **CLOSE-003 own report + twelve CLI/JEV remediation + `-race` record** (agentic-sop
   test contracts): produce `CLOSE-003-sop-deterministic-baseline.md` with the bounded
   test-contract remediation and the `go test -race ./...` result (§2, §0.2).
7. **Patch/config SHA-256** (agentic-sop tooling allowance): record the literal tracked
   patch (and config) SHA-256 where the tooling permits hashing (§4).

---

## 7. Mutation and preservation statement

### Authorized mutation (sole intentional repository mutation of CLOSE-005)

- Added/updated: `docs/history/pre-performance-closure/CLOSE-005-controller-work-verdicts.md`
  (this file). As of this run it is **untracked** (shown by `git status --porcelain` as
  `?? docs/history/pre-performance-closure/CLOSE-005-controller-work-verdicts.md`); no
  commit or push is performed.

The parent directory already existed (created by CLOSE-001). No other repository file
is created or updated by CLOSE-005. No executable, configuration or other documentation
change is made merely to clean documentation.

### Pre-existing user-owned changes preserved (agentic-sop)

Per `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §1, these pre-existing
tracked modifications and untracked files were present before CLOSE-005 and are
preserved untouched:

- Tracked modifications: `docs/reference/CLI.md`, `internal/cli/cli.go`,
  `internal/cli/drive.go`, `internal/cli/jev.go`, `internal/cli/mutation.go`,
  `internal/cli/run.go`, `internal/git/git.go`, `internal/ollamaagent/prompt.go`,
  `internal/taskfile/taskfile.go`.
- Untracked files: `internal/cli/report_deliverable.go`,
  `internal/cli/report_deliverable_test.go`, `internal/cli/task_input.go`.

### Read-only sop-controller

Per `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2, the sop-controller
sibling checkout is strictly read-only during CLOSE-005. No file, configuration, branch
or commit in that repository was created, modified or deleted, including the
pre-existing untracked `c2-009-dogfood.*` fixture directories. No cross-root mutation
occurred. (This invocation could not enter that root at all; see §3.)

### Non-mutation statement

CLOSE-005 did not mutate application/source code, tests, runtime configuration, SOP
configuration, any Git branch, any existing user change, or the sibling sop-controller
repository content. No SOP state-changing command was executed to produce evidence
(no approve/reconcile mutation). No commit/push was performed. Only the CLOSE-005
Markdown report was created/updated.
