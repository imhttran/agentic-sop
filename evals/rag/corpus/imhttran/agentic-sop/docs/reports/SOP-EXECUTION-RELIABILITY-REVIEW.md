# SOP Execution Reliability — Review Evidence

## Scope

This report covers the direct fixes made during the reliability review, plus the
bounded SOP-006 Rust demonstration. It does not claim that the original
SOP-001..SOP-005 task runs were replayed after those fixes.

- Earlier SOP outcomes: SOP-001..SOP-005 remain `LOCAL_DONE`; their artifacts
  are under `.agent-sdlc/runs/SOP-00{1..5}/`.
- Direct development fixes: the product diff in this change.
- SOP-006: a disposable two-task Rust fixture, not full Rust-migration
  readiness.

No product commit, push, merge, `state.db` edit, or installed-binary replacement
was performed while producing this evidence.

## Candidate

- Path: `.run/candidate/sop` (local, not committed)
- SHA-256: `c161b0aab7e18bf75b90ce940dc878d64148dabf15214f676d0b9d35c6602d03`
- Module: `github.com/imhttran/agentic-sop`
- VCS revision: `e99cde03aff969b458e6d23579d6b3bb3a6fbe56`
- VCS modified: `true`
- Build: `go build -trimpath -o .run/candidate/sop ./cmd/sop`

## Production defects fixed

- Cargo structural safety is checked before the permission flag; malformed or
  dangerous forms are denied even when the permission is off.
- `cargo.agent_tool` now reaches the real native tool harness through the CLI
  composition root, `internal/ollamaagent`, and `internal/toolharness`.
- Agent-tool `--manifest-path` containment uses the primary repository root.
- PENDING and DECLINED approvals block execution before required-evidence
  retrieval, run reset, or model work. A mapped acceptance failure records a
  pending approval instead of requeueing without a gate.
- Mapped acceptance evidence is enforced; a valid waiver clears advancement;
  manual decisions bind to criterion text, manual-evidence text, and attempt.
- Check-log and outcome-artifact write failures are returned, not swallowed.
- Recovery helpers are wired on the production path; unchanged/discovery-only
  retries are refused, recovery history is persisted, and safety/fail-closed
  decisions run before a disabled escalation policy.
- Lifecycle counts are written as absolute totals without seeded double-counting;
  approvals count approved decisions; candidate provenance is recorded.

## Configuration compatibility

Canonical keys are `cargo.agent_tool` and `cargo.acceptance_check` (default
`false`, independent).

Legacy spellings `agent.tool_cargo` and
`validation.acceptance_checks.cargo` are accepted only when the canonical key is
omitted. Equal values are accepted; conflicting values fail configuration
loading with an explicit error naming both keys. Migration is documented in
`docs/reference/CONFIGURATION.md`.

## SOP-006 bounded workflow

Fixture: `.run/sop-006-rust-approval/` (local, not committed).

1. Controlled failing acceptance: S001 `PARTIAL`, PENDING approval recorded,
   `WAITING_FOR_HUMAN`; S002 did not run.
2. Second run without a decision: `S001 WAITING_FOR_APPROVAL`; counts stayed
   `invocations=1`, `model_calls=6`.
3. `sop approve S001 --by operator`: head became `APPROVED`.
4. Approved rerun: S001 and S002 `VERIFIED` / `LOCAL_DONE`, `SOP COMPLETE`,
   `Final gate: PASS`.

Final S001 counts: `approvals=1`, `invocations=2`, `attempts=2`,
`continuations=0`, `model_calls=9`. S002: `invocations=1`. No budget reset or
state edit was used.

## Agent-tool Cargo evidence

Backend: `harness: tool`, `provider: ollama`, model
`deepseek-v4.1-flash:cloud`, through the candidate binary.

| Case | Result |
|---|---|
| Enabled, `cargo test --quiet` | allow / ok |
| Disabled, `cargo test --quiet` | deny / REQUIRES_APPROVAL |
| Unsupported, `cargo publish` | deny / unsupported subcommand |
| Manifest escape | deny / path escapes repository |

Acceptance-check Cargo stayed `false` in all four cases. The Rust acceptance
workflow used `acceptance_check: true` with `agent_tool: false`, so the
permissions remained independent.

Local audit files: `.run/sop-cargo-tool/evidence/`.

## Validation

```bash
go build ./...
go test ./... -count=1
go vet ./...
git diff --check
```

All passed. Named-test gate: 51/51 named tests matched via `go test -list` and
51/51 emitted test-level pass events via `go test -json`; zero-match filters were
not counted as evidence.

## Limitations

- SOP-006 used the `command` backend, which does not pass through
  `internal/toolharness`; it does not prove agent-tool Cargo enforcement.
- Agent-tool Cargo was verified for the native Ollama tool harness only; no claim
  is made for other providers or backends.
- A DECLINED approval remains authoritative; this change adds no automatic
  re-request operation.
- SOP-006 is a bounded two-task Rust workflow, not full Rust-migration readiness.
