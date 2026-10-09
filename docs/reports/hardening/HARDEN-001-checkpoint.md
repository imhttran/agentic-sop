# HARDEN-001 Checkpoint — Trusted Acceptance-Criterion Verification

**Branch:** `harden-001` (isolated worktree `sop-harden-001/`, based on `d9f7722`)
**Status:** checkpoint — implementation complete for 001b/001c/001d; enforcement **default OFF**; deferred items below.
**Scope:** HARDEN-001b (trusted criterion verification), HARDEN-001c (gate integration plumbing), HARDEN-001d (completion-boundary re-check + integration regressions).

## 1. Implementation summary

- **`internal/criteria`** (new): operator-owned verifier bindings (`ParseBindings`/`ParseBindingSpecs` — the only source of authorised bindings), trusted execution (shared `toolharness` command policy, no shell, bounded time/output), `MET`/`NOT_MET`/`UNAVAILABLE` classification, machine-readable `Evidence` (task/attempt/revision/workspace/workspace-state/bindings-digest/per-outcome verifier-digest), `WorkspaceDigest` (includes untracked files, excludes `.agent-sdlc`/`.git`), `Valid`/`AllMetVerified`/`Fresh`, and `WriteEvidence` (`criteria.json`, audit-only).
- **`internal/config`**: additive `verification:` section (`enforce` + `bindings`); `enforce` is a **temporary migration switch, default OFF**.
- **`internal/quality`**: `Input.Acceptance` + deterministic block (unsatisfied/enforced ⇒ failure) preserving severity/FIX/human rules.
- **`internal/cli/run.go`**: `evaluateAcceptance` runs operator verifiers after IMPLEMENT, before `quality.Evaluate`; returns the trusted gate input **and the observed workspace fingerprint**; `lifeResult` carries the context through the PASS return.
- **`internal/cli/drive.go`**: `completeTask` recomputes `criteria.WorkspaceDigest` **immediately before LOCAL_DONE** and refuses when enforcement is on and verification is missing or the fingerprint changed; both persisted completion paths (graph run + resume) are behind the re-check. Resume fails closed.
- **`internal/cli/task.go`**: `task complete --external` fails closed while enforced when the task declares required criteria.
- **`internal/cli/cli.go`**: `deps.afterVerify` deterministic test hook.
- `criteria.json` is **audit-only**; the gate and completion boundary use the trusted in-memory result. `criteria.json` can never authorise completion.

## 2. Changed files (all hardening; none unrelated)

Modified: `internal/cli/cli.go`, `internal/cli/drive.go`, `internal/cli/run.go`, `internal/cli/task.go`, `internal/config/config.go`, `internal/quality/quality.go`.
Added: `internal/criteria/` (`criteria.go`, `criteria_test.go`, `specs_test.go`, `workspace_test.go`), `internal/config/verification_test.go`, `internal/quality/acceptance_test.go`, `internal/cli/acceptance_enforcement_test.go`, this report.

No `.agent-sdlc` state, no `docs/plans` in this worktree, no unrelated or user-owned changes.

## 3. Validation evidence (this checkpoint)

`gofmt -l .` clean; `go vet ./...` clean; `go build ./...` OK; `go test -count=1 ./...` all packages ok; `go test -race -count=1 ./...` all packages ok; `git diff --check` clean. Focused `-run TestAcceptance ./internal/cli/` → 11/11 PASS.

**Intermittent (recorded, not discarded):** `TestCrossModuleTimeoutAndCancellation` failed once under `-race` earlier in this work (pre-existing, cross-module decision-provider process/PID test, unrelated to acceptance); it passes on reruns and in this checkpoint run.

## 4. Completed / partial / deferred

**Completed (end-to-end):** trusted verifier execution + authorization provenance; deterministic gate enforcement; completion-boundary re-check (workspace changed after verification ⇒ blocked); external completion fail-closed; forged `criteria.json` cannot authorise; no-criteria tasks unchanged; first-attempt identity (asserted `attempt=1`/`attempt_id=T001-a1`); severity/human/FIX preservation (unit).

**Partial (unit-tested, not integrated):** #7 wrong task/attempt/revision/workspace/digest; #9 timeout/cancellation; #11 human boundary; #12 FIX exhaustion — covered at the gate/criteria unit level.

**Deferred (Phase 4E):** dedicated resume/alternate-path test (#4); retry/continuation multi-attempt identity test (#7).

**Deferred (Phase 5):** incremental migration of the legacy acceptance-criteria fixture corpus (14 files / ~95 `seedTask` sites) with real operator bindings — **not started**.

## 5. Governance

- The active reliability assessment plan (`plan-sop-end-to-end-reliability`, **ACTIVE**) and its historical evidence are preserved and untouched.
- Enforcement remains **default OFF**: no existing behavior changes.
- **Recommended disposition:** **HOLD / deferred** — this hardening is a checkpoint on a branch, not an activation. Do not mark incomplete work PASS/`LOCAL_DONE`; do not complete or historicalize any plan without explicit operator approval.
- SOP lifecycle: no `plan activate`/`complete`/`historicalize` was run for this hardening (it has its own proposed plan `PLAN-SOP-E2E-Reliability-Hardening.md`, still PROPOSED).

## 6. Transition

- Next proposed workstream: **Developer Preview** (`PREVIEW-*`), independent of this experimental hardening (enforcement default OFF).
- Critical correctness preconditions for the preview: see the preview plan; no blocker identified in this checkpoint beyond the deferred items above.
