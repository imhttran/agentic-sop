# SEC-H-005 — Required-mode and quality-gate verification

Status: executed verification. No configuration changed; blocking remains disabled.

## Required-mode projection

Command (executed in this worktree):

```sh
sop security scan --required
```

Result:

- Verdict `INDETERMINATE`; coverage `UNKNOWN`; exit status `1` (fail-closed).
- Duration `7667 ms`.
- Adapters: gitleaks `COMPLETED` / `COMPLETE`; semgrep `COMPLETED` / `COMPLETE`;
  osv-scanner `FAILED` / `UNKNOWN`.
- Reasons: `adapter osv-scanner is FAILED`, `adapter osv-scanner coverage is
  UNKNOWN`.
- Required-mode readiness projection: `false` (projected `INDETERMINATE`).
- Artifact: `.agent-sdlc/runs/security-20261108T172543Z/security/scan.{md,json}`.

Why `INDETERMINATE` rather than `PASS`: dependency coverage is incomplete
because the OSV offline database is absent (SEC-H-004). SAST and secrets coverage
are complete. Required mode does not convert incomplete coverage into a pass.

## Quality-gate mapping

The single quality gate remains the only verdict authority. Existing tests prove
the mapping (executed):

```sh
go test ./internal/quality/ -run 'Securit' -v -count=1
```

| Test | Result |
| --- | --- |
| `TestSecurityNilEvidenceDoesNotChangeGate` | PASS |
| `TestSecurityReportOnlyNeverBlocks` | PASS |
| `TestSecurityRequiredBlocksFailAndIndeterminate` | PASS |
| `TestSecurityRequiredPassesOnlyOnPass` | PASS |
| `TestSecurityRequiredUnconfiguredFailsClosed` | PASS |
| `TestSecurityRequiredRespectsFixBudget` | PASS |
| `TestHumanApprovalStillOutranksSecurityFailure` | PASS |

`ok github.com/imhttran/agentic-sop/internal/quality`.

## Configuration state (unchanged)

- `.agent-sdlc/config.yaml` still has `mode: report-only` and no
  `required_categories`.
- No severity, coverage, or scope setting was changed in this stage.
- Because required coverage is incomplete (dependencies), required/blocking mode
  is intentionally not enabled.
