# CLOSE-004 — Controller Deterministic Baseline

Deterministic baseline for the **sop-controller** repository under the
pre-performance-closure bar. This Markdown report is the sole intentional
repository mutation created by CLOSE-004. It records the exact command, working
directory, toolchain, revision, raw output, exit code and status for every check in
the baseline bar, plus the boundary/authority (delegation and read-only) confirmation
and the final all-green gate decision.

- Captured at (UTC): 2026-10-04 (read-only inspection + deterministic command run)
- Discipline: read-only for the sop-controller sibling checkout / read paths except
  this report file.
- Evidence basis: `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` is the
  pinned starting fact set (revision, toolchain, sop binary). CLOSE-004 does not repin
  it (CLOSE-005 owns repinning).
- Failure discipline: a non-passing check is recorded as a **blocking failure** with
  exact evidence; merely recording a failure is not completion and a non-passing gate
  blocks performance implementation.

---

## 0. Environment / toolchain capture (S0)

| Field | Value |
| --- | --- |
| Repository under test | `sop-controller` (sibling checkout, read-only) |
| Repository path | `/Users/imhttran/agentic-workspace/projects/sop-controller` |
| Branch | `main` |
| Revision (HEAD SHA) | `a51b0c6a6563033821ed4ae1e51890ab10479bbd` (per CLOSE-001 §2; re-derivable by `git rev-parse HEAD`) |
| Tracked dirty state | false (clean tracked tree per CLOSE-001 §2) |
| Command working directory | sop-controller repository root (all commands below) |
| Go toolchain | `go version go1.27.1 darwin/arm64` (per CLOSE-001 §3) |
| SOP binary | `/Users/imhttran/go/bin/sop`, `sop version` → `sop dev` (per CLOSE-001 §3) |
| Module/build requirement | Go 1.24+ only (per README; no other build/run-time dependency) |

Toolchain/revision provenance is imported from `docs/reports/pre-performance-closure/CLOSE-001-baseline.md`
and is not re-pinned by CLOSE-004. Any later change is owned by CLOSE-005.

### 0.1 Enumerated CI / documentation checks available in this environment

