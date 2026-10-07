# SEAM — Provider-Neutral Decision Integration Seam

Architecture analysis for the smallest provider-neutral seam that lets an external
decision adapter supply decision **evidence** to the live SOP policy path without
becoming a policy authority.

- **Repository:** `~/agentic-workspace/agentic-sop`
- **Scope:** `agentic-sop` only. `sop-decision-adapters` is not modified.
- **Status:** DESIGN ONLY — no production code changes; nothing committed or pushed.
- **Evidence HEAD:** `b743cfa` (the Clef-readiness closure commit).
- **Governing invariant:** *Providers evaluate. SOP governs.*

This analysis re-verifies the relevant production symbols at the current HEAD rather
than trusting the Clef-readiness reports. The readiness reports
(`docs/reports/clef-readiness/`) are used as prior evidence, and the findings below
were re-derived by reading the live code.

---

## 0. Scope, method, and non-negotiables

Method: read the live decision/policy/execution code and its wiring; do not assume
the readiness reports are still sufficient. Symbols inspected at HEAD:
`internal/decision/decision.go`, `internal/autonomy/autonomy.go`,
`internal/autonomy/early.go`, `internal/failure/failure.go`,
`internal/cli/early_jev.go`, `internal/cli/cli.go` (`deps`),
`internal/run/jev.go`, `internal/jev/jev.go`, `internal/agent/command.go`,
`internal/provider/command/provider.go`, `internal/config/config.go`
(`DecisionConfig`), and `internal/archtest/{arch_test.go,guard_test.go}`.

Non-negotiables for any design considered below:

- No Clef/provider-specific behavior, dependency, or branch in `agentic-sop`.
- No change to SMALL/MEDIUM/LARGE execution-model defaults.
- Execution-model routing (`internal/model`) stays independent of decision-provider
  selection.
- An external provider result is **data** until SOP validates and interprets it.
- An external provider must not be able to: transition task state, approve work,
  authorize execution, bypass validation/review/human approval, commit/merge, choose
  lifecycle transitions, decide Continue/Block/Approval, or mutate governance state.
- With no external decision provider configured, existing SOP behavior is unchanged.
- Do not move/export `internal/decision` merely to satisfy Go `internal/` visibility.

---

## 1. PHASE 1 — Current architecture trace

### 1.1 The live policy path (`internal/autonomy`)

The live decision path is the pure policy engine `internal/autonomy`:

- `autonomy.Decide(failure.Classification, autonomy.Policy) autonomy.Decision`
  (`internal/autonomy/autonomy.go`) is the single central place that maps a typed
  failure classification to a lifecycle action.
- `autonomy.DecideEarly(jev.Evidence, []string, autonomy.Policy)` and
  `autonomy.DecidePlanChange(kind, policy)` are the two other entry points.
- The package is **pure**: "no I/O, no clock, no LLM" (package doc). It "starts no
  work and transitions no state".
- Output vocabulary (`Action`): `AUTO_RETRY`, `AUTO_CONTINUE`, `AUTO_FIX`,
  `AUTO_RECONCILE`, `TERMINAL`, `HUMAN_APPROVAL_REQUIRED`. Only
  `HUMAN_APPROVAL_REQUIRED` requires a human; `TERMINAL` is deliberately not a human
  decision.
- Live call sites (verified by `grep` at HEAD): `internal/cli/early_jev.go:184`,
  `internal/cli/autonomy.go:22`, `internal/cli/reconcile.go:126`.

`Decide` is fail-closed: an authority-boundary classification
(`DestructiveOperation`, `SecurityBoundary`, `ApprovalRequired`, `AmbiguousContract`),
a `NeedsHuman`/`Replan` disposition, an automation the level forbids, or an unknown
disposition all route through `humanDecision(...)` → `ActionHumanApproval`,
`RequiresHuman: true`.

### 1.2 The decision abstraction (`internal/decision`)

`internal/decision` is a **structurally separate** surface with its own vocabulary
(`internal/decision/decision.go`):

