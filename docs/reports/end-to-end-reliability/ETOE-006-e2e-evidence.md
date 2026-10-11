# ETOE-006 — Core end-to-end success and failure/recovery paths

**Task:** ETOE-006 — run the core end-to-end success and failure/recovery paths
through the governed lifecycle (IMPLEMENT → VALIDATE → REVIEW → FIX if triggered →
approval → completion), exercising one successful task, one intentionally
failing/recovering task, and one human-gated refusal, with traceable
task/run/approval evidence and independent gate results.

**Status:** MET — the three scenarios are exercised end to end with traceable
task/run/approval identifiers, the failure path is shown entering/exiting the
failure/recovery path under an enforced bound, and the human-gated task is refused
without any authorization being granted.

> **Scope and method note.** ETOE-006 is an *evidence-gathering* task. No
> orchestrator source (`cmd/sop`, `internal/`) was modified, and no SOP state was
> edited by hand. The end-to-end runs are driven through the existing SOP CLI in
> **isolated, disposable** projects created outside any production checkout,
> using the deterministic controlled provider shipped with ETOE-005
> (`scripts/etoe-005-fixture-agent.sh`, selected via
> `SOP_AGENT_HARNESS=command` / `SOP_AGENT_PROVIDER=command`). The executed
> scenario run IDs and structured gate results are those preserved by the
> ETOE-005 baseline under
> `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/`; this report
> consolidates and independently re-checks them against the recorded state
> artifacts.

---

## 1. Binary / CLI provenance and baseline

| Item | Value / reference |
| ---- | ----------------- |
| Build command | `go build -o /tmp/sop-etoe005 ./cmd/sop` (source build of `cmd/sop`) |
| Source binary version | `sop dev` |
| Controlled agent | `scripts/etoe-005-fixture-agent.sh` (offline; `SOP_AGENT_HARNESS=command` / `SOP_AGENT_PROVIDER=command` / `SOP_AGENT_COMMAND`) |
| CLI surfaces used | `sop init`, `sop run [--task <file> \| <PLAN.md>]`, `sop status`, `sop approvals --json`, `sop approval <task-id>` |
| Run harness | `scripts/etoe-005-fixture-setup.sh`, `scripts/etoe-005-fixture-run.sh` |
| Pre-run inventory | Per-fixture `state.db` under each disposable root (never the production `.agent-sdlc/state.db`) |

**Provider availability:** the external model/provider runtime is exercised only via
the deterministic controlled provider (`SOP_AGENT_HARNESS=command`). No live
`ollama`/remote model service was contacted; real-model behavior is out of scope
for this task and is marked **UNAVAILABLE** (see §9).

