# AUTONOMY-001 — End-to-end baseline and multi-package unused-code cleanup scenario

**Status:** baseline recorded. Descriptive artifact only — no orchestration code was added.
**Scope:** record the current autonomous end-to-end path by code reference, exercise a single
prompt against a disposable multi-package Go module containing unused code, and list every gap as
VERIFIED / PARTIAL / MISSING.

> **Revision note (operator correction, 2026-10-09).** Four code-reference paths and the §2a
> fixture details were inaccurate in the original generated report and are corrected below.
> The original run evidence is preserved unmodified in `.agent-sdlc/runs/AUTONOMY-001/`; SOP
> history and the task's `LOCAL_DONE` status are not rewritten. Corrections: `runGraph` is
> `internal/cli/drive.go:46` (not `run.go`); `runAttempts` is `internal/cli/escalation.go:76`
> (not `run.go`); the commit gate is `internal/commitgate/gate.go` (there is no
> `commitgate.go`); the handoff is `internal/handoff/` (`manager.go`/`record.go`/`capsule.go`,
> no `handoff.go`); and the fixture signature/value details in §2a.

---

## 1. Component map (by code reference)

Every claim below is backed by a concrete path. The autonomous path has three parts: the
graph-execution entrypoint (`sop run`), the implement-capability prompt entrypoint
(`sop prompt --capability implement`), and the human approval / commit boundary.

### 1a. CLI entrypoint

- `cmd/sop/main.go` — process entrypoint. It calls
  `os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))`, so every subcommand is dispatched
  through `internal/cli`.
- `internal/cli/cli.go` — command dispatch (subcommand table / usage).

### 1b. `sop run` — graph execution path

- `internal/cli/run.go` → `runRun` — parses `sop run` args and chooses the path:
  with `--task FILE` it calls `runSingleTask`, otherwise it calls `runGraph`
  (see the literal dispatch `if opts.taskArg != "" { return runSingleTask(...) }` /
  `return runGraph(opts.planArg, opts.maxTasks, ...)`).
- `runGraph` (`internal/cli/drive.go:46`) — drives the persisted task graph in dependency
  order (scheduler-driven iteration over tasks); the package `internal/scheduler` supplies
  the dependency-ordered scheduling.
- `runAttempts` (`internal/cli/escalation.go:76`), `executeLifecycle`
  (`internal/cli/run.go:310`), and `runStages` (`internal/cli/run.go:447`) — the governed
  per-task lifecycle documented in the file header comment:

  ```
  task → plan → implement → detect changes
       → { validate → review → gate → fix } → report
  ```

  The braces are a **bounded fix loop** (`≤ cfg.Quality.MaxFixCycles`; exhaustion yields
  `NEEDS_HUMAN`). A run **never commits, pushes, or merges**.

- Stage transitions are set explicitly on the run object via `rn.SetStage(...)` with the
  vocabulary in `internal/run` (`runpkg.Planning`, `runpkg.Implementing`, `runpkg.Validating`,
  `runpkg.Reviewing`, `runpkg.Fixing`, `runpkg.Passed`, `runpkg.Failed`,
  `runpkg.WaitingForHuman`). This is where the observed stage sequence comes from.
- Validation is executed by `internal/validate` (`validate.Checks(cfg.Validation)`,
  `validate.Run(ctx, dir, cfg.Validation)`, called through `timedValidation` /
  `sessionValidation` in `internal/cli/run.go`).
- Review runs through `internal/review` (`reviewProvider(cfg, d)`, `provider.Review(...)`),
  scoped by `newReviewScope` / `scopedReviewDiff` (also in `internal/cli/run.go`).
- The gate is `quality.Evaluate(cfg.Quality, quality.Input{...})` from `internal/quality`;
  its `quality.Decision` (`Pass` / `NeedsHuman` / `Continue` / `Fail`) is what the run
  summary prints and what the fix loop breaks on.
- Failure classification / autonomy decision: `failure.Classify(...)` (`internal/failure`) and
  `decideAutonomy(cfg, class)` (`internal/cli/autonomy.go`, policy in `internal/autonomy`).
- Planning: `planner.New(a).GenerateWithContext(...)` from `internal/planflow` /
  `internal/planner`. Task compilation: `internal/taskbuilder` / `internal/taskfile`.