| Symbol | Shape |
|---|---|
| `Request` | `{UseCase string; Subject string; Signals map[string]float64}` |
| `Decision` | `{Choice Choice; Confidence float64; Metadata map[string]string}` |
| `Choice` | closed set `LOW`, `MEDIUM`, `HIGH`, `HUMAN` (`knownChoice`) |
| `Provider` | `interface { Name() string; Decide(ctx, Request) (Decision, error) }` |
| `NewProvider` | maps `""`/`"deterministic"` → `DeterministicProvider`; **hard-errors** on every other name (no silent fallback) |
| `Thresholds` | `{RouteToStrongModel, RequireHuman float64}` — policy config |
| `Route` | maps `Decision` → `Target` (`SMALL_MODEL`/`STRONG_MODEL`/`HUMAN`), fail-closed on unknown choice / invalid confidence |

**Current wiring status (re-verified at HEAD):** `decision.NewProvider` and
`decision.Route` have **no production caller** — the only importers that use those
symbols are tests (`internal/e2e/lifecycle/early_decision_test.go`,
`internal/decision/governance_test.go`). `internal/decision` *is* imported by
`internal/config/config.go` (for `decision.Thresholds`) and referenced by the arch
guard, but `config.DecisionConfig{Provider, Enabled, Thresholds}` is **validated but
never wired to a provider constructor** in production.

> Latent-coupling note: `decision.Route`'s output vocabulary (`SMALL_MODEL` /
> `STRONG_MODEL`) is a **model-tier** vocabulary. Wiring a decision provider into
> `Route` and then into execution would couple decision-provider selection to
> execution-model routing — forbidden here. The seam must map provider evidence to
> **policy** (Continue/Block/Human), never to an execution class.

### 1.3 Where evidence comes from and where it becomes policy

Two typed evidence vocabularies reach `autonomy` today:

- `failure.Classification{Kind, Disposition, Confidence, Reason}`
  (`internal/failure/failure.go`) produced by `failure.Classify(...)` from the
  lifecycle (validation, review, outcomes, errors) and consumed by `autonomy.Decide`.
- `jev.Evidence{Purpose, Status, Confidence, Items[]}` (`internal/jev/evidence.go`)
  produced by the optional JEV analyzer and consumed by `autonomy.DecideEarly`.

Both are **typed and closed**: prose (`Summary`, `Detail`, `Evidence` text) is
provenance only and never matched. `DecideEarly`'s doc states "Confidence is
deliberately not a decision input here".

### 1.4 Construction / bootstrap / configuration

- The composition root is `defaultDeps()` in `internal/cli/cli.go`. It injects the
  external boundaries as function fields.
- The JEV analyzer is constructed through `deps.newJEVAnalyzer func(cfg) (jev.Analyzer, error)`
  (`internal/cli/cli.go`), defaulting to `jev.NewOllamaAnalyzerFromEnv()`. A nil
  factory (or nil analyzer, or error) leaves JEV **absent**, which is never fatal
  (`internal/cli/early_jev.go`).
- The agent is constructed through `deps.newAgent func(harness, provider, model) (agent.Agent, error)`.
- Config: `config.DecisionConfig{Provider, Enabled, Thresholds}` exists but is inert;
  `config.EarlyJEV.Enabled *bool` + `EarlyJEVActive()` is the pattern for an
  OFF-by-default optional capability.

### 1.5 Existing dependency-injection / registration mechanisms

- **`deps` factory injection** (`internal/cli/cli.go`) — the preferred repository
  pattern for an optional, replaceable capability (agents, JEV analyzer, resources,
  diff reader, snapshot observer). This is the natural host for a decision-provider
  factory.
- **`provider` registry** (`internal/provider/registry.go`) — a read-only identity
  layer, explicitly *not* an execution or policy layer.
- **`internal/decision.NewProvider`** — a name→provider selector, already
  fail-closed. In-process only; unreachable from an external module.

### 1.6 Existing provider/plugin/agent boundaries (preferred architecture)

