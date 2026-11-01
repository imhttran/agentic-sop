# PLAN — Phase 5.6: Windows Distribution & Clean-Room Installation

**Type:** Implementation plan.

**Status:** Implemented except P56-011. P56-001–P56-010 and P56-012 shipped: the
Windows installer, its tests, the `windows-latest` CI job, the documentation, and the
script-organization cleanup. P56-011 is the clean-room
test on a real Windows machine, which has **not** been run — CI is not a clean-machine
test — so that stage carries no `execution_mode: done` and remains open. See
[../guides/WINDOWS-INSTALLATION.md](../guides/WINDOWS-INSTALLATION.md) and
[../testing/WINDOWS-CLEAN-ROOM.md](../testing/WINDOWS-CLEAN-ROOM.md).

## Objective

Make SOP installable on a clean Windows machine with native PowerShell, and with nothing
else: no WSL, no Git Bash or Cygwin, no Make, and no administrator rights. Windows gets
the same SOP CLI and the same canonical command surface as every other platform, and no
second execution engine: `install.ps1` builds the existing `cmd/sop`, and the commands
it installs call `sop prompt`.

## Design

```text
            Windows                                  macOS / Linux
               │                                          │
          install.ps1                                  install.sh
               │                                          │
       ┌───────┴────────┐                    ┌────────────┴────────────┐
       │                │                    │                         │
  go build →        copy skills/       go build →            scripts/install/install-skills.sh
  sop.exe           + .sop-managed      sop                   (symlinks)
       │                │                    │                         │
       └────────────────┴────────────────────┴─────────────────────────┘
                                       ↓
                              sop prompt → WorkItem → JEV → router
                              → model.Resolve → provider validation
                              → agent/harness → SOP governance
```

The installers are adapters over one CLI and one canonical skill tree
([`skills/`](../../skills)). They select nothing, resolve no provider, and name no model.

## Tasks

### P56-001 — PowerShell installer

- **Status:** Done.
- **Execution:** done.
- **Objective:** Add a native PowerShell installer with the same conceptual contract as
  `install.sh`.
- **Scope:** `install.ps1` with `-Skills <none|zed|claude|all>`, `-Plugin <none|claude>`,
  `-All`, `-BinDir`, `-DryRun`, `-UninstallSkills`, `-Force`, and `-Help`; comment-based
  help so `Get-Help` works; PowerShell 5.1 as the baseline (no PowerShell 7-only syntax);
  a repo sanity check before doing anything; a `param` block with `ValidateSet` so a bad
  value fails natively. `install.sh` and the Claude plugin package are unchanged.
- **Files:** `install.ps1`.
- **Depends on:** —
- **Acceptance:** `.\install.ps1 -Help` documents every parameter; an unknown parameter
  or value fails with a usage message; the script parses under Windows PowerShell 5.1 and
  PowerShell 7.

### P56-002 — Windows CLI installation

- **Status:** Done.
- **Execution:** done.
- **Objective:** Build and install `sop.exe` into a user-owned directory, atomically.
- **Scope:** `go build` against `cmd/sop`; the binary directory resolves as `-BinDir`,
  then `$env:SOP_BIN_DIR`, then `%USERPROFILE%\.local\bin`, with a leading `~` expanded
  and the path normalized; the build writes a uniquely named temporary file beside the
  destination and is moved into place, so a failed build never truncates a working
  `sop.exe` and a locked target fails with a clear message instead of corrupting the
  installation; no elevation, and nothing under `C:\Program Files` or `C:\Windows`.
- **Files:** `install.ps1`.
- **Depends on:** P56-001
- **Acceptance:** an install produces a `sop.exe` that runs `sop version`; installing
  twice succeeds; a bin directory containing spaces works.

### P56-003 — Windows PATH guidance

- **Status:** Done.
- **Execution:** done.
- **Objective:** Tell the operator how to put the bin directory on PATH without changing
  anything for them.