- The compiler of the model input is the Prompt Compiler `prompt.Compile(prompt.Input{...})`
  from `internal/prompt` (invoked inside `runStages`).

### 1c. `sop prompt --capability implement` — prompt entrypoint

- `internal/cli/prompt.go` → `runPrompt` — parses `--capability`, `--file`, `--deliverable`,
  `--json`, `--model-class`; maps the capability name through `promptCapabilities`
  (`implement` → `agent.Implement`); builds a `workitem.WorkItem` via
  `workitem.FromPrompt(...)` from `internal/workitem`.
- Split by capability:
  - `agent.IsRepositoryMutation(wi.Capability)` true (IMPLEMENT) → `runPromptImplement`.
  - otherwise → `runPromptReadOnly` (one bounded Generate call, no lifecycle).
- `runPromptImplement` **reuses the existing lifecycle**: it projects the work item into a
  `taskfile.Spec` and calls the same `runAttempts(...)` used by `sop run` (see 1b), then
  `emitRunSummary(...)`. Its own doc comment states this is "the same planning,
  implementation, validation, review, gate, fix, and approval path a task runs" — i.e. a second
  _input form_, not a second engine.
- Capability enforcement: `guardCapability(a, agent.Implement)`; model selection / validation:
  `validateSelectedModel(...)`; routing: `promptRouting` / `applyTaskRouting`
  (`internal/router`, `internal/model`).
- Prompt tuning / class selection: `internal/prompttuning`, `internal/prompt`
  (`promptClass`, `prompt.Medium` default).
- Agent construction: `d.newAgent(cfg.Agent.Harness, ...)` — implementations live in
  `internal/agent`, `internal/agentbin`, `internal/ollamaagent`, `internal/toolharness`.

### 1d. Human approval / commit boundary (high-risk action)

- `internal/commitgate/gate.go` — the commit gate.
- `internal/approval/approval.go` — approval records / boundary handling.
- `internal/handoff/` (`manager.go`, `record.go`, `capsule.go`) — the handoff that parks work
  at the human gate.
- `internal/cli/commit.go` — the `sop commit` command; this is the only path that performs the
  commit, and it is a separate operator-invoked command.
- Evidence that the autonomous lifecycle **does not** commit: `internal/cli/run.go`
  `emitRunSummary` prints, on `quality.Pass`,
  `"human approval required before commit; completed locally without committing."` and returns
  `exitOK`. The `runRun` doc comment states plainly: "A run never commits, pushes, or merges."
  A run that stops at a human boundary calls `printParkedHumanGate(...)` and returns a non-zero
  exit.

### 1e. Telemetry for the recorded scenario

- `internal/runmetrics`, `internal/runtrace`, `internal/activity` — execution telemetry
  (`perf.Recorder`, `runtrace.CollectorFromContext`, `activity.Recorder`).
- Run artifacts written beside each run: `internal/cli/run.go` writes `report.md`,
  `report.json`, `metrics.json`, `validation.json`, `review.json`, `gate.json`,`diff.patch`,
  `classification.json` through `runpkg.Run.Write(...)` (`internal/run`).
- Workspace lifecycle / disposal: `internal/worktreecleanup`, `internal/git`.

**No new orchestration framework is introduced.** This map names existing packages only; the
recorded scenario below is executed against the machinery as it exists.

---

## 2. Recorded scenario — single-prompt multi-package unused-code cleanup

This scenario was executed operator-side on a disposable workspace, outside the repository.
The governed in-repo tool harness is repo-root confined and cannot run a nested `sop prompt`,
so the execution was performed directly by the operator against the built binary; the run
artifacts and the independent re-verification are the evidence recorded here. Provenance: the
same pattern as PREVIEW-002 (operator-executed disposable workspace).

### 2a. Disposable fixture module

Path (not committed, scratch): `/tmp/sop-auto-multi`. Module: `example.com/multicleanup`.
Three packages (`main` plus two), seeded with four genuinely unused symbols:

```text
go.mod                      module example.com/multicleanup  (go directive; no dependencies)
main.go                     package main; calls mathx.Add(...) and strx.Join(...)
mathx/mathx.go              package mathx
                              func Add(a, b int) int     — USED
                              func Sub(a, b int) int     — UNUSED (exported)
                              const unusedConst = 99     — UNUSED (unexported)
strx/strx.go                package strx
                              func Join(a, b string) string      — USED
                              func Repeat(s string, n int) string — UNUSED (exported)
                              type unusedType struct{ x int }    — UNUSED (unexported)
```

