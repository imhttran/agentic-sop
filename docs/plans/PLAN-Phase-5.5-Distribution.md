# PLAN — Phase 5.5: Distribution, Unified Installer & Plugin Packaging

**Type:** Implementation plan (normative for the work it describes; the specifications
win wherever they disagree).

**Status:** Implemented and committed. Phase 5.5 is distribution and packaging work: it
adds no routing, provider, recovery, or lifecycle behaviour, and every integration it
adds delegates to `sop prompt`. Its work items are tracked as `P55-NNN`, the same
convention Phase 3.5 used for `P35-NNN`. Each stage declares `execution_mode: done`
because the work exists in the tree: running `sop run docs/plans/PLAN-Phase-5.5-Distribution.md`
records the plan as complete without invoking an agent. See
[../guides/INSTALLATION.md](../guides/INSTALLATION.md) and
[../guides/CLAUDE-PLUGIN.md](../guides/CLAUDE-PLUGIN.md).

## Objective

Make SOP installable by a developer without Make, and package the SOP command surface
for Claude Code as a native plugin — without creating a second execution path or
duplicating SOP policy. The canonical execution boundary stays `sop prompt`.

## Design

```text
                    install.sh                    (the one installer)
                         │
        ┌────────────────┼────────────────────┐
        │                │                    │
  build the CLI   scripts/install-skills.sh   integrations/claude/
  (user bin dir)   ├── zed                    (plugin; generated mirror of skills/)
                   ├── claude
                   └── all                            │
        └────────────────┴────────────────────────────┘
                                 ↓
                       sop prompt → WorkItem → JEV → router → model.Resolve
                                 → provider validation → agent/harness → governance
```

The plugin and the skills are adapters. They select a capability and call `sop prompt`;
they carry no policy, name no model, and never touch a provider.

## Tasks

### P55-001 — Unified root installer

- **Status:** Done.
- **Execution:** done.
- **Scope:** `install.sh` becomes the documented installation entry point. It parses
  `--skills <none|zed|claude|all>`, `--plugin <none|claude>`, `--all`, `--bin-dir`,
  `--dry-run`, and `--help`; resolves the bin directory as `--bin-dir`, `$SOP_BIN_DIR`,
  `$GOBIN`, `go env GOPATH`/bin, then `~/.local/bin`; builds the CLI with `go build`
  into a temporary file beside the destination and renames it into place, so a running
  `sop` keeps its inode and a failed build never truncates a working binary; and prints
  the exact `export PATH=...` line when the bin directory is not on `PATH`.
- **Files:** `install.sh`, `scripts/install.sh` (thin wrapper), `Makefile`.
- **Depends on:** —
- **Acceptance:** `./install.sh` produces a runnable `sop`; Make delegates to it;
  `scripts/install.sh` still works; a bin directory containing spaces works.

### P55-002 — Installer safety and idempotency

- **Status:** Done.
- **Execution:** done.
- **Scope:** no root (warn if run as root); no shell startup file is ever edited (the
  PATH line is printed, not appended); only SOP-owned files are written; every step is
  idempotent; each step reports its own result and a failing step leaves the others'
  results in place and makes the exit status non-zero; user input is never `eval`ed.
- **Files:** `install.sh`.
- **Depends on:** P55-001
- **Acceptance:** installing twice succeeds; a `--dry-run` writes nothing; a missing Go
  toolchain fails with an actionable message; a foreign skill at a SOP name survives.

### P55-003 — Skill installer delegation

- **Status:** Done.
- **Execution:** done.
- **Scope:** the CLI build is the only thing the root installer implements itself. The
  agent skills are installed by `scripts/install-skills.sh`, which owns that logic and
  keeps its existing interface (`zed|claude|all`, `--project`, `--uninstall`,
  `--dry-run`, `--force`), so there is one implementation of skill installation.
- **Files:** `install.sh`, `scripts/install-skills.sh`.
- **Depends on:** P55-001
- **Acceptance:** `--skills zed|claude|all` land in the right agent roots and nowhere
  else; a `--skills all` with no agent present reports it and still installs the CLI.

### P55-004 — Remove stale installation assumptions

- **Status:** Done.
- **Execution:** done.
- **Scope:** the documentation and the install scripts describe the current
  architecture: the seven-command surface (`/sop`, `/sop-prompt`, `/sop-plan`,
  `/sop-review`, `/sop-diagnose`, `/sop-test`, `/sop-implement`) installed by one
  installer, and no reference to a separately installed global end-to-end skill (the
  repository does not ship one). One CLI installer, not two.
