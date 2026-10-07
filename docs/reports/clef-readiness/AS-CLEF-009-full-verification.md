# AS-CLEF-009 — Run Full Verification

Status: verification complete (read-only). This record is the AS-CLEF-009 deliverable.

Filing note: this record is filed under **both** names —
`docs/reports/clef-readiness/AS-CLEF-009-verification.md` (the name in the
authoritative plan) and `docs/reports/clef-readiness/AS-CLEF-009-full-verification.md`
(the name in the operator's GO instruction). The content is identical in both.

## 0. Scope

This task runs the repository's required gates plus the repository-specific
architecture/provider-neutrality guard and the execution/decision separation check
discovered during AS-CLEF-001 through AS-CLEF-007. It is a **read-only
verification**: no production source, test, configuration, or default is modified.
The only tree changes in the AS-CLEF-008..010 sequence are the readiness documents.

The required gates (`gofmt`, `go vet`, `go test`, `go test -race`, `go build`,
`git diff --check`) are the ones named in this plan and in the repository `Makefile`.
The repository-specific gates are:

- `internal/archtest` — the AS-CLEF-007 architecture guard that forbids a core or
  policy package from acquiring a provider-implementation import.
- the execution/decision independence check — `internal/model` (execution routing)
  must have no dependency edge to or from `internal/decision` (decision provider).

## 1. Repository state at verification

- Branch: `main`
- HEAD: `05d2396381ffdf74bf3010d4d6ff0abb3bb61814`
- Upstream: `origin/main` (local `main` == `origin/main` at start; no divergence)
- Working tree: clean at start; the single pending change during the run is the
  AS-CLEF-008 deliverable edit
  (`docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md`)
- Go toolchain: `go version go1.27.1 darwin/arm64`
- Unrelated user changes: none present at start (working tree was clean).

## 2. Required gates

All commands were run from the repository root.

| Command | Result | Notes |
|---|---|---|
| `gofmt -l .` | PASS | No files listed (all Go sources are gofmt-clean). |
| `go vet ./...` | PASS | No diagnostics. |
| `go build ./...` | PASS | Builds, exit 0. |
| `go test -count=1 ./...` | PASS | Every package with tests reported `ok`; **0 FAIL**, 0 panic. |
| `go test -race -count=1 ./...` | PASS | Every package reported `ok`; **no `DATA RACE`**, 0 FAIL. |
| `git diff --check` | PASS | No whitespace/conflict-marker errors. |

`go test -count=1 ./...` covered the decision/policy packages directly, including
`internal/decision` (the AS-CLEF-005 fail-closed suite), `internal/autonomy`,
`internal/config`, `internal/cli`, `internal/model`, `internal/e2e/lifecycle`, and
`internal/archtest`.

## 3. Architecture / provider-neutrality guard (repository-specific gate)

Command:

```bash
go test -count=1 -v ./internal/archtest/
```

Result: **PASS** — every guard test passed:

```text
--- PASS: TestArchitectureGuard
--- PASS: TestCorePackageDirectoriesExist
--- PASS: TestPhase8CoveragePreserved
--- PASS: TestProviderImplementationsExist
--- PASS: TestGuardDetectsRepresentativeForbiddenDependency
--- PASS: TestGuardAcceptsNeutralImports
--- PASS: TestProviderRegistryCoversProviderAdapters
--- PASS: TestSameCorePipelineAcrossAdapters
--- PASS: TestCapabilityGateIsBehavioral
--- PASS: TestPromptCacheIsolatesProviderModel
--- PASS: TestAdaptiveRoutingIsCapabilityAndEvidenceDriven
ok  	github.com/imhttran/agentic-sop/internal/archtest
```

Meaning: no core or policy/governance package (`internal/commandpolicy`,
`internal/approval`, `internal/commitgate`, `internal/mergegate`,
`internal/autonomy`, `internal/decision`, `internal/quality`, plus the Phase-8 core
inventory) acquires a direct dependency on a known provider implementation
(`internal/provider/*`, `internal/ollamaagent`) or on the provider contract package
`internal/provider`. The positive control (`TestGuardAcceptsNeutralImports`) confirms
the neutral imports (`internal/agent`, `internal/model`) are never flagged, and the
negative control (`TestGuardDetectsRepresentativeForbiddenDependency`) confirms the
guard actually fires.

## 4. Execution/decision separation (repository-specific gate)

Decision-provider selection must remain independent of execution-model routing.

Dependency-graph check (no import edge in either direction):

```bash
go list -deps ./internal/model    | grep 'internal/decision'   # -> no output
go list -deps ./internal/decision | grep 'internal/model'      # -> no output
```

Result: **PASS** — `internal/model` has no dependency on `internal/decision`, and
`internal/decision` has no dependency on `internal/model`.

Related test coverage:

```bash
go test -count=1 ./internal/model/ ./internal/cli/ -run 'ModelRoute|Execution|Class|Routing'
```

Result: **PASS** (`ok internal/model`, `ok internal/cli`).

This confirms the AS-CLEF-006 finding at a structural level: `SOP_MODEL_DEFAULT_CLASS`
and the SMALL/MEDIUM/LARGE execution models are resolved from configuration,
environment, CLI, and the deterministic router only — never from a decision provider,
`Choice`, `Confidence`, or `Thresholds`.

## 5. Gate decision

**AS-CLEF-009 gate: PASS.** Every required and repository-specific gate passes. No
task in this sequence is marked complete with a failing required verification gate.

The only known non-blocking limitation (carried to AS-CLEF-010) is that
`internal/decision` has no production consumer: the contract is satisfiable by an
external adapter, but wiring one into the live policy path is a separate,
provider-neutral integration step. This is not a gate failure and does not fail any
command above.

## 6. Commands as executed (verbatim)

```bash
go version                                              # go1.27.1 darwin/arm64
git rev-parse --abbrev-ref HEAD                         # main
git rev-parse HEAD                                      # 05d2396381ffdf74bf3010d4d6ff0abb3bb61814
gofmt -l .
go vet ./...
go build ./...
go test -count=1 ./...
go test -race -count=1 ./...
go test -count=1 -v ./internal/archtest/
go list -deps ./internal/model | grep 'internal/decision'
go list -deps ./internal/decision | grep 'internal/model'
go test -count=1 ./internal/model/ ./internal/cli/ -run 'ModelRoute|Execution|Class|Routing'
git diff --check
```

## 7. Acceptance-criteria mapping

| Acceptance criterion | Where satisfied |
|---|---|
| Required gates run where applicable, with exact commands and results recorded. | §2 (command/result table), §6 (verbatim commands). |
| Repository-specific architecture, policy, integration, or end-to-end gates discovered during AS-CLEF-001 are run and recorded. | §3 (architecture guard), §4 (execution/decision separation). |
| The verification record is produced. | This file (filed under both the plan name and the operator-requested name). |
| Gate: no task is marked complete with a failing required verification gate. | §5 (gate PASS). |
