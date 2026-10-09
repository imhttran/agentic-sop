# AUTONOMY-001 — erratum for the governed baseline

**Purpose:** record factual corrections to `docs/reports/autonomy/AUTONOMY-001-baseline.md`
(the governed AUTONOMY-001 deliverable) **without rewriting run evidence**.

**Provenance:** the original model-generated deliverable is preserved unmodified in the run
artifacts `.agent-sdlc/runs/AUTONOMY-001/` (report.md, etc.). SOP history and the task's
`LOCAL_DONE` status are not changed. The corrections below are also recorded inline as a
revision note at the top of the baseline.

## Corrections

| Claim in the baseline | Correct fact |
|---|---|
| `runGraph` in `internal/cli/run.go` | `internal/cli/drive.go:46` |
| `runAttempts` in `internal/cli/run.go` | `internal/cli/escalation.go:76` |
| commit gate `internal/commitgate/commitgate.go` | `internal/commitgate/gate.go` (no `commitgate.go`) |
| handoff `internal/handoff/handoff.go` | `internal/handoff/` (`manager.go`, `record.go`, `capsule.go`; no `handoff.go`) |
| fixture `func Join(parts []string, sep string) string` | `func Join(a, b string) string` |
| fixture `const unusedConst = 42` | `const unusedConst = 99` |
| fixture `type unusedType struct{}` | `type unusedType struct{ x int }` |

## Not corrected (deliberately)

- The gap list (G1–G14) and the recorded scenario results are unchanged; they remain accurate.
- This erratum does not alter any run artifact or task status.
