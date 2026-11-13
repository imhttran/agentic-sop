# Baseline drift detection — stale reviewed entries

**Status:** investigation plus one **policy-neutral** implementation. **The P2 recorded
against PR #472 is fixed:** the warning is now emitted only when a full-workspace scan
observed every adapter the baseline relies on, so changed-files, partial, failed, disabled
and no-op scans stay silent. The defect is preserved below as the reason for the gate. No
entry is added, removed, or rewritten, and no enforcement setting is touched.

**Context:** the D4 reconciliation found that **16 of 56 committed baseline entries match no
current finding**. They name the 13 production exec sites under fingerprints recorded on
2026-11-03 and 2026-11-08. Nothing warned about this; the entries looked live while
suppressing nothing.

## 1. Why entries go stale

Suppression identity is the exact fingerprint, computed as
`sha256(provider, rule, path, line, column)` (`internal/security/normalize.go:118`). A
fingerprint therefore changes whenever the flagged construct moves.

Consequences, in order of importance:

- **Drift is fail-closed.** When a fingerprint changes, the entry stops matching and the
  finding reappears **unsuppressed**, so it counts toward `FAIL`. A stale entry can never
  hide a finding.
- **The failure mode is a redundant entry, not a missed one.** The cost is an exception that
  no longer does anything plus a blocking finding that was previously reviewed. In this
  repository that is exactly what happened: 13 reviewed sites reappeared as blocking.
- **Silence is the defect.** `Baseline.Apply` reports invalid entries and (when they match)
  expired entries. It says nothing about an entry that matches nothing, so an inert review
  is indistinguishable from a live one until someone diffs the file against a scan.

## 2. Warning or failing validation?

**Recommendation: a warning, not a failing result.**

| Criterion | Warning (recommended) | Failing validation |
| --- | --- | --- |
| Security effect | None: drift is already fail-closed | None |
| Signals the operator | Yes, on every scan | Yes, louder |
| Punishes a correct fix | No | **Yes**: fixing the defect makes the entry stale and would fail the build |
| Blocks unrelated work | No | Yes, until someone edits the baseline |
| Matches existing semantics | Yes: diagnostics are already advisory | Introduces a new blocking condition |

The decisive argument is the second row. An entry becomes stale **when the finding is fixed**
as well as when it drifts, so failing validation would turn a successful remediation into a
build failure. A warning reports the same fact without taxing the fix.

A failing result is still a coherent operator choice; it would need a defined scope (for
example, required mode only) and a grace mechanism. That is a policy decision, not a
default, and is left open below.

## 3. The change

One method, one call site per scanner entry point:

- `internal/security/baseline.go` — `Baseline.UnmatchedDiagnostics(findings, now)` returns at
  most one aggregated diagnostic naming the reviewed, unexpired entries whose fingerprint
  matched nothing. Sorted, capped at 5 named entries plus "and N more". It returns `nil` when
  every active entry matched, so a clean baseline produces no output.
- `internal/cli/security.go` and `internal/cli/security_gate.go` — called **once**, after the
  scan, with the complete finding set.

Calling it once avoids one class of false positive because `Baseline.Apply` runs per
adapter. It does **not** prove that the finding set covers the full baseline: under
`--scope changed-files` unchanged files are absent by design, and failed or disabled adapters
leave their entries absent. The first implementation ignored that and could report valid
entries as drifted; a changed-files scan with no changes reported all 56 entries as
unmatched.

The fix is a gate at the call site: `security.BaselineDriftDiagnostics` returns the warning
only when the scope is `workspace` and every adapter the baseline holds entries for
completed with complete coverage and real content. The adapter set comes from the
fingerprint prefix, which `StableFingerprint` namespaces with the same identifier the scan
reports as `AdapterID`.

## 4. Why it is policy-neutral

- The verdict is computed from findings and coverage only. `EvaluatePolicyWithDetection`
  never reads `Evaluation.Diagnostics` (`internal/security/policy.go`), so a diagnostic
  cannot produce `PASS`, `FAIL`, or `INDETERMINATE`.
- The quality gate uses `scan.Verdict`; diagnostics only ever supply a fallback *reason*
  string (`internal/cli/security_gate.go`).
- Evidence reuse is keyed by `policyID`, which hashes baseline **content**
  (`internal/security/report.go:84`), not diagnostics, so the added line does not invalidate
  recorded evidence.

Confirmed on the real repository. Before and after the change, on the same tree and the same
pinned scanners:

| | Findings | Suppressed | Blocking | Verdict |
| --- | --- | --- | --- | --- |
| Before | 101 | 40 | 45 | `FAIL` |
| After | 101 | 40 | 45 | `FAIL` |

and the scan now emits exactly one added line:

```
baseline: 16 reviewed entries matched no finding at the recorded fingerprint
(drifted line/column or a corrected finding); review and remove:
internal/agent/command.go (dangerous-exec-command), ...; and 11 more
```

## 5. Test evidence

`go test ./internal/security/ -run TestBaseline -v`:

- `TestBaselineSuppressionAndExpiry` — unchanged and still passing (existing semantics).
- `TestBaselineUnmatchedDiagnostics` — a drifted entry is reported; a matched, an expired,
  and an invalid entry are not; a fully matched baseline returns `nil`; the result is
  deterministic; suppression state is not mutated.
- `TestBaselineUnmatchedDiagnosticsCapsTheList` — 9 unmatched entries produce one
  diagnostic with the full count and the remainder after the cap.

Suites run: `go test ./internal/security/... ./internal/quality/...` pass;
`go test ./internal/cli/` passes (65.7 s); `gofmt -l` clean; `go vet` clean.

## 6. Limits, stated plainly

- It **warns**; it does not remove the entry and does not force a review.
- The warning is emitted only for a complete workspace scan that covered the relevant
  adapters. Nothing is reported for changed-files, partial, failed, disabled or no-op scans,
  so a real drift in one adapter stays quiet while another relevant adapter is unavailable.
- An entry that is **both** expired and unmatched still produces no diagnostic: `Apply`
  reports expiry only when a finding matches. The expiry date remains the control there.
- It reports at most 5 names per scan; the count is always complete.
- It does not detect a *semantically* wrong entry whose fingerprint still matches, which no
  fingerprint mechanism can.

## 7. Operator decisions still required

- **B-1.** Accept the advisory warning as the disposition for stale entries (implemented in
  this change; no further action), **or** escalate to a failing validation.
- **B-2 (only if escalating).** Define the scope of the failure — for example required mode
  only — and whether a grace period or an explicit acknowledgement is allowed, so that a
  genuine fix does not break the build on the commit that fixes it.
- **B-3.** Approve removal of the 16 stale entries. This is **not** done here: it edits the
  committed baseline, which belongs with the D4 disposition package (D4-4) rather than with a
  detection change.
