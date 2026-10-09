# AUTONOMY — independent verification and plan reconciliation

**Author:** operator-side verification (not a governed SOP run).
**Scope:** verify the governed AUTONOMY deliverables against their exact acceptance criteria,
and reconcile the autonomy plan with the verified implementation. No governed deliverable is
rewritten; all discrepancies are recorded here.

---

## Part A — Independent verification of the governed deliverables

The governed run produced `AUTONOMY-001 LOCAL_DONE`, `AUTONOMY-004 LOCAL_DONE`,
`AUTONOMY-005 LOCAL_DONE`, final gate `PASS`. A gate pass is **not** proof of acceptance
(see the E2E assessment, KI-01), so each criterion is re-verified below.

### AUTONOMY-001 — `docs/reports/autonomy/AUTONOMY-001-baseline.md`

| Acceptance criterion | Verdict | Notes |
|---|---|---|
| Autonomous path identified by code reference, not asserted | **MET (with defects)** | Most references are exact (`cmd/sop/main.go`→`cli.Run` (cli.go:203); `runRun`/`runSingleTask`/`executeLifecycle`/`runStages`/`emitRunSummary` in `internal/cli/run.go`; `sop commit` at cli.go:251). **Four references are wrong** — see below. |
| Single prompt performs a multi-package unused-code cleanup, with stage/gate/fix-cycles/workspace recorded | **MET** | Matches the operator-run evidence in `autonomy-001-scenario/README.md`. |
| Every gap listed VERIFIED / PARTIAL / MISSING with evidence | **MET** | 14 rows (G1–G14); G11 MISSING, G13/G14 PARTIAL, G12 informational. |
| High-risk commit action behind human approval | **MET** | Run printed the no-commit notice; change left uncommitted; `human.approval_before_commit: true`. |
| No new orchestration framework | **MET** | Only existing packages named; no new engine. |

**Code-reference defects (baseline §1b/§1d) — independently verified:**

| Cited in baseline | Reality | Type |
|---|---|---|
| `runGraph` in `internal/cli/run.go` | `internal/cli/drive.go:46` | wrong file |
| `runAttempts` in `internal/cli/run.go` | `internal/cli/escalation.go:76` | wrong file (the readiness report cites this correctly) |
| `internal/commitgate/commitgate.go` | **does not exist**; actual `internal/commitgate/gate.go` | nonexistent path |
| `internal/handoff/handoff.go` | **does not exist**; actual `capsule.go`/`manager.go`/`record.go`/… | nonexistent path |
| fixture `Join(parts []string, sep string)`, `unusedConst = 42`, `type unusedType struct{}` | actual fixture: `Join(a, b string)`, `unusedConst = 99`, `type unusedType struct{ x int }` | paraphrased/hallucinated detail |

None of these change the substantive verdict, but they are factual defects a reviewer must not
trust blindly — the exact failure mode the request's "PASS is not proof of acceptance" warning
anticipates.

### AUTONOMY-004 — determination recorded in `AUTONOMY-001-baseline.md` §6

| Acceptance criterion | Verdict | Notes |
|---|---|---|
| Determination recorded with evidence | **MET** | **NOT_REQUIRED**, with code-reference evidence (§6a) that `sop prompt --capability implement` → `runPromptImplement` → `runAttempts` already produces and executes from one prompt. |
| No new scheduler/recovery/approval/routing logic | **MET** | `changed-files.json` = only `AUTONOMY-001-baseline.md` (§6c). |
| Existing autonomy/approval boundaries unchanged | **MET** | No code file touched. |

### AUTONOMY-005 — `docs/reports/autonomy/AUTONOMY-005-readiness.md`

| Acceptance criterion | Verdict | Notes |
|---|---|---|
| GO/HOLD verdict backed by evidence with VERIFIED/PARTIAL/MISSING/BLOCKED listed | **MET** | **HOLD**; G11 MISSING (blocking-class), G13/G14 PARTIAL, G12 informational. |
| No new framework / no acceptance-enforcement change claimed | **MET** | Explicitly asserted and accurate. |
| No plan activated/completed/historicalized by this task | **MET** | The task changed only its report. (Note: the *plan* was activated by the operator before the run, and remains ACTIVE; the report's phrase "PLAN-SOP-Autonomy.md remains PROPOSED" refers to the doc's status line, not SOP state — a minor wording imprecision, not a violation.) |