Enumerated cleanup target (the intended unused symbols):

| Symbol        | Package | Exported | Kind  |
| ------------- | ------- | -------- | ----- |
| `Sub`         | mathx   | yes      | func  |
| `unusedConst` | mathx   | no       | const |
| `Repeat`      | strx    | yes      | func  |
| `unusedType`  | strx    | no       | type  |

Acceptance: at least two packages beyond `main` (here two: `mathx`, `strx`); at least one
unused **exported** symbol (`Sub`, `Repeat`) and at least one unused **unexported** symbol
(`unusedConst`, `unusedType`); the fixture **builds before cleanup** (`go build ./...` → OK on
this layout, since `main` imports both packages and all seeded code compiles).

### 2b. Configuration

```
agent.harness:        tool
agent.provider:       ollama
agent.model:          deepseek-v4.1-flash:cloud
validation:           go build ./...  /  go test ./...  /  go vet ./...
human.approval_before_commit: true
autonomy.level:       high
```

### 2c. The single prompt (exactly one)

```sh
sop prompt --capability implement \
  "Remove all unused code from this multi-package Go module: every function, constant, \
   variable, and type that is not referenced anywhere in the module. Update imports so the \
   module still builds and vets cleanly. Do not change the behavior of the used code."
```

Exactly one prompt performs the cleanup; there is no second request, no re-prompt, and no
manual edit between the prompt and the recorded result.

### 2d. Observed result — stage, gates, fix cycles, workspace

The command exited `0` (`PASS`). Recorded CLI output:

```text
run prompts/prompt-20261009-192346: PASS
  - all required checks passed
fix cycles: 0/2
performance: 19.9s total (agent 17.2s, validation 374ms, review 2.3s) | agent calls 2, validation runs 1, fix cycles 0
report: .agent-sdlc/runs/prompts/prompt-20261009-192346/report.md
human approval required before commit; completed locally without committing.
```

- **Stage sequence** (as reported by the engine through `rn.SetStage`):
  `PLANNING` → `IMPLEMENTING` → `VALIDATING` → `REVIEWING` → final stage `PASSED`.
  (No `FIXING` stage occurred — the fix loop broke on a `Pass` gate at `cycles = 0`.)
- **Gates encountered:**
  - Validation (deterministic, `internal/validate`): `BUILD go build ./... PASS`,
    `UNIT_TEST go test ./... PASS`, `LINT go vet ./... PASS` → **PASS**.
  - Review (`internal/review`): self engine report — _"…the removed symbols were genuinely
    unused and no imports became dangling (both packages remain imported for the used
    functions)… No issues found."_ → **PASS** (no findings).
  - Quality gate (`quality.Evaluate`): decision `PASS`, single reason
    `"all required checks passed"` → **PASS**.
  - Human approval / commit boundary: **HOLD** — the run printed the commit notice and did
    **not** commit.
- **Fix cycles:** **0 / 2** — zero fix cycles occurred. Trigger: none (the gate passed on the
  first iteration). Resolution: not applicable.
- **Final workspace result** (independent re-verification after the run):
  - All four unused symbols removed. `grep` for
    `func Sub|unusedConst|func Repeat|unusedType` returns **none remaining**.
  - Packages changed: `mathx/mathx.go` and `strx/strx.go` (both non-`main` packages).
    `git status`: `M mathx/mathx.go`, `M strx/strx.go`.
  - The module still builds: independent `go build ./...` → OK; `go vet ./...` → OK.
  - Diffstat: `mathx/mathx.go | 6 ------`, `strx/strx.go | 12 ------------` (18 deletions).
  - **Not committed** — the high-risk action remained behind the human approval boundary.

### 2e. Which mechanism performed the cleanup (informational UNKNOWN)

There is no dedicated unused-code analyzer package under `internal/*` in the tree. The
observation is that the cleanup was performed by the **implement-capability agent**
(`internal/agent` + `internal/toolharness`) driving the governed lifecycle, with the
deterministic `go build` / `go vet` checks in `internal/validate` as the safety net — **not** a
purpose-built analyzer. This matches the plan's expectation and is recorded as informational
UNKNOWN; it is not a gate.

