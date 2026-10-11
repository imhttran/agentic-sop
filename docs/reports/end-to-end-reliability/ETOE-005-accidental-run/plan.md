# Implementation Plan

## Project

ETOE-005 — Disposable dogfood fixture and deterministic baseline

## Summary

The repository already contains a complete ETOE-005 implementation: a reproducible, disposable fixture (setup/run/teardown scripts) that runs outside any production checkout, plus a controlled offline command-agent that makes the lifecycle deterministic, and the report docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md that records setup/teardown, the run-id surface, and the three predeclared expected outcomes. The only remaining gap against the PRD acceptance criteria is that the report's §5.1 still shows PENDING concrete run IDs (acceptance criterion 3 is self-reported as PARTIALLY MET), because the governed harness is repository-root confined and cannot execute the fixture project under /tmp. The plan below is therefore a corrective/finishing plan: (A) verify the existing fixture scripts and controlled agent against SOP's actual CLI surfaces (init, status, run --task, report, approvals --json, the command-agent contract, and the .agent-sdlc/runs/<id> artifact layout) so the fixture only calls capabilities that exist; (B) verify the fixture is genuinely disposable and isolated (refuses in-checkout and unsafe roots, no production-checkout mutation); (C) confirm the three-task contract with predeclared expected outcomes; (D) confirm the deterministic failure path is pinned by config-driven validation rather than model interpretation; and (E) record operator-run concrete run IDs and observed outcomes into the report/records file, or — where the harness cannot execute the fixture — keep a non-fabricated PENDING/UNAVAILABLE provenance note with the exact scripted capture procedure, so criterion 3 is honestly satisfied to the extent the environment permits. Stages are decomposed along the natural boundaries of this work: environment/contract verification, isolation verification, fixture task contract, deterministic failure path, and baseline record finalization. No stage requires any capability whose status is UNKNOWN.

## Capabilities

### sop init initializes a disposable SOP project — EXISTS

- Location: scripts/etoe-005-fixture-setup.sh; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §3.1–§3.2
- Owner: SOP CLI (cmd/, internal/ orchestrator)
- Evidence: scripts/etoe-005-fixture-setup.sh runs `"$SOP_PATH" init` in the fixture root and then asserts `.agent-sdlc/state.db` and `.agent-sdlc/config.yaml` exist (setup steps 1 and 1a). Report §3.2 records the expected `sop init` layout (.agent-sdlc/state.db, .agent-sdlc/config.yaml).

### sop run --task executes a single named task — EXISTS

- Location: scripts/etoe-005-fixture-run.sh; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §3.3
- Owner: SOP CLI (cmd/, internal/ scheduler/execution)
- Evidence: scripts/etoe-005-fixture-run.sh probes `"$SOP_PATH" run` usage for `--task` before use and aborts with a pointer to the source build when absent; the run script then invokes `"$SOP_PATH" run --task "$taskfile"` for each of the three tasks. Report §3.3 records the same invocation order.

### sop report / sop status / sop approvals --json evidence surfaces — EXISTS

- Location: scripts/etoe-005-fixture-run.sh; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §5
- Owner: SOP CLI (cmd/, internal/ run artifacts and approval surface)
- Evidence: scripts/etoe-005-fixture-run.sh reads run artifacts under `.agent-sdlc/runs/<id>/` (state.json, validation.json, report.json) and appends `"$SOP_PATH" approvals --json` to the records file, asserting a PENDING approval is present. Report §5 documents the run-id surface (.agent-sdlc/runs/<id>/state.json, report.json, trace.json, metrics.json, validation.json, review.json, task.md; `sop report [run-id]`; `sop status`; `sop approvals --json`).

### Command-agent harness contract (SOP_AGENT_HARNESS=command / SOP_AGENT_PROVIDER=command / SOP_AGENT_COMMAND) — EXISTS

- Location: scripts/etoe-005-fixture-agent.sh; internal/agent/command.go; scripts/etoe-005-fixture-run.sh
- Owner: SOP agent harness (internal/agent)
- Evidence: scripts/etoe-005-fixture-agent.sh reads a JSON Request on stdin, writes raw output on stdout, and emits a structured outcome JSON (status/summary/reason/changes_expected) for mutating capabilities; its header cites the contract source `internal/agent/command.go`. scripts/etoe-005-fixture-run.sh selects it via SOP_AGENT_HARNESS=command SOP_AGENT_PROVIDER=command SOP_AGENT_COMMAND="bash '$AGENT_SCRIPT' $mode".

