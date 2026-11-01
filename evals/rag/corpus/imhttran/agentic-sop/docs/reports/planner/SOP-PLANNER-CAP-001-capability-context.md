# SOP-PLANNER-CAP-001 — Per-task planner capability context

**Status:** FIXED_WITH_NOTES\
**Scope:** `agentic-sop` only (planning/compiler capability handling).\
**Issue:** SOP-PLANNER-CAP-001 — the per-task planner lacks compiled capability
context, causing recurring capability repair.\
**Baseline:** `main` at `d307138b9101b700ae471d1b8e608845640cae06`, Go
`go1.27.1 darwin/arm64`.

This report is point-in-time decision evidence. The normative behavior it changes
is owned by the plan-compiler/planner code it links; it does not define behavior
itself.

## 1. Reproduction

The defect was reproduced from real run artifacts, not from prose. The external
project `projects/sop-decision-adapters` runs a compiled plan,
`docs/plans/PLAN-Clef-Provider-Integration.md`, whose task-level planning is the
failure surface.

`CLEF-001 — Capture Adapter Repository Truth` is a discovery-and-report task. Its
prose names discovery targets: "the current provider interface", "existing provider
implementations, including Nimble and Julia where present", "the repository's
current provider registration/factory and CLI selection mechanism", "configuration
and provider enablement behavior", "the existing test structure". The compiled plan
already declares the capability inventory and the only prerequisite the task needs:

```text
## Capabilities
### Go build/test toolchain — EXISTS
### agentic-sop provider-neutral decision seam — EXISTS
### Apple Silicon MLX runtime / oMLX with clef-4bit — UNKNOWN

## CLEF-001 — Capture Adapter Repository Truth
### Requires
- Go build/test toolchain
```

Observed artifacts under `projects/sop-decision-adapters/.agent-sdlc/runs/CLEF-001/`:

- `metrics.json`: `"plan_repairs": 1` — the task-level planner returned one invalid
  plan that had to be sent back to the model for correction.
- `plan.md`: the delivered task-level plan re-declared every discovery target as a
  capability, e.g. `### Provider registration/factory and CLI selection mechanism —
PARTIAL` and `### Existing provider implementations (including Nimble and Julia) —
PARTIAL`, and required them from stages. These are repository facts the task
  exists to discover, modelled as prerequisites.

The source plan was unchanged and the persisted state was unchanged across the
attempts; only the model-driven task-level plan generation differed run to run.

A separate failure in the same run — `CLEF-001` classifying as
`IMPLEMENT_NO_PROGRESS` — is a distinct defect (see §13).

## 2. Root cause

The compiled plan carries a capability inventory, and every stage's `Requires` was
decided against it at compile time by the ownership gate
(`planner.acceptPlan` / `Plan.CapabilityGaps`). None of that context reached the
**task-level** planner.

`internal/cli/run.go` generated a task-level plan from the task prose alone:

```go
plan, err = planner.New(a).Generate(ctx, d.taskInput(spec.ID, spec.Render()))
```

`planner.Generate` sent `planTaskPrompt` (which asks the model to discover
capabilities) with the rendered task prose as the only input. `specFromTask`
(`internal/cli/drive.go`) builds that prose from `ID`, `Title`, `Objective`,
`AcceptanceCriteria`, and `ExecutionMode` — it does not carry the plan's capability
inventory or the task's compiled `Requires`.

So the model re-derived a capability inventory from prose that deliberately names
discovery targets. Two deterministic outcomes follow, both observed:

- A requirement the model invented but did not declare is rejected by
  `Plan.validateCapabilities` and returned for repair — the `plan_repairs: 1`.
- A requirement the model declared as UNKNOWN/MISSING and required is rejected
  ("`requires capability ... whose status is UNKNOWN`") or diverted to
  `CapabilityGapError` (NEEDS_HUMAN).

Because the model is the source of the inventory and is not seeded with the
compiled one, repeated generation of an equivalent plan is not stable: sometimes
the first response validates, sometimes it needs a repair. That is the
nondeterminism the defect manifested as.

## 3. Exact production data flow (with source evidence)