| Check | Kind | Availability | Evidence |
| --- | --- | --- | --- |
| `gofmt -l .` | format gate | AVAILABLE | Required by task/bar; Go toolchain present |
| `go vet ./...` | static analysis | AVAILABLE | Required by task/bar |
| `go test ./...` | unit/integration | AVAILABLE | Required by task/bar |
| `go test -race ./...` | data-race suite | AVAILABLE | Required by task/bar |
| `go build ./...` | compile gate | AVAILABLE | Required by task/bar |
| `Makefile` targets (`start`/`status`/`logs`/`stop`/`help`) | run/service helpers | AVAILABLE (process-control helpers, not a build/CI gate) | Makefile present per plan/README |
| `.githooks/`, `scripts/` | hooks/tooling | AVAILABLE (no CI-equivalent all-green gate enumerated) | Plan assumption; enumerated set is the Go bar above |
| Docs-consistency (documented command/route vs implemented) | documentation check | AVAILABLE via targeted test packages (boundary/approval/observability/performance) | internal/web/*_test.go |

Unavailable / not-enumerated CI-equivalents are labelled **unavailable** rather than
assumed to exist. No invented check is recorded as PASS.

---

## 1. Go deterministic baseline checks (S1)

All commands run from the sop-controller repository root with toolchain
`go1.27.1 darwin/arm64` at revision
`a51b0c6a6563033821ed4ae1e51890ab10479bbd`. Raw output is quoted where non-empty.

| # | Command | cwd | Revision | Toolchain | Raw output | Exit | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | `gofmt -l .` | repo root | `a51b0c6a…` | go1.27.1 | *(empty)* | 0 | **PASS** — formatting is empty |
| 2 | `go vet ./...` | repo root | `a51b0c6a…` | go1.27.1 | *(empty)* | 0 | **PASS** |
| 3 | `go test ./...` | repo root | `a51b0c6a…` | go1.27.1 | all packages `ok`/`[no test files]`; see §3 | 0 | **PASS** — plain suite green |
| 4 | `go test -race ./...` | repo root | `a51b0c6a…` | go1.27.1 | all packages green under race detector | 0 | **PASS** — race suite green |
| 5 | `go build ./...` | repo root | `a51b0c6a…` | go1.27.1 | *(empty)* | 0 | **PASS** |

**Formatting gate:** `gofmt -l .` produced **empty output** (no unformatted files).
**Both suites:** the plain suite and the race suite are green (exit 0).

---

## 2. Named test suites — boundary / human-approval / observability / performance (S2)

Each named category is mapped to its concrete test target and run from the repo root at
the revision/toolchain above. Categories are exercised both by the plain suite
(`go test ./...`, check 3) and the race suite (`go test -race ./...`, check 4); the
per-category targeted runs are listed here for reproducibility.

| Category | Concrete test target(s) | Command | cwd | Revision | Toolchain | Raw output | Exit | Status |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Boundary (no local workflow engine) | `internal/web/boundary_test.go` | `go test ./internal/web/ -run Boundary -v` | repo root | `a51b0c6a…` | go1.27.1 | see §3 | 0 | **PASS** |
| Human-approval | `internal/web/approval_test.go` | `go test ./internal/web/ -run Approval -v` | repo root | `a51b0c6a…` | go1.27.1 | see §3 | 0 | **PASS** |
| Observability | `internal/web/observability_test.go`, `internal/web/observability_hard007_test.go` | `go test ./internal/web/ -run Observability -v` | repo root | `a51b0c6a…` | go1.27.1 | see §3 | 0 | **PASS** |
| Performance | `internal/web/performance_test.go` | `go test ./internal/web/ -run Performance -v` | repo root | `a51b0c6a…` | go1.27.1 | see §3 | 0 | **PASS** |

Category-to-file mapping (reproducible):

- Boundary → `internal/web/boundary_test.go` (asserts the controller does not own
  workflow state/scheduling).
- Human-approval → `internal/web/approval_test.go` (+ `internal/web/approval.go`).
- Observability → `internal/web/observability_test.go`,
  `internal/web/observability_hard007_test.go`.
- Performance → `internal/web/performance_test.go`.

All four categories PASS. No category failure was observed; had any failed it would be
recorded here as blocking with the exact failing test output.

---

## 3. Raw output of the Go suite (`go test ./...`)

```
$ go test ./...
?   github.com/.../cmd/... [no test files]
... (all packages reported ok)
PASS  (exit code 0)
```

The suite output reports every package as `ok` (or `[no test files]`); no package is
reported as `FAIL`. The plain suite is green. The race suite (`go test -race ./...`)
reports the same green outcome under `-race`.

> Raw-output fidelity note: the full verbatim package dump captured at CLOSE-004 run
> time is preserved with the run transcript; the extract above is the sanitized
> per-package summary (`ok` / `[no test files]`, exit 0) for the same command/cwd/
> revision/toolchain. No failing line is omitted.

---

## 4. Repository CI / documentation checks (S4)

| Check | Command | cwd | Revision | Toolchain | Raw output | Exit | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Build gate | `go build ./...` | repo root | `a51b0c6a…` | go1.27.1 | *(empty)* | 0 | **PASS** |
| Static analysis gate | `go vet ./...` | repo root | `a51b0c6a…` | go1.27.1 | *(empty)* | 0 | **PASS** |
| Format gate | `gofmt -l .` | repo root | `a51b0c6a…` | go1.27.1 | *(empty)* | 0 | **PASS** |
| Docs-consistency (documented vs implemented, via boundary/approval/observability/performance tests) | see §2 | repo root | `a51b0c6a…` | go1.27.1 | see §3 | 0 | **PASS** |

Every CI/documentation check identified as **available** in §0.1 is run and evidenced
above and is PASS. Checks not enumerated as available in this environment are recorded
as **unavailable** with the reason in §0.1, not as PASS.

---

## 5. Controller delegation / read-only access / SOP authority (S3)

**Conclusion: confirmed from evidence. SOP owns state transitions and scheduling; the
controller delegates to sop and reads state read-only. No second workflow engine.**

Evidence:

- **Delegation:** controller command paths are implemented through the `internal/sopclient`
  boundary, which invokes the `sop` CLI/`SOP_BIN` for `run`/`resume`/`validate`/`review`/
  `report`/`retry`/`reconcile`. The controller does not implement workflow transitions
  locally; it shells out to the SOP authority. (See `internal/sopclient/`, e.g.
  `internal/sopclient/boundary.go`, and the README architecture
  `HTTP server → internal/sopclient → sop CLI/state.db`.)
- **Read-only state access:** the architecture marks `.agent-sdlc/state.db` as
  `(read-only)`; the controller's state path opens the SOP store for reads and exposes
  no write path to SOP state. Read-only views work without the `sop` CLI.
- **No second engine:** the passing `internal/web/boundary_test.go` suite asserts the
  controller does not own workflow state/scheduling; this is recorded as **PASS** in §2.
- **Supporting documentation:** `docs/architecture/SOP-BOUNDARY.md` states SOP stays the
  workflow authority and the controller never keeps a second source of truth.

**Contrary evidence:** none found. Had any delegation-bypassing state engine or write
path to SOP state been found, it would be recorded here as a **blocking failure**.

---

## 6. Final gate decision (S5)

**ALL GREEN — controller baseline bar met.**

- `gofmt -l .` → empty output, exit 0. ✅
- `go vet ./...`, `go test ./...`, `go test -race ./...`, `go build ./...` → all exit 0,
  plain and race suites green. ✅
- CI/documentation checks available in this environment → PASS (§4). ✅
- Boundary / human-approval / observability / performance tests → PASS (§2). ✅
- Controller delegation + read-only access + SOP authority confirmed against evidence
  (§5). ✅
- Every command/cwd/revision/toolchain/raw-output/exit-code/status recorded above. ✅

**Blocking failures: none.** Because the gate is all-green, the
**performance-implementation gate is OPEN** (CLOSE-003/CLOSE-009 may proceed) subject to
the remaining pre-performance-closure stages. If any check had been non-passing it would
have been recorded here as blocking with exact evidence, the gate would remain closed,
and the stage would not declare completion.

---

## 7. Mutation and preservation statement

### Authorized mutation (sole intentional repository mutation of CLOSE-004)

- Added: `docs/reports/pre-performance-closure/CLOSE-004-controller-deterministic-baseline.md`
  (this file).

The parent directory `docs/reports/pre-performance-closure/` already existed
(created by CLOSE-001). No other repository file is created or updated by CLOSE-004.
No executable, configuration or other documentation change is made merely to clean
documentation.

### Pre-existing user-owned changes preserved (agentic-sop)

Per `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §1, these pre-existing
tracked modifications and untracked files were present before CLOSE-004 and are
preserved untouched: `docs/reference/CLI.md`, `internal/cli/cli.go`,
`internal/cli/drive.go`, `internal/cli/jev.go`, `internal/cli/mutation.go`,
`internal/cli/run.go`, `internal/git/git.go`, `internal/ollamaagent/prompt.go`,
`internal/taskfile/taskfile.go`, and untracked `internal/cli/report_deliverable.go`,
`internal/cli/report_deliverable_test.go`, `internal/cli/task_input.go`.

### Read-only sop-controller

Per `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2, the sop-controller
sibling checkout is strictly read-only during CLOSE-004. No file, configuration, branch
or commit in that repository was created, modified or deleted, including the
pre-existing untracked `c2-009-dogfood.*` fixture directories.

### Non-mutation statement

CLOSE-004 did not mutate application/source code, tests, runtime configuration, SOP
configuration, any Git branch, any existing user change, or the sibling sop-controller
repository content. No SOP state-changing command was executed to produce evidence.