- **Files:** `README.md`, `docs/guides/GETTING-STARTED.md`, `docs/guides/DEVELOPMENT.md`,
  `scripts/install.sh`, `Makefile`.
- **Depends on:** P55-001
- **Acceptance:** no document tells a user to install SOP in a way that no longer
  applies; `scripts/check-doc-links.sh` reports no broken link.

### P55-005 — Claude plugin packaging

- **Status:** Done.
- **Execution:** done.
- **Scope:** a native Claude Code plugin using the official format: the plugin manifest
  at `integrations/claude/.claude-plugin/plugin.json` (plugin name `sop`), its skill
  files under `integrations/claude/skills/`, and the repository as a marketplace
  (`.claude-plugin/marketplace.json`) listing the plugin by a relative path. The plugin's
  skill files are a **generated verbatim mirror** of `skills/`, produced by
  `scripts/build-claude-plugin.sh`, so `skills/` stays the single source of truth; the
  generator also supports `--check` for drift.
- **Files:** `integrations/claude/`, `.claude-plugin/marketplace.json`,
  `scripts/build-claude-plugin.sh`, `scripts/claude-plugin-README.md`.
- **Depends on:** —
- **Acceptance:** `claude plugin validate --strict integrations/claude` and
  `claude plugin validate .` both pass; the mirror is byte-identical to `skills/`.

### P55-006 — Plugin-to-SOP command mapping

- **Status:** Done.
- **Execution:** done.
- **Scope:** the plugin exposes the seven commands, each fixing one capability and
  delegating to `sop prompt`: `plan`→`/sop:sop-plan`, `review`→`/sop:sop-review`,
  `diagnose_failure`→`/sop:sop-diagnose`, `design_tests`→`/sop:sop-test`,
  `implement`→`/sop:sop-implement`, and the capability-free `/sop:sop` and
  `/sop:sop-prompt`. Claude Code namespaces plugin components under the plugin name, so
  these are `/sop:<name>`; the same names also work unprefixed, which is Claude Code's
  own behaviour for plugin skills. `/sop:sop-implement` keeps
  `disable-model-invocation: true`, so a repository change is always an explicit operator
  choice.
- **Files:** `integrations/claude/skills/` (generated), `docs/guides/CLAUDE-PLUGIN.md`.
- **Depends on:** P55-005
- **Acceptance:** every packaged command contains the `sop prompt` call for its
  capability; `implement` is the capability of exactly one command; no packaged file
  names a provider, a model, or a policy setting.

### P55-007 — Plugin validation tests

- **Status:** Done.
- **Execution:** done.
- **Scope:** `internal/dist` validates the packaged plugin as an artifact: the manifest
  exists and parses with the expected metadata, the marketplace lists it under a name
  that matches the manifest and a source that resolves to the plugin, the skill mirror is
  current and exposes exactly the canonical command surface, each command fixes its own
  capability, `sop-implement` is hidden from the autonomous catalog and states the
  governed lifecycle, and no packaged file invokes a provider, restates a policy setting,
  names a model, or contains a direct-mutation instruction.
- **Files:** `internal/dist/plugin_test.go`, `internal/skill/surface.go` (the shared
  command-surface and content invariants).
- **Depends on:** P55-005, P55-006
- **Acceptance:** the tests fail if the plugin drifts from `skills/`, if a capability
  mapping changes, or if policy or provider access appears in the package.

### P55-008 — Installer tests

- **Status:** Done.
- **Execution:** done.
- **Scope:** `internal/dist` runs the root installer against temporary HOME, bin, and
  project directories: `--help`, unknown arguments, `--dry-run` writing nothing, a real
  CLI install that runs and is idempotent, PATH guidance appearing only when needed, a
  bin directory containing spaces, delegation of `--skills zed|claude|all`, a
  `--skills all` with no agent (loud failure, CLI still installed), preservation of
  unrelated skills, a missing Go toolchain, and the plugin step never writing into
  Claude's configuration.
- **Files:** `internal/dist/install_test.go`.
- **Depends on:** P55-001, P55-003
- **Acceptance:** no test touches the developer's real `~/.agents`, `~/.claude`, or
  installed `sop`.

### P55-009 — GitHub CI

