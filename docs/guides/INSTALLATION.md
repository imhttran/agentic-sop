# Installation

Agentic SOP installs with one script from a checkout. No root, no Make, and no edit to
your shell startup files:

```bash
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
./install.sh
sop version
```

`./install.sh` builds the `sop` CLI from the checkout and writes it to a user-writable
bin directory. The agent integrations are opt-in, so nothing is added to an agent you
do not use:

```bash
./install.sh --skills zed      # the /sop* commands in Zed
./install.sh --skills claude   # the same commands in Claude Code
./install.sh --plugin claude   # the Claude Code plugin package
./install.sh --all             # the CLI + every present agent's skills + the plugin
```

## Windows

Windows uses a native PowerShell installer — no WSL, Git Bash, Make, or administrator
rights, and no PowerShell profile is edited:

```powershell
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
.\install.ps1
sop version
```

It builds the same `sop` CLI as `sop.exe`, installs it under `<home>\.local\bin`
unless `-BinDir` (or `SOP_BIN_DIR`) says otherwise, and prints the PATH line to add
instead of changing PATH for you. The agent skills are **copied** rather than
symlinked, because symlinks on Windows need Developer Mode or elevation, and each
copied skill carries a `.sop-managed` marker so a later install can update it and an
uninstall can remove it without ever touching a skill SOP does not own.

```powershell
.\install.ps1 -Skills zed       # -Skills claude, -Skills all
.\install.ps1 -Plugin claude
.\install.ps1 -All              # the CLI + every present agent's skills + the plugin
.\install.ps1 -UninstallSkills all
.\install.ps1 -DryRun
```

Full detail, including the ownership marker and what the installer never does:
[WINDOWS-INSTALLATION.md](WINDOWS-INSTALLATION.md).

## Options

| Option                           | Meaning                                                              |
| -------------------------------- | -------------------------------------------------------------------- |
| `--skills <none\|zed\|claude\|all>` | Link the SOP commands for those agents (default: `none`).          |
| `--plugin <none\|claude>`        | Prepare the Claude Code plugin package (default: `none`).            |
| `--all`                          | `--skills all --plugin claude`.                                       |
| `--bin-dir <path>`               | Where to install the `sop` binary.                                    |
| `--dry-run`                      | Print what would happen and change nothing.                           |
| `-h`, `--help`                   | Show the usage.                                                       |

Every step is idempotent: running `./install.sh` twice is safe, and each run reports
what it did. A step that cannot do its work fails loudly and leaves the others' results
in place — the exit status is non-zero, and the CLI is still installed.

## Where the binary goes

The bin directory is the first of: `--bin-dir`, `$SOP_BIN_DIR`, `$GOBIN`, the Go
toolchain's own bin directory (`go env GOPATH`/bin, where `go install ./cmd/sop` would
put it), then `~/.local/bin`. Whichever it is, it is user-writable and needs no root.

If that directory is not on `PATH`, the installer says so and prints the line to add —
it never edits `~/.zshrc`, `~/.bashrc`, or `~/.profile` for you:

```text
SOP installed to:

  ~/.local/bin/sop

Add this directory to PATH:

  export PATH="$HOME/.local/bin:$PATH"
```

If a *different* `sop` is already on `PATH`, the installer names it, so a stale binary
is never silently preferred — use `--bin-dir` to install over it.

## Agent skills

`--skills` delegates to [`scripts/install/install-skills.sh`](../../scripts/install/install-skills.sh),
which owns skill installation for every supported agent. It links the canonical skill
folders from [`skills/`](../../skills) into the agent's skills root, so the checkout
stays the single source of truth and an edit to a `SKILL.md` takes effect without
reinstalling:

| Agent       | User scope                | Project scope                     |
| ----------- | ------------------------- | --------------------------------- |
| Zed         | `~/.agents/skills/`       | `<project>/.agents/skills/`        |
| Claude Code | `~/.claude/skills/`       | `<project>/.claude/skills/`        |

