# AS-CLEF-010 — Agentic-SOP Readiness Decision

**Decision: `READY_WITH_NOTES`.** Clef-Flash can be added through the external
decision-adapter boundary **without introducing Clef-specific behavior into
agentic-sop**. The boundary is provider-neutral and sufficient; one non-blocking
integration limitation is recorded (§11).

Readiness vocabulary: the operator GO instruction specifies `READY` /
`READY_WITH_NOTES` / `NOT_READY`; the plan specifies `READY` / `READY_WITH_NOTES` /
`BLOCKED`. These are the same three-way decision (`NOT_READY` == `BLOCKED`). This
decision uses `READY_WITH_NOTES`, which is common to both vocabularies.

## 1. Headline

agentic-sop exposes a provider-neutral, fail-closed decision boundary that an
external `sop-decision-adapters` Clef adapter can satisfy without adding any
Clef-specific policy or lifecycle behavior to agentic-sop; the remaining
limitations are non-blocking integration notes.

## 2. Repository state

- Branch: `main`
- HEAD: `05d2396381ffdf74bf3010d4d6ff0abb3bb61814`
- Upstream: `origin/main` (equal to local `main` at start; no divergence, no push
  performed)
- Working tree: clean at start; this sequence adds only readiness documents
  (§10). No production source, test, configuration, or default is changed.

## 3. Task state (AS-CLEF-001 … AS-CLEF-010)

Authoritative states are read from the SOP store (`sop status`). AS-CLEF-001
through AS-CLEF-007 are preserved as found and are not redone.

| Task | State | One-line evidence |
|---|---|---|
| AS-CLEF-001 Capture Repository Truth | `LOCAL_DONE` | Baseline report + run record (`.agent-sdlc/runs/AS-CLEF-001/implementation.md`). |
| AS-CLEF-002 Trace the Live Decision Path | `LOCAL_DONE` | `docs/reports/clef-readiness/AS-CLEF-002-decision-path-trace.md` (live path = `internal/autonomy`; `internal/decision` unwired outside tests). |
| AS-CLEF-003 Audit Provider Neutrality | `LOCAL_DONE` | `docs/reports/clef-readiness/AS-CLEF-003-provider-neutrality-audit.md` — no material abstraction or policy leak. |
| AS-CLEF-004 Harden the Generic Contract if Required | `NOT_REQUIRED` | `.agent-sdlc/runs/AS-CLEF-004/not-required.json` — AS-CLEF-003 found no gap; operator-recorded disposition. |
| AS-CLEF-005 Verify Failure and Approval Semantics | `LOCAL_DONE` | `internal/decision/governance_test.go` — fail-closed suite (unavailable/timeout/malformed/empty/invalid/low/unknown/internal, no state mutation). |
| AS-CLEF-006 Verify Execution/Decision Separation | `LOCAL_DONE` | `docs/reports/clef-readiness/AS-CLEF-006-execution-decision-separation.md` — no coupling; SMALL/MEDIUM/LARGE defaults unchanged. |
| AS-CLEF-007 Add an Architecture Guard | `LOCAL_DONE` | `internal/archtest/guard_test.go` — `TestArchitectureGuard` blocks forbidden provider imports into core/policy. |
| AS-CLEF-008 Define the Adapter-Side Contract | deliverable complete this run | `docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md` (finalized). |
| AS-CLEF-009 Run Full Verification | deliverable complete this run | `docs/reports/clef-readiness/AS-CLEF-009-full-verification.md` (and `…-verification.md`); all gates PASS. |
| AS-CLEF-010 Readiness Decision | this report | `docs/reports/clef-readiness/AS-CLEF-010-readiness-decision.md`. |

Note on governance: the SOP store still lists AS-CLEF-008/009/010 as `PLANNED`.
Their **deliverables are produced and evidenced** by this run; advancing their
authoritative task state is a governed SOP lifecycle action (through the normal
`run`/approval path) and is intentionally **not** performed by this document. No
task is being reported as `LOCAL_DONE` without a governed completion, and no
`NOT_REQUIRED`/`BLOCKED` state is fabricated. AS-CLEF-004 remains `NOT_REQUIRED`.

