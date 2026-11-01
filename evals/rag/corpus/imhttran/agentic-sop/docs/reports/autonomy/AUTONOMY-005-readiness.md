# AUTONOMY-005 — Readiness and disposition

**Status:** readiness report recorded. Descriptive/disposition artifact only — no orchestration code added, no acceptance-enforcement change made.
**Scope:** adjudicate the recorded AUTONOMY-001 baseline (`docs/reports/autonomy/AUTONOMY-001-baseline.md`) and the AUTONOMY-004 NOT_REQUIRED determination (baseline §6) into a single readiness verdict, with a VERIFIED / PARTIAL / MISSING / BLOCKED listing backed by code references re-verified against the working tree.

---

## 1. Verdict

**HOLD.**

The autonomy path is demonstrated end to end by recorded, code-referenced evidence, and the human approval/commit boundary holds. However, a readiness criterion is genuinely unmet: there is **no automated in-repo multi-package unused-code cleanup scenario** (gap G11 is MISSING), and the end-to-end run was **not** exercised within the in-repo governed tool harness (gap G14 is PARTIAL, because the harness is repo-root confined and cannot run a nested `sop prompt`). Per the PRD's failure behavior for this stage, unmet readiness criteria mean the disposition is **HOLD rather than GO**.

Reasons for HOLD (each backed below):

1. G11 MISSING — no multi-package unused-code cleanup test exists under `internal/e2e` / `evals`. The recorded scenario is operator-executed on a disposable workspace outside the repo.
2. G14 PARTIAL — the autonomous end-to-end run was not exercised within the in-repo governed tool harness.

A GO verdict would require closing G11 (or an explicit, recorded operator decision that G11 is out of scope for the activation it would authorize). That closure is **not** performed here; this task reports readiness and recommends HOLD.

---

## 2. VERIFIED items

Each item re-verified against the working tree (file path or code reference).

| # | Item | Status | Evidence (re-verified) |
|---|------|--------|------------------------|
| G1 | `sop run` graph-execution path identified by code reference, not asserted | **VERIFIED** | `cmd/sop/main.go` → `cli.Run`; `internal/cli/run.go` (`runRun`/`runGraph`/`runAttempts`/`runStages`); stage ids via `internal/run`; validation `internal/validate`; review `internal/review`; gate `internal/quality`; planner `internal/planflow`; compiler `internal/prompt` |
| G2 | `sop prompt --capability implement` path identified by code reference | **VERIFIED** | `internal/cli/prompt.go` `runPrompt` → `runPromptImplement` → same lifecycle; capability map `promptCapabilities` (`"implement"` → `agent.Implement`); `internal/workitem`, `internal/router`, `internal/model`, `internal/agent` |
| G3 | Single prompt performs multi-package unused-code cleanup on a disposable module | **VERIFIED** | Baseline §2c (one `sop prompt --capability implement` invocation); fixture has two non-`main` packages (baseline §2a); exit `0` PASS (baseline §2d) |
| G4 | Observed stage sequence recorded as the engine reports it | **VERIFIED** | Baseline §2d: `PLANNING → IMPLEMENTING → VALIDATING → REVIEWING → PASSED`; transitions set by `rn.SetStage` in `internal/cli/run.go` |
| G5 | Each gate recorded with outcome | **VERIFIED** | Baseline §2d: validation PASS, review PASS, quality gate PASS (`quality.Evaluate`), commit boundary HOLD (notice text from `emitRunSummary`) |
| G6 | Every fix cycle recorded (or stated as zero) | **VERIFIED** | Baseline §2d: `fix cycles: 0/2`; no `FIXING` stage occurred; trigger/resolution stated as not applicable |
| G7 | Final workspace result recorded (symbols removed, packages changed, module still builds) | **VERIFIED** | Baseline §2d: four symbols removed, `mathx` + `strx` modified, `go build ./...` / `go vet ./...` OK, diffstat 18 deletions |
| G8 | High-risk commit action remains behind the human approval boundary | **VERIFIED** | `internal/approval/approval.go` (package `approval`) present; `internal/commitgate/gate.go` (package `commitgate`) present; `internal/handoff/` (package `handoff`, e.g. `record.go`, `manager.go`) present; separate `sop commit` in `internal/cli/commit.go`; `internal/cli/run.go` `emitRunSummary` prints the no-commit notice |
| G9 | No new orchestration framework introduced | **VERIFIED** | Baseline §1 maps only existing `cmd/` + `internal/*` packages; `runPromptImplement` is an input form over `runAttempts`, not a second engine (see §4) |
| G10 | Disposable multi-package fixture lives outside the repo, not committed as fixtures | **VERIFIED** | Baseline §2a path `/tmp/sop-auto-multi`; `internal/worktreecleanup` + `internal/git` provide workspace lifecycle; no fixture files added under the repo |
| — | `sop prompt --capability implement` → `runPromptImplement` → `runAttempts` delegation present (no duplicate engine) | **VERIFIED** | `internal/cli/prompt.go` doc comment ("the governed implementation lifecycle … exactly as a task runs") and `runPromptImplement` calls `runAttempts(...)`; `internal/cli/escalation.go:76` defines `func runAttempts(...)` |
| — | AUTONOMY-004 single-command composition determination | **VERIFIED (determination recorded)** | Baseline §6: **NOT_REQUIRED**; §6a evidence by code reference (`runPrompt` → `runPromptImplement` → `runAttempts`; planner `internal/planflow`; scheduler `internal/scheduler`); §6c records that no code was added; §6e is the handoff note to AUTONOMY-005 |

