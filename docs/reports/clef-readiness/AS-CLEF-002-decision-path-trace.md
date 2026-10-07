# AS-CLEF-002 — Trace the Live Decision Path

Status: investigation complete (read-only). This report is the only repository change.
Branch at AS-CLEF-001 baseline time: `main` tracking `origin/main` at HEAD `3e4f20f577ca4ce711382e90fd702d0dcb3f2578` (per the AS-CLEF-001 run record).

## 0. Baseline reuse and targeted reads

This report reuses the AS-CLEF-001 baseline. The authoritative AS-CLEF-001 evidence is the run record at `.agent-sdlc/runs/AS-CLEF-001/implementation.md`, which reports a read-only baseline and recorded decision-related packages `internal/decision`, `internal/provider`, `internal/autonomy`, `internal/router`, `internal/adaptiveroute`, `internal/approval`, `internal/decisionmemory`, `internal/jev`, `internal/archtest`, plus live wiring names `decision.NewProvider`, `DeterministicProvider`, `decision.Route`, and config keys `decision.provider` and `Thresholds` (`route_to_strong_model`, `require_human`).

Reused baseline facts:

- decision package = `internal/decision`; default provider = deterministic; SMALL/MEDIUM/LARGE execution routing tiers exist (`SMALL_MODEL`/`STRONG_MODEL`/`HUMAN`); build/test/vet pass; `internal/jev` exists; no `nimble`/`systemone`/`clef`/`laya`/`julia` package directory.

Targeted reads/searches performed in AS-CLEF-002 to close specific unanswered questions (nothing broader):

- `read_file internal/decision/decision.go` — request/result types, provider interface, default provider, thresholds, routing policy.
- `search_files internal decision.Route` — consumers (only `internal/e2e/lifecycle/early_decision_test.go`).
- `search_files internal decision.NewProvider` — construction sites (only `internal/e2e/lifecycle/early_decision_test.go`).
- `search_files internal autonomy.Decide` — call sites (`internal/cli/autonomy.go:22`, `internal/cli/early_jev.go:184` via `DecideEarly`, `internal/cli/reconcile.go:126` via `DecidePlanChange`).
- `list_files internal` — confirms no `nimble`/`systemone`/`clef`/`laya`/`julia` directory; `internal/jev` present.
- `read_file internal/autonomy/autonomy.go`, `read_file internal/autonomy/early.go` — policy interpretation and human boundary.
- `read_file internal/cli/early_jev.go` — live call site producing an `autonomy.Decision` from evidence.

UNKNOWN targets are called out inline with the exact searches and files inspected.

## 1. Traced representative governed decision path

The representative live path traced is the early-JEV checkpoint seam, the only in-tree path that constructs a decision request, runs a provider, and converts the decision result into a policy interpretation.

```
SOP operation            cli: early JEV checkpoint (runEarlyGate / runJEVCheckpoint)      internal/cli/early_jev.go
   |  builds invocation  runpkg.JEVInvocation {Purpose, Task, Criteria, RepositoryContext} internal/cli/early_jev.go
   |  provider call       runpkg.RunJEV(ctx, analyzer, inv)                                 (runpkg boundary)
   |  evidence            jev.Evidence (validated via ev.Validate())                        internal/jev
   |  DECISION REQUEST    autonomy.DecideEarly(ev, cfg.EarlyJEVFailOn(), cfg.AutonomyPolicy()) internal/cli/early_jev.go:184
   |  policy boundary     internal/autonomy — Decide / DecideEarly                        internal/autonomy/autonomy.go, early.go
   |  DECISION RESULT     autonomy.Decision {Action, Level, Risk, RequiresHuman, Reason,
   |                      Classification failure.Classification}                           internal/autonomy/autonomy.go
   |  human boundary      res.Escalate = res.Decision.RequiresHuman                         internal/cli/early_jev.go
   |  routing / action    Action* -> AUTO_CONTINUE | AUTO_RETRY | AUTO_FIX |
   |                      AUTO_RECONCILE | TERMINAL | HUMAN_APPROVAL_REQUIRED              internal/autonomy/autonomy.go
   |  artifact            runpkg.EarlyArtifact (best-effort, diagnostic only)              internal/cli/early_jev.go
```

