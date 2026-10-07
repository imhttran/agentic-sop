# SEAM-008 — Integration-Seam Readiness Decision

## Headline

**READY_WITH_NOTES.**

A real external decision provider can now be implemented in
`sop-decision-adapters` and supply provider-neutral evidence to the live
`agentic-sop` policy path **without any provider-specific policy, governance,
lifecycle, authorization, or approval change in `agentic-sop`**.

All `READY` conditions are materially satisfied. Six non-blocking `LOW/NOTE`
items (L1–L6) remain — operational considerations and documentation staleness —
none of which blocks external provider integration or threatens governance.

Scope: `agentic-sop` only. Authority:
`docs/plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md` §SEAM-008.
This report makes no production change.

---

## Repository state

| Fact | Value |
|---|---|
| Branch | `main` |
| HEAD | `b743cfa93c057eb1499ce72332d6481edb4643a6` |
| Upstream | `origin/main` (in sync) |
| Commits made by the seam | **none** (entire seam is uncommitted working-tree state) |
| Changed since SEAM-007 | **nothing** (same HEAD; SEAM-007's verification is current) |
| SOP task store | `.agent-sdlc/state.db` holds only the committed `AS-CLEF-*` tasks; the SEAM plan was executed outside SOP's task store |

Cumulative seam diff (tracked files only): `internal/cli/cli.go`,
`internal/cli/run.go`, `internal/config/config.go`, `internal/decision/decision.go`
(4 files, +186/−14). Untracked seam additions: the plan, the
`docs/reports/decision-integration/` reports, the decision seam
(`decision_evidence.go`, `decision_provider.go`), the process adapter
(`internal/decision/command/`), the guards (`internal/archtest/decision_seam_test.go`),
the tests, and the out-of-module fixture (`testdata/fake-decision-provider/`).

---

## Evidence basis

- `SEAM-001-current-boundary.md` — traced the live policy path and the cross-module
  visibility constraint.
- `SEAM-002-decision-contract.md` — the provider-neutral JSON request/result contract.
- `SEAM-003 … SEAM-006` production + tests (validated by SEAM-005/007).
- `SEAM-007-verification.md` — the full verification and architecture review
  (all gates pass; `CRITICAL 0 / HIGH 0 / MEDIUM 0`).
- Re-verified at HEAD `b743cfa` by SEAM-008 (this report): repository state unchanged
  since SEAM-007, and all six gates re-run and passing.

---

## SEAM-001 … SEAM-008 task states (one-line evidence)

| Task | State | One-line evidence |
|---|---|---|
| SEAM-001 | DONE | Boundary traced from source → `SEAM-001-current-boundary.md`. |
| SEAM-002 | DONE | Contract fixed → `SEAM-002-decision-contract.md`. |
| SEAM-003 | DONE | Contract + validator + process adapter implemented; tests pass. |
| SEAM-004 | DONE | Optional, disabled-by-default seam wired into `executeLifecycle`; adds attention only. |
| SEAM-005 | DONE | Failure/fallback/governance hardening; bounded output + drain bound; 31 tests pass. |
| SEAM-006 | DONE | Out-of-module fixture crosses the real process boundary; imports no SOP type. |
| SEAM-007 | DONE | Full verification + architecture review → `SEAM-007-verification.md` (GO_WITH_NOTES). |
| SEAM-008 | DONE | This readiness decision: READY_WITH_NOTES. |

(The plan was operated stage-by-stage outside SOP's task store, so these states are
the stage-completion record evidenced by the deliverables and the passing gates, not
`.agent-sdlc` rows.)

### Verified seam architecture

```
External Decision Provider  (sop-decision-adapters; any module/process)
        │  provider-neutral JSON  (stdin)
        ▼
Process / Provider Boundary   internal/decision/command.Adapter.Decide (command.go:108)
        │  provider-neutral JSON  (stdout)
        ▼
SOP-Owned Validation          decision.Validate (decision.go:229) / ValidateResult (:255)
        ▼
SOP-Owned Translation         decisionEvidenceAdverse / decisionEvidenceClassification
        ▼
failure.Classification        {Kind: BLOCKING_FINDINGS, Disposition: NEEDS_HUMAN}
        ▼
autonomy.Decide               (autonomy.go:162) — the single policy path
        ├── Continue
        ├── Block
        └── Human Boundary
        ▼
SOP-Owned Governance/Lifecycle  approval.go:400 → drive.go:770 / run.go:219
```

**Providers evaluate. SOP governs.** Provider output is **data/evidence**, never
authorization. Option A — validated evidence → `failure.Classification` →
`autonomy.Decide` — is the implemented and only interpretation path.

### Contract / protocol

Provider-neutral JSON DTOs (SEAM-002 §4–§5): request
`{contract_version, kind, question, signals, choices, deadline_ms}`; result
`{contract_version, status(OK|UNSUPPORTED|ERROR|INDETERMINATE), kind, choice, confidence, diagnostics, error}`.
No lifecycle, approval, commit, merge, or policy action is expressible.

### Failure semantics

No provider / disabled → strict no-op; unavailable / missing executable / startup
failure / non-zero exit / crash / timeout / cancellation → typed failure, baseline
preserved; malformed / unknown choice / invalid confidence / indeterminate →
validation failure, baseline preserved; oversized/empty output → provider failure.
A **configured** provider failure is recorded distinctly (secret-free) while the
governed baseline stands. No failure mode becomes success, approval, or authorization.

---

## Governance ownership and assertions

`agentic-sop` owns: the provider-neutral boundary; provider-neutral validation;
evidence interpretation; governance; policy; lifecycle; authorization; human approval.

A future external provider **cannot**:

| Capability | Answer |
|---|---|
| transition task state | **NO** |
| transition plan state | **NO** |
| approve work | **NO** |
| satisfy approvals | **NO** |
| authorize execution | **NO** |
| authorize commit | **NO** |
| authorize merge | **NO** |
| bypass validation | **NO** |
| bypass review | **NO** |
| bypass human approval | **NO** |
| directly select lifecycle transitions | **NO** |

A provider result carries no such surface (it is a bounded DTO validated by SOP and
interpreted by `autonomy.Decide`).

### Monotonic authority

`attentionable` (decision_evidence.go:107) permits escalation only from `AUTO_*`
baselines, and the escalation target is always `autonomy.Decide`'s human boundary.
Verified by SEAM-007/005 tests: Block + evidence ≠ Continue; Human Boundary +
evidence ≠ Continue; failure/rejection + evidence ≠ authorization; confidence=1.0 ≠
authority.

### Human approval

SOP-owned and authoritative; unchanged. Provider evidence cannot create approval
evidence, mark one satisfied, remove `NeedsHuman`, bypass a pending approval, turn a
Human Boundary into Continue, reinterpret confidence as approval, or use diagnostics
as approval.

### Provider neutrality

No production branch on `clef|nimble|ollama|omlx|systemone|laya|julia`, provider name,
model name, or executable name. Identical validated evidence from different provider
identities (and different metadata) yields identical SOP interpretation.

### Cross-module and `internal/decision`

The SEAM-006 fixture is a genuinely external Go module (stdlib only, no SOP import,
no lifecycle/approval authority). **`internal/decision` remains internal** — external
adapters satisfy the JSON protocol without any SOP Go type. SMALL/MEDIUM/LARGE
defaults and execution-model routing are unchanged; decision evidence selects no
model class.

---

## External-adapter readiness

An implementation in `sop-decision-adapters` **can**:

- connect through the approved boundary without importing SOP internals — yes;
- provide decision evidence without receiving lifecycle or approval authority — yes;
- own provider transport, endpoint handling, request/response translation, capability
  discovery, provider-specific pre-return validation, and provider-specific tests — yes.

`agentic-sop` **remains** responsible for provider-neutral validation, policy
interpretation, governance, lifecycle, authorization, and human approval.

**The cross-repository handoff is ready.**

## Clef-adapter readiness

The architecture permits the next project to implement Clef in
`sop-decision-adapters` **without Clef-specific policy changes in `agentic-sop`**.
The intended flow is Clef/oMLX → `sop-decision-adapters` → provider-neutral evidence →
the `agentic-sop` decision seam → SOP validation + autonomy policy. Clef remains an
**evidence provider**, never a policy authority. Only generic adapter/configuration/
transport work is required on the adapter side, which supports a READY/READY_WITH_NOTES
result.

## Execution-model separation

The decision-provider seam is independent of SMALL/MEDIUM/LARGE. No readiness
conclusion requires changing execution-model defaults or routing. A future Clef
decision provider must **not** become a SMALL/MEDIUM/LARGE execution model; the two
dimensions stay separate.

---

## L1–L6 disposition

| ID | Description | Severity | Blocking | Why | Recommended follow-up | Owner/scope |
|---|---|---|---|---|---|---|
| L1 | `decisionEvidenceTimeout` is a fixed 60 s constant (not configurable) | LOW | **NO** | an operational bound, not a governance limit; it only ever fails closed | optionally expose a configurable deadline | `agentic-sop` (future, config) |
| L2 | the provider process inherits the parent environment | LOW | **NO** | the executable is trusted operator configuration (matches the repo's other command adapters); no provider-supplied value is used | document; optionally restrict env for providers treated as less trusted | `agentic-sop` (future) |
| L3 | a provider that daemonizes grandchildren is not reaped | LOW | **NO** | the direct child is killed and reaped and the caller is not stalled (`WaitDelay` bound); only an abnormal provider is affected | optionally add process-group termination (platform-specific) | `agentic-sop` (future) |
| L4 | the seam's insertion point is guarded behaviourally, not structurally | LOW | **NO** | the behaviour is proven by tests; a future refactor could move it, not bypass it | add a structural guard if desired | `agentic-sop` (future, test) |
| L5 | `attentionable` is keyed to the current autonomy action vocabulary | LOW | **NO** | an unknown/new action defaults to *no escalation* (safe direction) | review when the autonomy action set changes | `agentic-sop` (future) |
| L6 | documentation staleness (§ below) | LOW (documentation) | **NO** | affects prose only; it does **not** misrepresent the security or governance model (the seam's guarantees are stated in SEAM-005/007 and in code comments, all accurate) | reconcile the analysis when authorized | docs |

### L6 detail — documentation staleness

Two design-stage statements in `SEAM-architecture-analysis.md` are stale relative to
the implemented, plan-sanctioned behaviour:

1. §6 says a live provider failure "fail closes (human)"; the implementation (and
   SEAM-005 acceptance) **preserves governed behavior** on provider failure. This is a
   design→plan evolution, not a defect — the implemented behaviour is authority-safe
   (it never converts a failure into approval or authorization).
2. §6/§7 describe a `disabled → shadow → selectable → live` ladder; the implementation
   provides OFF and enabled (=live). No runtime `shadow` mode was required by the plan.

Neither affects the security/governance model, so L6 remains a non-blocking
documentation cleanup. Consumers should rely on SEAM-005/SEAM-007 semantics: **provider
absence and provider failure preserve governed behavior; never authorization.**

---

## Residual risk

| Category | Risk | Blocking |
|---|---|---|
| Architecture | none identified (single policy path, monotonic escalation, fail-closed) | NON-BLOCKING |
| Governance | none identified (SOP retains all authority; approval authoritative) | NON-BLOCKING |
| Process/transport | L2 (env inheritance), L3 (grandchild not reaped) | NON-BLOCKING |
| Operational | L1 (fixed timeout) | NON-BLOCKING |
| Documentation | L6 (stale analysis §6/§7) | NON-BLOCKING |
| Future-provider | a provider could return poor evidence; SOP still governs, and evidence can only add attention | NON-BLOCKING |

No blocking risk is demonstrated.

---

## Verification

Re-run at HEAD `b743cfa` for this decision (repository state unchanged since SEAM-007):

| Command | Outcome |
|---|---|
| `gofmt -l .` | clean (exit 0) |
| `go vet ./...` | exit 0 |
| `go test -count=1 ./...` | exit 0 — all packages pass |
| `go test -race -count=1 ./...` | exit 0 — no data races |
| `go build ./...` | exit 0 |
| `git diff --check` | exit 0 |
| `internal/archtest` guard suite | all pass (incl. decision-seam + out-of-module guards) |

Nothing changed after SEAM-007; its conclusions remain supported by current HEAD.

---

## Handoff boundary

If the result is READY or READY_WITH_NOTES — and it is — the ownership boundary is:

**`agentic-sop` owns:** provider-neutral boundary · provider-neutral validation ·
evidence interpretation · governance · policy · lifecycle · authorization · human
approval.

**`sop-decision-adapters` owns:** provider implementations · Clef implementation ·
oMLX/provider transport · endpoint handling · provider request/response translation ·
provider capability handling · provider-specific tests.

Providers produce evidence. SOP determines what that evidence means.

---

## Exactly one next action

**Create a separate plan to implement the Clef provider in `sop-decision-adapters`.**

Do not start it in this repository. Do not make Clef a default. Do not change
SMALL/MEDIUM/LARGE defaults. Do not wire Clef directly into SOP policy. Suggested
progression: adapter contract mapping → Clef/oMLX transport → response translation →
deterministic/provider tests → shadow evaluation → benchmark → readiness review →
only then consider selectable/live use.

---

## Readiness decision

**READY_WITH_NOTES.**