- **`jev.Analyzer`** (`internal/jev/jev.go`) — the canonical "non-owner analysis
  capability" boundary: bounded read-only `Request` in, structured `Result` out;
  "may not transition task state, mutate persistence, mark validation/review
  successful, commit/push/PR/merge, or bypass human gates". This is the closest
  template for a decision boundary.
- **`agent.CommandAgent`** (`internal/agent/command.go`) — an **external-process**
  boundary: a shell command that reads a JSON `Request` on stdin and writes output to
  stdout, using `exec.CommandContext` (context cancellation terminates the process).
  This is the proof that a JSON-over-process boundary is an accepted repository
  pattern for keeping a provider *out of the application*.
- **`provider/command`** (`internal/provider/command/provider.go`) — an allow-listed
  provider-neutral adapter that reports "unknown/unsupported" honestly rather than
  inventing metadata.
- **`runpkg.RunJEV` / `JEVOutcome`** (`internal/run/jev.go`) — the invocation seam:
  an absent analyzer is a no-op; an analyzer error yields `FailClosed()` with a zero
  `Result` (a provider error can never be mistaken for a pass).

### 1.7 Error / failure behavior

- `RunJEV`: absent analyzer → `Ran:false` (no evidence, not an error); analyzer error
  → `Ran:true, Error!=nil`, zero `Result`, `FailClosed()==true`.
- `runJEVCheckpoint` (`internal/cli/early_jev.go`): disabled gate / nil factory /
  factory error → strict no-op; provider failure or `StatusError`/`StatusIncomplete`
  → `ProviderFailed=true`, `Decision` stays zero, `Escalate` stays false; malformed
  evidence (`ev.Validate()` error) → `ProviderFailed=true`, never a pass. All of
  these **continue under deterministic SOP policy** (advisory) — because JEV is
  advisory, not a gate.
- `decision.NewProvider`: unknown name → hard error, never a fallback.
- `decision.Route`: unknown choice or invalid confidence → `HumanTarget`.
- No `context` deadline is enforced at the decision boundary today.

### 1.8 Human-approval enforcement

`humanDecision(...)` in `internal/autonomy/autonomy.go` sets
`Classification.Disposition = failure.NeedsHuman` and returns
`Action=HUMAN_APPROVAL_REQUIRED, RequiresHuman=true`. The lifecycle converts
`RequiresHuman`/`Escalate` into the existing approval boundary
(`internal/approval`, `recordHumanApprovalRequest`, `WAITING_FOR_HUMAN`). Approval
is never inferred from prose, a task status, or a provider claim.

### 1.9 Architecture guards (`internal/archtest`)

- `TestArchitectureGuard` fails if any core or policy/governance package
  (`internal/commandpolicy`, `internal/approval`, `internal/commitgate`,
  `internal/mergegate`, `internal/autonomy`, `internal/decision`, `internal/quality`,
  plus the Phase-8 inventory) imports a provider implementation
  (`internal/provider/*`, `internal/ollamaagent`) or the provider contract package.
- `TestProviderRegistryCoversProviderAdapters` forces each `internal/provider/*`
  adapter to be declared or allow-listed.
- Positive/negative controls (`TestGuardAcceptsNeutralImports`,
  `TestGuardDetectsRepresentativeForbiddenDependency`) prove the guard is neither
  vacuous nor over-broad.

These guards protect against provider-implementation imports; they do **not** yet
cover a decision *command adapter* surface (a new `internal/decision/command`
package would need its own guard coverage; see the plan).

### 1.10 Current-state diagram

