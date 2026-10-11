# RM-004 — Reproducibility, Coverage, and Neutrality Verification

Status: verification-only. This report creates, modifies, or deletes no production
file. It is the sole write of the stage and lives under `docs/reports/run-metrics/`.

This report independently verifies four properties of the run-metrics aggregation
(`internal/runmetrics`, RM-003):

1. **Determinism** — a re-run of the aggregation over identical inputs yields
   byte-identical output.
2. **Coverage reconciliation** — every metric's coverage percentage reconciles to
   the RM-001 census denominators.
3. **Absence is never read as zero** — a fixture with a missing artifact yields
   `UNAVAILABLE`, while a fixture with a recorded `0` yields `0`.
4. **Production-path neutrality** — no production path imports or references
   `internal/runmetrics`.

It also re-confirms the full Go toolchain gates and a clean `git diff --check`.

Terminology is preserved from the PRD and RM-001: **artifact**, **run**,
**denominator**, **eligible runs**, **UNAVAILABLE**, **coverage**, **census
**denominators**, and **provider/model neutrality**.

## 1. Determinism — re-run identity check

The aggregation is a pure Go library reached through the exported entry point
`runmetrics.AggregateProject(projectDir string) (Aggregate, error)`. There is no
CLI wiring (`TestNoCLIModification`), so the deterministic re-run is exercised the
same way the library's own reproducibility tests exercise it: two independent
aggregations over the same input corpus, JSON-encoded, compared byte-for-byte.

### 1.1 Exact commands

```console
$ go test ./internal/runmetrics/ -run 'TestReproducible' -v
$ go test ./internal/runmetrics/ -run 'TestGoldenOutput' -v
```

`TestReproducible` calls `AggregateProject` twice per fixture over the identical
inputs and asserts `string(json.MarshalIndent(first)) == string(json.MarshalIndent(second))`
for every fixture (`present`, `absent`, `zero`). `TestGoldenOutput` compares the
aggregate JSON of each fixture against a committed golden file, so any drift in a
metric value, denominator, or coverage percentage is caught against a recorded
identity.

### 1.2 Input identity

The inputs are the fixture corpora under
`internal/runmetrics/testdata/{present,absent,zero}` and, for the live aggregate,
the recorded run artifacts under `.agent-sdlc/runs/`. The library reads only
directory listings and recorded artifact bytes — no model call and no network call
— so the input identity is the byte content of those directories. Two aggregations
that consume the same bytes are provably over identical inputs; the comparison is
made on the marshalled output, not on a timestamp or any wall-clock field.

### 1.3 Byte-identical result

Both invocations pass with exit status 0. The two outputs per fixture compare
equal byte-for-byte (no differing bytes). Recorded verdict: **PASS** — a re-run of
the aggregation over identical inputs yields byte-identical output.

Supporting evidence in the source tree:

- `internal/runmetrics/runmetrics_test.go` — `TestReproducible` marshals each
  aggregate twice and compares the strings.
- `internal/runmetrics/golden_test.go` — `TestGoldenOutput` pins the aggregate JSON
  against `internal/runmetrics/testdata/golden/{present,absent,zero}.json`.
- `internal/runmetrics/aggregate.go` — `aggregate` emits metrics in the fixed
  register order and sorts artifacts by name; the `Aggregate` struct carries no
  wall-clock field.

## 2. Coverage reconciliation against RM-001 census denominators

RM-001 (`docs/reports/run-metrics/RM-001-artifact-inventory.md`) fixes the census
denominator as **164 run directories (census 2026-10-08)**. The RM-001 per-metric
eligible-run denominators are quoted in RM-001 §5.2. RM-003 §1 records that the
corpus grew to 168 run directories at aggregation time, because it now includes the
RM-001/RM-002/RM-003 runs themselves. Coverage in both reports is computed as

```
coverage_pct = valid_measurements / total_runs
```

Reconciliation recomputes each metric's percentage from the numerator and
denominator and compares it to the reported value.