Two secondary live consumers of the same policy engine were found in the same search:

- `internal/cli/autonomy.go:22` — `return autonomy.Decide(cls, cfg.AutonomyPolicy())`; failure-classification → policy → action.
- `internal/cli/reconcile.go:126` — `return autonomy.DecidePlanChange(kind, policy)`; plan-change → policy → action.

There is a **second, structurally separate** decision boundary: `internal/decision` (`Decision`/`Choice`/`Provider`/`Route`). It is a distinct package with its own request (`decision.Request`), result (`decision.Decision`), and provider interface (`decision.Provider`). The search for `decision.Route` and `decision.NewProvider` in `internal` returned **only** `internal/e2e/lifecycle/early_decision_test.go` hits; see §9 (path status) and Q9/Q12 for the consequence.

## 2. Documented aspects

### 2.1 Request type

- `internal/autonomy` path: the request is not a single named struct; the policy input is the tuple `(jev.Evidence, []string failOn, autonomy.Policy)`, consumed by `autonomy.DecideEarly(ev jev.Evidence, failOn []string, p Policy) Decision` (`internal/autonomy/early.go`). Its structured fields are `jev.Evidence.Purpose`, `jev.Evidence.Status`, and each `jev.EvidenceItem.{Severity, Category}`; prose (`Summary`, `Detail`, `Evidence`) is explicitly provenance-only.
- `internal/decision` path: the request IS a named type — `decision.Request` with fields `UseCase string`, `Subject string`, `Signals map[string]float64` (`internal/decision/decision.go`). Its construction point in-tree is **only** the test `internal/e2e/lifecycle/early_decision_test.go`; no non-test construction site was found (search `decision.NewProvider` in `internal`).

### 2.2 Result type

- `autonomy.Decision` (`internal/autonomy/autonomy.go`): `Action Action`, `Level Level`, `Risk ApprovalRisk`, `RequiresHuman bool`, `Reason string`, `Classification failure.Classification`. **Live**, produced at `internal/cli/early_jev.go:184` and consumed there (`res.Escalate = res.Decision.RequiresHuman`).
- `decision.Decision` (`internal/decision/decision.go`): `Choice Choice`, `Confidence float64`, `Metadata map[string]string`. Consumer in-tree is test-only (`internal/e2e/lifecycle/early_decision_test.go`, via `decision.Route`).

### 2.3 Capability / interface used

- `autonomy.` path uses no interface: it is a pure function boundary (`autonomy.Decide` / `DecideEarly` / `DecidePlanChange`). The upstream analysis capability is the JEV analyzer invoked through `runpkg.RunJEV`, not a `DecisionCapability`-style interface.
- `decision.Provider` interface (`internal/decision/decision.go`):

  ```go
  type Provider interface {
      Name() string
      Decide(ctx context.Context, req Request) (Decision, error)
  }
  ```

  This is the closest thing in-tree to a provider-neutral decision capability. There is **no** `DecisionCapability` interface by that name — recorded as a valid finding (searches: `decision.NewProvider`, `decision.Route` over `internal`).

### 2.4 Construction / injection point

The JEV analyzer used by the live early-JEV path is injected through the `deps` struct field `newJEVAnalyzer` (`internal/cli/early_jev.go`, `runJEVCheckpoint`): `if d.newJEVAnalyzer == nil { return earlyGateResult{} }` then `analyzer, err := d.newJEVAnalyzer(cfg)`. The `decision` provider is constructed via `decision.NewProvider(name string) (Provider, error)` (`internal/decision/decision.go`); the only in-tree call is `decision.NewProvider("deterministic")` in `internal/e2e/lifecycle/early_decision_test.go:25`.

### 2.5 Default implementation

- `decision` path: **`DeterministicProvider` is the default and the only implemented provider** (`internal/decision/decision.go`). `NewProvider` maps `""` and `"deterministic"` to `DeterministicProvider{}`; any other name — including `"jev"` — returns `fmt.Errorf("decision: provider %q is not implemented (only \"deterministic\")", name)`. The package doc states the default is deterministic and Jev is opt-in/comparison-only.
- `autonomy` path: no provider default; the default *policy* is `DefaultLevel = Balanced` and unknown/empty levels normalize to `Balanced` (`normalize` in `internal/autonomy/autonomy.go`). Early-JEV gates default to OFF (both gates ANDed with `early_jev.enabled`; `internal/cli/early_jev.go` `earlyGateEnabled`).