```text
                       ┌──────────────────────────────────────────────────────────┐
                       │ internal/cli  (composition root = defaultDeps)           │
                       │   deps.newAgent, deps.newJEVAnalyzer, deps.readDiff, ...  │
                       └───────────────┬───────────────────────────────┬──────────┘
                                       │                               │
             optional early-JEV path    │                               │  lifecycle path
             (OFF by default)           │                               │  (internal/run, run.go)
                                       v                               v
        runJEVCheckpoint(early_jev.go)                    failure.Classify(evidence)
          d.newJEVAnalyzer(cfg) -> Analyzer                 -> failure.Classification
          runpkg.RunJEV -> JEVOutcome (fail-closed)                    │
          ev.Validate() (fail closed)                                  │
          autonomy.DecideEarly(ev, failOn, policy)                     │
                       │                                               │
                       └───────────────┬───────────────────────────────┘
                                       v
                       ┌──────────────────────────────────────────────┐
                       │ internal/autonomy  (pure, deterministic)     │
                       │   Decide / DecideEarly / DecidePlanChange    │
                       │   -> AUTO_* | TERMINAL | HUMAN_APPROVAL       │
                       └───────────────┬──────────────────────────────┘
                                       v
                       lifecycle action + human boundary (internal/approval)

  SEPARATE, UNWIRED SURFACE:
  ┌───────────────────────────────────────────────────────────────────────┐
  │ internal/decision  Request / Decision{Choice,Confidence,Metadata}      │
  │                    Provider{Name,Decide} / NewProvider / Thresholds    │
  │                    Route -> SMALL_MODEL|STRONG_MODEL|HUMAN            │
  │   production consumers: NONE (tests only; config validates, never      │
  │   constructs a provider)                                               │
  └───────────────────────────────────────────────────────────────────────┘
```

---

## 2. PHASE 2 — The exact missing seam

Between **external provider → provider-neutral evidence → live autonomy**, the
following is missing (none exists at HEAD):

1. **A construction seam** — no factory is wired to construct a non-deterministic
   `decision.Provider`; `config.DecisionConfig` is inert.
2. **A consumption seam** — no production code invokes a `decision.Provider` or
   consumes its result; `decision.Provider` has no production caller.
3. **A cross-module boundary** — an external Go module cannot implement
   `internal/decision.Provider` (Go `internal/` visibility), and there is no
   transport/DTO boundary for it to satisfy instead.
4. **A validation+interpretation step under SOP ownership** — `knownChoice` and
   `validConfidence` are unexported and there is no exported fail-closed validator,
   and there is no SOP-owned function that turns a *validated decision result* into
   a policy input.
5. **A timeout/cancellation owner** — no deadline is established at a decision
   invocation seam.

Answering the Phase-2 questions:

| Question | Answer for this repository |
|---|---|
| Does SOP need a public Go package? | **No.** A public Go contract exports types and creates a semver obligation; the same evidence can cross a process boundary as neutral JSON (the `agent.CommandAgent` precedent). |
| Could the boundary be transport/process based? | **Yes — and this is the smallest fit.** `internal/agent/command.go` already proves the JSON-over-process pattern. |
| Could an existing public package own the contract? | **No public package exists** — the module is `cmd/` + `internal/` only. |
| Could dependency inversion keep the implementation internal? | **Yes.** The internal `decision.Provider` stays internal; a factory injected through `deps` constructs an internal adapter that talks to an external process. |
| Is a registration/factory boundary sufficient? | **As the construction half, yes** (mirroring `deps.newJEVAnalyzer`). It is not sufficient alone for external modules — the transport half is also required. |
| Does the external adapter need to import SOP types? | **No.** It implements a documented JSON protocol; it never imports `internal/decision`. |
| Can a small DTO/protocol boundary prevent leaking lifecycle authority? | **Yes.** The DTO carries only request context and choice/confidence/diagnostics — no lifecycle, approval, commit, or merge fields. |
| Where should provider construction occur? | At the composition root (`defaultDeps`), via a config-driven factory, **OFF by default**. |
| Who owns timeout/cancellation? | **SOP** (a `context` deadline + process kill at the invocation seam). |
| Who validates returned choices and confidence? | **SOP** — a fail-closed validator (`knownChoice`/`validConfidence` promoted to an exported, tested validator). |
| Unavailable provider? | Provider failure → never success; fail closed (defined in §6). |
| Unsupported capability? | Explicit error category → fail closed / no-op per mode; never a guess. |
| Malformed/indeterminate result? | Fail closed; never approval. |
| No external provider configured? | Strict no-op; **behavior unchanged**. |

---

## 3. PHASE 3 — Design options

### Option A — Public provider-neutral Go contract package