### 2.1 Per-metric reconciliation (live corpus, RM-003)

| Metric | Numerator | Denominator | Denom. source | Recomputed | Reported | Match |
| --- | ---: | ---: | --- | ---: | ---: | --- |
| `agent_success_failure_rate` | 126 | 135 | RM-001 §5.2 / RM-003 | 93.3% | 93.3% | YES |
| `routing_finding_severity` | 53 | 53 | RM-001 §5.2 (model-selection.json) | 100.0% | 100.0% (53/53) | YES |
| `fix_loop_convergence` | 84 | 150 | RM-001 §5.2 (metrics.json) | 56.0% | 56.0% (84/150 mean) | YES |
| `retry_efficiency` | 61 | 61 | RM-001 §5.2 (attempt/continuations/classification) | 100.0% | 100.0% (61/61) | YES |
| `human_approval_frequency` | 9 | 135 | RM-001 §5.2 / RM-003 | 6.7% | 6.7% | YES |
| `execution_latency` | 17000500 | 150 | RM-001 §5.2 (metrics.json) | 113336.7 ms/run | 113336.7 ms/run | YES |
| `token_usage` | 0 | 0 | RM-001 §4 dimension 7 (UNAVAILABLE) | 0.0% | 0.0% | YES |

### 2.2 Coverage against the RM-001 census denominator (164)

RM-001 §2 and §5.2 state each artifact's coverage of 164. RM-003's eligible-run
counts are compared below against the RM-001 census denominator, with the delta
explained by the corpus growth from 164 to 168.

| Metric (artifact) | RM-001 eligible | RM-001 coverage of 164 | RM-003 valid | RM-003 coverage of 168 | Delta cause |
| --- | ---: | ---: | ---: | ---: | --- |
| Total run latency (`metrics.json`) | 146 | 89.0% | 150 | 89.3% | corpus grew 164→168 |
| Validation cost (`metrics.json`) | 146 | 89.0% | 150 | 89.3% | corpus grew 164→168 |
| Review cost (`metrics.json`) | 146 | 89.0% | 150 | 89.3% | corpus grew 164→168 |
| Agent/call efficiency (`metrics.json`) | 146 | 89.0% | 150 | 89.3% | corpus grew 164→168 |
| Model routing (`model-selection.json`) | 50 | 30.5% | 53 | 31.5% | corpus grew 164→168 |
| Task classification (`classification.json`) | 31 | 18.9% | 32 | 19.0% | corpus grew 164→168 |
| Change footprint (`changed-files.json`) | 39 | 23.8% | (not in RM-003 table) | — | artifact not emitted by RM-003 |
| Token usage | 0 — UNAVAILABLE | 0.0% | 0 | 0.0% | no recorded source; unchanged |

Reconciliation note: every count that RM-003 reports is consistent with the RM-001
census denominator of 164 and the four additional run directories recorded between
census and aggregation. No metric widens its denominator by zero-filling runs whose
artifact or field is UNAVAILABLE, which is the failure mode RM-001 §5.2 forbids.
The `changed-files.json` dimension has no row in RM-003's emitted metric table, so
its coverage is recorded here as **UNKNOWN** for the aggregation output rather than
being assumed; it does not enter any of the seven emitted metrics.

### 2.3 Verdict

All emitted metrics reconcile to their RM-001 denominators and recomputed
percentages match the reported percentages. Verdict: **PASS**, with one recorded
**UNKNOWN** (the `changed-files.json` change-footprint dimension is not emitted by
the aggregation, so no recomputation applies). No denominator was invented.

## 3. Zero-vs-UNAVAILABLE demonstration

The load-bearing rule (RM-001 §5.1) is that an absent artifact is `UNAVAILABLE` and
is never a recorded zero. Two fixtures under `internal/runmetrics/testdata/`
exercise the two cases over the same metric (`execution_latency`), read from
`metrics.json.total_ms`.

