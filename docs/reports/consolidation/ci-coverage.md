# CI coverage and suite relocation (MIG-009)

Status: CI configuration retained and extended; relocated browser suite is
development-only. No live-service manipulation is used anywhere.

## Suite destinations

| Original suite | Destination | Notes |
| --- | --- | --- |
| agentic-sop core Go suites | unchanged (`./...`) | `go build`/`test`/`vet`, tagged JEV evidence, doc links, packaging, Windows installer/skills smoke |
| sop-controller Go suites | `internal/controller/{config,sopclient,web}/` | byte-exact imports + documented rewrites |
| sop-controller browser (Playwright) suite | `tests/controller/` | **development-only**; Node/WebKit + disposable controller; launcher paths updated (`playwright.config.ts` testDir/globalSetup/teardown → `./e2e`; `state.ts` `REPO_ROOT` depth; `fixtures.ts` fixture paths) |
| sop-decision-adapters Go suites | `internal/decisionadapter/...` | byte-exact imports + documented rewrites |
| adapter CLI suite | `internal/decisionadapter/cli/` | relocated `clef_selection_test.go` (package `cli`, unified imports) |
| adapter fixtures | `internal/tests/fixtures/` | placed so the unmodified nimble golden path resolves |

Frozen historical evidence (archives, prior run artifacts) is **not** rewritten;
fixture references survive relocation.

## Check matrix

| Platform | Build | Test | Installer | Notes |
| --- | --- | --- | --- | --- |
| Linux | `check` job | `check` + `race` + `acceptance-evidence` | `install.sh` (POSIX sh) | primary matrix |
| macOS | shared POSIX-sh path | same Go module | `install.sh` (POSIX sh) | **not** a separate CI job; POSIX-sh path shared with Linux; keychain/HTTPS conveniences are operator-local |
| Windows | `windows` job | `install.ps1` smoke | `install.ps1` (PowerShell + pwsh) | native installer smoke |
| Browser (Playwright) | — | `tests/controller/` | — | **development-only**, not run in Go CI (Node required); unavailable checks are NOT labeled passing |

## Failure injection

Failure paths use **test servers and dependency injection** (httptest providers,
scripted agents, disposable SOP projects, ephemeral listeners/session stores). No
live oMLX, Nimble, controller or Caddy instance is stopped, restarted or
manipulated.

## Unavailable / not run

- oMLX and Bespoke local live contracts: **BLOCKED** (not probed).
- macOS CI job: **NOT RUN** (relies on the shared POSIX-sh path; not a separate job).
- Browser suite: **NOT RUN in CI** (development-only).
