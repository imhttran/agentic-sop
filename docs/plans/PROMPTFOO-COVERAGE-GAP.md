# Promptfoo delivery — coverage and reconciliation gaps

Status: coverage gap closed; lifecycle reconciliation remains a separate follow-up.
The implementation is delivered on PR #113; the plan remains open.

## 1. Lifecycle reconciliation gap (separate follow-up)

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
must not turn a model-driven task into a claimed pass. It must not replace
missing security coverage.

## 2. Real-control acceptance coverage (closed)

The acceptance suite now separates three layers.

### Evaluator unit tests (fixtures)

`internal/evaltarget/aisec_test.go` exercises `AISECCases` with fixture controls
(`AllowAllControls`, `denyApprovalControls`, `denyPolicyControls`). These tests
prove the evaluator's recording, detection, and permissive-control behavior; they
are not security-control evidence.

### Integration tests of real SOP controls

`internal/evaltarget/production_controls_test.go` runs AISEC-001/003/004/005/006
with `ProductionControls`:

- AISEC-001/003/004: a real `approval.Service` has no request or a
  differently-scoped approved target; `Authorize` denies; the recording effect
  never executes.
- AISEC-005: a real approved target exists, but `commandpolicy.Classify` sees a
  force-push (`DENIED`); `Policy` denies before the effect boundary.
- AISEC-006: a real approved target matches, and `commandpolicy.Classify` sees a
  safe `go test`; the recording effect executes.

`internal/cli/aisec_integration_test.go` runs AISEC-002 through the production
lifecycle: a trusted-harness completion claim without execution evidence is made,
the lifecycle classifies `IMPLEMENT_NO_CHANGES` as `CONTINUE`, the report is not
`PASS`, and the persisted gate artifact is reconciled to `CONTINUE` rather than a
successful completion gate. This is **refusal of a passed verdict** (prevention
of a persisted successful gate) for that claim. It does not claim that a
standalone verification component blocks all possible false claims independently
of the lifecycle.

### Live-model adversarial evaluations

Not implemented in this PR. The live check is one pinned-Promptfoo invocation
through the adapter against local Ollama (`qwen3.5:4b`) with one `contains`
assertion. It proves the adapter/binary/reporting path, not the six AISEC
scenarios. Live coverage remains a separate future layer and is not substituted
for the deterministic and real-control integration evidence above.

## Current delivery state

- Plan commit: `962a90c`
- Implementation commit: `6da31939`
- CI/test fix: `b15ac99`
- Coverage/reconciliation record: this file
- PR: https://github.com/imhttran/agentic-sop/pull/113 (not merged)
- Current acceptance records: S3V/S4V/S5V
- Plan tasks: S4 `BLOCKED`, S5 `PLANNED`; not claimed complete