### Repository-root confined governed tool harness (blocks fixture execution from inside the checkout) — EXISTS

- Location: internal/toolharness/files.go; internal/toolharness/harness.go; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §Scope, §5.1, §7
- Owner: SOP tool harness (internal/toolharness)
- Evidence: docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §Scope/§5.1/§7 states the governed implementation harness is repository-root confined and cites `internal/toolharness/files.go` and `internal/toolharness/harness.go` as rejecting `../` escapes and unauthorized roots; hence fixture setup/run/teardown is an operator-run step outside the harness.
- Gap: This confinement means the governed agent cannot itself create or execute the fixture at /tmp/etoe-005-fixture, so concrete run-ID values cannot be produced from inside the harness.
- Resolution: Plan the baseline-record stage so concrete run IDs are captured by the operator-run scripted step (scripts/etoe-005-fixture-run.sh writing <fixture-root>/etoe-005-run-records.txt); where that execution is unavailable, record PENDING/UNAVAILABLE with the exact capture procedure rather than fabricating values (report §5.1 pattern).

### Deterministic config-driven validation gate (go build/go test/go vet) — EXISTS

- Location: scripts/etoe-005-fixture-setup.sh; scripts/etoe-005-fixture-run.sh; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §4, §4.1
- Owner: SOP validation/quality subsystem driven by fixture .agent-sdlc/config.yaml
- Evidence: scripts/etoe-005-fixture-setup.sh intentionally replaces `.agent-sdlc/config.yaml` with a complete deterministic config (validation build/test/lint = go build ./..., go test ./..., go vet ./...; human.approval_before_commit: true; workflow.mode: local) and validates it immediately with `sop status`. The always-failing `TestBroken` is shipped in `broken_test.go.disabled` and enabled by the run script, so `go test ./...` exits non-zero deterministically. Report §4.1 explains this.

### Human approval boundary (human.approval_before_commit: true) producing a pending approval — EXISTS

- Location: scripts/etoe-005-fixture-setup.sh; scripts/etoe-005-fixture-agent.sh; scripts/etoe-005-fixture-run.sh; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §4
- Owner: SOP human-approval subsystem (internal/ approval)
- Evidence: The fixture config sets `human.approval_before_commit: true`; the controlled agent's `gate` mode returns `{"status":"needs_human",...}`; the run script treats expected=gated as MATCH when stage is WAITING_FOR_HUMAN or FAILED and asserts a PENDING entry appears in `sop approvals --json`. Report §4 predeclares the FIX-GATE expected outcome as a pending human approval.

### Disposable-root isolation and safety guards for setup/run/teardown — EXISTS

- Location: scripts/etoe-005-fixture-setup.sh; scripts/etoe-005-fixture-run.sh; scripts/etoe-005-fixture-teardown.sh; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §2, §3.4
- Owner: ETOE-005 fixture operator scripts (scripts/)
- Evidence: All three scripts resolve the fixture root physically and refuse roots outside the disposable base root and roots inside the resolved checkout root; setup also refuses the base root itself, run refuses paths containing `..`, and teardown refuses unsafe paths (/ , /tmp, /var, /usr, /etc, /Users) and is idempotent. Report §2 and §3.4 document these guards.

### Concrete ETOE-005 run IDs recorded in the baseline report — MISSING

- Location: docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §5.1, §9
- Owner: Operator execution of scripts/etoe-005-fixture-run.sh outside the repository-root-confined harness; the report/records file is the owning artifact.
- Evidence: docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §5.1 records Run ID and Observed outcome as `PENDING — captured by etoe-005-fixture-run.sh` for all three tasks, and §9 acceptance criterion 3 is self-reported as `PARTIALLY MET — ... concrete run-ID values require one operator execution of the scripted run step`.
- Gap: The report carries no concrete run-ID values or observed outcomes for FIX-SUCCESS, FIX-FAIL, FIX-GATE; criterion 3 is only partially met.
- Resolution: Add a baseline-record finalization stage that runs the scripted capture (or, when the governed harness cannot execute the fixture project, records PENDING/UNAVAILABLE with the exact capture command and provenance) so the recorded run IDs and observed outcomes are sufficient to reproduce the baseline without fabricating values.

### Go toolchain for building the source sop binary — EXISTS

