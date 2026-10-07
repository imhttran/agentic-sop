# AS-CLEF-003 — Provider Neutrality Audit

Status: audit complete (read-only). This report is the only repository change.

## 0. Scope and evidence baseline

This audit verifies that SOP's decision boundary is model/provider neutral. It reuses
the [AS-CLEF-002 decision-path trace](./AS-CLEF-002-decision-path-trace.md) as its
structural baseline and the AS-CLEF-001 run record
(`.agent-sdlc/runs/AS-CLEF-001/implementation.md`) for package inventory facts, and it
re-runs the provider-name searches in-tree because dependency classification is this
report's own deliverable.

Baseline facts reused from AS-CLEF-002 (unchanged here):

- The **live** decision boundary is the pure-function policy engine `internal/autonomy`
  (`Decide`/`DecideEarly`/`DecidePlanChange`), reached from live call sites
  `internal/cli/autonomy.go:22`, `internal/cli/early_jev.go:184`,
  `internal/cli/reconcile.go:126`.
- `internal/decision` (`Decision`/`Request`/`Choice`/`Provider`/`Route`) is a
  **structurally separate** boundary whose only in-tree consumers are tests
  (`internal/e2e/lifecycle/early_decision_test.go`); no production caller was found.

Searches performed for this audit (patterns over the repository):

| Pattern | Scope | Result |
|---|---|---|
| `clef` | `internal/` | no matches — no package, symbol, or wiring |
| `nimble` | `internal/` | no matches |
| `laya` | `internal/`, `.` | no matches |
| `julia` | `internal/`, `.` | no matches |
| `systemone` | `internal/`, `.` | no matches |
| `ollama` | `internal/decision/`, `internal/autonomy/` | no matches in those two packages |
| `ollama` | `.` | matches only in Ollama **transport/harness/config/docs**, not in decision semantics (see §1.6) |
| `jev` | `internal/` | `internal/jev` package + `internal/cli/early_jev.go` seam + test-only references |
| `decision.NewProvider` / `decision.Route` | `internal/` | only `internal/e2e/lifecycle/early_decision_test.go` (per AS-CLEF-002) |

Key files inspected: `internal/decision/decision.go`, `internal/autonomy/autonomy.go`,
`internal/autonomy/early.go`, `internal/jev/jev.go`, `internal/jev/evidence.go`,
`internal/jev/ollama.go`, `internal/jev/policy.go`, `internal/cli/cli.go`,
`internal/cli/early_jev.go`.

## 1. Dependency inventory and classification

The PRD categories are: **acceptable adapter-boundary dependency**, **configuration-only
dependency**, **abstraction leak**, **policy leak**, **dead/inert code**, **test-only
dependency**. Each PRD-named dependency below has at least one classified entry, and
explicit absent-from-the-live-path entries cite the confirming search.

### 1.1 Clef

| Occurrence | Class | Evidence |
|---|---|---|
| No package, symbol, config key, or wiring | **Absent from the live path** (absence finding) | `search_files clef internal` → no matches. Clef appears only as the readiness-program name in `docs/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md`, not as a code dependency. |

### 1.2 Jev

Jev is the only named provider with real in-tree code. It has two distinct seams:

| Occurrence | Class | Evidence |
|---|---|---|
| `internal/jev` package (boundary types `Request`/`Result`/`Analyzer`/`Evidence`) | **Acceptable adapter-boundary dependency** | `internal/jev/jev.go` defines a bounded analysis boundary with an `Analyzer` interface explicitly documented as non-owner, non-transitioning, and replaceable. It carries no lifecycle authority. |
| `internal/jev/ollama.go` — `OllamaAnalyzer` implementing `Analyzer` against the reused Ollama chat provider | **Acceptable adapter-boundary dependency** | `internal/jev/ollama.go` has `var _ Analyzer = (*OllamaAnalyzer)(nil)` and drives a `ChatProvider` (`Chat`/`ChatStructured`). It maps bounded `jev.Request` → prompt → validated `jev.Result` and never transitions state. This is the adapter seam by design. |
| Optional early-JEV checkpoint gate in `internal/cli/early_jev.go` / `internal/cli/jev.go`, selection via `d.newJEVAnalyzer(cfg)` | **Configuration-only dependency** | `internal/cli/cli.go:58-61,163` wires `newJEVAnalyzer func(cfg config.Config) (jev.Analyzer, error)`; `internal/cli/early_jev.go:155,158` is a strict no-op when nil/disabled. The early-JEV gates are ANDed with `early_jev.enabled` (per AS-CLEF-002 §2.5), so Jev only participates when configuration enables it. |
| `"jev"` name recognized-but-not-implemented in `decision.NewProvider` | **Dead/inert code** (with respect to `internal/decision`) | `internal/decision/decision.go` `NewProvider` maps only `""`/`"deterministic"`; every other name — including `"jev"` — returns `fmt.Errorf("decision: provider %q is not implemented...")`. The package doc states the Jev experiment is opt-in/comparison-only; nothing selects it. `decision.NewProvider` itself has no production caller. |