- **Scope:** the installer reports when the directory is already on PATH (comparing PATH
  entries case-insensitively, with separators normalized) and names a different `sop.exe`
  that resolves first; otherwise it prints the current-session line
  (`$env:Path = "<dir>;$env:Path"`) and the user-level mechanism
  (`[Environment]::SetEnvironmentVariable(..., 'User')`), with the caveat that the latter
  appends. It edits no profile, no registry key, and no user or system environment.
- **Files:** `install.ps1`.
- **Depends on:** P56-002
- **Acceptance:** the guidance appears only when the directory is not on PATH; the
  installer contains no `setx` and no environment write of its own.

### P56-004 — Windows skill installation

- **Status:** Done.
- **Execution:** done.
- **Objective:** Install the canonical skills on Windows without assuming symlink
  support.
- **Scope:** the skills are copied from [`skills/`](../../skills), which stays the single
  source of truth; the roots are the documented agent locations with `~` resolved to
  `%USERPROFILE%` — `%USERPROFILE%\.agents\skills` for Zed,
  `%USERPROFILE%\.claude\skills` for Claude Code; a leading `~` and path separators are
  handled through PowerShell's own path handling; `-Skills all` installs only an agent
  whose directory exists, and fails loudly when none does.
- **Files:** `install.ps1`, `skills/sop/SKILL.md` (the stale `make install-skills-*`
  instruction became `./install.sh --skills …`).
- **Depends on:** P56-001
- **Acceptance:** all seven commands are installed with their `SKILL.md`; the copies
  match canonical byte for byte; an agent that is absent is reported, not invented.

### P56-005 — SOP ownership, update and uninstall

- **Status:** Done.
- **Execution:** done.
- **Objective:** Make a copied installation identifiable, updatable, and removable
  without ever touching anything SOP does not own.
- **Scope:** each installed copy carries a `.sop-managed` marker recording
  `marker=agentic-sop`, a version, the skill name, and its canonical source — no policy
  of any kind; installed copies are compared to canonical by content, so a current copy
  is left alone and a changed or stale one is replaced (`-Force` refreshes a current
  one); a directory without the marker, and any link or junction, is reported and left
  untouched; a SOP-managed copy of a skill this version no longer ships is reported, and
  `-UninstallSkills` removes only marked directories.
- **Files:** `install.ps1`.
- **Depends on:** P56-004
- **Acceptance:** a foreign skill at a SOP name survives install, update, `-Force`, and
  uninstall; a tampered installed copy is restored from canonical; uninstall removes
  exactly the SOP-managed directories.

### P56-006 — Claude plugin integration

- **Status:** Done.
- **Execution:** done.
- **Objective:** Offer the Claude Code plugin package on Windows with the same semantics
  as `install.sh --plugin claude`.
- **Scope:** the package stays platform-neutral (`integrations/claude/` and
  `.claude-plugin/marketplace.json`, no Windows copy); the step verifies the package,
  runs `claude plugin validate --strict` when the `claude` CLI is available, reports a
  problem as a failure, and prints the official registration commands; a missing `claude`
  CLI skips validation and says so; Claude's configuration is never written and Claude is
  never installed.
- **Files:** `install.ps1`.
- **Depends on:** P56-001
- **Acceptance:** the printed steps are the documented ones; no Claude configuration file
  or directory is created.

### P56-007 — Windows installer tests

- **Status:** Done.
- **Execution:** done.
- **Objective:** Cover the Windows installer's behaviour automatically, against temporary
  directories.
- **Scope:** `internal/dist/install_windows_test.go` runs `install.ps1` under both
  Windows PowerShell and `pwsh` (when present) with a scratch `USERPROFILE` and a
  controlled PATH, covering: the language parser accepting the script, `-Help`, `-DryRun`
  writing nothing, a real install whose `sop.exe` runs, idempotency, a bin directory
  containing spaces, copied skills with markers, a foreign skill preserved at a SOP name,
  refresh from canonical plus `-Force`, uninstall safety, a missing Go toolchain, and the
  plugin step. The Unix installer's tests are excluded on Windows and the shared helpers
  moved to `helpers_test.go` so the package builds on both.
