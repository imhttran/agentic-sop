# Import Verification — MIG-003D

Status: Complete

This report records the reconciliation of `go.mod`/`go.sum` for the imported controller
and adapter code under the existing core toolchain (go 1.27.1), verifies every imported
package builds and tests pass in the single root module, and records the architecture
guard behavior after the transport-subtree extension.

## 1. Module reconciliation

- Module: `github.com/imhttran/agentic-sop` (go 1.27.1), declared in `go.mod` at the
  repository root.
- `go.sum` is present alongside `go.mod` at the repository root.
- Direct requirements: `gopkg.in/yaml.v3 v3.0.1`, `modernc.org/sqlite v1.59.0`.
- Indirect requirements: `github.com/dustin/go-humanimize v1.0.1`,
  `github.com/google/uuid v1.6.0`, `github.com/mattn/go-isatty v0.0.24`,
  `github.com/ncruces/go-strftime v1.0.0`,
  `github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec`,
  `golang.org/x/sys v0.47.0`, `modernc.org/libc v1.75.7`,
  `modernc.org/mathutil v1.7.1`, `modernc.org/memory v1.12.1`.
- No nested `go.mod` files exist under the imported subtrees
  (`internal/controller`, `internal/decisionadapter`, `internal/decision`). The only
  module boundary is the root `go.mod`, so no nested module silently excludes migrated
  tests. (Note: some tests create their own temporary fixture modules under `testdata`
  or `t.TempDir()`; these are test-runtime fixtures, not source-tree nested modules, and
  do not exclude any migrated test.)

## 2. Imported package inventory

### internal/controller

- `internal/controller/assets` — static web assets (no Go packages).
- `internal/controller/config` — discovery/config helpers.
- `internal/controller/sopclient` — SOP client. Test files:
  `accept_changed_test.go`, `approvals_client_test.go`, `approvals_test.go`,
  `approvals_wiring_test.go`, `boundary_test.go`, `changed_task_test.go`,
  `commands_test.go`, `decision_test.go`, `discovery_test.go`,
  `observability_test.go`, `pending_approvals_test.go`, `performance_test.go`,
  `run_test.go`, `status_test.go`, `store_test.go`.
- `internal/controller/web` — web handlers/server. Test files:
  `activity_isolation_test.go`, `activity_stream_test.go`, `approval_test.go`,
  `approvals_inbox_test.go`, `boundary_test.go`, `commands_progress_test.go`,
  `dashboard_controls_test.go`, `decisions_test.go`, `discovery_test.go`,
  `dogfood_test.go`, `login_test.go`, `model_selection_test.go`,
  `observability_hard007_test.go`, `observability_test.go`,
  `performance_test.go`, `reconcile_test.go`, `recovery_test.go`,
  `server_test.go`, `session_persist_test.go`, `startup_test.go`,
  `status_test.go`.

### internal/decisionadapter

- `internal/decisionadapter/providers/clef` — provider adapter.
- `internal/decisionadapter/providers/fallback` — provider adapter.
- `internal/decisionadapter/providers/julia` — provider adapter.
- `internal/decisionadapter/providers/nimble` — provider adapter.
- `internal/decisionadapter/shadow` — shadow evidence. Test files:
  `evidence_test.go`, `shadow_test.go`.

### internal/decision

- `internal/decision` — decision logic. Test files: `benchmark_fixtures_test.go`,
  `decision_test.go`, `governance_test.go`, `hardening_test.go`, `select_test.go`,
  `validator_test.go`.
- `internal/decision/bridge` — bridge package.
- `internal/decision/command` — command package.
- `internal/decision/commandprofile` — command profile package.

## 3. Fixture accounting

Every imported component's tests and fixtures are enumerated below. No build tag,
build constraint, or nested module excludes any of these from `go test ./...`.

| Package | Test files | Fixtures |
| --- | --- | --- |
| internal/controller/sopclient | accept_changed_test.go, approvals_client_test.go, approvals_test.go, approvals_wiring_test.go, boundary_test.go, changed_task_test.go, commands_test.go, decision_test.go, discovery_test.go, observability_test.go, pending_approvals_test.go, performance_test.go, run_test.go, status_test.go, store_test.go | none (in-memory) |
| internal/controller/web | activity_isolation_test.go, activity_stream_test.go, approval_test.go, approvals_inbox_test.go, boundary_test.go, commands_progress_test.go, dashboard_controls_test.go, decisions_test.go, discovery_test.go, dogfood_test.go, login_test.go, model_selection_test.go, observability_hard007_test.go, observability_test.go, performance_test.go, reconcile_test.go, recovery_test.go, server_test.go, session_persist_test.go, startup_test.go, status_test.go | dogfood fixture built at runtime via t.TempDir |
| internal/decisionadapter/shadow | evidence_test.go, shadow_test.go | none (in-memory) |
| internal/decision | benchmark_fixtures_test.go, decision_test.go, governance_test.go, hardening_test.go, select_test.go, validator_test.go | testdata fixtures referenced by benchmark_fixtures_test.go |

No imported package under the three subtrees is excluded by a build constraint.

## 4. Verification results

- `go build ./...` — succeeds under one module.
- `go test ./...` — passes for all imported packages (including
  `./decision/...`, `./internal/controller/...`, `./internal/decisionadapter/...`,
  `./internal/decision/...`, `./internal/archtest/...`).
- `go vet ./...` — passes (including `./decision/...`,
  `./internal/controller/...`, `./internal/decisionadapter/...`).

`go.mod` and `go.sum` are consistent; no unresolved or missing requirements remain.

## 5. Architecture guard behavior

The architecture guard (`internal/archtest/guard_test.go`) now enforces two invariants:

1. **Provider coupling in core remains forbidden.** Core SOP and policy/governance
   packages (`corePackages`) must never depend on a provider implementation or any
   declared forbidden direct dependency. Covered by `TestArchitectureGuard`,
   `TestGuardDetectsRepresentativeForbiddenDependency`, and
   `TestProviderRegistryCoversProviderAdapters`.

2. **Transport code is permitted only in the designated adapter subtree.** Transport
   code (concrete HTTP/socket transports, wire protocols, and vendor SDKs — e.g.
   `net/http`, `net/url`, `net/rpc`, `crypto/tls`, `golang.org/x/net`, and the
   `internal/provider/httpx` adapter) is permitted only under
   `internal/decisionadapter`. Any transport import in a core SOP or
   policy/governance package is flagged with category
   `transport-outside-adapter-subtree`. Covered by
   `TestTransportRestrictedToAdapterSubtree` (positive control),
   `TestTransportRuleDetectsViolationOutsideAdapter` (violation control), and
   `TestTransportRuleAcceptsAdapterSubtree` (adapter-subtree positive control).

The guard remains stdlib-only, offline, and CI-safe: no third-party dependency or
framework was introduced.
