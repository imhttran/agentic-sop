# SEC-H-005 — Required-mode and quality-gate verification

Status: executed verification. No configuration changed; blocking remains disabled.

## Required-mode projection (pre-provisioning)

This section records the result before the OSV database was provisioned. See
"Post-provisioning re-run" below for the current result.

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

Why `INDETERMINATE` rather than `PASS` at that time: dependency coverage was
incomplete because the OSV offline database was absent (SEC-H-004). SAST and
secrets coverage were complete. Required mode does not convert incomplete coverage
into a pass.

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

## Post-provisioning re-run

After the authorized OSV provisioning, the same required simulation was re-run:

- `sop security scan --required` -> exit status `0`, verdict `PASS`, coverage
  `COMPLETE`, 9131 ms.
- Adapters: gitleaks `COMPLETE`, semgrep `COMPLETE`, osv-scanner `COMPLETE`.
- `required_mode.ready: false`, projected verdict `INDETERMINATE`, reason:
  `required mode needs at least one required category`.
- The artifact records `policy.mode: required` (from the CLI flag) and an empty
  `required_categories`.

Precise cause: `config.RequiredGateActive()` is true only when the mode is
`required` **and** `required_categories` is non-empty. The repository config still
has `mode: report-only` and no required categories, so no active required gate
exists; the readiness projection therefore stays `false` even though every
category is now covered.

Not bypassed or changed: `.agent-sdlc/config.yaml` remains `report-only` with no
required categories. If an active required gate is desired, the minimal delta is a
policy decision: set `mode: required` and add `required_categories` (for example
`sast`, `secrets`, `dependencies`). That was intentionally not applied.