1. **Source plan → compiled plan.** `planflow.Prepare` →
   `buildPlan` (`internal/planflow/planflow.go`) → `planner.Compile`
   (`internal/planner/markdown.go`). A recognizably structured document is parsed
   deterministically by `PlanFromMarkdown`; the model is used only for an
   unrecognizable document.
2. **Compiled capability inventory.** `planner.Plan.Capabilities`
   (`internal/planner/plan.go`), persisted at `.agent-sdlc/plan.json`.
3. **Compiled plan → tasks.** `taskbuilder.Build` →
   `internal/taskbuilder/taskbuilder.go` — copies `ID`, `Title`, `Objective`,
   `AcceptanceCriteria`, `Dependencies`, `ExecutionMode`. **It drops
   `Capabilities` and per-stage `Requires`.**
4. **Task selection → task-level plan.** `driveGraph` → `runScheduledTask`
   (`internal/cli/drive.go`) → `runStages` (`internal/cli/run.go`).
5. **`planner.Generate`.** The single per-task call site:
   `internal/cli/run.go` (`ar.Emit(activity.StagePlan, ...)`).
6. **Generated `Requires`.** `planTaskPrompt` + `planOutputRequirements`
   (`internal/planner/planner.go`); the model derives `requires` from prose.
7. **Validation.** `decodePlan` → `Plan.Validate` → `validateCapabilities`
   (`internal/planner/plan.go`).
8. **UNKNOWN / missing.** An undeclared requirement, or a requirement whose
   declared status is UNKNOWN, is a structural validation error; a required
   MISSING/PARTIAL capability with no owner or resolution is a
   `CapabilityGapError`.
9. **Repair.** `generateValid` returns the deterministic error to the model,
   bounded by `maxPlanRepairs = 2`.

Answers to the issue's questions:

- The authoritative inventory lives in `planner.Plan.Capabilities`
  (`plan.json`); at the task level it is currently **not consulted at all**.
- The planner is given only task prose; the compiled inventory (step 2) and the
  task's compiled `Requires` are omitted.
- Capability inference is entirely model-generated at the task level; the only
  deterministic authority (`validateCapabilities`, `CapabilityGaps`) runs **after**
  generation and can only reject, never inform.
- The planner does not distinguish external prerequisite / existing capability /
  repository fact / task-local discovery: nothing in its input carries that
  distinction.
- **The defect is not provider-specific.** Any task whose prose names a repository
  artifact it inspects (a module, a config file, a test suite) can have that target
  promoted to a prerequisite. The Clef reproduction is one instance.

## 4. Why rewording the source plan could not reliably fix it

The task-level planner never sees the source plan. It sees
`specFromTask(task).Render()` — `ID`, `Title`, `Objective`, `AcceptanceCriteria`,
`ExecutionMode`. Rewording the _source plan_ changes the compiled plan, but the
model that re-derives the inventory at the task level only reads the task's own
prose; even when the prose is reworded to state the invariant, the model still has
no authoritative inventory to anchor against, so whether it promotes a discovery
target varied run to run.

## 5. Chosen architecture fix

The fix makes the compiled capability inventory authoritative at the task-level
planning boundary, which is exactly the boundary the issue identified.

**Typed authoritative context (`internal/planner/context.go`):**

```go
type PlanContext struct {
    Capabilities []Capability // the compiled plan's authoritative inventory
    Requires     []string     // the capabilities this task's compiled stage requires
}
```

`PlanContext` is derived deterministically from `.agent-sdlc/plan.json`; it is
never model-generated and carries no authority (no provider, model, approval, or
lifecycle signal).

**Delivered to the planner.** `Planner.GenerateWithContext(ctx, prd, pctx)`
(and `Generate` as the empty-context form) prepends a deterministic prompt block
stating the two invariants in general terms: the compiled inventory is
authoritative and must not be re-derived; a repository inspection/search/trace/
configuration/adapter/registration/CLI/test/package/report target named in the task
is work, described in the objective or acceptance criteria, not a `requires` entry.
A `requires` entry is only an externally supplied runtime, permission, tool,
service, or artifact the inventory does not already provide. `planTaskPrompt` also
states that any supplied authoritative context is binding. No keyword list, no
provider name, no task-name branch.

