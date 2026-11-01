# Lifecycle Verdict Gate (SOP-HARD-01)

The lifecycle verdict gate ties the local lifecycle's terminal `LOCAL_DONE`
transition to the objective-level **outcome verdict** (`VERIFIED` / `PARTIAL` /
`HOLD`) computed by [`internal/outcome`](../../internal/outcome/outcome.go) and
surfaced by the run's outcome artifact (`outcome.json`, the CLI summary, and the
run report).

It is **operator-enabled and OFF by default**. The config key is:

```yaml
lifecycle:
  verdict_gate: false # operator-enabled; OFF by default
```

`lifecycle.verdict_gate` is the single source of truth
(`config.Config.VerdictGateEnabled()`). Nothing infers it, and no other
configuration value enables it. When the block is omitted, the field's zero
value keeps the gate OFF, so the lifecycle is byte-identical to before.

## Flag OFF (default): the lifecycle is unchanged

With the gate OFF, the verdict is **reported only** and never changes the
transition: `ResolveLifecycle` returns `LOCAL_DONE` for every verdict the
lifecycle reaches (VERIFIED, PARTIAL, and HOLD alike). No waiver path is
consulted and the artifact's lifecycle/waiver fields are not added beyond the
`omitempty` contract.

## Flag ON: the pinned lifecycle table

With the gate ON, the mapping is:

| Outcome verdict                                              | Local lifecycle |
| ------------------------------------------------------------ | --------------- |
| `VERIFIED`                                                   | `LOCAL_DONE`    |
| `PARTIAL`, unmet REQUIRED deliverable/check, **no waiver**   | `NEEDS_HUMAN`   |
| `PARTIAL`, unmet REQUIRED deliverable/check, **valid waiver** | `LOCAL_DONE`   |
| `PARTIAL`, informational-only (no unmet required criterion) | `LOCAL_DONE`    |
| `HOLD`                                                       | `NEEDS_HUMAN`   |

A `PARTIAL` from an unmet **required** deliverable or required executable check
does **not** yield `LOCAL_DONE` unless a valid waiver exists. An informational-only
`PARTIAL` (unmet non-required criteria, e.g. free-text acceptance criteria that are
recorded but unverifiable) does not gate and yields `LOCAL_DONE`.

The denial is observable: when the gate is on and the result is not `LOCAL_DONE`,
the outcome artifact records `waiver_denied: true` and the summary/report render an
explicit denial line — a missing or invalid waiver is never a silent downgrade.

## Waivability

A `PARTIAL` from an unmet required deliverable/check may be waived by a
**designated approver**. A `HOLD` is **never** waivable: an authority boundary
cannot be waived into `LOCAL_DONE`, and an informational-only `PARTIAL` has nothing
to waive.

A waiver must satisfy every requirement enforced by
[`outcome.ValidateWaiver`](../../internal/outcome/outcome.go):

- the result is a `PARTIAL` with at least one unmet required criterion;
- a non-empty **approver** identifies the designated approver (attribution);
- a **reason** records the justification;
- a **timestamp** (`RecordedAt`) records the grant;
- the waiver names, **criterion-scoped**, every unmet required criterion of the
  result.

Validation fails closed: a waiver that is missing the approver, reason, timestamp,
or any unmet required criterion does not restore `LOCAL_DONE`, so a partial waiver
can never silently waive the rest.

## Waiver supply path

The repository provides no designated-approver identity source. The only supply
path today is an explicit **operator input**; absent that, there is no valid waiver
and the gate fails closed to `NEEDS_HUMAN` (with the denial recorded).
`waiverForTask` returns `nil` until an operator-supplied waiver is provided, and
`outcome.ErrNoDesignatedApprover` documents that the unsupported path never
silently accepts a waiver. Identity and authorization policy remain an operator
responsibility; this gate never invents an identity provider.

## Scope

The gate is a verifier only: it starts no work, mutates no repository files, and
transitions no SOP state. It records the resolved local lifecycle disposition on
the diagnostic outcome artifact and surfaces it in reporting.

## Related

- [`internal/outcome`](../../internal/outcome/outcome.go) — `ResolveLifecycle`,
  `ValidateWaiver`, `HasUnmetRequired`, and the `Waiver` type.
- [`internal/config`](../../internal/config/config.go) — the `lifecycle.verdict_gate`
  flag and `VerdictGateEnabled()`.
- [`internal/cli`](../../internal/cli/outcome.go) — the `applyLifecycleGate` wiring.
- [`CONFIGURATION.md`](CONFIGURATION.md)

## Supplemental verification (operator, post-HARD-01)

HARD-01's recorded run outcome is **HOLD**, caused by a plan-authoring defect: a
scoping-note blockquote inside the task's `### Acceptance Checks` was parsed as four
extra (invalid) acceptance-check commands. The plan text was corrected
(`sop reconcile --accept-changed SOP-HARD-01`); the historical HOLD record is preserved
and **not** rewritten.

Independent re-verification of the shipped behavior (unchanged by the plan correction):

- `go test ./internal/outcome/... ./internal/config/...` — PASS
- `go test ./internal/cli/... -run ApplyLifecycleGate` — PASS (6 tests: flag-off leaves
  the lifecycle unchanged; flag-on denies a missing required deliverable; VERIFIED and
  informational-only PARTIAL stay `LOCAL_DONE`; HOLD stays `NEEDS_HUMAN`)
- `go build ./...`, `go vet ./...` — PASS

One code/test inconsistency left by the HARD-01 run was corrected here: with the gate
**OFF**, `applyLifecycleGate` recorded nothing instead of `LOCAL_DONE`; it now records
`LOCAL_DONE` (the unchanged lifecycle), matching the task's acceptance tests.