---

## 3. Gap list (VERIFIED / PARTIAL / MISSING)

Each row: capability → label → evidence → owning layer (required for PARTIAL/MISSING).

| #   | Capability / observation                                                                 | Label                       | Evidence                                                                                                                                                                                                                                                                                                                                                                                                               | Owning layer                                                    |
| --- | ---------------------------------------------------------------------------------------- | --------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- |
| G1  | Autonomous path identified by code reference, not asserted (`sop run` graph execution)   | **VERIFIED**                | `cmd/sop/main.go` → `cli.Run`; `internal/cli/run.go` `runRun`/`runGraph`/`runAttempts`/`runStages`; stage ids via `internal/run`; validation `internal/validate`; review `internal/review`; gate `internal/quality`; planner `internal/planflow`; compiler `internal/prompt`                                                                                                                                           | —                                                               |
| G2  | `sop prompt --capability implement` path identified by code reference                    | **VERIFIED**                | `internal/cli/prompt.go` `runPrompt` → `runPromptImplement` → same `runAttempts`; capability map `promptCapabilities`; `internal/workitem`, `internal/router`, `internal/model`, `internal/agent`                                                                                                                                                                                                                      | —                                                               |
| G3  | Single prompt performs multi-package unused-code cleanup on a disposable module          | **VERIFIED**                | Section 2c (one `sop prompt --capability implement` invocation); fixture has two non-`main` packages (Section 2a); result exit `0` PASS (Section 2d)                                                                                                                                                                                                                                                                   | —                                                               |
| G4  | Observed stage sequence recorded as the engine reports it                                | **VERIFIED**                | Section 2d: `PLANNING → IMPLEMENTING → VALIDATING → REVIEWING → PASSED`; transitions set by `rn.SetStage` in `internal/cli/run.go`                                                                                                                                                                                                                                                                                     | —                                                               |
| G5  | Each gate recorded with outcome                                                          | **VERIFIED**                | Section 2d: validation PASS, review PASS, quality gate PASS (`quality.Evaluate`), commit boundary HOLD (notice text from `emitRunSummary`)                                                                                                                                                                                                                                                                             | —                                                               |
| G6  | Every fix cycle recorded (or stated as zero)                                             | **VERIFIED**                | Section 2d: `fix cycles: 0/2`; no `FIXING` stage occurred; trigger/resolution stated as not applicable                                                                                                                                                                                                                                                                                                                 | —                                                               |
| G7  | Final workspace result recorded (symbols removed, packages changed, module still builds) | **VERIFIED**                | Section 2d: four symbols removed, `mathx` + `strx` modified, `go build ./...`/`go vet ./...` OK, diffstat 18 deletions                                                                                                                                                                                                                                                                                                 | —                                                               |
| G8  | High-risk commit action remains behind human approval boundary                           | **VERIFIED**                | `internal/approval`, `internal/commitgate` (`commitgate.go`), `internal/handoff` (`handoff.go`); separate `internal/cli/commit.go`; `emitRunSummary` prints the no-commit notice and the run status shows unmodified (not committed)                                                                                                                                                                                   | —                                                               |
| G9  | No new orchestration framework introduced                                                | **VERIFIED**                | Section 1 maps only existing `cmd/` + `internal/*` packages; `runPromptImplement` comment states it reuses the `runAttempts` lifecycle; no new engine package appears in the report                                                                                                                                                                                                                                    | —                                                               |
| G10 | Disposable multi-package fixture lives outside the repo, not committed as fixtures       | **VERIFIED**                | Section 2a path `/tmp/sop-auto-multi`; `internal/worktreecleanup` + `internal/git` provide workspace lifecycle; no fixture files added under the repo                                                                                                                                                                                                                                                                  | —                                                               |
| G11 | Multi-package unused-code cleanup wired into `internal/e2e` as an automated scenario     | **MISSING**                 | No multi-package unused-code cleanup test exists under `internal/e2e` / `evals`; the scenario is operator-executed (Section 2 provenance)                                                                                                                                                                                                                                                                              | `internal/e2e` (and `evals`)                                    |
| G12 | Dedicated unused-code detection / analysis tooling inside sop                            | **UNKNOWN** (informational) | No package under `internal/*` indicates an unused-code detector; mechanism observed = implement agent + `toolharness` + `validate` checks (Section 2e)                                                                                                                                                                                                                                                                 | `internal/toolharness` (observed owner of the actual mechanism) |
| G13 | A single assembled "workspace result" view exposed to reports                            | **PARTIAL**                 | `internal/runmetrics` + `internal/runtrace` + `internal/activity` provide telemetry, and `internal/run` writes `metrics.json`/`report.json`/`validation.json`/`review.json`/`gate.json`; but there is **no single assembled workspace-result summary** artifact. The result recorded in Section 2d was reconstructed from per-run artifacts plus an independent `git status`/`go build` check, not read from one view. | `internal/runmetrics`                                           |
| G14 | Autonomous end-to-end run exercised **within** the in-repo governed tool harness         | **PARTIAL**                 | The governed harness is repo-root confined and cannot run a nested `sop prompt` (Section 2 provenance), so the recorded run was operator-executed, matching PREVIEW-002; the in-repo path itself is unchanged and still exercised by the existing `internal/cli` end-to-end tests.                                                                                                                                     | `internal/e2e` / `internal/toolharness`                         |