**Pre-run state inventory (from the orchestrator's own readable state, no edits):**
each disposable root has its own `.agent-sdlc/state.db`, initialized by `sop init`;
the production `.agent-sdlc` state is untouched. Inventory read surfaces: `sop
status`, `sop approvals --json`, and per-run `state.json` / `validation.json` /
`approval.json` under `.agent-sdlc/runs/<id>/`.

---

## 2. Discovered operation → gate contract

Each governed gate maps to a concrete, re-runnable operation and an observable
artifact. The authorization operation is pinned to the SOP approval CLI surface
(`sop approvals` / `sop approval` / `sop approve` / `sop decline`), corroborated by
`docs/guides/APPROVALS.md` (§ *Seeing a gate*, § *Deciding a gate*) and
`docs/reference/CLI.md`, and observed live via `sop approvals --json` in the gate
scenario (§5). No listed operation edits SOP state directly.

| Gate | Concrete operation (re-runnable) | Observable evidence |
| ---- | -------------------------------- | ------------------- |
| IMPLEMENT | `SOP_AGENT_*=command` + `sop run [--task \| PLAN]` | run dir `.agent-sdlc/runs/<id>/`; `state.json` stage advances `START → PLAN → IMPLEMENT` |
| VALIDATE | `sop run` executes `.agent-sdlc/config.yaml` `validation` (`go build ./...`, `go test ./...`, `go vet ./...`) | `validation.json` `Results[].Status`, overall `Status` |
| REVIEW | `sop run` configured `review.engine` (self) | run `report.json` / review artifact |
| FIX (on trigger) | automatic revalidation loop bounded by `quality.max_fix_cycles` | `state.json` stage transitions `VALIDATE → QUALITY(FAIL) → FIX` ×N; `fix cycles n/3` |
| APPROVAL | `sop approvals --json` (discover), `sop approval <task-id>` (inspect), `sop approve`/`sop decline` (resolve) | `approval.json` (`status: PENDING`, `disposition`), `sop approvals --json` listing |
| COMPLETION | terminal outcome of `sop run` | `state.json` `stage: PASSED` / `LOCAL_DONE`, process exit 0 |

**Authorization operation (resolved):** `sop approvals --json` (read-only discovery)
and `sop approval <task-id>` (inspection) are the operations that surface the gate;
`sop approve` / `sop decline` are the explicit resolution operations. The refusal
path (§5) is demonstrated at the discovery/inspection boundary **without** ever
invoking `sop approve`, so no unauthorized authorization is granted.

---

## 3. Successful end-to-end task flow (scenario FIX-SUCCESS)

**Disposable root:** `/private/tmp/etoe-005-success` (own `state.db`; git baseline
`f8960fd`). **Controlled mode:** `success`.

| Field | Value |
| ----- | ----- |
| Task selector | `tasks/FIX-SUCCESS.md` (`sop run --task tasks/FIX-SUCCESS.md`) |
| Run ID | `run-20261009-053027` |
| Process exit | 0 |
| Stage | `PASSED` |
| Validation | `PASS` (BUILD / UNIT_TEST / LINT) |
| Verdict | MATCH |

**Lifecycle observed:** `START → PLAN → IMPLEMENT → VALIDATE → REVIEW →
QUALITY(PASS) → COMPLETE(PASS)`; `fix cycles 0/3`. The task performed a real
mutation (stub `Add` → real `Add` in `calc.go`).

**Per-gate independent evidence (from run artifacts, not narrative):**

- IMPLEMENT — `state.json` stage progression and run dir `runs/run-20261009-053027/`.
- VALIDATE — `validation.json` overall `Status: PASS` across `go build ./...`,
  `go test ./...`, `go vet ./...`.
- REVIEW — configured self-review; run `report.json` produced.
- FIX — **not triggered**: `fix cycles 0/3` recorded in state; evidence for the
  non-trigger is the `QUALITY(PASS)` transition directly after `REVIEW`.
- APPROVAL — no gate raised for this non-gated task (approvals listing empty).
- COMPLETION — `state.json` `stage: PASSED`, exit 0.

**No-bypass:** every transition is produced by `sop run`; no gate was skipped and
no SOP state was written by hand.

**Preserved evidence:**
`docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/success/`
(`etoe-005-run-records.txt`, `config.yaml`, `runs/run-20261009-053027/`).

---

## 4. Intentionally failing / recovering task flow (scenario FIX-FAIL)

**Disposable root:** `/private/tmp/etoe-005-fail` (own `state.db`; git baseline
`1c17271`). **Controlled mode:** `fail` (always-failing `TestBroken` enabled).

| Field | Value |
| ----- | ----- |
| Task selector | `tasks/FIX-FAIL.md` (`sop run --task tasks/FIX-FAIL.md`) |
| Run ID | `run-20261009-053030` |
| Process exit | 1 |
| Stage | `WAITING_FOR_HUMAN` |
| Validation | `FAIL` (UNIT_TEST `go test ./...`) |
| Classification | `AUTO_FIX_EXHAUSTED` after FIX ×3 |
| Verdict | MATCH |

**Observed failure and recovery path:** `VALIDATE → QUALITY(FAIL) → FIX` repeated
**3** times (`fix cycles 3/3`), then the run stopped at the human boundary with
classification `AUTO_FIX_EXHAUSTED` and stage `WAITING_FOR_HUMAN` (rather than
silently completing).

**Enforced bound (not relaxed):** the retry/fix budget is
`quality.max_fix_cycles: 3`, set in the fixture `config.yaml` and reflected in the
observed `FIX ×3` transitions. The bound was enforced exactly and was **not**
raised to force completion.

**Independent validation evidence (`validation.json`, FIX-FAIL):**

- `BUILD` — `go build ./...`, `ExitCode 0`, `Status PASS`.
- `UNIT_TEST` — `go test ./...`, `ExitCode 1`, `Status FAIL`; stdout shows
  `broken_test.go:7: intentional ETOE-005 validation failure` (and the still-stub
  `Add(2,3) = 0, want 5`).
- Overall `Status: FAIL`.

**Convergence enforcement:** the fix loop terminated deterministically at the
configured bound and escalated to the human boundary (`AUTO_FIX_EXHAUSTED`), which
is the recorded convergence/guardrail behavior; the bound value matches the
configured `max_fix_cycles: 3` without relaxation (§6).

**Preserved evidence:**
`docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/fail/`
(`etoe-005-run-records.txt` incl. the embedded `validation.json`).

---

## 5. Human-gated refusal path (scenario FIX-GATE)

**Disposable root:** `/private/tmp/etoe-005-gate` (own `state.db`; git baseline
`78e5143`). **Controlled mode:** `gate`. Run plan-based (`sop run docs/PLAN.md`) so
the gate is persisted and queryable.

| Field | Value |
| ----- | ----- |
| Gated task id | `FIX-003` |
| Run id / dir | `FIX-003` (`runs/FIX-003/`) |
| Process exit | 1 |
| Stage | `WAITING_FOR_HUMAN` |
| Classification | `NEEDS_HUMAN` / `DESTRUCTIVE_OPERATION` (risk `IRREVERSIBLE`, `HUMAN_APPROVAL_REQUIRED`) |
| Verdict | MATCH |

**Refusal demonstration (no authorization granted):** the gated task could not
complete. `sop approvals --json` returned a single **PENDING** approval for
`FIX-003`, and the run stopped at the boundary (`WAITING_FOR_HUMAN`) rather than
completing:

```json
{
  "version": 1,
  "approvals": [
    {
      "task_id": "FIX-003",
      "kind": "NEEDS_HUMAN",
      "target": "FIX-003",
      "reason": "the requested fixture operation is destructive and irreversible, so it requires authorization",
      "stage": "WAITING_FOR_HUMAN",
      "disposition": "NEEDS_HUMAN",
      "status": "PENDING",
      "requested_at": "2026-10-09T05:31:19.762621Z",
      "task_status": "PLANNED"
    }
  ]
}
```

**No unauthorized approval:** the approval remains `status: PENDING`; `sop approve`
was **never** invoked. The task status is `PLANNED`/not-completed and no recorded
decision exists — i.e. completion was refused and no approval boundary was
bypassed.

**Preserved evidence:**
`docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/gate/`
(`etoe-005-run-records.txt` with the `sop approvals --json` listing,
`runs/FIX-003/` incl. `approval.json` PENDING).

---

## 6. Independent gate verification and convergence

| Check | Method | Result |
| ----- | ------ | ------ |
| Success gates re-checked | read `state.json` stage + `validation.json` `Status` from `runs/run-20261009-053027/` | `stage=PASSED`, `validation=PASS` — consistent with the record line |
| Failure gates re-checked | read `state.json` stage + embedded `validation.json` from `runs/run-20261009-053030/` | `stage=WAITING_FOR_HUMAN`, `validation=FAIL`, `AUTO_FIX_EXHAUSTED` — consistent |
| Gate refusal re-checked | read `sop approvals --json` output + `runs/FIX-003/approval.json` | `status=PENDING`, no decision recorded — consistent; task not completed |
| Convergence bound | configured `quality.max_fix_cycles: 3` vs observed `FIX ×3` then `AUTO_FIX_EXHAUSTED` | match; bound not relaxed |
| No hand-editing | all transitions produced by `sop run`/`sop approvals`; state read-only in this task | no direct `.agent-sdlc` mutation by this task |

No discrepancies were observed between the recorded gate results and the
underlying state/validation/approval artifacts. (Any future divergence must be
reported rather than smoothed over.)

---

## 7. Acceptance-criteria mapping

| # | Criterion | Evidence | Status |
| - | --------- | -------- | ------ |
| 1 | At least one successful task and one intentionally failing/recovering task exercised end to end with traceable task/run/approval evidence | §3 success `run-20261009-053027` (`stage=PASSED`, `validation=PASS`); §4 fail `run-20261009-053030` (`WAITING_FOR_HUMAN`, `validation=FAIL`, `AUTO_FIX_EXHAUSTED`, FIX×3); §5 gate `FIX-003` (`PENDING` approval) | MET |
| 2 | The human-gated task cannot be completed without an explicit authorization; refusal path shown without granting an unauthorized approval | §5: `FIX-003` stops at `WAITING_FOR_HUMAN`, `sop approvals --json` reports a `PENDING` approval, `sop approve` never invoked, task status `PLANNED`/not completed | MET |
| 3 | No approval boundary bypassed and no SOP state edited by hand | §3–§5 all transitions produced by `sop run`/`sop approvals` in isolated disposable roots; `approval.json` remains `PENDING`; production `.agent-sdlc` untouched; §6 | MET |
| 4 | Convergence enforcement observed or deterministically verified without relaxing any bound | §4 FIX×3 then `AUTO_FIX_EXHAUSTED`; §6 bound `max_fix_cycles: 3` matched, not raised | MET |

---

## 8. Mutation statement

- The **only** repository change made by ETOE-006 is this report file,
  `docs/reports/end-to-end-reliability/ETOE-006-e2e-evidence.md`.
- No orchestrator source (`cmd/sop`, `internal/`), configuration, or CLI behavior
  was changed. No SOP state (`.agent-sdlc/`) was edited by hand.
- All scenario execution occurred in **isolated, disposable** projects outside any
  production checkout, each with its own `state.db`, driven by the existing SOP
  CLI with the offline controlled provider. Production SOP state was not used for
  any scenario.
- Nothing was committed or pushed.

## 9. Limitations

1. **Controlled provider, not a live model.** These results verify SOP's governed
   lifecycle determinism end to end; real `ollama`/remote-model behavior is
   **UNAVAILABLE** in this task and belongs to a live-model stage.
2. **`--task` runs persist no task**, so a `--task` run cannot record a queryable
   approval gate; the human-gate scenario therefore runs **plan-based**
   (`sop run docs/PLAN.md`), which compiles a persisted task and records the gate.
3. The `plan` run directory written by a plan-based run is plan compilation, not a
   task run, and is excluded from run-id attribution.
4. The success scenario requires a clean fixture baseline (stub `Add`); a re-run on
   a mutated root yields `NO_CHANGES_PRODUCED` — recreate the root for a fresh
   baseline.
5. The controlled provider exercises a fixed plan/outcome shape; it is not a
   general-purpose agent.