`--skills all` installs only the agents actually present on the machine, so it never
creates configuration for an agent you do not have; name an agent explicitly to install
it regardless. The guides are [ZED-SKILLS.md](ZED-SKILLS.md) and
[CLAUDE-SKILLS.md](CLAUDE-SKILLS.md).

Use the script directly for a project-local install, a dry run, or removal:

```bash
scripts/install/install-skills.sh zed --project          # ./.agents/skills
scripts/install/install-skills.sh claude --project ~/work/api
scripts/install/install-skills.sh all --dry-run
scripts/install/install-skills.sh zed --uninstall
```

## The Claude Code plugin

`--plugin claude` prepares the plugin package under
[`integrations/claude/`](../../integrations/claude) and prints the official steps to
register it. Claude Code installs plugins from inside Claude Code, so the installer does
not write into Claude's settings; see [CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md).

## Make is optional

`make` targets still work and delegate to the same implementation:

```bash
make install             # ./install.sh
make install-all         # ./install.sh --all
make install-skills      # the skill roots, without rebuilding the CLI
make install-plugin      # prepare the Claude plugin package
make check               # fmt, vet, test, build, plugin package current
make plugin              # regenerate the plugin package from skills/
```

`scripts/install.sh` remains a thin wrapper around `./install.sh`, so an older workflow
that calls it keeps working.

## Safety

- **No root.** Everything is per-user; the installer warns if it is run as root.
- **Only SOP-owned files.** It writes the `sop` binary and SOP-owned skill entries. It
  never overwrites or deletes an unrelated skill, plugin, or file, and it never edits a
  shell startup file.
- **Idempotent.** Repeated installs report what was already in place.
- **Failures are actionable and local.** A missing Go toolchain, a missing agent
  environment, or a missing plugin package is reported with what to do about it.
- **Paths are handled as data.** A bin directory containing spaces works; no user input
  is passed through `eval` or string-interpolated into a command.
- **Project isolation is untouched.** Installing global skills makes the commands
  available in every project, but each invocation runs `sop` in the project you are in,
  so `.agent-sdlc/` state stays per-project.

## Uninstall

```bash
scripts/install/install-skills.sh all --uninstall   # remove the SOP skill links
claude plugin uninstall sop                 # remove the plugin (from Claude Code)
rm "$(command -v sop)"                      # remove the CLI
```

Uninstall removes only entries SOP owns: a symlink pointing at this checkout's `skills/`
tree, or the plugin SOP packaged.

## Verifying an install

```bash
sop version
./install.sh --dry-run          # show, without changing, what a run would do
scripts/packaging/build-claude-plugin.sh --check   # the plugin package is current
```

The installer and the plugin package are covered by
[`internal/dist`](../../internal/dist) (temporary HOME and bin directories, idempotency,
unrelated-file preservation, dry runs, missing prerequisites) and by
[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml).

## Troubleshooting

| Symptom                                    | Cause and fix                                                                       |
| ------------------------------------------ | ----------------------------------------------------------------------------------- |
| `the Go toolchain ('go') is required`      | Install Go, then re-run `./install.sh`.                                             |
| `command not found: sop`                   | The bin directory is not on `PATH`; the installer printed the line to add.           |
| A `sop` that is not the one just installed | Another `sop` earlier on `PATH`; re-run with `--bin-dir <that directory>`.           |
| `no supported agent environment found`     | `--skills all` found no agent directory; name one, or install that agent.            |
| A command is missing from the `/` menu     | The skills are not linked: `./install.sh --skills zed` (or `claude`).                |
| The plugin is not in Claude Code's `/` menu | Register it as in [CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md).                            |

## See also

- [CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md) — the Claude Code plugin package.
- [ZED-SKILLS.md](ZED-SKILLS.md) and [CLAUDE-SKILLS.md](CLAUDE-SKILLS.md) — the agent
  command surfaces.
- [GETTING-STARTED.md](GETTING-STARTED.md) — running SOP against a project.
- [../reference/CLI.md](../reference/CLI.md) — the command reference.
