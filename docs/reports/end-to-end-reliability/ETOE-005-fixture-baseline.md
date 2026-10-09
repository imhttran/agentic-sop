# ETOE-005 — Disposable dogfood fixture and deterministic baseline

**Status:** MET — the three scenarios were executed end to end in three isolated
disposable projects with an **offline, controlled test provider**, and their run
identifiers and observed outcomes are recorded below.

> **Deterministic controlled-provider testing ≠ real Ollama execution.** This
> baseline drives SOP with a controlled agent (`scripts/etoe-005-fixture-agent.sh`)
> selected via `SOP_AGENT_HARNESS=command` / `SOP_AGENT_PROVIDER=command` /
> `SOP_AGENT_COMMAND`. No external model service is contacted, so the lifecycle is
> deterministic and reproducible. This is **not** a measurement of real
> `ollama`/model behavior; it verifies SOP's _lifecycle_ (selection, IMPLEMENT →
> VALIDATE → REVIEW → FIX → gate). Real-model behavior is exercised separately by
> ETOE-006/ETOE-008.

---

## 1. Binary provenance

| Item             | Value                                                                                          |
| ---------------- | ---------------------------------------------------------------------------------------------- |
| Source build     | `go build -o /tmp/sop-etoe005 ./cmd/sop` (HEAD of the active checkout)                         |
| Source binary    | `sop version` = `sop dev`                                                                      |
| Go toolchain     | go1.27.1                                                                                       |
| Controlled agent | `scripts/etoe-005-fixture-agent.sh` (bash; `python3` used only to read the request capability) |

The installed `PATH` binary is not used for the baseline: it lacks the surfaces
the fixture needs. The source build is used throughout.

---

## 2. Isolation method

Three **separate** disposable projects are created, each with its **own**
`sop init` state database:

| Scenario | Fixture root (physical)         | Own state DB           |
| -------- | ------------------------------- | ---------------------- |
| success  | `/private/tmp/etoe-005-success` | `.agent-sdlc/state.db` |
| fail     | `/private/tmp/etoe-005-fail`    | `.agent-sdlc/state.db` |
| gate     | `/private/tmp/etoe-005-gate`    | `.agent-sdlc/state.db` |

Isolation guarantees (enforced by the scripts):

- the fixture root must be a strict descendant of the disposable base root
  (`${TMPDIR:-/tmp}`, physically resolved), so nothing is written outside it;
- the setup/teardown scripts **refuse any root that resolves inside the
  production checkout** (the parent of `scripts/`, symlink-resolved);
- every SOP invocation runs with the process CWD set to the fixture root
  (`cd "$FIXTURE_ROOT"`), so `sop` reads/writes the **disposable** `state.db`,
  never the active project's;
- the runner invokes `sop run` with an explicit selector (`--task <file>` or
  `docs/PLAN.md`) — never a bare `sop run` — so it can never discover or execute
  the active production plan;
- each root is a `git init` baseline so SOP's change detection has a reference.

The active project's `.agent-sdlc/state.db` was **not** used for any fixture step;
production SOP state was unchanged before and after (see §7).

---

## 3. Exact reproduction commands