## 4. Current decision architecture

Two surfaces exist:

- **Live policy boundary (production)** — `internal/autonomy`:
  `Decide`/`DecideEarly`/`DecidePlanChange`, pure and deterministic. Reached from
  `internal/cli/early_jev.go:184`, `internal/cli/autonomy.go:22`,
  `internal/cli/reconcile.go:126`. It consumes typed evidence/classification only
  and produces a lifecycle action from `AUTO_CONTINUE | AUTO_RETRY | AUTO_FIX |
  AUTO_RECONCILE | TERMINAL | HUMAN_APPROVAL_REQUIRED`.
- **Structured decision-provider surface** — `internal/decision`:
  `Request{UseCase, Subject, Signals}`, `Decision{Choice, Confidence, Metadata}`,
  `Choice{LOW, MEDIUM, HIGH, HUMAN}`, `Provider{Name, Decide}`, `NewProvider`,
  `Thresholds`, `Route`. In-tree it is **live code with no production consumer**
  (`decision.NewProvider`/`decision.Route` are used only by tests), and
  `config.DecisionConfig.Provider` is validated (`deterministic | jev`) but not
  wired to a provider constructor in production.

Default behavior at the decision boundary is **deterministic and fail-closed**:
`Route` maps an unknown choice or invalid confidence to `HumanTarget`; the
deterministic provider is the only implemented provider and `NewProvider` hard-errors
on any other name (no silent fallback). `internal/decision` never executes actions.

## 5. Provider-neutrality finding

**`YES`.** No Clef-, Ollama-, SystemOne-, Nimble-, Laya-, Julia-, Jev-, or other
provider-specific semantics exist in the decision boundary or policy:

- `internal/decision` and `internal/autonomy` contain no provider/model/vendor
  identity and branch only on typed data and policy flags.
- The contract types are abstract (a use-case label, free text, numeric signals, a
  closed choice, a normalized confidence, opaque string metadata); none mirrors a
  provider wire protocol.
- The only Ollama wire concern (the `/api/chat` JSON-schema `format`) is isolated
  inside `internal/jev/ollama.go` behind the provider-neutral `jev.Analyzer`
  interface.
- The AS-CLEF-007 architecture guard (`internal/archtest/guard_test.go`) now
  enforces this: no core/policy package may import a provider implementation or the
  provider contract package. It passes on the valid tree and fires on a
  representative forbidden import.

## 6. Jev finding

Jev is **present, optional, and disabled by default; not on the `internal/decision`
path.** `internal/jev` provides a bounded analysis boundary (`Analyzer`,
`Evidence`) used only by the optional early-JEV checkpoint (gates are ANDed with
`early_jev.enabled`, default OFF). Its typed `jev.Evidence` feeds
`autonomy.DecideEarly` when enabled. `decision.NewProvider("jev")` returns a
not-implemented error — the "jev" string in `internal/decision` is an inert,
dead name, not a wire protocol or policy branch. Jev is not a default and carries no
lifecycle authority.

## 7. Execution/decision separation

**Independent.** `internal/model` (execution-model routing) and
`internal/decision` (decision-provider surface) have **no dependency edge in either
direction** (`go list -deps` evidence, AS-CLEF-009 §4). Changing or adding a
decision provider cannot change `SOP_MODEL_DEFAULT_CLASS`, the SMALL/MEDIUM/LARGE
execution models, execution fallback policy, or local/cloud policy. The
SMALL/MEDIUM/LARGE defaults in `internal/model.DefaultRoute()` are **unchanged**.

## 8. Governance finding

**Provider results cannot bypass SOP approval/policy boundaries.** The boundary is
fail-closed by construction and by test:

- Unknown/indeterminate `Choice`, malformed/empty results, invalid confidence
  (NaN / out of range), low confidence, and HIGH/HUMAN choices all route to a human
  — confidence alone never authorizes execution.
- Provider failure (unavailable, timeout, internal error) surfaces as an error and
  is never an implicit success (AS-CLEF-005 suite).