### 2.6 Fallback behavior

- `decision.NewProvider`: no silent fallback — an unsupported name is a hard error (comment: "fails clearly rather than silently falling back").
- Early-JEV seam (`internal/cli/early_jev.go`): disabled gate, nil analyzer, or analyzer construction error all return a zero `earlyGateResult{}` — a strict no-op. Provider failure / `StatusError` / `StatusIncomplete` → `ProviderFailed=true` and the advisory default **continues**: `"early JEV analysis could not run; continuing under deterministic SOP policy"`. Invalid evidence (`ev.Validate()` error) → `ProviderFailed=true`, "never a pass and never interpreted as a finding". No structured evidence → advisory continue. In all these cases `res.Decision` stays zero and `res.Escalate` stays false.
- `autonomy.Decide` fallback: step 4 "fail closed to a human" — a needs_human classification, a disallowed automation, or an unknown disposition returns `humanDecision(...)` with `ActionHumanApproval`.

### 2.7 Error behavior

- Provider boundary: errors are surfaced as `error` from `decision.Provider.Decide` / `decision.NewProvider`; the live seam never treats a provider error as a finding (explicit comment at the `out.FailClosed()` branch in `internal/cli/early_jev.go`).
- Policy: `autonomy.Decide` fails closed to human for unknown dispositions; `ActionTerminal` is deliberately distinct from `ActionHumanApproval` for `failure.NoProgress` and (at Level `High`) `failure.AutoFixExhausted`.
- No timeout mechanism was found on either traced decision path (no `context.WithTimeout`/deadline at `decision` or `autonomy` boundaries in the inspected files). Its absence is reported, not assumed; behavioral proof is deferred to AS-CLEF-005.

### 2.8 Confidence / probability handling

- `decision.Decision.Confidence float64` (range [0,1] per the package doc) is **read** by `decision.Route`:

  ```go
  if d.Choice == Human || d.Choice == High || d.Confidence < t.RequireHuman { return HumanTarget }
  if d.Choice == Medium || d.Confidence < t.RouteToStrongModel { return StrongModel }
  return SmallModel
  ```

  Thresholds live in config: `decision.Thresholds{RouteToStrongModel, RequireHuman}`. `DeterministicProvider` emits fixed confidences 0.9 / 0.8 / 0.9.
- `autonomy` path: confidence is explicitly **not** a decision input. `failure.Classification.Confidence` exists (e.g. `failure.High` set inside `DecideEarly`), and `jev.Evidence.Confidence` is **display-only** in `earlyGateDetail`/`earlyConfidence` — "it never feeds a decision, gate, or task state". `DecideEarly`'s doc: "Confidence is deliberately not a decision input here."

### 2.9 Human-approval boundary

- Enforcement symbol: `humanDecision` in `internal/autonomy/autonomy.go`, which sets `c.Disposition = failure.NeedsHuman` and returns `Decision{Action: ActionHumanApproval, RequiresHuman: true, ...}`. Boundary kinds are chosen by `boundaryRisk`: `failure.DestructiveOperation → RiskIrreversible`; `failure.SecurityBoundary`, `failure.ApprovalRequired`, `failure.AmbiguousContract → RiskHigh`.
- The traced live path reaches it: `internal/cli/early_jev.go:184` calls `autonomy.DecideEarly`, which calls `autonomy.Decide(c, p)` for a blocking finding; the caller then reads `res.Escalate = res.Decision.RequiresHuman`. `ActionHumanApproval` is documented as "the only action that requires a human"; `ActionTerminal` is deliberately not a human decision.
- `decision.Route` also maps to `HumanTarget`, but only the test consumes `Route` (see §9).

### 2.10 Path status (live / optional / shadow-only / inert)