```bash
# 0. Build the deterministic binary once.
go build -o /tmp/sop-etoe005 ./cmd/sop

# 1. Create three isolated disposable projects (own state DB each).
TMPDIR=/tmp bash scripts/etoe-005-fixture-setup.sh /tmp/etoe-005-success /tmp/sop-etoe005
TMPDIR=/tmp bash scripts/etoe-005-fixture-setup.sh /tmp/etoe-005-fail    /tmp/sop-etoe005
TMPDIR=/tmp bash scripts/etoe-005-fixture-setup.sh /tmp/etoe-005-gate    /tmp/sop-etoe005

# 2. Execute each scenario in its own project, controlled offline provider.
TMPDIR=/tmp ETOE005_SCENARIO=success bash scripts/etoe-005-fixture-run.sh /tmp/etoe-005-success /tmp/sop-etoe005
TMPDIR=/tmp ETOE005_SCENARIO=fail    bash scripts/etoe-005-fixture-run.sh /tmp/etoe-005-fail    /tmp/sop-etoe005
TMPDIR=/tmp ETOE005_SCENARIO=gate    bash scripts/etoe-005-fixture-run.sh /tmp/etoe-005-gate    /tmp/sop-etoe005

# 3. Inspect the structured records for each scenario.
cat /tmp/etoe-005-success/etoe-005-run-records.txt
cat /tmp/etoe-005-fail/etoe-005-run-records.txt
cat /tmp/etoe-005-gate/etoe-005-run-records.txt

# 4. Tear down (removes ONLY each fixture root; idempotent).
TMPDIR=/tmp bash scripts/etoe-005-fixture-teardown.sh /tmp/etoe-005-success
TMPDIR=/tmp bash scripts/etoe-005-fixture-teardown.sh /tmp/etoe-005-fail
TMPDIR=/tmp bash scripts/etoe-005-fixture-teardown.sh /tmp/etoe-005-gate
```

Note: `TMPDIR=/tmp` pins the disposable base root to `/private/tmp`; the guard
rejects any root that is not a strict descendant of it.

---

## 4. Scenarios — expected vs observed

Predeclared expectations (declared in `tasks/*.md` before execution):

| Scenario    | Invocation                            | Controlled mode | Expected                                |
| ----------- | ------------------------------------- | --------------- | --------------------------------------- |
| FIX-SUCCESS | `sop run --task tasks/FIX-SUCCESS.md` | `success`       | completed / LOCAL_DONE, validation PASS |
| FIX-FAIL    | `sop run --task tasks/FIX-FAIL.md`    | `fail`          | validation FAIL; does NOT complete      |
| FIX-GATE    | `sop run docs/PLAN.md` (plan-based)   | `gate`          | a genuine pending human approval        |