- **Files:** `internal/dist/install_windows_test.go`, `internal/dist/helpers_test.go`,
  `internal/dist/install_test.go` (non-Windows), `internal/dist/plugin_test.go`.
- **Depends on:** P56-002, P56-004, P56-005, P56-006
- **Acceptance:** no test touches a real profile, PATH, or agent configuration; the tests
  run on `windows-latest` and pass there.

### P56-008 — Windows GitHub Actions

- **Status:** Done.
- **Execution:** done.
- **Objective:** Validate Windows on every push, without replacing the Linux CI.
- **Scope:** a `windows` job alongside the existing jobs: `go build ./...`, `go vet ./...`,
  a PowerShell language-parser check on `install.ps1`, the distribution test packages, and
  a smoke run of the documented entry points (`-Help`, `-DryRun`, a real install into a
  path containing spaces, `sop.exe version`, and `-Skills claude -Plugin claude` against a
  scratch `USERPROFILE`, asserting seven skills and their markers). The full test suite
  stays on Linux because several lifecycle tests execute POSIX commands, and the job says
  so.
- **Files:** `.github/workflows/ci.yml`.
- **Depends on:** P56-007
- **Acceptance:** the Windows job is additive (the Linux jobs are unchanged) and needs no
  administrator rights on the runner.

### P56-009 — Cross-platform regression tests

- **Status:** Done.
- **Execution:** done.
- **Objective:** Keep the two installers from drifting apart, and keep SOP policy out of
  both.
- **Scope:** `internal/dist/contract_test.go` asserts that the canonical `skills/` tree
  and the command surface are the same set; that both installers offer the documented
  operations; that the PowerShell parameters (with their accepted values) are declared;
  that neither installer names a provider, a model, or a policy setting; that
  `install.ps1` contains no `Invoke-Expression`, `iex`, or `setx`; and a cheap structural
  proxy for the PowerShell syntax where PowerShell is unavailable.
- **Files:** `internal/dist/contract_test.go`.
- **Depends on:** P56-001, P56-004
- **Acceptance:** a skill added to the tree but not to the command surface (or the
  reverse) fails; a policy or provider statement in either installer fails.

### P56-010 — Documentation

- **Status:** Done.
- **Execution:** done.
- **Objective:** Make the platform split obvious and keep the detail out of the README.
- **Scope:** the README's quick start shows the two installers and nothing more;
  `docs/guides/INSTALLATION.md` gains a Windows section; a new
  `docs/guides/WINDOWS-INSTALLATION.md` documents the parameters, the binary directory and
  PATH behaviour, the copied skills and their marker, update and uninstall, the plugin,
  what the installer never does, and troubleshooting; `docs/testing/WINDOWS-CLEAN-ROOM.md`
  records the checklist and results; the documentation index and the backlog list the new
  pages. The shipped skill's own install instruction was corrected from the stale
  `make install-skills-*` to `./install.sh --skills …` (and the generated plugin mirror
  regenerated).
- **Files:** `README.md`, `docs/README.md`, `docs/guides/INSTALLATION.md`,
  `docs/guides/WINDOWS-INSTALLATION.md`, `docs/testing/WINDOWS-CLEAN-ROOM.md`,
  `docs/plans/BACKLOG.md`, `skills/sop/SKILL.md`, `integrations/claude/`.
- **Depends on:** P56-001, P56-004
- **Acceptance:** a Windows user can install, enable an agent, and uninstall from the
  documentation alone, without reading the source; every documentation link resolves.

### P56-011 — Clean-room Windows validation

- **Status:** Not done — the test has not been run. CI runs on `windows-latest` against
  temporary directories; that is not a clean-machine test.
