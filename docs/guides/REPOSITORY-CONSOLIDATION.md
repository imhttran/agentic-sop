# Repository consolidation

This guide records how the SOP controller and the decision adapters were
consolidated into the single `agentic-sop` module, and the compatibility limits that
follow from that.

## What moved where

| Source repository | Destination in `agentic-sop` |
| --- | --- |
| controller `internal/config`, `internal/sopclient` | `internal/controller/{config,sopclient}/` |
| controller `internal/web`, `assets.go` + `templates/` + `static/` | `internal/controller/web/`, `internal/controller/assets/` |
| controller startup wiring | `internal/controller/service.go` (`controller.Serve`) |
| controller browser (Playwright) suite | `tests/controller/` (development-only) |
| controller launch/HTTPS scripts, Caddyfile | `scripts/controller/`, `integrations/controller/` |
| adapter public `decision/` | `decision/` (module root) |
| adapter `internal/providers/{clef,nimble,julia,fallback}`, `internal/shadow` | `internal/decisionadapter/providers/…`, `internal/decisionadapter/shadow/` |
| adapter CLI | `internal/decisionadapter/cli/` (`cli.Run`) |
| adapter fixtures | `internal/tests/fixtures/` |
| Julia helper | `tools/julia/` (retained) |
| legacy executables | `cmd/sop-controller/`, `cmd/sop-decision-adapter/` (thin wrappers) |

Imports were copied byte-exact and then rewritten by the documented, idempotent
mapping in `docs/reports/consolidation/import-rewrites.md`. Core package locations,
the canonical decision seam and the profile bridge are unchanged.

## Unified commands

- `sop run` — the governed lifecycle.
- `sop controller serve` — the read-only dashboard.
- `sop decision decide|shadow` — the advisory decision adapter.

The legacy executables remain available and behave identically.

## Compatibility limits and optional requirements

- **Go module**: one module, `go 1.27.1`; the controller's `modernc.org/sqlite`
  dependency is unified.
- **Node / Playwright**: development-only for `tests/controller/`; never required to
  build or run SOP.
- **Python / Julia**: the Julia decision provider is an optional capability — the
  repository-owned helper `tools/julia/infer.py` plus external model weights. Without
  them, Julia decisions report unavailable.
- **Caddy / HTTPS**: optional; the proxy scripts and Caddyfile are namespaced under
  `scripts/controller/` and `integrations/controller/` and never overwrite a core
  Make target.
- **Advisory authority**: decisions can only add attention; they cannot lower risk,
  approve work or bypass verification.

## History

Full source histories are preserved outside tracked artifacts (bundles + provenance),
and the ancestry-preserving integration is prepared but **not committed** — its merge
commit needs separate authorization.