**Mechanism check.** Neither Jev seam introduces provider-specific decision *semantics*:
`autonomy.DecideEarly` consumes only the typed, closed-enumeration
`jev.Evidence` (`Purpose`/`Severity`/`Category`/`Status`), and Jev summary prose is
explicitly not a decision input (`internal/jev/evidence.go`; AS-CLEF-002 §2.8).
The `"jev"` string in `internal/decision` is an inert name, not a wire-protocol or
policy branch.

### 1.3 Nimble

| Occurrence | Class | Evidence |
|---|---|---|
| No package, symbol, config key, or wiring | **Absent from the live path** (absence finding) | `search_files nimble internal` → no matches (confirms AS-CLEF-002 Q2). |

### 1.4 Laya

| Occurrence | Class | Evidence |
|---|---|---|
| No package, symbol, config key, or wiring | **Absent from the live path** (absence finding) | `search_files laya internal` and `search_files laya .` → no matches (confirms AS-CLEF-002 Q5). |

### 1.5 Julia

| Occurrence | Class | Evidence |
|---|---|---|
| No package, symbol, config key, or wiring | **Absent from the live path** (absence finding) | `search_files julia internal` and `search_files julia .` → no matches (confirms AS-CLEF-002 Q5). |

### 1.6 Ollama-specific decision semantics

This is the only named item that has substantial in-tree code, so the audit separates
Ollama **transport/config** (legitimate, outside the decision semantics) from any
Ollama-specific **decision** semantics.

| Occurrence | Class | Evidence |
|---|---|---|
| `internal/decision/`, `internal/autonomy/` contain no `ollama` reference | **Provider-neutral** (no dependency) | `search_files ollama internal/decision` and `... internal/autonomy` → no matches. The decision/policy packages never name Ollama. |
| `internal/jev/ollama.go` — schema-constrained ChatStructured call, JSON schema for JEV output | **Acceptable adapter-boundary dependency** | The Ollama-specific wire concern (the `format` JSON-schema field, `/api/chat` prompt shapes) is confined to the Jev analyzer adapter, behind the provider-neutral `jev.Analyzer` interface. `autonomy` consumes only the validated typed `jev.Evidence`. |
| `internal/ollamaagent`, `internal/agent/ollama.go`, `internal/provider/ollama/`, `providers.ollama.endpoint`, `SOP_MODEL_<CLASS>_PROVIDER=ollama` | **Configuration-only dependency** (transport) | `search_files ollama .` shows Ollama only in harness/transport/config/doc locations (`internal/ollamaagent`, `internal/provider/ollama`, `.env`, `docs/reference/CONFIGURATION.md`). Model/provider selection is data-driven (`docs/architecture/orchestration.md` documents the intent that nothing depends on a concrete provider). None of these encode *decision* semantics. |

**Mechanism check for an Ollama decision-semantics leak.** None was found. There is no
Ollama model name, endpoint, prompt field, or provider-name conditional inside
`internal/decision` or `internal/autonomy`. Ollama appears only as (a) a Jev adapter
transport and (b) a selectable execution/agent provider — both outside the decision
boundary.

### 1.7 SystemOne-specific decision semantics

| Occurrence | Class | Evidence |
|---|---|---|
| No package, symbol, config key, or wiring | **Absent from the live path** (absence finding) | `search_files systemone internal` and `search_files systemone .` → no matches (confirms AS-CLEF-002 Q3). |

### 1.8 Provider-name conditionals and model-specific policy behavior

| Occurrence | Class | Evidence |
|---|---|---|
| `switch strings.TrimSpace(name)` in `decision.NewProvider` | **Configuration-only dependency** (provider selection) + **dead/inert** in production | `internal/decision/decision.go`. It is a provider *selector*, not policy: it maps a configured name to a provider and hard-errors on unknown names. It encodes no decision semantics and has no production caller (AS-CLEF-002 Q9/Q12). |
| `riskyKeywords` in `DeterministicProvider.Decide` | **Provider-neutral** (not a leak) | `internal/decision/decision.go`. Generic change-risk keywords (`migration`, `concurrency`, `race`, `security`, `auth`, `encrypt`), not a provider or model identity. |
| `internal/autonomy` policy switches | **Provider-neutral** (not a leak) | `internal/autonomy/autonomy.go`. Branches on `failure.Kind`/`failure.Disposition` and `Policy` flags only — no provider, model, or vendor identity. |