- **Objective:** Install and use SOP on a real, clean Windows machine and record the
  results.
- **Scope:** follow [../testing/WINDOWS-CLEAN-ROOM.md](../testing/WINDOWS-CLEAN-ROOM.md)
  on a real machine: record the Windows, PowerShell, Go, Claude Code, and Zed versions;
  install the CLI and record the directory and PATH behavior; install and verify the Zed
  and Claude Code skills; install the plugin through Claude Code's own flow; run
  `/sop-plan` and `/sop-review`; run `/sop-implement` in a disposable repository; exercise
  update and uninstall; record every result and any issue in that document.
- **Files:** `docs/testing/WINDOWS-CLEAN-ROOM.md`.
- **Depends on:** P56-008
- **Acceptance:** the results log is filled in from an actual machine, with each step
  marked done or failing; nothing is inferred from CI.

### P56-012 — Script organization cleanup

- **Status:** Done.
- **Execution:** done.
- **Objective:** Organize the internal scripts by responsibility without changing any
  behavior.
- **Scope:** group the support tooling under `scripts/` — the skill and known-good-agent
  installers into `scripts/install/`, the Claude plugin generator and its README source
  into `scripts/packaging/`, the doc link check into `scripts/checks/`, and the
  command-agent adapters and the pin into `scripts/agents/`. The public entry points
  (`install.sh`, `install.ps1`) stay at the repository root, and `scripts/install.sh`
  stays at its historical path as a compatibility-only shim. Every reference is updated
  to the new canonical paths (the root installers, the Makefile, CI, the Go code and
  tests, and the documentation), and each moved POSIX script resolves the repository
  root two levels up.
- **Files:** `scripts/`, `install.sh`, `install.ps1`, `Makefile`, `.github/workflows/ci.yml`,
  `.gitignore`, `internal/`, `docs/`.
- **Depends on:** P56-001
- **Acceptance:** the public installer UX is unchanged; `make check`, the installer tests,
  the cross-platform contract tests, and the plugin-packaging check still pass on both
  platforms; no SOP policy or execution behavior changed.

## Definition of Done

- A native `install.ps1` exists; Windows needs no Bash, WSL, or Make.
- `sop.exe` installs into a user-owned directory, with no administrator rights.
- PATH guidance is correct, and PATH, the registry, and profiles are never changed for
  the operator.
- Windows paths with spaces work.
- The skills install without symlink privileges, from the canonical `skills/` tree.
- SOP-owned copies update safely; foreign skills are never overwritten or deleted.
- The plugin package stays platform-neutral and its installation still reaches
  `sop prompt`.
- Project isolation holds and prompt text remains data.
- Windows CI passes; the Linux CI is unchanged.
- The documentation explains both installers.

The phase is fully complete only after P56-011 is performed and recorded.

## Out of scope

A GUI installer, MSI, Chocolatey, Winget, Scoop, automatic Go/Claude/Zed installation,
WSL-specific installation, `irm | iex` installation, new providers, provider failover,
new routing, new JEV or recovery policy, MCP, SOP Hub, and marketplace publication.

## Validation

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
go test -race ./...
make check
bash scripts/checks/check-doc-links.sh
GOOS=windows go vet ./...          # the Windows build and test files type-check
```

On `windows-latest` (the `windows` job): `go build ./...`, `go vet ./...`, the PowerShell
parser check, `go test ./internal/dist/... ./internal/skill/...`, and the installer smoke
steps.

## See also

- [../guides/WINDOWS-INSTALLATION.md](../guides/WINDOWS-INSTALLATION.md)
- [../testing/WINDOWS-CLEAN-ROOM.md](../testing/WINDOWS-CLEAN-ROOM.md)
- [PLAN-Phase-5.5-Distribution.md](PLAN-Phase-5.5-Distribution.md)
- [../guides/CLAUDE-PLUGIN.md](../guides/CLAUDE-PLUGIN.md)
