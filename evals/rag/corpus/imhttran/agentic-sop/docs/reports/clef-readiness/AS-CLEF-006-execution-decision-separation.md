# AS-CLEF-006 — Verify Execution/Decision Separation

Status: verification complete (read-only). This report is the only repository change.

## 0. Scope and evidence baseline

This note verifies that specialized **decision providers** are independent of
**execution-model routing**: changing or adding a decision provider does not
inherently change `SOP_MODEL_DEFAULT_CLASS`, the SMALL/MEDIUM/LARGE execution
models, execution fallback policy, or local/cloud execution policy. It reuses the
[AS-CLEF-002 decision-path trace](./AS-CLEF-002-decision-path-trace.md) and the
[AS-CLEF-003 provider-neutrality audit](./AS-CLEF-003-provider-neutrality-audit.md)
as the structural baseline, and it re-inspects both routing surfaces in-tree.

Baseline facts reused (unchanged here):

- The **live** decision boundary is the pure-function policy engine
  `internal/autonomy` (`Decide`/`DecideEarly`/`DecidePlanChange`), reached from
  live call sites `internal/cli/autonomy.go:22`, `internal/cli/early_jev.go:184`
  (via `DecideEarly`), `internal/cli/reconcile.go:126`.
- `internal/decision` (`Request`/`Decision`/`Choice`/`Provider`/`NewProvider`/
  `DeterministicProvider`/`Thresholds`/`Route`) is a **structurally separate**
  boundary whose only in-tree consumers are tests
  (`internal/e2e/lifecycle/early_decision_test.go`); no production caller exists
  (AS-CLEF-002 §2.10, §9; AS-CLEF-003 §1.2, §2.3).
- AS-CLEF-002 **Q6** and AS-CLEF-003 **§5** explicitly defer the definitive
  execution-model routing vs decision-provider routing separation to
  **AS-CLEF-006** — this note.

Scope statement: no production source, test, configuration, or default was
modified. This report is the only repository change. Pre-existing user-owned
working-tree changes, if any, were left untouched.

## 1. The three surface definitions (PRD terminology)

The three concepts are distinct and are used consistently in this note:

- **Execution model** — *generates/reasons/implements work.* The concrete
  provider/model a run executes on, selected by a routing class
  (`small`/`medium`/`large`). Owned by `internal/model`
  (`Class`, `DefaultRoute`, `Resolve`, `Locality`, fallback policy,
  `ExecutionTarget`). It is what actually produces tokens.
- **Decision provider** — *evaluates a constrained decision and returns
  evidence.* A bounded classifier that maps a constrained request to a choice
  plus confidence/evidence; it must never execute actions. Owned by
  `internal/decision` (`Provider`, `NewProvider`, `DeterministicProvider`,
  `Request`, `Decision`, `Choice`, `Route`), with the live equivalent being the
  pure policy engine `internal/autonomy`.
- **SOP policy** — *governs what action is permitted.* The deterministic rules
  that interpret evidence/decisions into permitted lifecycle actions
  (`AUTO_*`/`TERMINAL`/`HUMAN_APPROVAL_REQUIRED`) and the human-approval
  boundary; owned by `internal/autonomy` (`Decide`, `DecideEarly`,
  `DecidePlanChange`, `humanDecision`) and by configuration (`config.Config`,
  `autonomy.Policy`).

The separation this note verifies: **a decision provider evaluates and returns a
bounded decision; the execution model is a separate, configuration-owned
selection; SOP policy is what decides whether an action is permitted. Changing
which decision provider exists or is selected touches neither the execution
model table nor the policy that governs execution routing.**

## 2. Execution-model routing surface (`internal/model`)

All execution-model routing is owned by `internal/model/model.go` and
`internal/model/execution.go`. The following symbols are the entire coupling
surface this note must show is independent of decision providers:

| Symbol | File | Role |
|---|---|---|
| `SOP_MODEL_DEFAULT_CLASS` (`EnvDefaultClass`) | `internal/model/model.go` (`Environment variable names` block) | Environment override for `models.default_class`; the env key named in the PRD. |
| `Class` (`ClassSmall`/`ClassMedium`/`ClassLarge`) | `internal/model/model.go` | The SMALL/MEDIUM/LARGE routing tiers. |
| `DefaultRoute()` | `internal/model/model.go` | The single owner of the built-in SMALL/MEDIUM/LARGE provider/model/locality defaults. |
| `Resolve(Inputs)` | `internal/model/model.go` | Pure resolution over config + environment (+ CLI/router class); returns a `Result`/`Selection`. |
| `Locality` (`LocalityLocal`/`LocalityCloud`) | `internal/model/model.go` | Local/cloud execution policy metadata and guard. |
| `allowCloudFallback` / `AllowCloudFallbackForLocal` (`SOP_MODEL_ALLOW_CLOUD_FALLBACK_FOR_LOCAL`) | `internal/model/model.go` | Execution fallback policy (config-completeness local→cloud guard). |
| `localFallback` / `FallbackConfig` (`Fallback`) | `internal/model/model.go` | Local-first runtime cloud-fallback candidate (`ReasonLocalFallback`). |
| `ExecutionTarget` / `ExecutionSource` | `internal/model/execution.go` | The actual executing provider/model/locality and its source. |