### 3.1 Fixture definitions

- **Missing-artifact fixture:** `internal/runmetrics/testdata/absent/` — a run
directory that records only `state.json` and `continuations.txt`. No
`metrics.json` is present.
- **Recorded-zero fixture:** `internal/runmetrics/testdata/zero/` — a run directory
  that records `metrics.json` with `total_ms: 0` and empty `counts`.

Both fixtures live under the package's own `testdata/` tree, never a production
path. They were created by the RM-003 stage (they are committed test fixtures of
`internal/runmetrics`); this verification stage adds no fixture file.

### 3.2 Exact commands

```console
$ go test ./internal/runmetrics/ -run 'TestAbsentIsUnavailableAndReducesCoverage|TestZeroIsRecordedMeasurement|TestZeroArtifactPresenceClassification' -v
```

### 3.3 Raw output excerpts

Missing-artifact fixture (`absent`) — `execution_latency` emitted as `UNAVAILABLE`:

```json
{
  "metric": "execution_latency",
  "status": "AVAILABLE",
  "value": "UNAVAILABLE",
  "unit": "ms",
  "total_runs": 1,
  "eligible_runs": 0,
  "artifacts_present": 0,
  "valid_measurements": 0,
  "unavailable_measurements": 1,
  "numerator": 0,
  "denominator": 0,
  "coverage_pct": 0
}
```

Recorded-zero fixture (`zero`) — `execution_latency` emitted as a recorded `0`:

```json
{
  "metric": "execution_latency",
  "status": "AVAILABLE",
  "value": "0",
  "unit": "ms",
  "total_runs": 1,
  "eligible_runs": 1,
  "artifacts_present": 1,
  "valid_measurements": 1,
  "unavailable_measurements": 0,
  "numerator": 0,
  "denominator": 1,
  "coverage_pct": 100
}
```

The two outputs are demonstrably distinct: `"value": "UNAVAILABLE"` with
denominator 0 and coverage 0 on the missing-artifact fixture, versus `"value": "0"`
with denominator 1 and coverage 100 on the recorded-zero fixture. Neither is coerced
into the other. A present artifact with an explicit `0` counts toward the
denominator; an absent artifact is `UNAVAILABLE` and lowers coverage.

Additionally, `TestZeroArtifactPresenceClassification` asserts the `metrics.json`
artifact presence state on the zero fixture is `zero` (present-with-zero), not
`absent`, so the three states (absent, present, present-with-zero) are never
conflated.

### 3.4 Verdict

Missing artifact → `UNAVAILABLE`; recorded `0` → `0`. Verdict: **PASS**.

## 4. Production-path neutrality for `internal/runmetrics`

### 4.1 Production package roots and exclusion boundary

For this search, **production** means every `.go` file in the module rooted at the
repository root except:

- the `internal/runmetrics/` package itself (including its tests and testdata),
- any path containing `testdata`,
- `.agent-sdlc/` and `.git/` (non-Go, SOP state),
- `vendor/` if present.

Test-only surfaces are therefore the `runmetrics` package's own tests and any file
under a `testdata/` directory. Everything else — `internal/*`, `cmd/*`, and any
root-level package — counts as production.

### 4.2 Exact search commands

Import-path search over the whole module tree (the same walk `TestNoProductionImportOfRunmetrics`
performs, using `go/parser` with `parser.ImportsOnly`):

```console
$ go test ./internal/runmetrics/ -run 'TestNoProductionImportOfRunmetrics' -v
```

Direct textual import search a reviewer can reproduce without running Go:

```console
$ grep -rn 'internal/runmetrics' --include='*.go' . \
    | grep -v '^./internal/runmetrics/'
```

CLI-surface search (the library must be wired to no CLI command):

```console
$ grep -rn 'runmetrics' internal/cli/
```

### 4.3 Results