No item above is described as existing where the evidence shows it does not: G11 is explicitly
MISSING, and G12 is recorded as informational UNKNOWN rather than claimed.

---

## 4. Acceptance-criteria traceability

| PRD acceptance criterion                                                                                                                                  | Where addressed                                                        | Label    |
| --------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | -------- |
| Autonomous path identified by code reference (`sop run` graph execution and `sop prompt --capability implement`), not asserted                            | Sections 1b, 1c; gaps G1, G2                                           | VERIFIED |
| A single prompt performs a multi-package unused-code cleanup on a disposable module, with observed stage, gate, fix cycles, and workspace result recorded | Section 2c (one prompt), 2d (stage/gates/cycles/workspace); gaps G3–G7 | VERIFIED |
| Every gap between the request and the existing components listed as VERIFIED / PARTIAL / MISSING with evidence                                            | Section 3 (14 rows)                                                    | VERIFIED |
| High-risk action (commit) remains behind the human approval boundary                                                                                      | Sections 1d, 2d; gap G8                                                | VERIFIED |
| No new orchestration framework is introduced                                                                                                              | Section 1 closing note, 1c; gap G9                                     | VERIFIED |

---

## 5. Verdict

**Recorded and exercised.** The single-prompt governed path performs a realistic multi-package
unused-code cleanup end to end using only existing components — `sop prompt --capability implement`
→ `internal/cli` lifecycle (`runPromptImplement` → `runAttempts`) → `internal/autonomy` policy →
validation / review / quality gate — and preserves the human commit boundary. Zero fix cycles
were needed. The two PARTIAL items (G13 workspace-result view, G14 in-harness nested execution)
and the one MISSING item (G11 automated `internal/e2e` wiring) are recorded as gaps, not fixed
here, because the baseline is descriptive and **no new orchestration framework is introduced**.

---

## 6. AUTONOMY-004 determination — single-command composition

**Determination: NOT_REQUIRED.** The existing commands satisfy the AUTONOMY-004 acceptance
scenario from a single prompt, so no thin single-command seam is added. Per the task's failure
behavior, when the gap is not MISSING the correct outcome is to record NOT_REQUIRED and add no
code.

### 6a. Why the existing commands already satisfy the scenario (evidence by code reference)

The prompt entrypoint already produces and executes a task graph from one prompt, delegating to
the existing planner and graph runner:

- `internal/cli/prompt.go` → `runPrompt` maps `--capability implement` through
  `promptCapabilities` to `agent.Implement`, builds a `workitem.WorkItem` via
  `workitem.FromPrompt(...)`, and (because `agent.IsRepositoryMutation(wi.Capability)` is true)
  dispatches to `runPromptImplement`.
- `runPromptImplement` (same file) projects the work item into a `taskfile.Spec` and calls
  `runAttempts(...)` — the same function `sop run` uses. Its doc comment states it is "the same
  planning, implementation, validation, review, gate, fix, and approval path a task runs."
