# Security Scanner Add-on — Report-Only Dogfood

**Date:** 2026-10-17
**Repository:** `agentic-sop`
**Mode:** `report-only`
**Scope:** full workspace
**Run ID:** `security-dogfood-real-20261017`
**Evidence:** `.agent-sdlc/runs/security-dogfood-real-20261017/security/scan.json`

## Result

| Field                   | Value                                                              |
| ----------------------- | ------------------------------------------------------------------ |
| Verdict                 | `FAIL`                                                             |
| Coverage                | `PARTIAL`                                                          |
| Commit                  | `469b61d5610988c23f569c2a3ccf322eac7af3a9`                         |
| Tree                    | `e1723411827339f6eca371c1a8670629f6ea955b`                         |
| Working tree            | `DIRTY`                                                            |
| Content digest          | `ed757ed8350f7ccde7f207089c1fbbd7113c171df5ba029da98847a725cf822b` |
| Measured total duration | 8581 ms                                                            |
| Semgrep duration        | 3833 ms                                                            |
| Gitleaks duration       | 4642 ms                                                            |
| Findings                | 7 (1 SAST, 6 secret)                                               |
| Execution failures      | 0                                                                  |

The verdict is `FAIL` because seven unsuppressed findings were at blocking
severities. Coverage is `PARTIAL` because Semgrep skipped targets. Report-only
recorded the result without blocking delivery.

## Tool and rules identity

| Adapter    | Version   | Source                                                                       | Rules identity                                                     |
| ---------- | --------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| Semgrep CE | `1.136.0` | isolated Python venv provisioned by `scripts/checks/security-integration.sh` | `9291c2b41e91f48066eab9fc8c1c69d479904130ce13015ffa79e046fcf33a71` |
| Gitleaks   | `8.30.1`  | pinned GitHub release binary                                                 | `6c74b5c274758f58948c18d8c3491c44579641b70e62c456f01e1d5b11424feb` |

Semgrep used explicit local rules (`internal/security/testdata/real/rules.yaml`).
Gitleaks used an explicit local config (`internal/security/testdata/real/gitleaks.toml`).
Neither scanner downloaded rules or uploaded code. No hosted service was used.

## Coverage

- **Semgrep:** `PARTIAL`. It scanned 658 files and reported 933 files skipped by
  `.semgrepignore` patterns plus 6 files larger than 1.0 MB. The adapter records this as partial coverage and fails closed
  for a required gate.
- **Gitleaks:** `COMPLETE` for the configured workspace source. It scanned about
  161.37 MB in 4.44 s.

## Findings

All findings are synthetic fixtures committed for integration testing. No live
credential or real vulnerability is claimed.

| Category | Rule ID                                                             | Path                                                   | Notes                            |
| -------- | ------------------------------------------------------------------- | ------------------------------------------------------ | -------------------------------- |
| SAST     | `internal.security.testdata.real.sop-integration-command-execution` | `internal/security/testdata/real/seeded/insecure.go:7` | Synthetic `exec.Command` fixture |
| Secret   | `sop-integration-synthetic-token`                                   | `internal/security/testdata/fixtures/secret.txt:1`     | Synthetic fixture                |
| Secret   | `sop-integration-synthetic-token`                                   | `internal/security/testdata/real/seeded/secret.txt:3`  | Synthetic fixture                |
| Secret   | `sop-integration-synthetic-token`                                   | `internal/security/testdata/gitleaks-report.json:9`    | Report fixture (`Match`)         |
| Secret   | `sop-integration-synthetic-token`                                   | `internal/security/testdata/gitleaks-report.json:10`   | Report fixture (`Secret`)        |
| Secret   | `sop-integration-synthetic-token`                                   | `internal/security/integration_test.go:173`            | Test literal                     |
| Secret   | `sop-integration-synthetic-token`                                   | `internal/security/security_test.go:155`               | Test literal                     |

The synthetic secret value is absent from `scan.json`, `scan.md`, normalized
findings, diagnostics, and the sanitized native reports. The integration checks
assert this over every persisted artifact.

## What ran

```bash
bash scripts/checks/security-integration.sh
go run ./cmd/sop security scan \
  --config /tmp/security-dogfood-config.yaml \
  --json --run-id security-dogfood-real-20261017
```

The scan configuration enabled report-only mode with both pinned adapters, the
explicit local Semgrep rules, and the explicit local Gitleaks config.

## Baseline

No baseline was created. The seven findings are test fixtures, so hiding them
with a production baseline would be misleading. If the repository later keeps
these fixtures, add reviewed baseline entries with an owner and expiry, or move
the fixtures outside the scanned scope.

## Limitations

- Semgrep CE skipped files under its `.semgrepignore` policy and skipped files
  larger than 1.0 MB. A required gate reports `FAIL` when blocking findings exist
  and `INDETERMINATE` when no blocking finding exists but coverage is incomplete.
- The findings demonstrate detection, normalization, coverage, and redaction.
  They are not a vulnerability assessment of the repository.
- The pinned CLIs were provisioned in an isolated tools directory on this machine.
  CI runs the same script with the same pins.
- A passing scan remains evidence for the configured tools and scope, not proof
  that the repository has no vulnerabilities.

## Related

- [Normative contract](../specs/SECURITY-SCANNER-ADDON.md)
- [Setup and extension guide](../guides/SECURITY-SCANNER-ADDON.md)
- [Executable plan](../plans/PLAN-Security-Scanner-Addon.md)
- [Integration script](../../scripts/checks/security-integration.sh)
