# Controller provenance record

This file records the **origin of the controller source** consolidated into this repository.
It is a **provenance and attribution record only**: it does **not** assign a license, does
**not** grant permission, and does **not** establish that redistribution is authorized.

## Source

| Field                 | Value                                                                                                               |
| --------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Source repository     | `imhttran/sop-controller` (`https://github.com/imhttran/sop-controller.git`)                                        |
| Imported tip          | `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` (`main`; no local-only commits)                                          |
| Observed working tree | clean (no staged, unstaged or untracked changes)                                                                    |
| Committers            | `Tom Tran <imhttran@gmail.com>`, `Tom Tran <trantom512@gmail.com>`                                                  |
| Source LICENSE        | **none found** — no `LICENSE`/`COPYING`, no per-file header, no README license statement, no `go.mod` license field |

## Imported paths and documented modifications

Everything below is a byte-exact copy of the tip **except** the listed deviations (which are
also recorded in `import-rewrites.md`).

| Source path (at `f8502a5`)                      | Destination                                                                   | Modification                                                                                                                                                                                                                        |
| ----------------------------------------------- | ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/config/` (4 files)                    | `internal/controller/config/`                                                 | one file, `config.go`: adds `ResolveSOPBin()` (SOP_BIN wins, else the running executable) per MIG-005; plus the import-path rewrite `sop-controller/internal/config` → `github.com/imhttran/agentic-sop/internal/controller/config` |
| `internal/sopclient/` (37 files)                | `internal/controller/sopclient/`                                              | one file, `start.go`: `StartOrContinue` dispatches `sop run` (not the read-only `sop resume`) per MIG-011; plus the `internal/config` import rewrite                                                                                |
| `internal/web/` (32 files)                      | `internal/controller/web/`                                                    | one file, `dogfood_test.go`: expectations updated to the `run` argv; plus the config/sopclient/assets import rewrites                                                                                                               |
| `assets.go`, `templates/`, `static/` (34 files) | `internal/controller/assets/` (`assets.go` beside `templates/` and `static/`) | none (byte-exact)                                                                                                                                                                                                                   |
| `cmd/sop-controller/main.go`                    | `cmd/sop-controller/main.go`                                                  | rewritten as a thin wrapper over the shared `internal/controller.Serve` (legacy executable compatibility)                                                                                                                           |
| `scripts/` (3 files)                            | `scripts/controller/`                                                         | none (byte-exact)                                                                                                                                                                                                                   |
| `caddy/Caddyfile`                               | `integrations/controller/Caddyfile`                                           | none (byte-exact)                                                                                                                                                                                                                   |
| `tests/` (browser e2e)                          | `tests/controller/`                                                           | renamed/relocated only (`e2e/`, `package.json`, `playwright.config.ts`)                                                                                                                                                             |

## Licensing status — RESOLVED (operator-confirmed)

**The operator has confirmed (2026-10-14)** that they own the copyright in
`imhttran/sop-controller` and **authorized licensing the imported controller source under
Apache-2.0**, the same license as this repository. The imported controller code is therefore
covered by this repository's existing `LICENSE` (Apache-2.0). No separate license file is
required from the source, and none was present.

**Applicable notices:** neither `sop-controller` nor `sop-decision-adapters` contains a
`NOTICE`/`COPYING` file, and no imported file carried a per-file license header, so there is
**no applicable notice to preserve** — none was dropped by the import. (Had a source `NOTICE`
existed, Apache-2.0 §4(d) would require retaining it; there is none.) Adapter attribution
remains Apache-2.0 and is unaffected.

This record does not itself grant rights beyond the operator's confirmation above; it documents
the provenance and the confirmed licensing decision.

## Ownership confirmation (received)

The operator confirmed the following on 2026-10-14:

1. They **own the copyright** in `imhttran/sop-controller`.
2. The imported controller source is **licensed under Apache-2.0** (this repository's license).
3. A provenance record (this file) is sufficient; no additional license text or `NOTICE` is
   provided because the source carried none.

This is consistent with the observed evidence (all three repositories share the GitHub owner
`imhttran` and the author `Tom Tran <imhttran@gmail.com>` / `<trantom512@gmail.com>`).