- **Live**: `internal/autonomy` policy engine (`Decide`, `DecideEarly`, `DecidePlanChange`) and its `internal/cli` call sites (autonomy.go, early_jev.go, reconcile.go). The early-JEV checkpoint is live but **optional and disabled by default** (gates ANDed with `early_jev.enabled`); when disabled it is a strict no-op.
- **Live-but-unwired-outside-tests**: `internal/decision` (`NewProvider`, `DeterministicProvider`, `Route`). Its only in-tree callers are in `internal/e2e/lifecycle/early_decision_test.go`. No production call site was found.
- **Inert / absent-from-live-path**: `internal/jev` is present and used by the optional early-JEV seam (so: optional, not inert, when the gate is on; no-op when off). `nimble`, `systemone`, `clef`, `laya`, `julia`: no package directory in `internal` (`list_files internal`) and no referenced wiring found. No specialized decision-provider wiring (nothing selects a non-deterministic `decision.Provider`) was found.
- **Shadow-only**: no shadow-mode decision path was found.

## 3. Question-by-question answers

### Q1 — Is Jev on the live decision path?

Partial, and conditionally. `internal/jev` exists and is invoked by the **optional, disabled-by-default** early-JEV checkpoint through `runpkg.RunJEV` (`internal/cli/early_jev.go`, `runJEVCheckpoint`). Its structured `jev.Evidence` feeds `autonomy.DecideEarly` when the gate is enabled. When the gate is off, the path is a strict no-op. **Jev is NOT on the `internal/decision` path**: `decision.NewProvider("jev")` returns the not-implemented error ("\"jev\" is recognized but not implemented"). Evidence: `internal/decision/decision.go`; `internal/cli/early_jev.go`; `internal/autonomy/early.go`.

### Q2 — Is Nimble on the live decision path?

No. No `nimble` package directory exists under `internal` (`list_files internal`), and targeted searches for `nimble` across `internal` returned no decision-path wiring. Search pattern used: `nimble` over `internal` (hits: none on the decision path). Recorded as a valid absence finding.

### Q3 — Is SystemOne on the live decision path?

No. No `systemone` (nor `system-one`) package directory exists under `internal` (`list_files internal`), and no decision-path wiring was found. Search pattern used: `systemone` over `internal` (no decision-path hits). Recorded as a valid absence finding.

### Q4 — Is Clef on the live decision path?

No. No `clef` package directory exists under `internal` (`list_files internal`); Clef appears only as the readiness-program name in `docs/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md` (baseline search sweep). No decision-path wiring found. Recorded as a valid absence finding.

### Q5 — Is Laya or Julia on the live decision path?

No. Neither `laya` nor `julia` has a package directory under `internal` (`list_files internal`), and no decision-path references were found (baseline search sweep for `laya`, `julia`). Recorded as a valid absence finding.

### Q6 — How does execution-model routing relate to decision-provider routing?

On the traced path they are **separate**: `internal/decision` has its own model-tier routing (`decision.Route` → `SmallModel`/`StrongModel`/`HumanTarget`, driven by `decision.Thresholds`), while the live `internal/autonomy` path produces lifecycle `Action`s (`AUTO_*`, `TERMINAL`, `HUMAN_APPROVAL_REQUIRED`) and does not call `decision.Route`. Because `decision.Route` is only consumed by `internal/e2e/lifecycle/early_decision_test.go`, the production separation cannot be fully asserted from the traced path. The definitive execution-vs-decision-separation audit is deferred to **AS-CLEF-006**.

### Q7 — What is the SOP operation entry point that triggers a governed decision?

`internal/cli/early_jev.go` — `runEarlyGate` (task checkpoints) and `runJEVCheckpoint` (shared core, also used by `sop prompt`); the decision itself is taken at `runJEVCheckpoint` line 184: `res.Decision = autonomy.DecideEarly(ev, cfg.EarlyJEVFailOn(), cfg.AutonomyPolicy())`. Secondary live entry points: `internal/cli/autonomy.go:22` (`autonomy.Decide(cls, cfg.AutonomyPolicy())`) and `internal/cli/reconcile.go:126` (`autonomy.DecidePlanChange(kind, policy)`). A structurally separate entry exists in `internal/decision` but no production caller was found (see Q9/Q12).

### Q8 — What capability/interface sits at the decision boundary?