### 2.1 Recorded (unchanged) SMALL/MEDIUM/LARGE defaults

`internal/model.DefaultRoute()` defines, verbatim:

- `DefaultClass: ClassMedium` — the built-in default class when no layer chooses one.
- `Small: {Provider: "ollama", Name: "qwen3:4b", Locality: LocalityLocal, Fallback: {Provider: "ollama", Name: "nemotron-3-nano:30b-cloud", Locality: LocalityCloud}}`
- `Medium: {Provider: "ollama", Name: "nemotron-3-super:cloud", Locality: LocalityCloud}`
- `Large: {Provider: "ollama", Name: "deepseek-v4.1-flash:cloud", Locality: LocalityCloud}`
- `FallbackClass` intentionally left empty; the fallback class defaults to the
  selected class unless a layer names one. `AllowCloudFallbackForLocal` defaults
  to false (`allowCloudFallback`).

These values are **not changed** by this task (see §5).

### 2.2 Class selection precedence (no decision-provider input)

`Resolve` → `selectClass(DefaultRoute(), in.Config, env, cli, in.RoutedClass, in.RoutedReason)`
selects the class in this precedence (highest first):

1. `--model-class` CLI override (`ReasonCLIClass`),
2. the deterministic router's `RoutedClass` (`ReasonRouter`/`SourceRouter`),
3. `SOP_MODEL_DEFAULT_CLASS` environment (`ReasonEnvClass`),
4. `models.default_class` config (`ReasonConfigClass`),
5. built-in default `ClassMedium` (`ReasonBuiltinClass`).

Every input to this rule is a configuration/environment/CLI/router value. There
is **no decision-provider, `Choice`, `Confidence`, or `Thresholds` input** in the
class-selection path.

### 2.3 Execution fallback and local/cloud policy (no decision-provider input)

Two distinct fallbacks exist in `internal/model`, both configuration-derived:

- **Config-completeness fallback (`fallback_class`)**: when the chosen class has
  no model, another class supplies one; a local class with no model and a cloud
  fallback fails rather than using cloud unless `allow_cloud_fallback_for_local`
  is set (`allowCloudFallback`, default false). Reason `ReasonFallbackClass`.
- **Local-first runtime fallback**: a local class's configured `fallback` model
  is reported as a **candidate** (`Result.LocalFallback`) and applied only by a
  caller's positive availability observation (`ReasonLocalFallback`,
  `SourceCloudFallback`); it is never triggered by a generation/validation/gate
  failure (package doc, `internal/model/model.go`; `internal/model/execution.go`).

Local/cloud policy is `Locality` (descriptive metadata + guard); it is never
inferred from a model name and carries no decision-provider input.

## 3. Decision-provider surface (`internal/decision`) and live policy boundary (`internal/autonomy`)

### 3.1 `internal/decision` (structured decision-provider surface)

`internal/decision/decision.go` defines, verbatim:

- `Provider interface { Name() string; Decide(ctx, Request) (Decision, error) }`
  — doc: "Implementations must not execute actions."
- `NewProvider(name string) (Provider, error)` — maps `""`/`"deterministic"` to
  `DeterministicProvider{}`; every other name (including `"jev"`) returns
  `fmt.Errorf("decision: provider %q is not implemented (only \"deterministic\")")`
  (a hard error; no silent fallback).
- `DeterministicProvider` — the default and only implemented provider; classifies
  from `riskyKeywords` and `Signals["files"]+Signals["criteria"]`.
- `Thresholds{RouteToStrongModel, RequireHuman}` and
  `Route(Thresholds, Decision) Target` — maps a decision to
  `SmallModel`/`StrongModel`/`HumanTarget`; **fails closed** to `HumanTarget` on
  an unknown choice or invalid confidence.
- `Request{UseCase, Subject, Signals}` / `Decision{Choice, Confidence, Metadata}`.

Consumers: `decision.NewProvider` and `decision.Route` were searched across
`internal`; the **only** call sites are in
`internal/e2e/lifecycle/early_decision_test.go`
(`decision.NewProvider("deterministic")` at lines 25/243; `decision.Route` at
lines 53/101/127/147/183/210). No production caller exists.