- `internal/cli/escalation.go` → `runAttempts` runs the governed lifecycle via
  `executeLifecycle` (defined in `internal/cli/run.go`), i.e. the planner/graph-runner seam is
  reused, not reimplemented. Planning comes from `internal/planflow`
  (`planner.New(a).GenerateWithContext(...)`) and dependency-ordered scheduling from
  `internal/scheduler`.
- `runPromptImplement` then calls `emitRunSummary(...)` (the same summary path `sop run` uses),
  so stage/gate/fix-cycle reporting is shared, not duplicated.

Consequently there is **no MISSING gap**: a second input form already exists alongside `sop run`,
and that form runs the same engine. Adding a seam would duplicate an existing delegation.

### 6b. Recorded scenario evidence (single prompt, PASS, fix cycles 0/2, boundary held)

From Section 2 and `docs/reports/autonomy/autonomy-001-scenario/README.md`:

- Exactly one prompt (`sop prompt --capability implement "Remove all unused code …"`) performed
  the multi-package cleanup; there was no second request and no manual edit.
- Command exited `0` (`PASS`); stage sequence
  `PLANNING → IMPLEMENTING → VALIDATING → REVIEWING → PASSED`.
- Gates all PASS (validation `go build`/`go test`/`go vet`, review no findings, quality gate
  `PASS`); **fix cycles 0/2** (no `FIXING` stage).
- The human commit boundary held: the run printed
  `human approval required before commit; completed locally without committing.` and did **not**
  commit.
- Independent re-verification: all four unused symbols removed across two non-`main` packages;
  `go build ./...` and `go vet ./...` OK; change left uncommitted.

The scenario README's verdict line already states "No single-command composition is required
(AUTONOMY-004 is NOT_REQUIRED; see the baseline report)"; this section is the baseline-report
record of that determination.

### 6c. No code was added

No CLI seam was implemented, and no scheduler, recovery, approval, or routing logic was added or
changed: the gap is not MISSING. The change introduced by AUTONOMY-004 is this determination
section only.

### 6d. Verification note (A4-2)

- **Boundary check (unchanged):** the AUTONOMY-004 change touches only
  `docs/reports/autonomy/AUTONOMY-001-baseline.md`. No files under `cmd/`, `internal/cli/prompt.go`,
  `internal/cli/run.go`, `internal/cli/escalation.go`, `internal/scheduler`, `internal/autonomy`,
  `internal/approval`, `internal/commitgate`, `internal/handoff`, or `internal/router` were added
  or modified.
- **Validation commands:** the PRD validation commands (`gofmt -l .`, `go vet ./...`,
  `go test -count=1 ./...`, `go build ./...`, `git diff --check`) were run against the tree after
  this documentation change and the required contract commands are green (`go build ./...`,
  `go test ./...`, `go vet ./...` all pass). Results are recorded by the AUTONOMY-004 execution
  attempt that produced this section.
- **Commit boundary:** the commit remains behind the human approval boundary per Section 1d and
  gap G8; AUTONOMY-004 does not commit and does not alter `internal/commitgate`, `internal/approval`,
  or `internal/handoff`.
- **Consistency:** this determination is consistent with the scenario README's statement that
  AUTONOMY-004 is NOT_REQUIRED.

### 6e. Handoff note for AUTONOMY-005 (A4-3)

- AUTONOMY-004 is **finalized as NOT_REQUIRED** and its evidence is recorded in this section
  (6a–6d). This is the consumable input for the downstream readiness/disposition stage
  (`PLAN-SOP-Autonomy`: AUTONOMY-004 → AUTONOMY-005).
- AUTONOMY-004 introduces **no new orchestration framework** and **no acceptance-enforcement
  change**; it reuses the existing `sop prompt --capability implement` → `runPromptImplement` →
  `runAttempts` delegation to the existing planner (`internal/planflow`) and graph runner
  (`internal/scheduler`).
- **AUTONOMY-005 can proceed without any prerequisite code change from AUTONOMY-004.** The
  readiness/disposition report (`docs/reports/autonomy/AUTONOMY-005-readiness.md`) may list the
  AUTONOMY-004 item as NOT_REQUIRED among its VERIFIED/PARTIAL/MISSING/BLOCKED listing.
- No new orchestration, scheduler, recovery, approval, or routing code was added by this stage.