---

## 3. PARTIAL items

| # | Item | Status | Evidence | Owning layer |
|---|------|--------|----------|--------------|
| G13 | A single assembled "workspace result" view exposed to reports | **PARTIAL** | `internal/runmetrics` + `internal/runtrace` + `internal/activity` provide telemetry, and `internal/run` writes `metrics.json` / `report.json` / `validation.json` / `review.json` / `gate.json`; but there is **no single assembled workspace-result summary** artifact. The readiness evidence is assembled from per-run artifacts plus independent `git status` / `go build` re-verification, not read from one view. | `internal/runmetrics` |
| G14 | Autonomous end-to-end run exercised **within** the in-repo governed tool harness | **PARTIAL** | The governed harness is repo-root confined and cannot run a nested `sop prompt`, so the recorded run was operator-executed on a disposable workspace (matching PREVIEW-002); the in-repo `internal/cli` path itself is unchanged and still exercised by existing `internal/cli` end-to-end tests. | `internal/e2e` / `internal/toolharness` |

---

## 4. MISSING items

| # | Item | Status | Evidence | Owning layer |
|---|------|--------|----------|--------------|
| G11 | Multi-package unused-code cleanup wired into `internal/e2e` as an automated scenario | **MISSING** | No multi-package unused-code cleanup test exists under `internal/e2e` / `evals`; the recorded scenario is operator-executed (baseline §2 provenance). This task reports the gap and does **not** create the missing scenario. | `internal/e2e` (and `evals`) |

The absence of the automated scenario is asserted here only because it was re-verified by inspection during AUTONOMY-005-A, and it is recorded in baseline gap G11.

---

## 5. BLOCKED / informational items

There is **no item classified BLOCKED** in the sense of a hard external blocker: every recorded gap is either MISSING (G11, reportable) or PARTIAL (G13, G14), and G12 is informational only. The blocking-class item is G11, because it is a readiness criterion that cannot be met by the recorded evidence alone and is not closed by this documentation-only task.

| # | Item | Status | Evidence | Owning layer |
|---|------|--------|----------|--------------|
| G12 | Dedicated unused-code detection / analysis tooling inside sop | **UNKNOWN** (informational — *not* claimed as existing) | No package under `internal/*` indicates a purpose-built unused-code detector. The observed mechanism is the implement-capability agent (`internal/agent` + `internal/toolharness`) with the deterministic `go build` / `go vet` checks in `internal/validate` as the safety net (baseline §2e). | `internal/toolharness` (observed owner of the actual mechanism) |
| G11 | (blocking-class) automated in-repo scenario absent | **MISSING / BLOCKED-class** | See §4. Weighs directly on the HOLD verdict; not closed by this task. | `internal/e2e` (and `evals`) |

No item above is described as existing where the evidence shows it does not: G11 is explicitly MISSING, and G12 is recorded as informational UNKNOWN rather than claimed.

---

## 6. Constraints asserted by this report

- **No new framework is claimed.** This report names only existing `cmd/` + `internal/*` packages and the recorded baseline gaps. `sop prompt --capability implement` is an *input form* over the existing `runAttempts` lifecycle, not a second engine; the delegation is confirmed by code reference (`internal/cli/prompt.go` → `runPromptImplement` → `runAttempts`, defined at `internal/cli/escalation.go:76`).
- **No acceptance-enforcement change is claimed.** No validation, gate, review, routing, approval, commit, or recovery behavior was added or modified by this task.
- **No plan is activated, completed, or historicalized by this task.** `docs/plans/PLAN-SOP-Autonomy.md` remains **PROPOSED**; `.agent-sdlc/plan.json` and `.agent-sdlc/plan.meta.json` are SOP-owned state and are untouched by this documentation-only task. SOP, not AUTONOMY-005, owns activation/closure/historialization.

---

## 7. Disposition

**HOLD.** The recorded, code-referenced evidence is sufficient to describe the autonomous path and confirm its human approval/commit boundary, but the readiness criterion represented by gap G11 (automated in-repo multi-package unused-code cleanup scenario) is **MISSING**, and the end-to-end run is **PARTIAL** with respect to in-harness execution (G14). Per the PRD failure behavior, unmet readiness criteria yield **HOLD**, not GO. Closing G11 (and, optionally, G13/G14) in an owning layer is a prerequisite to a GO disposition; that closure is out of scope for this documentation-only readiness task.

---

## 8. Validation and boundary closure (AUTONOMY-005-C)

Validation commands and the diff boundary for this documentation-only change are recorded in the executing attempt's evidence for this stage (PRD validation commands: `gofmt -l .`, `go build ./...`, `go test -count=1 ./...`, `git diff --check`; plus the already-satisfied contract commands `go build ./...`, `go test ./...`, `go vet ./...`). The change introduced by AUTONOMY-005 is confined to `docs/reports/autonomy/AUTONOMY-005-readiness.md`. No file under `cmd/`, `internal/cli`, `internal/scheduler`, `internal/autonomy`, `internal/recovery`, `internal/continuation`, `internal/approval`, `internal/commitgate`, `internal/handoff`, or `internal/router` was added or modified, and no `.agent-sdlc` state or plan status was changed. This is consistent with the no-new-framework and no-acceptance-enforcement-change claims in §6.