**No entry was classified as an abstraction leak or a policy leak**, because no
mechanism (provider-name conditional in policy, wire-protocol field in a shared type)
was found; every candidate is listed above with the file/symbol that would exhibit it.

## 2. Generic contract assessment

The generic SOP decision contract is assessed at two surfaces:

- **Live policy surface** — `internal/autonomy` (`Decide`/`DecideEarly`/`DecidePlanChange`,
  `Decision{Action, Level, Risk, RequiresHuman, Reason, Classification}`).
- **Structured decision surface** — `internal/decision`
  (`Request{UseCase, Subject, Signals}`, `Decision{Choice, Confidence, Metadata}`,
  `Provider{Name, Decide}`, `Thresholds`, `Route`).

### 2.1 Concept-by-concept mapping

| PRD concept | Surfaced as | Verdict | Evidence |
|---|---|---|---|
| **Decision question/request** | `internal/decision.Request{UseCase, Subject, Signals}`; live path's input tuple `(jev.Evidence, failOn, Policy)` | Genuinely generic (structured) / provider-neutral but loose (live path) | `internal/decision/decision.go`; `internal/autonomy/early.go` `DecideEarly(ev, failOn, p)`. The live input is a typed evidence tuple, not a single named request struct — provider-neutral, but less crisp. |
| **Allowed choices** | `decision.Choice` closed set (`LOW`/`MEDIUM`/`HIGH`/`HUMAN`); autonomy `Action` closed set (`AUTO_*`/`TERMINAL`/`HUMAN_APPROVAL_REQUIRED`) | Genuinely generic | `internal/decision/decision.go`; `internal/autonomy/autonomy.go`. Both are closed enumerations, no provider-specific values. |
| **Score / binary / choice semantics** | Choice enum (multi-valued); autonomy `RequiresHuman bool` (binary); `Route` (target selection); `Action` (choice) | Genuinely generic | `internal/decision/decision.go` `Route`; `internal/autonomy/autonomy.go`. All three shapes are present as first-class generic concepts. No provider scoring rubric is hardcoded. |
| **Probability / confidence** | `decision.Decision.Confidence float64` (range [0,1]); `jev.Evidence.Confidence float64` (bounded, advisory) | Genuinely generic | `internal/decision/decision.go` (consumed by `Route` via `Thresholds`); `internal/jev/evidence.go` (documented as advisory, never a bare decision rule). No provider-specific probability encoding. |
| **Evidence / diagnostics** | `decision.Decision.Metadata map[string]string`; `jev.Evidence`/`EvidenceItem` typed diagnostics; `jev.Finding`; `ProviderError{Kind, Err, Attempts}` | Genuinely generic | `internal/decision/decision.go`; `internal/jev/evidence.go`; `internal/jev/jev.go`; `internal/jev/ollama.go`. Diagnostics are typed and bounded; `Metadata` is provider-neutral key/value. |
| **Provider failure** | `decision.Provider.Decide(...) (Decision, error)`; `jev.ProviderError` with `ProviderErrorKind` (`PROVIDER_UNREACHABLE`/`CONFIG_INVALID`/`TIMEOUT`/`MALFORMED_OUTPUT`) | Genuinely generic | `internal/decision/decision.go`; `internal/jev/ollama.go`. The focus-class error enum names failure *categories*, not a wire protocol. |
| **Unsupported capability** | `decision.NewProvider` hard error on unknown names | Provider-neutral but **partial** (not typed) | `internal/decision/decision.go`: `fmt.Errorf("decision: provider %q is not implemented (only \"deterministic\")")` — an error *string*, not a sentinel/typed unsupported-capability category, and there is no capability manifest. The live autonomy path has no provider factory at all. |

### 2.2 Does the contract encode a provider's wire protocol?

**No.** Evidence from the field definitions:

- `decision.Request{UseCase, Subject, Signals map[string]float64}` and
  `decision.Decision{Choice, Confidence, Metadata map[string]string}`
  (`internal/decision/decision.go`) are abstract: a use-case label, free-text subject,
  numeric signals, a closed choice, a normalized confidence, and opaque string metadata.
  None of these mirror a provider wire format (no model id, endpoint, message array,
  token/usage field, or provider "finish_reason"/"role" shape).
- The live `autonomy.Decision` (`internal/autonomy/autonomy.go`) carries
  `Action`/`Level`/`Risk`/`RequiresHuman`/`Reason`/`Classification` — all SOP lifecycle
  concepts, no transport fields.