Export (or re-home) the decision contract into a public package (for example
`decision/` at the module root) so an external module can import it.

| Criterion | Assessment |
|---|---|
| Dependency direction | external → agentic-sop (public import) |
| Go module visibility | works (public) |
| API surface | **large, permanent**: a public semver-committed API |
| Provider neutrality | ok |
| Testability | good |
| Failure isolation | **weak**: in-process provider shares SOP's failure domain (panic/hang) |
| Cancellation/timeout | in-process `ctx` only |
| Configuration complexity | moderate |
| Lifecycle-authority leakage | **risk**: exported structs invite lifecycle/approval fields |
| Backward compatibility | new public API to maintain forever |
| Migration cost | medium–high |
| Multi-provider (Clef, Nimble, future) | ok |

**Rejected.** It contradicts the explicit instruction not to move/export
`internal/decision` to work around visibility, creates a public API and semver
obligation, and gives an external provider the same process/failure domain as SOP.

### Option B — Internal interface with an external registration/factory boundary

Keep `decision.Provider` internal; add a factory (`deps.newDecisionProvider`).

| Criterion | Assessment |
|---|---|
| Dependency direction | cli → internal/decision |
| Visibility | fine internally |
| API surface | none (internal) |
| Failure isolation | in-process |
| **External module reachability** | **insufficient alone** — an external module still cannot implement an `internal/` interface |

**Insufficient alone; adopted as the construction half of the selected design.**

### Option C — Process/transport boundary with provider-neutral DTOs

An external command reads a bounded JSON request and writes a bounded JSON result;
an internal adapter (mirroring `internal/agent/command.go`) runs it and returns a
validated `decision.Decision`.

| Criterion | Assessment |
|---|---|
| Dependency direction | agentic-sop → external process (no import either way) |
| Visibility | works with internal types (DTOs are JSON) |
| API surface | small, documented protocol |
| Provider neutrality | **strong** (any language/module) |
| Testability | high (a fake command) |
| Failure isolation | **strong** (separate process; crash/timeout contained) |
| Cancellation/timeout | **SOP-owned** (`exec.CommandContext` + deadline) |
| Configuration complexity | low (one command/env key) |
| Lifecycle-authority leakage | none if the DTO carries only context + choice/confidence/diagnostics |
| Backward compatibility | additive; OFF by default |
| Migration cost | low |
| Multi-provider | **strong** |

