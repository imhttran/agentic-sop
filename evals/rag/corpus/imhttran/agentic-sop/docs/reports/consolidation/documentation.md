# Documentation map (MIG-010)

Human-facing entry points after consolidation:

| Document | Purpose |
| --- | --- |
| `README.md` | install, `sop run`, and the unified `sop controller serve` / `sop decision` commands |
| `docs/README.md` | the documentation index |
| `docs/guides/GETTING-STARTED.md` | first-run setup |
| `docs/guides/PROJECT-SETUP.md` | per-project configuration |
| `docs/guides/SOP-CONTROLLER-DASHBOARD.md` | the controller dashboard |
| `docs/guides/REPOSITORY-CONSOLIDATION.md` | what moved where and the compatibility limits |
| `docs/reference/CLI.md`, `docs/reference/CONFIGURATION.md` | command and configuration reference |
| `docs/reports/consolidation/` | the migration evidence (baseline, source map, import rewrites, decision compatibility, CI coverage, completion) |

## Compatibility limits recorded for readers

- Unified CLI: `sop run`, `sop controller serve`, `sop decision decide|shadow`; the
  legacy `sop-controller` / `sop-decision-adapter` wrappers are optional.
- Optional tooling: Node/Playwright (controller browser tests, development-only);
  Python/Julia helper plus external weights (Julia decision provider); Caddy (HTTPS
  convenience). None is required to build or run SOP.
- Advisory decisions cannot lower risk, approve work or bypass verification.

## Traceability

The active plan identity is `plan-repository-consolidation`
(`docs/plans/PLAN-Repository-Consolidation.md`); archived evidence and prior run
artifacts under `.agent-sdlc/` are preserved and are **not** rewritten.

Internal link check: `bash scripts/checks/check-doc-links.sh` → broken links: 0.
