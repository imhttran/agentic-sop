# Security Scanner Add-on — Report-Only Dogfood

**Date:** 2026-10-17
**Repository:** `agentic-sop`
**Mode:** `report-only`
**Scope:** full workspace
**Run ID:** `security-dogfood-20261017`
**Evidence:** `.agent-sdlc/runs/security-dogfood-20261017/security/scan.json`

## Result

| Field              | Value                                                              |
| ------------------ | ------------------------------------------------------------------ |
| Verdict            | `INDETERMINATE`                                                    |
| Coverage           | `UNKNOWN`                                                          |
| Commit             | `179258b09090bc41061b4800ee44d32e04402e82`                         |
| Tree               | `776dcf194be3398f4f75164d6721d5ffcc1686c6`                         |
| Working tree       | `DIRTY`                                                            |
| Content digest     | `38e4b2f7f9a6c2823bd1d27bbad2aa842b1ac0c62b0514edeb5f88bbe7cc0ead` |
| Measured duration  | 113 ms (92 ms and 122 ms on earlier runs of the same code path)    |
| Findings           | 0                                                                  |
| Execution failures | 2                                                                  |

The verdict is not `PASS`. The configured scanners did not run to completion, so
the dogfood run proves orchestration, identity, timing, and fail-closed policy
behavior — not repository security.

## Tool availability

| Adapter    | Category  | Status        | Coverage  | Evidence                                                               |
| ---------- | --------- | ------------- | --------- | ---------------------------------------------------------------------- |
| Semgrep CE | `sast`    | `FAILED`      | `UNKNOWN` | Semgrep rules were not configured; implicit rule download is disabled. |
| Gitleaks   | `secrets` | `UNAVAILABLE` | `UNKNOWN` | `gitleaks` is not installed on this machine.                           |

`semgrep` and `gitleaks` are both absent from the current environment. No
automatic installation or rule download was attempted.

## What ran

Command:

```bash
SOP_SECURITY_ENABLED=true \
SOP_SECURITY_MODE=report-only \
go run ./cmd/sop security scan --json --run-id security-dogfood-20261017
```

The command wrote a normalized JSON report, a Markdown report, and no native
reports (none were produced by a completed adapter). The dirty content digest
covers the working tree at the time of the scan, including the add-on changes
under review.

## Findings and baseline

No findings were produced because no adapter completed. This is not a
"clean repository" baseline and MUST NOT be used as one. The first meaningful
finding baseline requires the pinned Semgrep CE CLI with local rules and the
Gitleaks CLI.

## Limitations observed

- Required mode was not exercised on live scanners during this dogfood run.
  Deterministic conformance and gate-matrix tests cover the required behavior.
- Changed-file scope was not exercised against a live scanner. Its
  optimization-only status is enforced by policy tests.
- Semgrep rules and Gitleaks availability must be provisioned explicitly in the
  integration environment.
- A passing scan remains evidence for the configured tools and scope, not proof
  that the repository has no vulnerabilities.

## Next action

In an environment with the pinned CLIs:

1. Install or expose `semgrep` and `gitleaks` explicitly.
2. Provide local Semgrep rules and set `security.adapters[].rules`.
3. Set the pinned versions in `security.adapters[].version`.
4. Rerun the same scan in `report-only` mode.
5. Review findings and establish the first reviewed baseline with owner and
   expiry.

## Related

- [Normative contract](../specs/SECURITY-SCANNER-ADDON.md)
- [Setup and extension guide](../guides/SECURITY-SCANNER-ADDON.md)
- [Executable plan](../plans/PLAN-Security-Scanner-Addon.md)