Observed (from each run's structured artifacts):

| Scenario    | Run ID                | Exit | Stage               | Validation                         | Classification                                                                          | Verdict   |
| ----------- | --------------------- | ---- | ------------------- | ---------------------------------- | --------------------------------------------------------------------------------------- | --------- |
| FIX-SUCCESS | `run-20261009-053027` | 0    | `PASSED`            | `PASS` (BUILD/UNIT_TEST/LINT)      | PASS gate                                                                               | **MATCH** |
| FIX-FAIL    | `run-20261009-053030` | 1    | `WAITING_FOR_HUMAN` | `FAIL` (UNIT_TEST `go test ./...`) | `AUTO_FIX_EXHAUSTED` after FIX×3                                                        | **MATCH** |
| FIX-GATE    | `FIX-003`             | 1    | `WAITING_FOR_HUMAN` | n/a                                | `NEEDS_HUMAN` / `DESTRUCTIVE_OPERATION`, risk `IRREVERSIBLE`, `HUMAN_APPROVAL_REQUIRED` | **MATCH** |

Lifecycle evidence of interest:

- **FIX-SUCCESS:** `START → PLAN → IMPLEMENT → VALIDATE → REVIEW → QUALITY(PASS) →
COMPLETE(PASS)`; `fix cycles 0/3`; a real `calc.go` mutation (stub `Add` → real
  `Add`).
- **FIX-FAIL:** intended deterministic failure — `go test ./...` fails on the
  enabled `TestBroken` (`broken_test.go:7: intentional ETOE-005 validation
failure`); SOP entered FIX and revalidated **3** times
  (`VALIDATE → QUALITY(FAIL) → FIX` ×3) then stopped at the human boundary
  (`AUTO_FIX_EXHAUSTED`, `WAITING_FOR_HUMAN`).
- **FIX-GATE:** the controlled provider returned a destructive/authorization
  outcome; SOP classified `NEEDS_HUMAN`/`DESTRUCTIVE_OPERATION` (risk
  `IRREVERSIBLE`), recorded a **PENDING** approval for `FIX-003`, and stopped
  without approving it.

---

## 5. Preserved evidence paths

Copied out of the disposable roots **before** teardown:

| Scenario | Records                                                                                            | Run artifacts                                      |
| -------- | -------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| success  | `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/success/etoe-005-run-records.txt` | `.../success/runs/run-20261009-053027/`            |
| fail     | `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/fail/etoe-005-run-records.txt`    | `.../fail/runs/run-20261009-053030/`               |
| gate     | `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/gate/etoe-005-run-records.txt`    | `.../gate/runs/FIX-003/` (+ `.../gate/runs/plan/`) |

Each `runs/<id>/` holds `state.json`, `validation.json`, `review.json`/`report.json`
(where produced), `classification.json`, `trace.json`, and `activity.jsonl`. The
gate run also holds `approval.json` (PENDING). Each scenario's `config.yaml` and
the gate `PLAN.md` are preserved alongside.

Prior attempt evidence (retained): `ETOE-005-attempt-1-failure/`,
`ETOE-005-attempt-2-fix-exhausted/`, `ETOE-005-accidental-run/`.

---

## 6. Acceptance matrix (ETOE-005)

| #   | Acceptance criterion                                                                                                            | Evidence                                                                                                                                                          | Status  |
| --- | ------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| 1   | Reproducible setup/teardown creates a disposable, initialized SOP project outside any production checkout                       | three isolated roots with own `state.db`; guards refuse in-checkout/outside-base roots; teardown removes only the root (idempotent); §2/§3                        | **MET** |
| 2   | Fixture contains one success, one intentionally failing, one human-gated task, with expected outcomes recorded before execution | `tasks/FIX-SUCCESS.md`, `tasks/FIX-FAIL.md`, `docs/PLAN.md` (gate); expectations declared pre-execution (§4)                                                      | **MET** |
| 3   | The recorded run IDs and expected outcomes are sufficient to reproduce the baseline                                             | concrete run IDs `run-20261009-053027`, `run-20261009-053030`, `FIX-003`; observed stages/validations recorded; exact reproduction commands in §3; evidence in §5 | **MET** |

Criterion 3 is now satisfied with **actual** run IDs and observed outcomes (not a
procedure).

---

## 7. Mutation statement

- **Fixture execution:** mutations occurred **only** inside the three disposable
  roots (each its own `state.db`, its own `git` repo). No production checkout file
  and no sibling repository was modified. The controlled provider contacts no
  network/model service.
- **Production SOP state:** unchanged by fixture execution — the active plan
  remains `plan-sop-end-to-end-reliability`; ETOE-001…004 are `LOCAL_DONE`; ETOE-005
  remains `PLANNED` with its pre-existing pending `NEEDS_HUMAN` approval. The
  fixture was never run against the active project.
- Nothing was committed or pushed. The disposable roots were removed by teardown.

---

## 8. Limitations

1. **Controlled provider, not a real model.** These results verify SOP's lifecycle
   determinism, not real `ollama`/model behavior; the model-driven paths are
   ETOE-006/ETOE-008.
2. **`--task` runs persist no task**, so a `--task` run cannot record a queryable
   approval gate. The human-gate scenario therefore runs **plan-based**
   (`sop run docs/PLAN.md`), which compiles a persisted task and records the gate.
3. The `plan` run directory written by a plan-based run is excluded from run-id
   attribution (it is plan compilation, not a task run).
4. The success scenario requires its project to start from the **clean** fixture
   baseline (setup ships a stub `Add`); re-running a mutated root yields
   `NO_CHANGES_PRODUCED` — recreate the root for a fresh baseline.
5. The controlled provider exercises SOP's lifecycle with a fixed plan/outcome
   shape; it is not a general-purpose agent.