- **Status:** Done.
- **Execution:** done.
- **Scope:** `.github/workflows/ci.yml` runs gofmt checking, `go vet`, `go build`,
  `go test ./...`, `go test -race ./...`, the documentation link check, the plugin
  drift check, installer smoke steps against an isolated HOME, and the installer/plugin
  test packages.
- **Files:** `.github/workflows/ci.yml`.
- **Depends on:** P55-005, P55-007, P55-008
- **Acceptance:** every step passes on a clean checkout; no step modifies an agent
  configuration on the runner.

### P55-010 — Documentation

- **Status:** Done.
- **Execution:** done.
- **Scope:** `docs/guides/INSTALLATION.md` documents the installer, its options, the bin
  directory and PATH behaviour, the delegated skill installation, safety, and removal;
  `docs/guides/CLAUDE-PLUGIN.md` documents the plugin, the official local-install steps,
  the command mapping, generation from `skills/`, and its safety properties. The README
  keeps the simple path (`./install.sh`) and links out.
- **Files:** `README.md`, `docs/README.md`, `docs/guides/INSTALLATION.md`,
  `docs/guides/CLAUDE-PLUGIN.md`, `docs/guides/GETTING-STARTED.md`,
  `docs/guides/ZED-SKILLS.md`, `docs/guides/CLAUDE-SKILLS.md`,
  `docs/reference/CLI.md`, `docs/guides/DEVELOPMENT.md`.
- **Depends on:** P55-004, P55-005
- **Acceptance:** a new user can install, enable an agent integration, and remove it
  from the documentation alone, without Make and without reading the source.

### P55-011 — End-to-end distribution smoke test

- **Status:** Done.
- **Execution:** done.
- **Scope:** exercised end to end against temporary directories: `./install.sh` installing
  a runnable CLI; `./install.sh --skills zed|claude` linking the seven commands into an
  isolated HOME; and the plugin installed through Claude Code's own documented flow
  (`claude plugin marketplace add` → `claude plugin install sop@agentic-sop` →
  `claude plugin details sop`) with `CLAUDE_CONFIG_DIR` pointed at a temporary directory,
  proving the installed package exposes seven skills whose bodies call `sop prompt`.
- **Files:** `internal/dist/` (automated), plus the manual steps recorded in
  [../guides/CLAUDE-PLUGIN.md](../guides/CLAUDE-PLUGIN.md).
- **Depends on:** P55-001, P55-005, P55-006
- **Acceptance:** each integration reaches `sop prompt`; installing the plugin leaves the
  developer's real Claude configuration unchanged.

## Definition of Done

- `./install.sh` is the primary installation path and Make is optional.
- CLI installation is idempotent, user-scoped, and needs no root.
- Zed and Claude Code skill installation still work, through one delegated implementation.
- A valid native Claude Code plugin package exists, with a marketplace that lists it.
- Every plugin command delegates to `sop prompt`; none carries routing, provider,
  approval, or lifecycle policy, and none names a model.
- `/sop-implement` remains explicitly mutating, governed, and hidden from the autonomous
  catalog.
- A missing `sop` CLI fails closed in every adapter.
- Unrelated user files, shell configuration, and project isolation are preserved.
- Prompt arguments remain data.
- CI validates the repository, the installer, and the plugin package.
- Phase 5.4 behaviour is unchanged.

## Out of scope

New routing algorithms, learned routing, provider failover, new model classes, new JEV or
recovery policy, distributed or remote execution, SOP Hub, cloud accounts, telemetry,
marketplace publication to Anthropic's directory, a Zed extension, a VS Code extension,
MCP orchestration, and `curl | sh` installation.

## Validation

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
go test -race ./...
make check
bash scripts/check-doc-links.sh
./install.sh --help
./install.sh --dry-run
```

Installer and plugin tests use temporary HOME, bin, and project directories; the plugin
smoke test uses a temporary `CLAUDE_CONFIG_DIR`.

## See also

- [../guides/INSTALLATION.md](../guides/INSTALLATION.md)
- [../guides/CLAUDE-PLUGIN.md](../guides/CLAUDE-PLUGIN.md)
- [../guides/ZED-SKILLS.md](../guides/ZED-SKILLS.md) ·
  [../guides/CLAUDE-SKILLS.md](../guides/CLAUDE-SKILLS.md)
- [../specs/EXECUTION.md](../specs/EXECUTION.md) ·
  [../specs/PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md)
- [PLAN-Phase-5.4-Unified-Work-Items.md](PLAN-Phase-5.4-Unified-Work-Items.md)