- Location: go.mod; README.md; docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §1
- Owner: Development environment (developer workstation / CI image)
- Evidence: README.md Requirements list Go (see go.mod) and Git; report §1 records `go build -o /tmp/sop-etoe005 ./cmd/sop` and observes go1.27.1; the fixture go.mod pins `go 1.21`.

### Git available for fixture change detection — EXISTS

- Location: scripts/etoe-005-fixture-setup.sh; scripts/etoe-005-fixture-run.sh; README.md
- Owner: Development environment (developer workstation / CI image)
- Evidence: scripts/etoe-005-fixture-setup.sh checks `command -v git` and runs `git init -q` plus an initial baseline commit in the fixture; scripts/etoe-005-fixture-run.sh records the git baseline revision. README.md Requirements list Git.

### Python 3 available for structured evidence parsing in the run script — EXISTS

- Location: scripts/etoe-005-fixture-run.sh
- Owner: Development environment (developer workstation / CI image)
- Evidence: scripts/etoe-005-fixture-run.sh requires `command -v python3` and uses python3 heredocs to parse state.json/validation.json/report.json into structured `stage=... validation=... gate=...` summaries and JSONL, and to compute the MATCH/MISMATCH verdict.

## Assumptions

### The work is a corrective/finishing pass over the existing ETOE-005 implementation rather than a greenfield build; existing scripts and report are preserved and extended, not rewritten.

- Evidence: Repository already contains scripts/etoe-005-fixture-setup.sh, scripts/etoe-005-fixture-run.sh, scripts/etoe-005-fixture-teardown.sh, scripts/etoe-005-fixture-agent.sh, and docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md with a complete acceptance matrix (report §9).
- Consequence: Stages focus on verifying existing fixtures/scripts against real SOP surfaces, confirming isolation and expected outcomes, and finalizing the baseline record, avoiding out-of-scope reimplementation.

### The run-id/observed-outcome values must never be fabricated; where the governed harness cannot execute the fixture project, PENDING/UNAVAILABLE with the exact capture procedure is the correct recorded state.

- Evidence: Report §3.3 step 4 ('never invents a value: anything that cannot be read is recorded as UNAVAILABLE'), §5.1 (PENDING entries with reasons), and §9 criterion-3 finding.
- Consequence: The finalization stage must record only values actually read from .agent-sdlc/runs/<id>/ or sop output; otherwise it records the capture command and non-fabricated provenance, and criterion 3 is reported honestly rather than claimed as fully met.

### The fixture must be created and executed outside the production checkout because the governed harness is repository-root confined.

- Evidence: Report §Scope, §5.1, and §7 cite internal/toolharness/files.go and internal/toolharness/harness.go rejecting `../` escapes and unauthorized roots; setup/run scripts refuse roots inside the resolved checkout root.
- Consequence: Fixture creation/execution is an operator-run scripted step; it is not a repository-inspection prerequisite for the plan, and the report continues to document it as such.

### Determinism comes from pinning the fixture's own complete .agent-sdlc/config.yaml (deterministic local validation) plus a controlled offline command agent, not from any model or network service.

- Evidence: scripts/etoe-005-fixture-setup.sh intentionally replaces config.yaml with deterministic validation and validates it via `sop status`; scripts/etoe-005-fixture-agent.sh is offline and mode-driven; report §4.1 and the setup header note the controlled offline agent.
- Consequence: No external model/provider capability is required by any stage; the baseline reproduces deterministically from the pinned config and controlled agent.

### The three fixture tasks and their predeclared expected outcomes (success, intentional validation failure, human-gated) already match the PRD and should be preserved verbatim.

- Evidence: tasks/FIX-SUCCESS.md, tasks/FIX-FAIL.md, tasks/FIX-GATE.md and docs/PLAN.md are written by setup step 6 with 'Expected outcome (recorded before execution)' sections; report §4 tabulates them.
- Consequence: The verification stage confirms the contract rather than redesigning the tasks; terminology (FIX-SUCCESS / FIX-FAIL / FIX-GATE; completed / LOCAL_DONE; validation FAIL / BLOCKED; needs_human) is preserved.

## ETOE005-P0 — Development environment and CLI-surface contract verification