- `TestNoProductionImportOfRunmetrics` passes with exit status 0: the walk found
  **zero** offenders, i.e. no `.go` file outside `internal/runmetrics/`, `testdata`,
  `.agent-sdlc`, `.git`, or `vendor` imports
  `"github.com/imhttran/agentic-sop/internal/runmetrics"`.
- The direct `grep` over `--include='*.go'` excluding `./internal/runmetrics/`
  returns **no lines** (empty result set). The only textual references to the
  import path live inside the `runmetrics` package itself (its doc comment and its
  own `unwired_test.go`), which the exclusion boundary removes.
- `grep -rn 'runmetrics' internal/cli/` returns **no matches**, so no CLI command
  imports or invokes the library.
- Symbol/qualified-reference search: because the import path never appears in a
  production file, there is no `runmetrics.`-qualified usage to find; the parser
  walk in `TestNoProductionImportOfRunmetrics` is the authoritative surface for
  qualified references, and it is empty over production roots.

### 4.4 Provider/model neutrality assertion

The aggregation makes no model call and no network call, and no production path
references `internal/runmetrics`. Because the library is not wired into any runtime
path, nothing it emits can influence a gate, a budget, a routing decision, or an
acceptance decision — and it carries no provider- or model-specific branch. Token
usage is `UNAVAILABLE` and is never an input to budget, routing, or acceptance.
This grounds the PRD's **provider/model neutrality** requirement.

### 4.5 Verdict

No production path imports or references `internal/runmetrics`. Verdict: **PASS**.

## 5. Toolchain gates and clean diff

### 5.1 Exact commands and results

```console
$ gofmt -l .
$ go vet ./...
$ go build ./...
$ go test ./...
$ go test -race ./...
$ git diff --check
```

| Gate | Command | Exit status | Result |
| --- | --- | ---: | --- |
| Formatting | `gofmt -l .` | 0 | no unformatted files (empty output) |
| Vet | `go vet ./...` | 0 | pass |
| Build | `go build ./...` | 0 | pass |
| Test | `go test ./...` | 0 | pass |
| Race | `go test -race ./...` | 0 | pass |
| Whitespace diff | `git diff --check` | 0 | clean (empty output) |

### 5.2 No-production-file assertion

The only file this task adds is this report,
`docs/reports/run-metrics/RM-004-verification.md`. It is a documentation artifact
under `docs/`, not a production Go source file. No fixture, no `internal/` file, no
`cmd/` file, and no `go.mod`/`go.sum` entry is created, modified, or deleted by this
task. The determinism, zero-vs-UNAVAILABLE, and neutrality checks reuse the existing
test fixtures and tests under `internal/runmetrics/`; any transient fixture
locations used during the investigation live outside production paths.

Verdict: **PASS** — all gates pass with exit status 0 and `git diff --check` is
clean.

## 6. Final reconciliation checklist

| Acceptance criterion | Report section | Recorded evidence | Verdict |
| --- | --- | --- | --- |
| Re-run over identical inputs yields byte-identical output | §1 | `TestReproducible`, `TestGoldenOutput` | PASS |
| Every metric's coverage reconciles to the RM-001 census denominators | §2 | §2.1 + §2.2 tables | PASS (one UNKNOWN recorded: `changed-files.json` dimension not emitted) |
| Missing artifact → `UNAVAILABLE`; recorded `0` → `0` | §3 | `absent` vs `zero` fixtures, raw JSON excerpts | PASS |
| No production path references `internal/runmetrics` | §4 | import search + `grep` result sets | PASS |
| Toolchain gates pass; `git diff --check` clean | §5 | gate table | PASS |
| No production file created, modified, or deleted | §5.2 | only `docs/reports/run-metrics/RM-004-verification.md` added | PASS |

No criterion is asserted without its command and result. No production code was
modified to satisfy any criterion. The `UNKNOWN` for the `changed-files.json`
change-footprint dimension is recorded explicitly rather than being assumed, per
the RM-001 discrepancy rule, because the aggregation does not emit that dimension.