**Selected (with Option B's factory).**

### Option D — Reuse the existing extension mechanism as-is (agent command harness)

Point the decision adapter at the existing `agent.CommandAgent` / `Harness`.

**Rejected.** The agent `Request` carries `Capability`, `Deliverables`,
`AcceptanceCriteria`, and produces an `Outcome` (`completed`/`needs_human`/
`failed`) — that is an *execution* contract. Reusing it would (a) conflate
execution with decision, and (b) hand the adapter execution-shaped authority
(needs_human/failed outcome signals that the lifecycle already interprets). It
violates "Providers evaluate. SOP governs."

**Selected design = Option B (internal provider-neutral interface + `deps` factory)
+ Option C (provider-neutral process/DTO boundary for external adapters).** These
are not two seams; they are the two halves of one seam (construction + transport),
and both reuse existing repository patterns (`deps` injection, `agent.CommandAgent`).

---

## 4. Selected design

**The seam keeps the Go contract internal and lets external adapters satisfy a
provider-neutral JSON protocol over the existing process-boundary pattern, then
converts a SOP-validated result into typed evidence that the existing
`autonomy` policy interprets.**

Flow:

```text
external decision adapter (sop-decision-adapters; any module/process)
        │  neutral JSON request  (stdin)
        ▼
internal/decision/command  — process adapter
        │  neutral JSON result  (stdout)
        ▼
SOP validation (fail-closed): known choice + valid confidence + status
        │
        ▼
deterministic interpretation (SOP-owned) -> failure.Classification
        │
        ▼
internal/autonomy (existing pure policy: Decide)
        │
        ├── AUTO_* / TERMINAL  (no authorization from the provider)
        └── HUMAN_APPROVAL_REQUIRED  (human boundary, unchanged)
```

Key decisions:

- `internal/decision` **stays internal**; no public package is created.
- The external adapter is reached by **process/transport**, not by import.
- **SOP owns** validation, interpretation, timeout/cancellation, and the human
  boundary. The provider only evaluates and returns data.
- Interpretation reuses the **existing** `autonomy.Decide` by first converting the
  validated decision result into a typed `failure.Classification` (a SOP-owned
  mapping), so `autonomy` stays pure and gains no new coupling. (If a dedicated
  `autonomy.DecideDecision` proves cleaner during implementation, it must take an
  autonomy-owned evidence type — the plan permits either but defaults to reusing
  `Decide`.)
- The seam **must not** map a decision to an execution-model class; it maps only to
  policy actions/risk. `decision.Route`'s model-tier vocabulary is not used for
  execution routing.

---

## 5. PHASE 4 — Minimum stable contract

The contract contains only concepts that cross the boundary.

### Request (SOP → adapter)

| Field | Type | Meaning |
|---|---|---|
| `version` | int | protocol version (required) |
| `kind` | string | opaque decision kind / use-case label |
| `question` | string | bounded free-text subject |
| `signals` | `map[string]number` | numeric inputs; absent key = unknown |
| `choices` | `[]string` | allowed choices when the kind is constrained |

### Result (adapter → SOP)

| Field | Type | Meaning |
|---|---|---|
| `version` | int | protocol version (must match) |
| `status` | string | `OK` \| `UNSUPPORTED` \| `ERROR` |
| `choice` | string | one of the known choices (required when `status=OK`) |
| `confidence` | number | optional probability in `[0,1]` |
| `diagnostics` | `map[string]string` | optional; advisory only |
| `error` | string | human-readable failure text when `status != OK` |

**Deliberately absent from the contract** (must never cross it): task mutation,
lifecycle transitions, approval mutation, commit/merge authority, scheduler
authority, policy-outcome authority, execution-model/class selection.

SOP validates before policy consumes:

- `status` must be a known value; anything else is a failure.
- `status=OK` requires a `choice` in the known set; unknown choice → failure.
- `confidence`, when present, must be finite and in `[0,1]`; `NaN`/`Inf`/out-of-range
  → failure; **missing** confidence → treated as *indeterminate* (fail closed when
  consumed, never "confident").
- `diagnostics` is advisory and never read by policy.
- The result is **data** until this validation succeeds.

---

## 6. PHASE 5 — Failure / fallback semantics

Modes: `disabled` (**default**) → strict no-op; `shadow` → run + record, never
affect policy; `selectable`/`live` → evidence consumed, fail-closed.

| Situation | Behavior |
|---|---|
| No provider configured (`disabled`) | **Strict no-op.** Existing behavior unchanged. |
| Provider unavailable / non-zero exit | Provider failure → never success. In `shadow`: recorded only. In `live`: fail closed (human). |
| Timeout | SOP deadline fires; process is terminated; provider failure (never success). |
| Cancellation (`ctx` done) | Provider failure; never success. |
| Unsupported capability (`UNSUPPORTED`) | Explicit failure category; never a guess/low-confidence stand-in; fail closed in `live`. |
| Malformed response (bad JSON / wrong version) | Validation failure; fail closed. |
| Unknown choice | Validation failure; fail closed. |
| Confidence out of range / `NaN` / `Inf` | Validation failure; fail closed. |
| Missing confidence | Indeterminate; fail closed when consumed. |
| Indeterminate result | Fail closed. |
| Provider panic / process failure | Non-zero exit/signal → provider failure (process isolation limits blast radius). |

Rules that hold in every case:

- A provider failure is **never** converted into approval or authorization.
- `shadow` mode can never change policy or state.
- Disabling the capability restores current deterministic behavior exactly.
- A provider can only *add attention* (raise risk / require a human); it can never
  *reduce* risk, auto-approve, or authorize execution.

---

## 7. PHASE 6 — Adoption model

```text
disabled (default)
    └─► shadow/evaluation        (provider runs; evidence recorded; policy unaffected)
            └─► selectable       (evidence consumed at the policy seam; may add attention)
                    └─► live      (fully consumed; still only evidence)
```

- No provider becomes live or default as part of this plan.
- The first proof uses a **fake/deterministic** provider (in-process) and a **fake
  external command** (cross-module), before any real provider exists.
- SMALL/MEDIUM/LARGE execution-model routing remains independent throughout.

---

## 8. Dependency direction and governance boundary

Dependency direction (target):

```text
internal/cli ──► internal/decision/command ──► external process (no import either way)
internal/cli ──► internal/decision
internal/cli ──► internal/failure ──► internal/autonomy
internal/decision ✗──► internal/model   (no edge; execution routing stays separate)
internal/autonomy ✗──► internal/decision (default; reuse Decide via a typed classification)
```

Governance boundary: the external provider observes and returns data; SOP validates,
interprets, and owns every lifecycle action, approval, and state transition. The
provider is never a policy authority.

---

## 9. Desired-state diagram

```text
        External Decision Adapter  (sop-decision-adapters; any module/process)
                     │
        provider-neutral JSON (request/result DTO)
                     │
                     ▼
   ┌───────────────────────────────────────────────────────────────┐
   │ agentic-sop                                                    │
   │  internal/decision/command   (run external adapter + validate) │
   │  deps.newDecisionProvider(cfg)   (factory; OFF by default)     │
   │                     │                                          │
   │        validated typed evidence -> failure.Classification      │
   │                     ▼                                          │
   │  internal/autonomy  (pure, deterministic, fail-closed)         │
   │                     │                                          │
   │        ┌────────────┼────────────────┐                         │
   │        ▼            ▼                ▼                         │
   │     Continue       Block        Human Approval                 │
   └───────────────────────────────────────────────────────────────┘
```

No Clef-specific behavior anywhere in `agentic-sop`.

---

## 10. Identified risks

| Risk | Mitigation |
|---|---|
| Decision evidence coupling to execution routing | Never map a decision to a model class; execution routing stays in `internal/model`; keep the `decision`↔`model` zero-edge. |
| Lifecycle-authority leakage through the DTO | DTO carries only context + choice/confidence/diagnostics; a guard test asserts no lifecycle/approval/commit/merge fields or imports. |
| Provider becoming policy authority | Fail-closed interpretation; provider can only add attention; unknown/invalid/failed always yields a human boundary or no-op. |
| Provider failure mistaken for success | `FailClosed` semantics copied from `RunJEV`; failure never yields approval. |
| Process-boundary security (shell injection, env leakage) | No interpolation of provider output into a shell; bounded, documented command config; provider output parsed as bounded JSON only. |
| Flaky/slow providers blocking runs | OFF by default; explicit deadline; shadow mode for evaluation. |
| Accidental public API / semver pressure | No public Go package; JSON protocol only. |
| Duplicating or conflating the JEV abstraction | Reuse the JEV *pattern* (bounded request, fail-closed outcome, advisory diagnostics) but keep decision and JEV vocabularies distinct; do not merge them. |
| Silent behavior change to existing installations | Capability OFF by default; a guard test proves "no provider configured ⇒ unchanged behavior". |

---

## 11. Anti-goals (must remain true after implementation)

- `agentic-sop` contains no Clef-specific or provider-name policy branch.
- An external provider result cannot transition lifecycle state, create an approval,
  authorize a commit/merge, or bypass validation/review/human approval.
- SMALL/MEDIUM/LARGE defaults and execution-model routing are unchanged and
  independent of decision providers.
- `internal/decision` remains internal.
- With the capability disabled, behavior is byte-for-byte the existing behavior.

---

## 12. Deliverables and next step

- This analysis: `docs/reports/decision-integration/SEAM-architecture-analysis.md`.
- The implementation plan:
  `docs/plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md`.

The plan is design-only and must not be implemented until separately approved. It is
written to be executable by SOP without model-generated discovery prerequisites.