Confirm the development environment and the exact SOP CLI/agent surfaces the ETOE-005 fixture depends on exist and behave as the scripts assume, before relying on them for baseline record finalization. Inspect cmd/ and internal/ call paths for: `sop init` (creating .agent-sdlc/state.db and .agent-sdlc/config.yaml), `sop status`, `sop run --task <file>`, `sop report [run-id]`, and `sop approvals --json`; inspect the command-agent contract in internal/agent/command.go (stdin JSON request, stdout raw output, structured outcome JSON status/summary/reason/changes_expected); and inspect the .agent-sdlc/runs/<id>/ artifact layout (state.json, validation.json, report.json) that the run script parses. Record any surface the scripts probe for but the code does not provide, and adjust the scripts' probe/abort paths accordingly. This is repository inspection and contract discovery work, not a runtime prerequisite.

### Dependencies

None

### Deliverables

- A recorded CLI/agent-surface verification note (in the ETOE-005 report or an adjacent section) mapping each script dependency to EXISTS evidence: init, status, run --task, report, approvals --json, command-agent contract, run-artifact layout.
- Any corrective edit to scripts/etoe-005-fixture-setup.sh or scripts/etoe-005-fixture-run.sh probe/abort paths if a probed surface name, flag, or artifact filename differs from what the code provides.

### Acceptance Criteria