### 3.2 `internal/autonomy` (live policy boundary)

The live boundary is a pure function API, not a provider interface:
`autonomy.DecideEarly(jev.Evidence, []string, Policy) Decision` and
`autonomy.Decide(failure.Classification, Policy) Decision`, with the
human-approval boundary enforced by `humanDecision` (`Action:
ActionHumanApproval`, `RequiresHuman: true`). It branches only on typed failure
data and `Policy` flags — no provider, model, or vendor identity (AS-CLEF-003
§1.8).

### 3.3 Decision-provider capability

The "decision provider capability" is the `decision.Provider` interface
(`Name`, `Decide`) — a bounded evaluate-and-return-evidence contract that
"must not execute actions". The live path uses the equivalent pure
`autonomy.Decide*` functions. There is no `DecisionCapability`-named interface;
that absence is a recorded finding, not a gap (AS-CLEF-002 §2.3; AS-CLEF-003
§2.1).

## 4. No-coupling evidence

### 4.1 Inspection: no cross-reference in either direction

Targeted searches over the two surfaces:

| Search | Scope | Result |
|---|---|---|
| `internal/decision` | `internal/model` | **no matches** — `internal/model` never imports or names the decision package/provider |
| `decision` | `internal/model` | matches only in prose/comments: `internal/model/model.go:143` ("never parsed to drive a decision"), `model.go:567` ("a CANDIDATE, not a decision"), `internal/model/execution.go:48` ("never free-form decision text"), `internal/model/execution_test.go:12` (test message "the routing decision is unchanged") — **no symbol reference** |
| `internal/model` | `internal/decision` | **no matches** — `internal/decision` never imports or names the execution-model package |
| `decision.NewProvider` | `internal` | only `internal/e2e/lifecycle/early_decision_test.go:25,243` (test-only; no production caller) |
| `decision.Route` | `internal` | only `internal/e2e/lifecycle/early_decision_test.go:53,101,127,147,183,210` (test-only) |

Conclusion: there is no import edge and no symbol reference between the two
surfaces in either direction. The only textual overlaps are comments/test
messages, which do not couple behavior.

### 4.2 Per-item independence (the five required items)

For each required item, the owning symbol and the reason it cannot be reached by
a decision-provider change:

| Item | Owning symbol (file) | Observed independence |
|---|---|---|
| `SOP_MODEL_DEFAULT_CLASS` | `EnvDefaultClass` (`internal/model/model.go`) | Read and parsed only by `routeFromEnv(lookup)` inside `Resolve`; consumed by `selectClass` as `env.DefaultClass` (`ReasonEnvClass`). No decision-provider/`Choice`/`Confidence` input. |
| SMALL execution model | `DefaultRoute().Small` (`internal/model/model.go`) | Literal `{ollama, qwen3:4b, local}` + fallback `{ollama, nemotron-3-nano:30b-cloud, cloud}`; merged by `entryFor`/`fallbackEntry` from config/env/default only. No decision-provider input. |
| MEDIUM execution model | `DefaultRoute().Medium` (`internal/model/model.go`) | Literal `{ollama, nemotron-3-super:cloud, cloud}`; same merge path. No decision-provider input. |
| LARGE execution model | `DefaultRoute().Large` (`internal/model/model.go`) | Literal `{ollama, deepseek-v4.1-flash:cloud, cloud}`; same merge path. No decision-provider input. |
| Execution fallback policy | `allowCloudFallback` + `FallbackConfig`/`localFallback` (`internal/model/model.go`) | Driven only by `AllowCloudFallbackForLocal` (config/env) and a caller's availability observation; `Resolve` reports a candidate and never probes a runtime. No decision-provider input. |
| Local/cloud execution policy | `Locality` (`LocalityLocal`/`LocalityCloud`) (`internal/model/model.go`) | Descriptive metadata + local→cloud guard set by routing config; never inferred from a model name; `internal/model/execution.go` records the actual executing locality. No decision-provider input. |