**Deterministic enforcement (`applyPlanContext`).** Before validation — and so
before any repair classification — the generated plan is reconciled against the
authoritative inventory:

- a generated capability that names an authoritative capability (by the same
  normalization `validateCapabilities` and `CapabilityGaps` use) adopts the
  authoritative entry verbatim — a cosmetic re-derivation can never downgrade a
  known EXISTS capability to UNKNOWN or invent a parallel entry;
- a stage `Requires` entry that resolves to an authoritative capability is
  guaranteed declared and canonicalized to the authoritative spelling.

Nothing else changes. In particular the fix does **not** remove, rewrite, or
invent a `Requires` entry, does not touch ids, dependencies, acceptance criteria,
deliverables, or execution mode, and does not touch governed state.

**Wiring (`internal/cli/`).** `loadPlanContext(dir, taskID)` reads `plan.json`
read-only and returns `PlanContext{Capabilities, Requires-of-this-stage}`; a
missing/unreadable plan yields the empty context. `runStages` passes it to
`GenerateWithContext`. `--task` runs and projects without a machine plan keep the
previous behavior exactly.

## 6. Alternatives considered

| Option                                                                                                                  | Verdict                                                                                                                                                                                                                           |
| ----------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A. Pass the inventory into `planner.Generate`.**                                                                      | Chosen, in typed form (B).                                                                                                                                                                                                        |
| **B. Typed planner context.**                                                                                           | Chosen. Carries the authoritative inventory and the task's compiled `Requires` — enough semantic information to distinguish a known capability and task-local discovery from a genuinely missing external prerequisite.           |
| **C. Reconcile generated `Requires` against the inventory after generation, before the UNKNOWN/repair classification.** | Partially chosen: the deterministic reconciliation (`applyPlanContext`) runs before validation. It does **not** strip requirements the inventory lacks, because that would silently downgrade a genuine new external requirement. |
| **D. Strip/hard-fail any `Requires` outside the inventory.**                                                            | Rejected. Stripping silently downgrades a genuine new prerequisite to task-local discovery (forbidden); hard-failing turns every discovery mention into a permanent human stop.                                                   |
| **Keyword blacklist ("inspect", "registry", "factory", …).**                                                            | Rejected. Fragile, locale/case sensitive, and encodes the reproduction's nouns instead of a general rule.                                                                                                                         |
| **Replace the generated inventory with the authoritative one wholesale.**                                               | Rejected. It would discard a genuine new capability the task legitimately discovered, weakening the existing governed missing-capability path.                                                                                    |

The chosen boundary is `planner.GenerateWithContext`: it reuses the existing
compiler, validation, ownership gate, run loop, tool policy, and approvals, and
adds only authoritative input plus a semantics-preserving reconciliation.

## 7. Files changed

Added:

- `internal/planner/context.go` — `PlanContext`, the prompt block, and
  `applyPlanContext` (deterministic reconciliation).
- `internal/planner/context_test.go` — 11 deterministic unit tests.
- `internal/planner/context_fixture_test.go` — 3 real-path regression tests against
  the fixture below.
- `internal/planner/testdata/provider-discovery/PLAN.md` — a Clef-semantics
  fixture (generic ids, no Clef dependency).
- `internal/cli/plan_context.go` — `loadPlanContext` (read-only).
- `internal/cli/plan_context_test.go` — 5 read-only loader tests.
- `internal/cli/plan_context_integration_test.go` — 2 boundary integration tests.
- `docs/reports/planner/SOP-PLANNER-CAP-001-capability-context.md` — this report.

Changed:

- `internal/planner/planner.go` — `GenerateWithContext`; `generateValid`/
  `decodePlan` take a `PlanContext`; the repair prompt preserves the context block;
  `planTaskPrompt` states the authoritative-context rule.
- `internal/planner/markdown.go` — `Compile` passes the empty context (unchanged
  behavior).
- `internal/cli/run.go` — the task-level planning call uses
  `GenerateWithContext(..., loadPlanContext(dir, spec.ID))`.
- `docs/README.md` — indexes this report.

## 8. Planner API changes

