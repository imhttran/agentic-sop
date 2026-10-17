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
| Verdict                 | `INDETERMINATE`                                                    |
| Coverage                | `PARTIAL`                                                          |
| Commit                  | `db39cf5e2a05a046cf21b6ffff6c2b247fd19283`                         |
| Tree                    | `264f1cadd0d5744c189c7844ca95b4996e43fc8c`                         |
| Working tree            | `DIRTY`                                                            |
| Content digest          | `35c07fd6dbb840f70fbdd6dc3db16b88d7bd9472d6660d867379b1253aca1e32` |
| Measured total duration | 7615 ms                                                            |
| Semgrep duration        | 3025 ms                                                            |
| Gitleaks duration       | 4497 ms                                                            |
| Findings                | 7 (1 SAST, 6 secret)                                               |
| Execution failures      | 0                                                                  |

The verdict is `INDETERMINATE`, not `PASS`, because Semgrep coverage was
`PARTIAL`. Report-only recorded the result without blocking delivery.

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
  161.23 MB in 4.50 s.

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
  larger than 1.0 MB. A required gate correctly reports `INDETERMINATE` until a
  full-coverage scan policy is configured.
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
