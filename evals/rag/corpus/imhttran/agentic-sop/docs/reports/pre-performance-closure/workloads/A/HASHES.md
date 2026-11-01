# Workload A — Frozen Input Hashes

Frozen before any repetition. Values observed on the pinned fixture tree; where a
value could not be computed with the available tooling it is recorded `UNAVAILABLE`
rather than estimated.

| Artifact | Path | Role | SHA-256 |
| --- | --- | --- | --- |
| source | `A/sum.go` | production placeholder (`Add` returns 0) | `UNAVAILABLE` (hash tool not available in this environment) |
| test_input | `A/sum_test.go` | immutable acceptance table `Add(1,2)=3`, `Add(-2,5)=3`, `Add(0,0)=0` | `UNAVAILABLE` (hash tool not available in this environment) |
| module | `A/go.mod` | module `example.com/sop-baseline/a` | `UNAVAILABLE` (hash tool not available in this environment) |
| config | `A/config.yaml` | deterministic check config | `UNAVAILABLE` (hash tool not available in this environment) |
| task | `A/PLAN-Workload-A.md` | task A001 (change ONLY `sum.go`) | `UNAVAILABLE` (hash tool not available in this environment) |
| criteria | `A/PLAN-Workload-A.md` | acceptance criteria | `UNAVAILABLE` (hash tool not available in this environment) |
| fixture_patch | `n/a` | not applicable for A | `n/a` |

No test tampering. The fixture and tests are never changed between repetitions.