```go
// Before
func (p *Planner) Generate(ctx context.Context, prd string) (*Plan, error)

// After
func (p *Planner) Generate(ctx context.Context, prd string) (*Plan, error)                    // unchanged, empty context
func (p *Planner) GenerateWithContext(ctx context.Context, prd string, pctx PlanContext) (*Plan, error)
```

`Generate` is now `GenerateWithContext(ctx, prd, PlanContext{})`, so every
existing caller is unchanged. The only production caller that adopts the context is
the task-level planning call in `internal/cli/run.go`.

## 9. Deterministic enforcement added

`applyPlanContext(plan, pctx)` runs inside `decodePlan`, before `Plan.Validate`:

- authoritative capability wins by normalized name (prevents the recurring
  downgrade-to-UNKNOWN repair);
- a requirement resolving to an authoritative capability is declared and
  canonicalized.

It is pure, model-free, and idempotent. It never removes a stage, reorders a
dependency, changes an id, or alters a lifecycle field; the compiled inventory is
the single capability authority, so no second, model-owned representation can
compete with it.

## 10. Regression tests

Deterministic, offline, `internal/planner`:

1. `TestContextReachesPlanningBoundary` — the inventory, the task's compiled
   requires, and the discovery rule reach the planning input.
2. `TestExistingCapabilityStaysExisting` — an authoritative EXISTS capability is
   not downgraded; the plan validates on the first response (no repair).
3. `TestKnownCapabilityNotUnknownByCosmeticWording` — a case/spacing variant is
   matched by the shared normalization; no parallel entry.
4. `TestRequirementResolutionInjectsAuthoritativeDeclaration` — a resolving
   requirement is declared from the authoritative inventory.
5. `TestDiscoveryTargetsAreNotPrerequisites` — registration/factory,
   existing-adapter, configuration, test/package, and interface discovery targets
   do not become prerequisites or capabilities.
6. `TestGenuineMissingExternalCapabilityStillGoverned` — a genuinely missing
   external capability still reaches the ownership gate (`NEEDS_HUMAN`).
7. `TestAmbiguousUndeclaredCapabilityStillFailClosed` — an undeclared UNKNOWN
   requirement is still rejected (fail closed), not silently downgraded.
8. `TestContextDoesNotAlterTaskIdentityOrGraph` — ids, dependencies, execution
   mode, deliverables, and acceptance criteria are untouched.
9. `TestRepeatedPlanningIsStable` — repeated generation is byte-identical.
10. `TestEmptyContextMatchesGenerate` — the no-context path is unchanged.
11. `TestPlanContextCarriesNoAuthority` — the context block carries no
    provider/model/approval/lifecycle signal.
12. `TestProviderDiscoveryFixtureCompilesDeterministically` — the fixture plan
    compiles deterministically; the discovery task's only prerequisite is the
    declared toolchain.
13. `TestProviderDiscoveryFixtureContextPreventsRecurringRepair` — before/after
    contrast: with the context, the unsafe-shaped output validates with zero
    repairs and the authoritative status is kept; without it, the bounded repair
    loop never converges.
14. `TestProviderDiscoveryFixturePlanningIsStable` — repeated planning is stable.

Deterministic, offline, `internal/cli`:

15. `TestLoadPlanContextReadsAuthoritativeInventory` — the loader returns the
    compiled inventory and the stage's requires.
16. `TestLoadPlanContextDoesNotMutateRepoState` — read-only: `plan.json` is
    unchanged and no `state.db` is created or touched.
17. `TestLoadPlanContextMissingPlanIsEmpty`,
    `TestLoadPlanContextUnrelatedTaskHasNoRequires`,
    `TestLoadPlanContextIgnoresUnreadablePlan` — degraded inputs are inert.
18. `TestTaskRunPlanningReceivesAuthoritativeCapabilityContext` — the real
    `run --task` path delivers the authoritative context and spends no repair.
19. `TestTaskRunPlanningWithoutPlanCarriesNoContext` — an ad-hoc run with no
    machine plan carries no context.

Lifecycle preservation (LOCAL_DONE / NOT_REQUIRED / approvals) is covered by the
existing `internal/taskbuilder`, `internal/planflow`, and `internal/continuation`
suites, which the change does not touch; `applyPlanContext` operates only on the
in-memory task-level `*planner.Plan` before it is written as an artifact and never
persists a capability status.

