# Implementation Plan

## Project

DOGFOOD-001 — Synthetic addition

## Summary

Add a deterministic Add(a, b int) int function in the existing Go package `synthetic` along with a matching unit test. The repository is a disposable dogfood Go module (module `dogfood`, go 1.23) with a single existing package directory `synthetic/` containing only `doc.go` (package declaration, no code). No external dependencies are required: the standard Go toolchain and the standard `testing` package are sufficient. The work decomposes into a discovery/contract stage that confirms the package boundary and conventions, a feature stage that implements `Add` in `synthetic/example.go`, a test stage that adds a unit test covering the acceptance criterion, and a verification stage that runs the package tests. There is no scheduler, persistence, provider, adapter, CLI, or authorization surface involved.

## Capabilities

### Go toolchain (go build/test) — EXISTS

- Location: go.mod
- Owner: Development environment / Go distribution
- Evidence: go.mod declares `module dogfood` and `go 1.23`, indicating a Go module buildable/testable with the standard toolchain; go.mod and go.sum-style module metadata is present at repository root.

### Go standard library testing package — EXISTS

- Location: go.mod (module `dogfood`, go 1.23)
- Owner: Go standard library (distributed with the Go toolchain)
- Evidence: Go module with go 1.23; the standard library `testing` package ships with the toolchain and requires no external dependency, matching the PRD statement 'No external dependencies'.

### Target Go package `synthetic` — EXISTS

- Location: synthetic/doc.go
- Owner: Repository (existing code, stage `discover-package` confirms conventions)
- Evidence: synthetic/doc.go contains `package synthetic`, establishing the package declaration and directory the PRD's synthetic/example.go must belong to.

### Existing Add function implementation — MISSING

- Location: synthetic/
- Owner: Implementation stages of this plan (create synthetic/example.go)
- Evidence: synthetic/ contains only doc.go (package clause, no functions). A repository-wide search for `func Add` matched only synthetic.diff (an untracked/unrelated diff artifact, not source), so no `Add` exists in the package today.
- Gap: No `Add(a, b int) int` exists; the acceptance criterion is unmet.
- Resolution: Create `synthetic/example.go` with `func Add(a, b int) int { return a + b }` in stage `implement-add`.

### Unit test for Add — MISSING

- Location: synthetic/
- Owner: Test stage of this plan (create synthetic/example_test.go or example_test.go in package synthetic)
- Evidence: No *_test.go files exist under synthetic/ (directory listing shows only doc.go); no repository search hit for any test of Add.
- Gap: No unit test exercising `Add`.
- Resolution: Add a table-driven or simple unit test in stage `add-unit-test` using the standard `testing` package.

### Existing changes in working tree (untracked proxy-state.json, proxy.log, runner.log, g4a-probe-*.json, synthetic.diff) — UNKNOWN

- Location: repository root
- Owner: User / dogfood harness (outside this task)
- Evidence: git status shows untracked files at repository root (proxy-state.json, proxy.log, runner.log) plus g4a-probe-record.json, g4a-probe-response.json, synthetic.diff, task.md; these are not part of DOGFOOD-001 scope.
- Gap: Provenance of these untracked artifacts is not established; they are not required by this plan.
- Resolution: Treat as user-owned; do not modify, revert, or depend on them. No stage requires them.

## Assumptions

### The repository is a Go module and tests are run with `go test ./...` (or `go test ./synthetic/`) using the standard toolchain.

- Evidence: go.mod declares `module dogfood` and `go 1.23`; no custom build files (Makefile, CI config, or task runner) were observed in the repository root listing.
- Consequence: Verification stages use standard `go test` invocations; if a custom build harness exists outside the inspected paths, invocation may need adjustment but the deliverable (Add + unit test) is unaffected.

### The PRD's `synthetic/example.go` refers to a new file inside the existing package directory `synthetic`, not a new package.

- Evidence: task.md scope names `synthetic/example.go`; synthetic/doc.go already declares `package synthetic`, so the file belongs to that existing package.
- Consequence: The new file uses `package synthetic` and is added in the same directory; no new module or package scaffolding is needed.

### `Add` has no additional constraints (no overflow/error handling, no generics), since the PRD only requires the sum of two integers and is deterministic.