The live boundary is a **pure function API, not an interface**: `autonomy.DecideEarly(jev.Evidence, []string, Policy) Decision` (`internal/autonomy/early.go`) and `autonomy.Decide(failure.Classification, Policy) Decision` (`internal/autonomy/autonomy.go`). The only decision-related interface in-tree is `decision.Provider` (`Name() string; Decide(ctx, Request) (Decision, error)`) in `internal/decision/decision.go`. No `DecisionCapability` interface exists; absence of specialized decision-provider wiring is recorded as a valid finding.

### Q9 — Where are decision requests constructed and results consumed?

Live autonomy path: constructed as the tuple at `internal/cli/early_jev.go:184` (with `cfg.EarlyJEVFailOn()` and `cfg.AutonomyPolicy()`); result consumed in the same function (`res.Escalate = res.Decision.RequiresHuman`) and persisted as `runpkg.EarlyArtifact`. The `decision.Request`/`decision.Decision` pair is constructed/consumed **only in tests** (`internal/e2e/lifecycle/early_decision_test.go`), with no non-test construction site found (searches: `decision.NewProvider`, `decision.Route` over `internal`).

### Q10 — What are the fallback, error, malformed/empty, and indeterminate behaviors?

Provider error / `StatusError` / `StatusIncomplete` → `ProviderFailed=true`, advisory continue, no finding (`internal/cli/early_jev.go`). Malformed evidence (`ev.Validate()` error) → `ProviderFailed=true`, never a pass, never a finding. Empty/no structured evidence → advisory continue. Disabled gate / nil analyzer / analyzer build error → zero-result strict no-op. Policy unknown disposition → fail closed to human (`internal/autonomy/autonomy.go` step 4). No timeout mechanism was found on the traced path. Behavioral proof that these cannot become implicit success is **deferred to AS-CLEF-005**.

### Q11 — Where is the human-approval boundary enforced?

`humanDecision` in `internal/autonomy/autonomy.go` (sets `c.Disposition = failure.NeedsHuman`; returns `Action: ActionHumanApproval`, `RequiresHuman: true`), reached via `autonomy.Decide`'s boundary check `boundaryRisk` (DestructiveOperation/SecurityBoundary/ApprovalRequired/AmbiguousContract) and via `autonomy.DecideEarly`'s `kindForEarlyCategory` for a blocking JEV finding. The traced live path reaches it: `internal/cli/early_jev.go:184` → `autonomy.DecideEarly` → `autonomy.Decide` → `res.Decision.RequiresHuman` → `res.Escalate`. `internal/approval` was not needed on this path; no approval-package call site appears on the traced path (searches: `autonomy.Decide` over `internal`).

### Q12 — Is the traced path live, optional, shadow-only, or inert?

Mixed, stated per element: `internal/autonomy` policy engine + `internal/cli` call sites = **live**; early-JEV checkpoint = **live but optional and disabled by default**; `internal/decision` (`NewProvider`/`DeterministicProvider`/`Route`) = **live code but unwired outside tests** (effectively shadow/inert in production); `internal/jev` = optional (used only by the optional gate); absent providers (Nimble/SystemOne/Clef/Laya/Julia) = **absent from the live path**; no shadow-mode decision path found.

## 4. UNKNOWN targets and deferrals

Recorded as UNKNOWN with exact evidence, not asserted:

- **Behavioral failure semantics** (whether a provider failure can ever become implicit success under load/timeouts) — code branches inspected but behavioral proof not performed. Exact searches/files: `internal/cli/early_jev.go`, `internal/autonomy/autonomy.go`. Deferred to **AS-CLEF-005**.
- **Authoritative separation of execution-model routing vs decision-provider routing** — observed only via the traced path. Deferred to **AS-CLEF-006**.
- **Any production config selecting a non-deterministic `decision.Provider`** — searches for `decision.NewProvider`/`decision.Route` over `internal` found only test hits. Not asserted as impossible, only as not found in-tree.

BLOCKED is **not** used: no single missing fact prevents determining the architecture; every gap above is either answered with citations or explicitly deferred with named evidence.

## 5. Scope

No production source, test, configuration, or default was modified. This report is the only repository change, per the AS-CLEF-002 plan.
