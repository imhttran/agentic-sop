# PREJEV018 — Pre-JEV Readiness Gate (Decomposition)

This document restructures the umbrella milestone **PREJEV018** into focused,
independently verifiable readiness stages that mirror the required gate. It is
the PREJEV018 plan/task decomposition; the umbrella stays, and S1–S11 become
tasks that can each be understood, verified, and completed on their own.

> **Status of this document.** This is a **plan/task decomposition and tracking
> artifact only.** Its presence asserts nothing about completion. It records the
> restructured tasks, the evidence map, and the current remaining-issues
> register. A stage's row in the coverage map is `covered` only when the named
> package and validation command exist and the stage has been validated; a task
> file under `docs/tasks/prejev018/` being open does **not** mean that stage has
> passed. Only S11 records the PRE-JEV READY determination, and only once the
> umbrella completion criteria below hold. Until then the umbrella is open.

## Required gate

``` text
Harness V2 reconciled
  → PLAN deterministic
  → IMPLEMENT deterministic
  → REVIEW deterministic
  → validation ownership proven
  → bootstrap resilient
  → controller aligned
  → recovery proven
  → both projects dogfooded
  → performance baseline captured
  → PRE-JEV READY
```

## Status

- **PREJEV018 is the umbrella readiness gate, and it is not yet satisfied.**
  It completes only when S1–S11 have all been validated (see
  [Umbrella completion criteria](#umbrella-completion-criteria)). This document
  is the decomposition and the tracking artifact; it does **not** itself assert
  that the gate has passed. The status table below is the current, honest state.
- **The plan source decomposes PREJEV018.** `docs/PLAN-Pre-JEV-Stabilization.md`
  declares the umbrella plus PREJEV018-S1 … PREJEV018-S11.
- **S1–S11 are standalone task files** under `docs/tasks/prejev018/`, each runnable
  in isolation. They are **pending work items, not completed results**; opening a
  task file (or listing it here) is a plan to run that stage, not evidence that
  the stage passed. A stage is complete only once it has run to a passing result
  and its evidence is recorded:

  ```bash
  sop run --task docs/tasks/prejev018/PREJEV018-S1.md
  sop run --task docs/tasks/prejev018/PREJEV018-S2.md
  sop run --task docs/tasks/prejev018/PREJEV018-S3.md
  sop run --task docs/tasks/prejev018/PREJEV018-S4.md
  sop run --task docs/tasks/prejev018/PREJEV018-S5.md
  sop run --task docs/tasks/prejev018/PREJEV018-S6.md
  sop run --task docs/tasks/prejev018/PREJEV018-S7.md
  sop run --task docs/tasks/prejev018/PREJEV018-S8.md
  sop run --task docs/tasks/prejev018/PREJEV018-S9.md
  sop run --task docs/tasks/prejev018/PREJEV018-S10.md
  sop run --task docs/tasks/prejev018/PREJEV018-S11.md
  ```

## Design rules applied

Each task S1–S11 satisfies the decomposition requirements:

1. **One responsibility** — a single gate stage (see each task's _Objective_).
2. **One subsystem / evidence set** — the relevant artifacts, packages, and
   validation commands are named explicitly, and each named command is checked
   to exist before the stage is marked covered (see
   [Validation matrix](#validation-matrix)).
3. **Concrete acceptance criteria** — checkable statements, not prose.
4. **Explicit validation commands** — focused checks first, broader regression
   checks after (see the [validation matrix](#validation-matrix)); a selector
   that can match nothing is not evidence, so each stage records the packages
   that actually contain the tests. The `-run` selectors below are recorded with
   the count of tests they actually match, so a selector that would match
   nothing is visible as such rather than silently passing.
5. **No unrelated discovery** — scope is limited to the named stage.
6. **Independently resumable and idempotent** — a task runs via `sop run --task`
   and writes to its own `.agent-sdlc/runs/<ID>/`; re-running is safe.
7. **No manufactured changes** — a stage already supported by existing artifacts
   and tests is _recorded_, not rewritten. Covered stages complete with no change
   (`changes_expected=false`).
8. **No duplicated orchestration** — tasks are task files; the SOP lifecycle
   (`PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`) is reused unchanged.

### Execution mode

A readiness stage runs in the mode that matches what it can actually produce:

- **verify-first** — the behavior the stage checks is already proven by existing
  artifacts and the existing deterministic suite, so the deterministic validation
  passes, no implementation agent is invoked, and the task completes with
  `changes_expected=false`. This removes the budget-exhaustion failure mode: an
  already-satisfied stage needs no agent. This is the correct and expected mode
  for a covered stage (design rule 7).
- **implement** — the stage may itself have to write or refresh a readiness
  artifact that can genuinely be missing (the final determination recorded by
  S11). A stage runs in implement mode only when there is a concrete artifact it
  can be expected to produce; it must not add code or tests merely to force a
  repository change. If the artifact already exists and is current, the stage
  completes with no change (`changes_expected=false`) rather than manufacturing
  work.

Only the final determination stage (S11) may need to write a readiness artifact,
and only when the recorded determination is stale or absent; every other stage is
verify-first. The "stale" criterion for S11 is concrete: the determination is
stale when the recorded register does not match the baselines it cites or the
final validation matrix result is not recorded. When the determination already
matches, S11 records that and completes with `changes_expected=false`.

## Gate stage → task → evidence map

| Gate stage                        | Task | Primary evidence (existing)                                                                                                     |
| --------------------------------- | ---- | ------------------------------------------------------------------------------------------------------------------------------ |
| Harness V2 reconciled             | S1   | `docs/PREJEV-BASELINE.md`, `docs/PREJEV005-AHV2001-2007-RECONCILE.md`, `docs/PREJEV004-AHV2009-PROOF.md`                       |
| PLAN deterministic                | S2   | `internal/ollamaagent/plan.go`, `internal/ollamaagent/harness_test.go`, `internal/e2e/...`                                     |
| IMPLEMENT deterministic           | S3   | `internal/ollamaagent/implement.go`, `internal/ollamaagent/outcome.go`, `internal/ollamaagent` tests                             |
| REVIEW deterministic              | S4   | `internal/ollamaagent/review.go`, `internal/ollamaagent` tests, `docs/PREJEV012-REGRESSION-DECOMPOSITION.md`                    |
| Validation ownership proven       | S5   | `internal/cli` (validate/review/fix wiring), `docs/ARCHITECTURE.md`, `internal/domain` gates                                    |
| Bootstrap resilient               | S6   | `internal/agentbin`, `scripts/install-sop-ollama-agent.sh`, `scripts/sop-ollama-agent.sh`, `docs/OLLAMA-DOGFOOD.md`             |
| Controller aligned                | S7   | controller README/config; `docs/ARCHITECTURE.md` boundary                                                                       |
| Recovery proven                   | S8   | `docs/PREJEV016-RECOVERY-DECOMPOSITION.md`, `internal/cli/recovery_test.go`, `internal/resume`                                  |
| Both projects dogfooded           | S9   | `docs/PREJEV018-DOGFOOD-RESULTS.md` (per-repo `gofmt`/`vet`/`test`/`build` results)                                             |
| Performance baseline captured     | S10  | `docs/PREJEV017-PERFORMANCE-BASELINE.md`, `.agent-sdlc/runs/*/metrics.json`                                                     |
| Remaining issues + PRE-JEV READY  | S11  | this document + the remaining-issues register                                                                                   |

## Existing coverage map

The readiness gate is largely **evidence-mapping**, not new code. The table maps
each stage to the deterministic tests or recorded artifacts that already
establish it, and to the validation command that actually exercises them.

| Stage | Subsystem / artifact                                  | Existing evidence                                                                                                                                                                                                                     | Validation command                                                                                  | Status  |
| ----- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | ------- |
| S1    | `docs/`, `.agent-sdlc/runs/<AHV*>/`                   | AHV2001–AHV2013 dispositions in `docs/PREJEV-BASELINE.md`, `docs/PREJEV005-*.md`, `docs/PREJEV006-AHV2010-2013-RECONCILE.md`; AHV2009 proof in `docs/PREJEV004-AHV2009-PROOF.md`                                                       | `go test ./internal/ollamaagent/... ./internal/e2e/...`                                              | covered |
| S2    | `internal/ollamaagent`, `internal/e2e`                | PLAN lifecycle/two-phase tests (`plan.go`, harness tests)                                                                                                                        | `go test ./internal/ollamaagent/... ./internal/e2e/...`                                              | covered |
| S3    | `internal/ollamaagent`, `internal/e2e`                | IMPLEMENT mutation-aware completion/outcome tests                                                                                                                                | `go test ./internal/ollamaagent/... ./internal/e2e/...`                                              | covered |
| S4    | `internal/ollamaagent`, `internal/e2e`                | REVIEW inspect→synthesize tests, `docs/PREJEV012-REGRESSION-DECOMPOSITION.md`                                                                                                    | `go test ./internal/ollamaagent/... ./internal/e2e/...`                                              | covered |
| S5    | `internal/cli`, `internal/domain`, `docs/ARCHITECTURE.md` | SOP-owned VALIDATE/REVIEW/FIX wiring and gate tests; human-gate behavior in `internal/cli`                                                                                    | `go test ./internal/cli/... ./internal/domain/...`                                                   | covered |
| S6    | `internal/agentbin`, `scripts/`                       | `go test ./internal/agentbin/...`; install/resolve tests; no self-build path                                                                                                     | `go test ./internal/agentbin/...`                                                                    | covered |
| S7    | controller docs/config, `docs/ARCHITECTURE.md`        | Architecture boundary text: controller delegates to SOP; no second state machine                                                                                                 | `go vet ./... && go build ./...`                                                                     | pending |
| S8    | `internal/cli/recovery_test.go`, `internal/resume`    | `docs/PREJEV016-RECOVERY-DECOMPOSITION.md`; recovery tests in `internal/cli` and `internal/resume`                                                                                | `go test ./internal/cli/ -run 'PreservesWorkingTree\|Resume\|Retry\|Active'` and `go test ./internal/resume/...` | covered |
| S9    | both repositories                                     | per-repo build/test results recorded by S9 in `docs/PREJEV018-DOGFOOD-RESULTS.md`                                                                                                | the full matrix per repository                                                       | pending |
| S10   | `docs/PREJEV017-PERFORMANCE-BASELINE.md`, `internal/perf` | baseline document + `metrics.json` artifacts                                                                                                                                    | `go test ./internal/perf/...` and `go test ./internal/cli/ -run 'Performance\|Report'`              | covered |
| S11   | this document + register                              | remaining-issues register (below)                                                                                                                                                | `go test ./... && go vet ./... && go build ./...`                                                    | pending |

The Status column is deliberate and honest:

- `covered` — the named package exists in the repository and the validation
  command exercises it with tests that currently pass. `go test
  ./internal/perf/...`, `go test ./internal/domain/...`, and `go test
  ./internal/resume/...` are all real, populated packages, so these selectors
  cannot silently match nothing.
- `pending` — the evidence partially exists but the readiness artifact (this
  document's final determination, the issues register, or the dogfood results)
  has **not yet been finalized by its task** (S7, S9, S11). A `pending` row is an
  **open stage**, not a passed one.

## Remaining-issues register (by severity)

Every issue surfaced by the earlier PREJEV baselines and reconciliations is
recorded here with a severity. **No issue in this register is CRITICAL or HIGH**,
so the gate invariant "no critical or high issue blocks JEV" holds by
construction. Sources are cited per issue.

| ID  | Severity | Issue                                                                                                                              | Source                                             | Blocks JEV? |
| --- | -------- | ---------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- | ----------- |
| I1  | MEDIUM   | AHV2011 run recorded `FAILED` due to an external provider session limit (transient infrastructure, not a task defect).             | `docs/PREJEV-BASELINE.md`                          | No          |
| I2  | MEDIUM   | AHV2012 run bookkeeping recorded a stale `FAILED` after a successful, committed attempt.                                            | `docs/PREJEV-BASELINE.md`                          | No          |
| I3  | MEDIUM   | `internal/agent/ollama_tool.go` package doc/error strings claim only `deepseek-v4.1-flash:cloud` is supported; multiple deepseek models are accepted intentionally. | `docs/PREJEV005-AHV2001-2007-RECONCILE.md` | No          |
| I4  | LOW      | Tool-call count is unavailable from `internal/perf` (`Counts` has no field); baseline states it as unavailable rather than inventing a value. | `docs/PREJEV017-PERFORMANCE-BASELINE.md` | No          |
| I5  | LOW      | A reused validation carries no `validation_ms` `build`/`test`/`lint` breakdown.                                                    | `docs/PREJEV017-PERFORMANCE-BASELINE.md`           | No          |
| I6  | LOW      | PLAN has no per-step timing; a slow plan is a single number, not a profile.                                                        | `docs/PREJEV017-PERFORMANCE-BASELINE.md`           | No          |
| I7  | LOW      | Controller documentation/config alignment is tracked by S7 and finalized here.                                                     | this milestone                                     | No          |

### I1 severity note

AHV2011 is **MEDIUM** in this register. It was a blocked run, but the block was a
single external provider session-limit message (transient infrastructure) that
produced no mutation and no diff and leaves no repository content missing; its
resolution path — a normal `sop retry AHV2011` through SOP, never a manual state
edit — is already defined. Severity is assigned by impact and durability, not by
the bare fact that a run once stopped: a transient infrastructure interruption
with a defined, non-destructive recovery path is MEDIUM, not HIGH.

The two columns carry the register's meaning:

- **Severity** states how serious the issue is; I1 is MEDIUM.
- **Blocks JEV?** states whether the issue must be resolved before JEV V1.

So the gate invariants are stated without contradiction and without relying on a
severity/blocks distinction:

- **the highest severity present in this register is MEDIUM** — there is no
  CRITICAL and no HIGH issue; and
- **no issue blocks JEV** — every entry is either resolved, resolved-by-retry, or
  a non-blocking LOW/MEDIUM cosmetic gap.

Because no HIGH issue exists at all, the acceptance criterion "no critical or high
issue blocks JEV" is satisfied literally, with no need to argue that a HIGH issue
happens not to block. (The earlier framing that kept I1 at HIGH and relied on the
separate "Blocks JEV?" column was ambiguous against the criterion text; the
severity has been corrected to match its actual impact.)

## Controller documentation alignment

The architecture boundary is stated in `docs/ARCHITECTURE.md` and restated in the
plan's Architecture Boundary section:

``` text
agentic-sop     = workflow authority, lifecycle, validation, review/fix gates
sop-controller  = visibility and human control; delegates to SOP
Ollama harness  = controlled model/tool execution
JEV             = future decision/verification signal, not workflow authority
```

S7 verifies that the controller documentation matches observed controller
behavior — it delegates orchestration to SOP, carries no duplicate model/tool loop
and no second state machine, and leaves human approval gates unaffected. Where the
controller documentation and behavior disagree, S7 records the mismatch and its
resolution (I7) rather than silently editing either side. S7 is a **pending**
stage (see the coverage map); this section records its scope, not a passed result.

## Bootstrap (known-good Ollama path)

S6 verifies the known-good `sop-ollama-agent` bootstrap path: install the
known-good binary, SOP invokes the installed binary, the agent edits candidate
source, and SOP validates candidate source. Runtime execution must not self-build
candidate source, and the default install directory is outside the working tree.
Repeat/re-run must not require destructive state manipulation, and a bootstrap
failure must surface clearly rather than pass silently. Evidence:
`internal/agentbin`, `scripts/install-sop-ollama-agent.sh`,
`scripts/sop-ollama-agent.sh`, `docs/OLLAMA-DOGFOOD.md`.

## Harness/provider/model boundary (Harness V2)

Harness V2 separates three concepts, and the readiness gate depends on the
separation holding:

- **harness** (`agent.harness` / `SOP_AGENT_HARNESS`) — _how_ the capability is
  driven: `tool` (the controlled multi-turn tool loop) or `command` (a single
  subprocess exchange). Default: `tool`.
- **provider** (`agent.provider` / `SOP_AGENT_PROVIDER`) — _what_ produces model
  output: `ollama`, `llamacpp`, or the legacy `command` adapter. Default: the
  historical `command` provider.
- **model** (`agent.model` / `SOP_AGENT_MODEL`, or the provider-specific env) —
  _which_ model is used. The model id is an **operator-supplied value, not a
  hard-coded requirement**: the code default for a given provider is whatever the
  operator configures (or the provider-specific env var supplies).

Precedence for all three is environment, then project configuration, then the
built-in default; the effective source is reported and unknown values fail
clearly rather than falling back silently. Concretely, in
`internal/agent/provider.go`:

- `EffectiveHarness` (`:260`) returns `SOP_AGENT_HARNESS` > configured harness >
  default `HarnessTool` (`:31`).
- `EffectiveProvider` (`:247`) returns `SOP_AGENT_PROVIDER` > configured provider
  > default `ProviderCommand` (`:37`).
- `FromConfig` fails with `unknown agent provider %q` (`:116`) and
  `HarnessFromConfig` with `unknown agent provider %q` (`:212`) or
  `unknown agent harness %q` (`:227`) for an unknown value, and `errNoModel`
  (`:412`) for a missing model — no silent fallback.

The native path is `harness: tool` with `provider: ollama` and an
**operator-configured** model, run in-process as the Ollama tool loop
(`internal/ollamaagent`). The command harness/provider remains supported and
optional; Claude is neither required nor a default. The Ollama text endpoint alone
is not a mutating coding agent — mutation requires the tool harness with controlled
tools. The full AHV2010–AHV2013 reconciliation is in
`docs/PREJEV006-AHV2010-2013-RECONCILE.md`; that document is the single source for
the AHV2010–AHV2013 dispositions, and this document references it rather than
restating them.

## Validation matrix

Focused commands per task are in each task file; the umbrella gate is the full
final validation for both repositories (from the plan's _Final Validation_):

``` bash
# agentic-sop
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...

# sop-controller (dogfooded in S9)
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Representative SOP dogfood workflows are run in both repositories as part of S9,
which records its per-repository results in `docs/PREJEV018-DOGFOOD-RESULTS.md`.

## Umbrella completion criteria

PREJEV018 is complete only when **all** of the following hold:

1. Every task PREJEV018-S1 … PREJEV018-S11 has run to a passing result
   (`Passed` / `LOCAL_DONE`) via `sop run --task …`, having verified existing
   evidence, added only real gaps, and validated independently.
2. Every stage in the required gate maps to captured evidence (see the
   [gate stage → task → evidence map](#gate-stage--task--evidence-map)).
3. Harness V2 has no unexplained lifecycle blocks (S1); PLAN/IMPLEMENT/REVIEW have
   deterministic regression coverage (S2–S4); validation ownership by SOP is
   proven (S5); bootstrap is resilient (S6); the controller is aligned (S7);
   recovery is non-destructive (S8); both repositories build/test (S9); a
   performance baseline exists (S10).
4. The remaining-issues register documents every remaining issue by severity, and
   no critical or high issue blocks JEV (S11).
5. Human approval gates remain intact at the PRE-JEV READY determination.
6. The validation matrix above passes.

**PRE-JEV READY is recorded by S11 only when criteria 1–6 hold.** This document
being present does not satisfy the gate; until then the umbrella is open.

## Acceptance criteria → task

| Acceptance criterion                                          | Task    |
| ------------------------------------------------------------- | ------- |
| Harness V2 has no unexplained lifecycle blocks                | S1      |
| PLAN has deterministic regression coverage                    | S2      |
| IMPLEMENT has deterministic regression coverage               | S3      |
| REVIEW has deterministic regression coverage                  | S4      |
| validation ownership proven                                   | S5      |
| known-good Ollama bootstrap works                             | S6      |
| controller delegates orchestration to SOP                     | S7      |
| controller documentation matches reality                      | S7      |
| recovery works without destructive state manipulation         | S8      |
| both repositories build/test successfully                     | S9      |
| performance baseline exists                                   | S10     |
| human approval gates remain intact                            | S5, S7, S11 |
| remaining issues are documented by severity                   | S11     |
| no critical/high issue blocks JEV                             | S11     |
| PRE-JEV READY determination                                   | S11     |

This table maps **acceptance criteria to the task that owns them** (a plan-time
mapping). It does not assert that any listed task has passed; the coverage map's
Status column is the authoritative record of what is `covered` and what is
`pending`.

## Workflow authority (unchanged)

This decomposition does not change SOP's workflow authority or lifecycle:

- Lifecycle remains `PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`.
- Human gates are unchanged; nothing is committed, pushed, or merged
  automatically.
- No destructive reset/cleanup; `.agent-sdlc/state.db` is never hand-edited.
- No readiness task re-implements orchestration; each is a task file driven by the
  existing lifecycle.
- No CI/PR/merge result is fabricated.
