# SEC-H-004 — Dependency coverage readiness

Scope: verify that the osv-scanner adapter fails closed when the offline OSV
advisory database is absent, that the `stale_advisory_database: deny` policy is
covered by existing tests, and record the operator-authorized provisioning
command. No advisory data was downloaded in this work item.

## Verification record — fail-closed behavior

Package: `internal/security` (adapter implementation in `internal/security/osv.go`).

Cited tests covering fail-closed behavior when the offline database is absent
or incomplete. All use an in-memory fake executor; none needs a real database
and none performs a download:

| Test | File | Covered behavior |
| --- | --- | --- |
| `TestOSVMissingDatabasePathIsNotClean` | `internal/security/osv_test.go` | Empty `databasePath` => `StatusFailed` + `CoverageUnknown`. `resolveDatabases` returns "OSV database path is not configured; implicit download and the live OSV API are disabled". |
| `TestOSVMissingEcosystemDatabaseIsNotClean` | `internal/security/osv_test.go` | Present Go database, absent `crates.io` database => not COMPLETED/COMPLETE; diagnostic names the missing ecosystem. `resolveDatabases` returns "OSV database for ecosystem %s is missing". |
| `TestOSVExitAndOutputFailuresAreNotClean` | `internal/security/osv_test.go` | Malformed JSON, empty output, non-zero exit codes, exit-code/vulnerability mismatch, and output truncation never produce clean coverage. |
| `TestOSVVersionMismatchIsNotClean` | `internal/security/osv_test.go` | Pinned-version mismatch (`verifyToolVersion`) => `StatusFailed`. |
| `TestOSVUsesControlledConfigAndExplicitOfflineFlags` | `internal/security/osv_test.go` | Scan runs `--offline`, `--no-call-analysis=all`, a controlled empty `--config`, and an explicit `OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY`; repository `osv-scanner.toml` is not passed. |

Call path for the absent-database fail-closed path:

1. `osvAdapter.Scan` (`internal/security/osv.go`) calls `resolveInputPlan`.
2. `Scan` then calls `osvAdapter.resolveDatabases`.
3. `resolveDatabases` returns an error when `databasePath` is empty
   ("implicit download and the live OSV API are disabled"), when the cache root
   does not exist, has no `osv-scalibr` / `osv-scanner` layout, or when the
   per-ecosystem `all.zip` is missing.
4. `Scan` maps that error to `StatusFailed` / `CoverageUnknown` and records the
   sanitized diagnostic. It never downloads and never invokes the live OSV API.

## Verification record — `stale_advisory_database: deny`

Policy implementation: `internal/security/policy.go` (`Policy.StalePolicy`,
`EvaluatePolicyWithDetection`). Adapter path: `internal/security/osv.go`
(`osvAdapter.stalePolicyValue`, `osvDatabaseStale`).

Cited tests:

| Test | File | Covered behavior |
| --- | --- | --- |
| `TestStaleDatabasePolicy` | `internal/security/policy_test.go` | Default (deny, zero value) stale evidence yields `INDETERMINATE`, never `PASS`. Explicit `allow` yields `CoveragePartial` with a stale diagnostic and cannot PASS. |
| `TestOSVStaleDatabaseFollowsPolicy` | `internal/security/osv_test.go` | Adapter records `AdvisoryDatabaseStale`; `stalePolicy = allow` yields `CoveragePartial`; `stalePolicy = deny` keeps the stale marking and does not yield clean coverage. |
| `TestPolicyIdentityIncludesAdvisoryDatabase` | `internal/security/policy_test.go` | `stale_advisory_database` change alters policy identity, so stale policy cannot silently reuse prior evidence. |
| `TestOSVResumeRechecksFreshnessAndSelection` | `internal/security/osv_test.go` | Aging the database (unchanged digest) refuses resume reuse, proving freshness is rechecked. |
| `TestRequiredModeEvaluatesDetectedLanguagesSeparately`, `TestOneSuccessfulAdapterCannotHideAFailedRequiredObligation`, `TestNoApplicableContentCannotSatisfyRequiredCoverage` | `internal/security/policy_test.go` | Required mode never passes with missing or unsupported dependency coverage. |

The deny path is fail-closed: an empty or unrecognized value resolves to
`StaleDeny`; a stale database marks `InProvenance.AdvisoryDatabaseStale`, adds an
incomplete reason `(policy: deny)`, and forces `INDETERMINATE` instead of `PASS`.

## OSV identity/freshness test run

Command run (no database download):

```
go test ./internal/security -run 'TestOSV|TestStaleDatabasePolicy|TestPolicyIdentityIncludesAdvisoryDatabase|TestRequiredMode|TestOneSuccessfulAdapter|TestNoApplicableContent|TestCargoAudit|TestGlobalDisable|TestDisabledRequiredCategory' -v
```

Result: PASS. All selected tests use in-memory fake executors or temp-dir
fixture databases created by `writeOSVDatabase` (synthetic `TEST-0001` zips).
No test in this subset uses the live OSV API or a real advisory database.

Skipped tests in this work item:

- `TestOSVIntegration`-style live checks in `internal/security/osv_integration_test.go`
  (if present) — need a real offline database and/or the `osv-scanner` binary.
  Reason: no operator-provisioned database exists on this host; provisioning is
  not authorized in this work item.

Broader validation on this host: `go build ./...`, `go vet ./...`, and
`go test ./...` all exit 0.

## Provisioning record

The supported provisioning command is:

```
scripts/security/fetch-osv-databases.sh --dest .agent-sdlc/osv-db
```

- The script downloads external advisory data (network access).
- Therefore it requires explicit operator authorization.
- It was NOT run in this work item. No download occurred.

## Required-readiness projection

With no offline database present, in required mode:

1. `sop security scan --required` invokes the osv-scanner adapter.
2. `resolveDatabases` fails because `databasePath` is empty or the cache
   directory / per-ecosystem `all.zip` is missing.
3. The adapter returns `StatusFailed` / `CoverageUnknown` with a clear diagnostic.
4. `EvaluatePolicyWithDetection` records `required category dependencies has no
   COMPLETED adapter with COMPLETE coverage` and returns `INDETERMINATE`.
   The gate never passes on missing dependency coverage.
5. `sop security inspect` keeps dependencies listed as unsupported and
   `required_ready` stays false.

Operator-authorized next step (not performed here): after obtaining explicit
approval to download external advisory data, run
`scripts/security/fetch-osv-databases.sh --dest .agent-sdlc/osv-db`, confirm the
resulting `.agent-sdlc/osv-db/osv-scalibr/<ecosystem>/all.zip` layout, then re-run
`sop security inspect` and `sop security scan --required` to confirm dependency
coverage flips to COMPLETE and the gate can pass.

## Gaps

None identified for the covered behaviors. No download was performed. No test
was added; the plan's scope was verification of existing tests.