- `internal/decision` is pure: it never transitions lifecycle state, mutates
  persistence, or executes actions; only SOP policy produces a permitted action.

## 9. Verification

Full record: `docs/reports/clef-readiness/AS-CLEF-009-full-verification.md`.

| Gate | Result |
|---|---|
| `gofmt -l .` | PASS (no files) |
| `go vet ./...` | PASS |
| `go build ./...` | PASS |
| `go test -count=1 ./...` | PASS (0 FAIL) |
| `go test -race -count=1 ./...` | PASS (no data races) |
| `git diff --check` | PASS |
| `go test -count=1 -v ./internal/archtest/` | PASS (architecture guard + controls) |
| `go list -deps` execution/decision independence | PASS (no edge either way) |

## 10. Changes

Documentation only. No production source, test, configuration, or default is
changed in AS-CLEF-008/009/010.

- Updated: `docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md` (finalized:
  prior-task inputs AS-CLEF-005/007 marked complete, construction/transport
  neutrality noted, non-blocking limitation recorded).
- Created: `docs/reports/clef-readiness/AS-CLEF-009-full-verification.md` and
  `docs/reports/clef-readiness/AS-CLEF-009-verification.md` (identical content).
- Created: `docs/reports/clef-readiness/AS-CLEF-010-readiness-decision.md` (this
  report).

No Clef adapter is implemented in agentic-sop; `sop-decision-adapters` is not
modified; no SMALL/MEDIUM/LARGE default is changed; no decision provider is made
authoritative; human approval is not weakened.

## 11. Readiness decision

**`READY_WITH_NOTES`** (equivalent to the plan's positive non-blocking outcome).

The adapter boundary is **sufficient** and provider-neutral, so a Clef implementation
can proceed in `sop-decision-adapters` without adding Clef-specific policy or
lifecycle behavior to agentic-sop. Non-blocking limitations:

1. **No production consumer of a non-deterministic `decision.Provider`.**
   `internal/decision` is live but unwired in production; the live policy path is
   `internal/autonomy`. An external adapter is satisfiable by the AS-CLEF-008
   contract, but connecting it to the live policy path requires a **provider-neutral
   integration seam** owned by agentic-sop. This is not Clef-specific and does not
   change SOP policy, lifecycle, or approval semantics.
2. **Timeout/cancellation is an adapter-side obligation.** No in-tree
   `context` deadline is enforced at the decision boundary; the contract requires the
   adapter to honor cancellation and surface it as an error (§6 of the contract).
3. **Unsupported-capability signal is a diagnostic error string**, not a typed
   sentinel. Non-material (never parsed, no production caller) but noted for the
   adapter author.
4. Go `internal/` visibility means an external module cannot import
   `internal/decision`; the handoff is therefore the **semantic contract** plus a
   provider-neutral construction/transport seam, not a shared Go interface.

The explicit answer to the plan's question:

> Can Clef-Flash be added through the external decision-adapter boundary without
> introducing Clef-specific behavior into `agentic-sop`? **YES.**

## 12. Handoff boundary

- **agentic-sop** owns governance, policy, lifecycle, approval, execution
  authorization, task state, and the provider-neutral decision boundary.
- **sop-decision-adapters** owns Clef integration and transport (any wire protocol,
  endpoint, response translation, provider registration, and provider-specific
  tests).
- **Clef** produces **decision evidence** (a bounded choice + confidence +
  advisory diagnostics).
- **SOP** determines **what that evidence means** (policy converts it into a
  permitted action, a block, or a human approval; evidence never authorizes
  execution by itself).

## 13. Next action

**Begin the Clef provider work in `sop-decision-adapters`**, implementing the
adapter against the AS-CLEF-008 contract (bounded `Provider`-shaped
evaluate-and-return-evidence; valid `[0,1]` confidence; known choices; explicit
errors for unsupported/invalid/cancelled failures; advisory diagnostics only). Do
**not** add Clef-specific behavior to agentic-sop. If wiring the adapter into the
live policy path becomes the goal, open a separate, provider-neutral agentic-sop
task for the construction/consumption seam — it is not part of the Clef adapter and
is not Clef-specific.