## 11. Real-path validation result

The fixture `internal/planner/testdata/provider-discovery/PLAN.md` reproduces the
real plan's semantics without depending on Clef: the same capability-inventory
shape (EXISTS toolchain + seam, UNKNOWN runtime), the same discovery prose
(provider registration/factory, existing adapters), and `Requires: Go build/test
toolchain`. `TestProviderDiscoveryFixture…` compiles it deterministically,
derives the task-level `PlanContext` from it, and shows the observed unsafe-shaped
model output validating with zero repairs and a stable result across repetitions.

Read-only validation against `projects/sop-decision-adapters` was **not**
performed beyond reading the plan and its run artifacts, because running SOP there
would touch its `state.db` and could advance `CLEF-001`; the fixture carries the
same semantics without that risk.

## 12. `/sop-continue` behavior before/after

Unchanged in the skill; the fix is in the planner/compiler boundary.

- Before: recurring model-driven capability repair at task planning → the
  task-level plan is regenerated differently across attempts. `/sop-continue`'s
  determinism gate (`internal/continuation`, bounded `planflow.Inspect`) and its
  safety boundary were correct; a genuine compiled-plan change is still reported as
  `RECONCILE_CHANGED`/`RECONCILE_SEMANTIC_CHANGE` and still stops for review.
- After: with the compiled inventory reaching the planner, re-planning an
  equivalent source/state is stable at the deterministic boundary, so no recurring
  repair arises solely from missing capability context. `/sop-continue` was **not**
  weakened: it still stops on a real recurring repair
  (`RECONCILE_NONDETERMINISTIC`) if one occurs for another reason.

## 13. CLEF-001 no-progress status

**SEPARATE.** `CLEF-001`'s run classification is
`NO_PROGRESS` / `IMPLEMENT_NO_PROGRESS` /
`"the Ollama agent IMPLEMENT made no repository progress after 5 consecutive stale
iterations"` — an implementation-agent behavior, not a capability-planning one.
This report does not change implementation-agent behavior, so it neither fixes nor
exercises that problem. Recommend a separate issue, `SOP-IMPLEMENT-NOPROGRESS-001`.

## 14. Full gate results

```text
gofmt -l .            : clean (no files listed)
go vet ./...          : pass
go test -count=1 ./...      : pass
go test -race -count=1 ./...: pass
go build ./...        : pass
git diff --check      : clean
```

Focused planner/compiler/reconciliation suites, run independently:
`go test ./internal/planner/ ./internal/planflow/ ./internal/continuation/ ./internal/cli/ -count=1` — pass. Default tests remain offline and use deterministic/fake providers.

## 15. Residual risks

- **Discovery-prevention is delivered primarily by supplying the authoritative
  context, not by a keyword rule.** A model that ignores a binding, explicitly
  stated authoritative inventory could still declare a discovery target as a
  capability. The deterministic layer guarantees the authoritative inventory can
  never be re-derived, downgraded, or paralleled, and it keeps such an assertion
  governed (never silently authorized, never silently dropped), but it cannot prove
  a model will never mis-classify. This is deliberate: the general rule cannot be
  a keyword list, and silently stripping a requirement would violate the
  "do not silently downgrade a genuine external requirement" invariant.
- **A per-task capability the compiled plan does not declare but that the task
  genuinely requires** is left to the existing ownership gate. That is the intended
  fail-closed behavior, but it means such a case still surfaces as a human stop
  rather than a silent plan change.
- **Real-model confirmation** of the repair-rate reduction could not be run
  offline. The fixture demonstrates the deterministic boundary; a live Ollama run
  against the Clef plan was deliberately avoided to avoid mutating its SOP state.

## Recommendation

GO for returning to `sop-decision-adapters` and invoking `/sop-continue` on the
Clef plan — the planner capability-context defect is fixed at the boundary the
issue identified, with governed behavior preserved. Treat the separate
`CLEF-001` `IMPLEMENT_NO_PROGRESS` problem as its own issue before expecting the
Clef task itself to complete.