- The only provider wire concern in the analyzed code (Ollama's `/api/chat` and its
  `format` JSON-schema field) is isolated inside `internal/jev/ollama.go` behind the
  provider-neutral `jev.Analyzer` interface; `autonomy` consumes only the validated
  typed `jev.Evidence`.

### 2.3 Which contract surface SOP policy consumes vs. deliberately ignores

- The live policy surface (`autonomy.Decide`/`DecideEarly`/`DecidePlanChange`) consumes
  typed failure `Classification`, `Policy` flags, and typed `jev.Evidence`
  (`internal/autonomy/autonomy.go`, `internal/autonomy/early.go`).
- `decision.Decision.Confidence` is read **only** by `decision.Route` (via
  `Thresholds.RouteToStrongModel`/`RequireHuman`); `decision.Route` is consumed only in
  tests (AS-CLEF-002 §2.8, §9).
- On the live autonomy path confidence is **display-only**: `jev.Evidence.Confidence` is
  documented as advisory and "never directly controls lifecycle state"
  (`internal/jev/evidence.go`), and `DecideEarly`'s doc states "Confidence is
  deliberately not a decision input here" (`internal/autonomy/early.go`).
  `failure.Classification.Confidence` exists but is not a policy branch.

These are stated from cited code only; behavioral guarantees are deferred to AS-CLEF-005
(see §5).

## 3. Materiality determination

Candidates considered and their materiality:

| Candidate leak | Mechanism (if it existed) | Determination |
|---|---|---|
| Live autonomy path has no `Provider` interface (it is a pure function boundary) | Would be a mechanism only if policy branched on a provider identity | **Non-material.** `internal/autonomy` is deliberately pure/deterministic and branches only on typed failure data and `Policy` (per its package doc: "no I/O, no clock, no LLM"). The absence of an interface is a design choice, not a leak. |
| `decision.NewProvider` expresses unsupported capability only as an error string | Would be a leak only if a consumer depended on parsing the message or a provider-specific category | **Non-material.** The message is a diagnostic for unknown config names; no code parses it, and the factory has no production caller (AS-CLEF-002 Q9/Q12). It is a minor contract-tidiness observation, not an abstraction gap. |
| Jev's Ollama adapter might push wire concerns into shared types | Would be a leak if `jev.Request`/`Result` or `autonomy` consumed Ollama wire fields | **Non-material.** The wire concern is confined to `internal/jev/ollama.go` behind `jev.Analyzer`; shared types are abstract (§2.2). |
| Ollama provider names in model-routing config | Would be a policy leak if decision code branched on the provider name | **Non-material.** Provider selection is data-driven config (`.env`, `docs/reference/CONFIGURATION.md`); `internal/decision`/`internal/autonomy` contain no `ollama` reference. |
| Absent providers (Clef/Nimble/Laya/Julia/SystemOne) | n/a | **Non-material.** Absence is the intended state (adapters belong in `sop-decision-adapters`). |

**Overarching determination: no material abstraction leak and no policy leak exists.**
Per the PRD gate, the hardening task **AS-CLEF-004 is NOT_REQUIRED**.

## 4. AS-CLEF-004 gate statement

**AS-CLEF-004: NOT_REQUIRED.** The audit found no material abstraction leak and no
policy leak (§3). The one contract observation (the stringly-typed unsupported-capability
signal in `decision.NewProvider`) is non-material: it is a diagnostic for unknown
configured provider names, is never parsed, and its factory has no production caller.
Accordingly no hardening scope is opened. (Had a material leak been evidenced, the gate
would read REQUIRED with the smallest scope being the named mechanism and file/symbol.)

## 5. UNKNOWN / deferrals

- **Behavioral failure/approval semantics** (whether a provider failure can ever become
  implicit success under load/timeouts) — code branches inspected
  (`internal/cli/early_jev.go`, `internal/autonomy/autonomy.go`) but runtime behavior not
  proven here. **Deferred to AS-CLEF-005.** This report makes no behavioral guarantee.
- **Definitive execution-model routing vs decision-provider routing separation** —
  observed only through the traced path (`decision.Route` consumed in tests; live path
  produces lifecycle `Action`s). **Deferred to AS-CLEF-006.**
- **Absence of any architecture guard** preventing provider leakage — not present
  (scheduled as AS-CLEF-007). Recorded as a finding; this audit does not depend on it.
- **Non-absence of a production config selecting a non-deterministic
  `decision.Provider`** — searches for `decision.NewProvider`/`decision.Route` over
  `internal` found only test hits; not asserted as impossible, only as not found in-tree.

## 6. Scope statement

No production source, test, configuration, or default was modified. This report is the
only repository change. Pre-existing user-owned working-tree changes, if any, were left
untouched.