Because `internal/model` has no reference to `internal/decision` (§4.1), and
`Resolve`'s inputs are `Inputs{Config, Lookup, CLIClass, RoutedClass,
RoutedReason}` (all configuration/CLI/router values), a change to a decision
provider (adding one, changing `NewProvider`, or selecting a different
`decision.Provider`) cannot alter any of the six values above: it is not an
input to resolution, and there is no import path by which it could become one.

### 4.3 "Change the decision provider" — observed result

- Adding/altering a provider inside `internal/decision` changes only
  `internal/decision` symbols (`Provider`, `NewProvider`,
  `DeterministicProvider`, `Route`). Those symbols are referenced only from
  `internal/e2e/lifecycle/early_decision_test.go` (§4.1). Changing them cannot
  reach `internal/model`, because `internal/decision` does not import
  `internal/model` and `internal/model` does not import `internal/decision`.
- Selecting a different provider (for example passing a name other than
  `"deterministic"` to `decision.NewProvider`) is processed entirely inside
  `internal/decision` (`switch strings.TrimSpace(name)`): implemented names
  return a provider, unknown names return an error string. Neither branch reads
  or writes any `internal/model` value.
- Consequently `SOP_MODEL_DEFAULT_CLASS`, the SMALL/MEDIUM/LARGE defaults,
  execution fallback policy, and local/cloud policy are **unchanged** by a
  decision-provider change; their values remain exactly those in
  `DefaultRoute()` (§2.1).

### 4.4 Existing test evidence

- `internal/cli/modelroute_test.go` exercises `SOP_MODEL_DEFAULT_CLASS`
  resolution (including the unset/leaked-default-class case) — execution-model
  routing behavior, independent of any decision provider.
- `internal/model/execution_test.go` covers the execution source/`ExecutionTarget`
  distinction (routing class carried over unchanged; availability fallback never
  rewrites the class).
- `internal/e2e/lifecycle/early_decision_test.go` exercises
  `decision.NewProvider`/`decision.Route` in isolation as the decision-provider
  surface.

No test constructs a link between the two surfaces, which is consistent with the
absence of any production link.

No new test file was added: this task's scope is read-only verification, and the
separation is established by inspection (§4.1–§4.3) plus the existing tests
above. This matches the AS-CLEF-002/003 convention that the report is the only
repository change.

## 5. Confirmation: SMALL/MEDIUM/LARGE defaults unchanged

Confirmed: the SMALL/MEDIUM/LARGE execution model defaults are **not changed** by
this task. They remain exactly `internal/model.DefaultRoute()`:

- SMALL = `ollama` / `qwen3:4b` / local, with cloud fallback
  `ollama` / `nemotron-3-nano:30b-cloud` / cloud.
- MEDIUM = `ollama` / `nemotron-3-super:cloud` / cloud.
- LARGE = `ollama` / `deepseek-v4.1-flash:cloud` / cloud.
- Built-in `DefaultClass = medium`.

No production source, test, configuration, or default was modified. (A
configuration observation, not a routing change: `.env`/`.env.example` set
`SOP_MODEL_DEFAULT_CLASS=large`, while `docs/reference/CONFIGURATION.md` documents
`models.default_class` default `medium`. This is an environment/config value, not
a change caused by decision providers, and it is deliberately not "fixed" here.)

## 6. Acceptance-criteria mapping

| Acceptance criterion | Where satisfied |
|---|---|
| Changing/adding a decision provider does not inherently change `SOP_MODEL_DEFAULT_CLASS`, SMALL/MEDIUM/LARGE execution models, execution fallback policy, or local/cloud execution policy. | §4.1 (no cross-reference), §4.2 (per-item owners and inputs), §4.3 (observed result). |
| The architecture note documents the distinction between execution model, decision provider, and SOP policy as specified. | §1 (three definitions verbatim: execution model — generates/reasons/implements work; decision provider — evaluates a constrained decision and returns evidence; SOP policy — governs what action is permitted). |
| Evidence includes an architecture note plus tests/inspection showing no unintended coupling between execution-model routing and decision-provider routing. | This note; §4.1 inspection searches; §4.4 existing test coverage (`internal/cli/modelroute_test.go`, `internal/model/execution_test.go`, `internal/e2e/lifecycle/early_decision_test.go`). |
| SMALL/MEDIUM/LARGE execution model defaults are not changed. | §2.1 and §5 (exact `DefaultRoute()` values recorded as unchanged). |

## 7. Deferrals and UNKNOWNs

- **Behavioral failure/approval semantics** (can a provider failure become
  implicit success under load/timeouts?) — not proven here; the live seam keeps
  `res.Decision` zero and `res.Escalate` false on provider failure, but runtime
  behavior is not asserted. **Deferred to AS-CLEF-005.** This note makes no
  behavioral guarantee.
- **Architecture guard preventing provider leakage** — no `internal/archtest`
  guard exists; this note does not introduce one and does not depend on it.
  **Deferred to AS-CLEF-007.**
- **Non-absence of a production config selecting a non-deterministic
  `decision.Provider`** — searches over `internal` found only test hits; this is
  recorded as "not found in-tree", not as impossible.

BLOCKED is not used: every required separation fact is answered with file/symbol
citations and searches, and the remaining gaps are explicitly deferred with named
evidence.