**Overall:** all three tasks are **acceptance-MET**; AUTONOMY-001 carries four code-reference
defects plus two fixture-detail defects. Recorded, not corrected (SOP history preserved).

---

## Part B — Reconciliation of `docs/plans/PLAN-SOP-AUTONOMY-001.md` (operator plan)

A second, broader plan exists untracked in the **main** worktree:
`docs/plans/PLAN-SOP-AUTONOMY-001.md` — "SOP Autonomous Outcome Execution", milestone
AUTONOMY-001, workstreams A–E, marked *"PROPOSED — requires repository reconciliation before
activation."* Reconciled here read-only against the verified implementation:

| Workstream | Reconciliation vs. verified implementation | Disposition |
|---|---|---|
| **001A** Capability map & minimum design | Delivered by `AUTONOMY-001-baseline.md` §1 (code-referenced). "Smallest change for a bounded autonomous mode" → **NOT_REQUIRED**: `sop prompt --capability implement "<outcome>"` already runs discover→plan→implement→validate→review→gate→fix→report with no intermediate prompts. | Duplicate of delivered baseline; keep as evidence |
| **001B** Outcome execution loop | Exists: `runPromptImplement` → `runAttempts` (one prompt → governed lifecycle); `sop run` graph for multi-task plans. | **Duplicate** — remove or mark VERIFIED |
| **001C** Recovery & evidence | Exists: `internal/autonomy` policy (no-progress/transient/test/blocked classification), bounded retry/continue/fix, `internal/continuation`, `internal/recovery`, alternate model class via routing/fallback. **One genuine gap:** acceptance is not gate-enforced on `main` (KI-01) — owned by unmerged `harden-001`. | **Duplicate** except the acceptance-enforcement gap |
| **001D** Developer-facing UX | Mostly exists: one documented invocation (`sop prompt --capability implement`), one final report, commit boundary. Possible genuine gaps: **explicit authorization-scope** and **dry-run/preview** affordances are not evident. | **PARTIAL** |
| **001E** End-to-end acceptance scenario | The multi-package cleanup scenario (operator-run, this worktree) demonstrates the core. The operator plan's richer scenario — verify candidates against production imports, plans, docs, runtime entry points, migrations, compatibility surfaces before removal — is **not** demonstrated or automated. | **PARTIAL / gap** (links to G11) |

**Genuine gaps retained after reconciliation:** (1) automated in-repo end-to-end scenario (G11);
(2) acceptance not gate-enforced on `main` (KI-01 / `harden-001`); (3) explicit authorization
scope + dry-run for a bounded autonomous mode (001D); (4) richer compatibility-checked cleanup
scenario (001E).

**Recommendation:** consolidate the two autonomy plans — the operator's `PLAN-SOP-AUTONOMY-001.md`
is the broader spec; the reconciled `PLAN-SOP-Autonomy.md` is a verified subset already executed.
Do **not** activate `PLAN-SOP-AUTONOMY-001.md` without reconciling its A–E workstreams to the
above (most are already satisfied).

---

## Part C — Boundary preservation and validation

- **Isolation:** all work confined to `sop-autonomy-001` / `feature/autonomy-001`. Other worktrees
  unchanged: `main` (user-owned changes intact + operator's untracked plans), `sop-harden-001`
  (`1ed7189`, unmerged, untouched), `sop-preview-001` (untouched).
- **Approvals:** `human.approval_before_commit: true`; every governed run stopped at the commit
  boundary and did not commit; `sop approvals` → "no pending approvals".
- **SOP state:** `plan-sop-autonomy` remains **ACTIVE** (not completed/historicalized); the ETOE
  assessment and preview plans in their own worktrees are untouched.
- **`harden-001`:** not merged.
- **Validation:** `gofmt -l .` clean · `go vet ./...` OK · `go build ./...` OK ·
  `git diff --check` clean; the governed runs each ran `go build ./... && go test ./... && go vet ./...`
  and passed. No Go source changed (deliverables are Markdown).
