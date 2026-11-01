# Decision compatibility — local advisory integration and fallback (MIG-006)

Status: implemented and fixture-verified; live provider smoke **BLOCKED** (not run).

## Bounded advisory chain

The unified decision layer keeps the reconciled CLEF / oMLX and Bespoke-Nimble
implementations as normalized providers behind the single `decision.Provider`
contract, and composes a bounded failover with no new framework:

1. **Primary** — the configured provider (default `nimble`; `clef` is opt-in via
   `CLEF_ENABLED`). Selected by `sop decision decide -provider <name>`.
2. **Backup** — when `SOP_DECISION_FALLBACK` names a different provider,
   `internal/decisionadapter/providers/fallback.Provider` serves the primary first
   and the backup only when the primary errors.
3. **Deterministic SOP fallback** — SOP's own deterministic decision provider is
   the terminal authority; the advisory provider can only add attention.

The composite reports the **primary** provider's name so the provider-neutral
decision contract's profile-correlation check still matches; the serving backend
is carried only as advisory diagnostics (`served_provider`, `fallback`), never as
policy.

## Advisory-only invariants (preserved)

- Advisory results **cannot** lower risk, approve work, bypass verification, or
  alter execution-agent routing / JEV / model-picker behavior. The core decision
  layer treats a provider decision as evidence that may *raise* attention; SOP owns
  every outcome.
- A cancelled or deadline-expired primary does **not** spend the backup
  (`fallback.Provider.Decide` returns the primary error when `ctx` is done), so all
  deadlines fit a bounded overall consultation budget.
- Julia, existing SystemOne compatibility and shadow evaluation are preserved
  unchanged.

## Fixture coverage

- `internal/decisionadapter/providers/fallback` (existing + added bounds):
  primary serves; backup on primary failure; both unavailable; available-either;
  backup records diagnostics; **malformed primary falls back**; **invalid-confidence
  fallback is advisory-only (primary name preserved)**; **cancelled primary does not
  attempt the backup**; **deadline primary does not attempt the backup**; **both
  unavailable is an error, not a zero-value result**.
- `internal/decisionadapter/cli`: clef selection is OFF unless `CLEF_ENABLED`; a
  disabled clef is never accidentally selected; selection fail-closed paths.
- `internal/decisionadapter/providers/{clef,nimble,julia}`: transport, translation,
  golden payload and governance tests (imported byte-exact).

## Live checks

- **oMLX chat-completions and Bespoke `/score` local contracts: BLOCKED** — not
  probed. A bounded local live smoke requires the operator's running oMLX and
  Bespoke services; per the migration's no-service-manipulation rule they were
  neither stopped nor restarted, and the live contracts remain unverified here.
  Report as **BLOCKED**, never inferred from the committed `/v1/systemone`
  transport.
- **CLEF→Nimble failover: PASS** in fixtures (advisory-only); **live failover:
  BLOCKED** (not demonstrated against live infrastructure).

## Precedence note

`CLEF_ENABLED` remains the explicit opt-in; selecting `clef` without it yields an
unavailable provider rather than serving decisions. No global model default or
advisory authority is changed.