- Evidence: PRD acceptance criterion: 'Add returns the sum of two integers'; scope limited to `Add(a, b int) int`; task explicitly states 'No external dependencies'.
- Consequence: Implementation is a one-line `return a + b`; the unit test asserts integer-sum equality for representative and boundary integer inputs.

### The unit test file lives in package `synthetic` (internal test) and uses the standard `testing` package only.

- Evidence: Package `synthetic` exists with an exported function; no external test helpers or third-party assertion libraries are present in the module or permitted by the 'No external dependencies' constraint.
- Consequence: Test file is e.g. `synthetic/example_test.go` with `package synthetic` and `func TestAdd(t *testing.T)`; no go.mod changes or new dependencies are introduced.

## discover-package — Confirm package boundary and build/test conventions for `synthetic`

Inspect the repository to confirm the package declaration, module path, Go version, and how tests are expected to run, so implementation lands in the correct package and file path. Verify synthetic/ contents (only doc.go), read go.mod, and check for any build/test configuration that would change the `go test` invocation. Do not create or modify any file.

### Dependencies

None

### Requires

- Go toolchain (go build/test)

### Deliverables

- Recorded package/module facts: package name `synthetic`, module path `dogfood`, Go version `go 1.23`
- Confirmed target file paths `synthetic/example.go` and `synthetic/example_test.go`
- Confirmed test invocation (e.g. `go test ./synthetic/`) and absence of other packages

### Acceptance Criteria

- Package declaration `package synthetic` and module `dogfood` (go 1.23) are explicitly reported from repository inspection.
- The list of files in `synthetic/` and any root-level build/test configuration is reported, with the chosen test command stated.
- No repository file is created or modified during this stage.

## implement-add — Implement `Add(a, b int) int` in synthetic/example.go

Create `synthetic/example.go` in package `synthetic` containing a deterministic, dependency-free function `Add(a, b int) int` that returns the sum of its two integer inputs, using only the Go standard language and no imports beyond what is strictly needed (none required).

### Dependencies

- discover-package

### Requires

- Go toolchain (go build/test)

### Deliverables

- synthetic/example.go declaring `package synthetic` and `func Add(a, b int) int { return a + b }` with a doc comment

### Acceptance Criteria

- `synthetic/example.go` exists, declares `package synthetic`, and defines exactly `func Add(a, b int) int` returning `a + b`.
- The package compiles: `go build ./synthetic/` (or `go vet ./synthetic/`) succeeds with no errors.
- No new module dependency is introduced; go.mod is unchanged.

## add-unit-test — Add unit test for `Add`

Create a unit test in package `synthetic` (e.g. `synthetic/example_test.go`) using only the standard `testing` package, asserting that `Add` returns the sum of two integers for representative inputs including zero, negatives, and same-sign operands. Table-driven cases where practical.

### Dependencies

- implement-add

### Requires

- Go toolchain (go build/test)
- Go standard library testing package

### Deliverables

- synthetic/example_test.go with `func TestAdd(t *testing.T)` covering the acceptance criterion

### Acceptance Criteria

- Test file declares `package synthetic` and imports only the standard `testing` package (no third-party imports).
- Test cases include at least: (1, 2) -> 3, (0, 0) -> 0, negative operands (e.g. (-1, -2) -> -3), and mixed sign (e.g. (-1, 1) -> 0).
- `go test ./synthetic/` runs and the test compiles; any failure output would identify the case.

## verify — Run tests and confirm the acceptance criterion

Execute the package tests (`go test ./synthetic/`, and `go test ./...` if other packages exist) and report the result, confirming the PRD acceptance criterion that `Add` returns the sum of two integers is satisfied and no external dependencies are used.

### Dependencies

- add-unit-test

### Requires

- Go toolchain (go build/test)

### Deliverables

- Recorded `go test` output showing passing tests
- Confirmation that `Add` returns the sum of two integers per the acceptance criterion
- Confirmation that go.mod/go.sum have not gained dependencies

### Acceptance Criteria

- `go test ./synthetic/` exits 0 with the Add unit test reported as passing.
- The reported output demonstrates coverage of the acceptance criterion 'Add returns the sum of two integers'.
- No external (non-standard-library) dependency is added; go.mod is unchanged.
- User-owned untracked files (proxy-state.json, proxy.log, runner.log, g4a-probe-*.json, synthetic.diff) are not modified or removed.