- Each surface the fixture scripts depend on is traced to concrete code or CLI evidence and classified EXISTS or corrected; no surface is asserted without evidence.
- The `run --task` usage probe and the `approvals --json` invocation match the actual CLI surface (or the scripts' fallback-to-source-build guidance is updated to match).
- The run script's parsed artifact filenames (state.json, validation.json, report.json) match the artifact names SOP actually writes under .agent-sdlc/runs/<id>/.
- The structured outcome JSON fields the controlled agent emits (status/summary/reason/changes_expected) match internal/agent/command.go's parsed fields.

## ETOE005-P1 — Fixture isolation and disposability verification

Verify that setup/run/teardown create and remove a fixture that is strictly outside any production checkout and outside the disposable base root itself, and that no in-repo side effect occurs. Inspect and, where needed, exercise the guards in scripts/etoe-005-fixture-setup.sh (strict-descendant-of-base and in-checkout refusal, symlink-resolved CHECKOUT_ROOT, base-root refusal), scripts/etoe-005-fixture-run.sh (base-root only, `..` refusal, initialized-project check), and scripts/etoe-005-fixture-teardown.sh (unsafe-path refusal, in-checkout refusal, idempotent missing-root exit 0). Confirm the setup script's intentional config.yaml replacement is validated by `sop status` and that the fixture's git baseline is local and disposable.

### Dependencies

- ETOE005-P0

### Deliverables

- A recorded isolation verification note enumerating each guard and its evidence (path-comparison logic, symlink resolution, refusal messages, unsafe-path list).
- Corrective edits to any guard found to permit a root inside the checkout, the base root itself, a `..` escape, or an unsafe teardown path.
- Confirmation note that setup's config.yaml replacement follows docs/reference/CONFIGURATION.md and is machine-validated by `sop status` at setup time.

### Acceptance Criteria

- A fixture root resolving inside the resolved production checkout root is refused by setup, run, and teardown.
- A fixture root equal to the disposable base root (or outside it) is refused, and a path containing `..` is refused by the run script.
- Teardown refuses each documented unsafe path and exits 0 when the fixture root is already absent.
- Setup's replaced .agent-sdlc/config.yaml passes `sop status` (config schema accepted, no unknown keys).
- No script writes to the production checkout or its .agent-sdlc state.

## ETOE005-P2 — Three-task fixture contract with predeclared expected outcomes

Verify that the fixture contains exactly one success task, one intentionally failing task, and one human-gated task, and that each task's expected outcome is declared in the fixture files before any execution. Confirm setup step 6 writes tasks/FIXTURE.md, tasks/FIX-SUCCESS.md, tasks/FIX-FAIL.md, tasks/FIX-GATE.md, and docs/PLAN.md with the 'Expected outcome (recorded before execution)' declarations, and that the report §4 table matches those fixture files exactly (task name, kind, expected outcome). Correct any drift between the fixture files, docs/PLAN.md, and report §4.

### Dependencies

- ETOE005-P0

### Deliverables

- A recorded task-contract verification note cross-referencing each fixture task file to its declared expected outcome and to report §4.
- Corrective edits to setup-written task/PLAN content or report §4 where the declared outcome, task kind, or task count diverges.

### Acceptance Criteria

- Setup produces exactly three tasks: FIX-SUCCESS (success), FIX-FAIL (intentional fail), FIX-GATE (human-gated).
- Each task file and docs/PLAN.md declares its expected outcome before execution, using the PRD terminology (completed / LOCAL_DONE; validation FAIL / BLOCKED; needs_human / pending human approval).
- report §4 matches the fixture files one-to-one with no added or removed tasks.
- The fixture writes no task whose expected outcome is undeclared.

## ETOE005-P3 — Deterministic intentional-failure and human-gate path verification

Verify that the intentional failure is produced deterministically by the pinned config's validation commands rather than by model interpretation, and that the human-gated task reaches the approval boundary without bypassing it. Confirm the always-failing test is shipped disabled (broken_test.go.disabled) and enabled by the run script before FIX-FAIL and disabled before FIX-SUCCESS; confirm the run script's expected-outcome verdicts (completed for FIX-SUCCESS, not-completed for FIX-FAIL, gated for FIX-GATE) derive from structured artifacts (stage/validation) and the approval listing, not substring matches on raw prose; and confirm the controlled agent's mode selection (success/fail/gate) is fixed by the run script per task.

### Dependencies

- ETOE005-P0
- ETOE005-P2

### Deliverables

- A recorded determinism note showing the failure is caused by `go test ./...` failing (TestBroken t.Fatalf) under the pinned fixture config, independent of any model.
- A recorded gate note showing FIX-GATE yields a pending approval via `sop approvals --json` and needs_human handling, not a bypassed gate.
- Corrective edits to run-script enabling/disabling of broken_test.go or to verdict logic where a verdict could mismatch on non-structured evidence.

### Acceptance Criteria

- broken_test.go is enabled only for FIX-FAIL and disabled for FIX-SUCCESS, driven by the run script, and TestBroken fails unconditionally.
- The FIX-FAIL verdict is computed from the run's structured validation/stage artifacts (FAIL / not PASSED), not from a raw substring match.
- The FIX-GATE verdict requires a PENDING approval in `sop approvals --json`; the run script exits non-zero if no pending approval is found.
- The controlled agent requires no network, model, or cloud service and its behavior is pinned per task by mode arguments.

## ETOE005-P4 — Baseline record finalization (concrete run IDs and observed outcomes)

Finalize docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md so the recorded run IDs and expected outcomes are sufficient to reproduce the baseline. Capture concrete values by the scripted procedure (scripts/etoe-005-fixture-run.sh writing <fixture-root>/etoe-005-run-records.txt and `sop approvals --json`), and record them in report §5.1 and, where observed outcomes diverge from predeclared expectations, in §6. Where the governed harness cannot itself execute the fixture project (repository-root confinement), keep non-fabricated PENDING/UNAVAILABLE entries with the exact capture command, binary provenance, and reason, and update §9's acceptance matrix to reflect the actual status honestly. Confirm §7's mutation statement and §8's reproduction steps still match the scripts.

### Dependencies

- ETOE005-P1
- ETOE005-P3

### Deliverables

- Updated docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md §5.1 with concrete run IDs and observed outcomes (or PENDING/UNAVAILABLE plus the exact capture command and reason where execution is not possible from the harness).
- Updated §6 discrepancy table populated from observed records, or an explicit 'no discrepancies observed' entry consistent with the records file.
- Updated §9 acceptance matrix reflecting the true status of criterion 3 (MET when concrete run IDs are recorded; otherwise PARTIALLY MET with the outstanding operator step and reason stated).
- A recorded statement of binary provenance (installed `sop` vs source build from HEAD) and the records-file path/format used for the capture.

### Acceptance Criteria

- Every run ID and observed outcome recorded in the report is read from .agent-sdlc/runs/<id>/ artifacts, `sop report`, or `sop approvals --json` — no fabricated values.
- The report states the exact, reproducible commands (setup, run, records inspection, teardown) and the records file format (# format: <task> | <run-id> | <observed outcome> | <surface>).
- Any divergence between predeclared expected outcomes (§4) and observed outcomes is recorded in §6 with a reason; §4 is not edited after observation.
- Criterion 3's status in §9 matches the evidence: fully MET only when concrete run IDs and observed outcomes are present; otherwise honestly reported as PARTIALLY MET with the outstanding operator step.
- The reproduction steps (§8) and mutation statement (§7) remain accurate against the current scripts.

