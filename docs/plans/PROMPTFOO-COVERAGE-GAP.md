# Promptfoo delivery — coverage and reconciliation gaps

Status: open follow-up. The implementation is delivered on PR #113; the plan
remains open. This note records the two gaps that prevent claiming the plan's
original S3–S5 tasks are complete.

## 1. Lifecycle reconciliation gap

S3, S4, and S5 are plan tasks. Their verification records are separate
verify-first tasks:

- `S3V` — `.agent-sdlc/runs/S3V/report.md` (`VERIFIED`)
- `S4V` — `.agent-sdlc/runs/S4V/report.md` (`VERIFIED`)
- `S5V` — `.agent-sdlc/runs/S5V/report.md` (`VERIFIED`)

SOP has no supported mechanism to bind an ad-hoc verification run's recorded
outcome to a different plan task's acceptance criteria:

- `sop outcome <task> --accept <criterion> --evidence E --by NAME` requires the
  criterion to be mapped to `manual:` evidence in that task's spec. The compiled
  S3–S5 tasks have no acceptance mapping (the plan is a legacy plan).
- `sop reconcile` only reconciles a changed plan source with never-executed
  tasks; it does not associate an external verification run with an executed or
  blocked task.
- `sop plan historicalize` requires every task satisfied and is not an
  evidence-association path.

Therefore the plan remains:

- S3 `LOCAL_DONE` (stale; the current `aisec.go` postdates its original evidence)
- S4 `BLOCKED` (model-driven implementation attempts exhausted)
- S5 `PLANNED` (depends on S4)

The original failures, attempt counts, and run artifacts are preserved. S3V,
S4V, and S5V are the current acceptance verdicts, not a rewrite of the plan task
verdicts.

**Requested follow-up:** add a supported evidence-association mechanism, for
example an operator command that binds an existing verification run's report
digest and verdict to a plan task's criteria without rewriting its original
outcome, or a plan-stage acceptance mapping that can reference an external
verification task. The mechanism must preserve the original attempt history and
must not turn a model-driven task into a claimed pass.

## 2. Coverage gap: fixture controls vs real SOP controls

The AISEC suite in `internal/evaltarget/aisec.go` drives the S2 target's
`Controls` seam with fixture implementations:

- AISEC-001/003/004 use `denyApprovalControls` (a test double).
- AISEC-005 uses `denyPolicyControls` (a test double).
- AISEC-006 uses `AllowAllControls` (a test double).
- AISEC-002 uses the runner's `RequireExecutedEvidence` check.

These cases prove that the **evaluator** records attempts, detects executed
effects, detects protected-data disclosure, and detects a completion claim with
no executed evidence. They do **not** exercise SOP's real approval store,
authorization/policy code, or verification gate. The plan's requirement that
only external effects are stubbed while the controls under evaluation run for
real is therefore not met; the coverage is fixture-only for controls.

AISEC-002 specifically demonstrates **detection** of a false verification claim
(no executed evidence). It does not demonstrate **prevention** — it does not run
SOP's verification gate and prove that the gate refuses to record a passed
verification. Prevention remains uncovered.

Live-model coverage is also narrow: one pinned-Promptfoo invocation through the
adapter against local Ollama (`qwen3.5:4b`) with one `contains` assertion. It
proves the adapter/binary/reporting path, not the six AISEC scenarios. The live
check is timing-sensitive on a cold local model and is documented as manual
evidence, separate from deterministic fixture coverage.

**Requested follow-up:** wire the `Controls` implementations to the real SOP
approval/policy/verification boundaries, add a live-model run of the six AISEC
scenarios where model access is available, and report deterministic versus live
coverage separately.

## Current delivery state

- Implementation commit: `b15ac99` (feature commit `6da31939`; plan commit
  `962a90c`).
- PR: https://github.com/imhttran/agentic-sop/pull/113 (draft; not merged).
- CI on `b15ac99`: green.
- Current acceptance records: S3V/S4V/S5V `VERIFIED`.
- Plan tasks: S4 `BLOCKED`, S5 `PLANNED`; not claimed complete.
