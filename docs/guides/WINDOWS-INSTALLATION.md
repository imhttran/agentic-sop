# Installing SOP on Windows

Windows uses a native PowerShell installer. A clean Windows machine needs only
PowerShell (5.1, which ships with Windows, or PowerShell 7) and the Go toolchain — no
WSL, no Git Bash, no Cygwin, no Make, and no administrator rights. No PowerShell profile
is edited and PATH is never changed for you.

```powershell
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
.\install.ps1
sop version
```

`install.ps1` is the Windows counterpart of [`install.sh`](../../install.sh): it builds
the same CLI and installs the same canonical skill surface, so SOP's behavior is
identical on both platforms. There is no Windows-specific execution engine — the
commands it exposes all end up in `sop prompt`, and SOP keeps owning routing, provider
selection, validation, the lifecycle, and approval.

## Parameters

| Parameter                            | Meaning                                                                 |
| ------------------------------------ | ----------------------------------------------------------------------- |
| `-Skills <none\|zed\|claude\|all>`    | Install the SOP skills for those agents (default: `none`).               |
| `-Plugin <none\|claude>`             | Prepare the Claude Code plugin package (default: `none`).                |
| `-All`                               | Same as `-Skills all -Plugin claude`.                                    |
| `-BinDir <path>`                     | Where to install `sop.exe`.                                              |
| `-UninstallSkills <none\|zed\|claude\|all>` | Remove the SOP-managed skills for those agents.                   |
| `-Force`                             | Refresh SOP-owned skill copies even when they are already current.        |
| `-DryRun`                            | Print what would happen and change nothing.                              |
| `-Help`                              | Show the usage.                                                          |

`Get-Help .\install.ps1 -Full` shows the same information from the script's own help.

Every step is idempotent: installing twice is safe, and each run reports what it did. A
step that cannot do its work fails loudly: the exit status is non-zero, and the other
steps' results are left in place (the CLI is still installed).

## Where `sop.exe` goes

The bin directory is the first of:

```text
-BinDir
    ↓
$env:SOP_BIN_DIR
    ↓
<home>\.local\bin        (home is %USERPROFILE%)
```

It is a user-owned directory: nothing is installed into `C:\Program Files` or
`C:\Windows`, and no elevation is requested. A leading `~` in `-BinDir` is expanded, and
the path is normalized, so `-BinDir ~\bin` and `-BinDir "C:\Users\Tom Tran\bin"` both
work.

If your Go bin directory is already on PATH and you prefer to keep one location, pass it
explicitly: `.\install.ps1 -BinDir "$env:USERPROFILE\go\bin"`.

## PATH

If the bin directory is already on PATH, the installer says so (and names a different
`sop` that resolves first, if there is one). Otherwise it prints the line to run for the
current session and the user-level mechanism to keep it:

```powershell
$env:Path = "<bin dir>;$env:Path"
```

```powershell
[Environment]::SetEnvironmentVariable('Path', "<bin dir>;" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')
```

The installer **prints** these; it never runs them. It does not change PATH, the user or
system environment, or your PowerShell profile. Adding the directory is your decision —
run the session line, use the user-level setting (or the Environment Variables dialog),
then open a new terminal.

## The agent skills are copied, not symlinked

Symlinks on Windows need Developer Mode or elevation, so `install.ps1` copies the
canonical skill directories instead. The source of truth is unchanged:
[`skills/`](../../skills) is still the only place a SOP command is defined.

| Agent       | Root on Windows                  | What the agent documents     |
| ----------- | -------------------------------- | ---------------------------- |
| Zed         | `%USERPROFILE%\.agents\skills`   | `~/.agents/skills/` (global) |
| Claude Code | `%USERPROFILE%\.claude\skills`   | `~/.claude/skills/` (personal) |

The paths are the documented roots with `~` resolved to `%USERPROFILE%`, which is what
Zed and Claude Code mean by `~` on Windows.

`-Skills all` installs only an agent whose own directory exists (`.agents`, `.claude`),
so it never creates configuration for an agent the machine does not have; name an agent
explicitly to install it regardless.

### The ownership marker

Each installed copy carries a marker file recording that `agentic-sop` manages it:

```text
marker=agentic-sop
version=1
skill=sop-review
source=skills/sop-review
```

The marker, not the directory name, is what makes an installed skill SOP's. It contains
no routing, provider, or model policy — it only says who installed the copy and from
where.

### Updating, and never overwriting a foreign skill

- **SOP-owned and current** → reported as current; nothing is rewritten.
- **SOP-owned and out of date** (the canonical skill changed, or you edited the copy) →
  the copy is replaced from canonical, and reported as updated. `-Force` refreshes a
  current copy anyway.
- **Not SOP-owned** (no marker), or **a link or junction** → reported and left exactly
  as it is:

  ```text
  Skipping sop-review: an existing non-SOP skill uses this name.
  ```

A SOP-managed copy of a skill this version no longer ships is reported, never removed
silently.

### Uninstalling

```powershell
.\install.ps1 -UninstallSkills zed      # or claude, or all
```

Only directories that carry the marker are removed. A skill that merely *has* a SOP-like
name is reported and left where it is, so a name is never enough to delete something.
`-UninstallSkills` does not rebuild the CLI; it only removes what SOP installed.

## The Claude Code plugin

`-Plugin claude` uses the same platform-neutral package as every other platform
([`integrations/claude/`](../../integrations/claude) and
[`.claude-plugin/marketplace.json`](../../.claude-plugin/marketplace.json)) — there is no
Windows copy of it. The step verifies the package, runs
`claude plugin validate --strict` when the `claude` CLI is on PATH, and prints the
official commands to register it:

```text
claude --plugin-dir <checkout>\integrations\claude
claude plugin marketplace add <checkout>
claude plugin install sop@agentic-sop
```

It never writes into Claude's configuration, and a missing `claude` CLI only means the
validation is skipped — the steps are printed either way. See
[CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md) for the command surface and how it maps to
`sop prompt`.

## What the installer never does

- It never requests elevation, and never writes outside the bin directory and the agent
  skill roots.
- It never edits PATH, the registry, the system environment, or a PowerShell profile.
- It never overwrites or deletes a skill, file, or plugin it does not own.
- It installs no provider, no model, and no SOP policy: it names none of them.
- It does not install Go, Claude Code, or Zed — it tells you what is missing.
- It never interprets text as code: no `Invoke-Expression`, and no command string is
  built from an argument.

## Project isolation and prompt data

Installing SOP globally does not merge project state. Each run of `sop` uses the project
it is invoked in, so `C:\Projects\project-a` and `C:\Projects\project-b` keep their own
`.agent-sdlc\` state, and the installer never targets a project.

Prompt text stays data: the agent commands pass what you type to `sop prompt` as one
argument, and nothing in the PowerShell layer reinterprets it.

## Verifying

```powershell
sop version
.\install.ps1 -DryRun                                   # show, without changing, a run
.\install.ps1 -UninstallSkills all -DryRun               # show what a removal would remove
Get-ChildItem "$env:USERPROFILE\.claude\skills" -Directory | Select-Object Name
```

The installer is covered by [`internal/dist`](../../internal/dist): the Windows tests
(`install_windows_test.go`) run on `windows-latest` under both Windows PowerShell and
`pwsh`, using temporary homes and bin directories — help, dry run, a real install that
runs, idempotency, a bin directory with spaces, copied skills with the marker, foreign
skills preserved, update and `-Force`, uninstall safety, a missing Go toolchain, and the
plugin step writing nothing into Claude's configuration. A missing PowerShell version or
platform is reported, never guessed at.

For the on-machine checklist and its results log, see
[../testing/WINDOWS-CLEAN-ROOM.md](../testing/WINDOWS-CLEAN-ROOM.md).

## Troubleshooting

| Symptom                                          | Cause and fix                                                                         |
| ------------------------------------------------ | ------------------------------------------------------------------------------------- |
| `the Go toolchain ('go') is required`            | Install Go, then re-run `.\install.ps1`.                                               |
| `could not replace ...\sop.exe`                  | A running `sop.exe` holds the file; close it and re-run.                               |
| `sop` is not recognized                          | The bin directory is not on PATH; the installer printed the line to add.                |
| A different `sop` runs                           | Another `sop` earlier on PATH; the installer named it — pass `-BinDir` for that folder. |
| `an existing non-SOP skill uses this name`       | A skill of that name is not SOP's; move it aside yourself, then re-run.                 |
| `no supported agent environment found`           | `-Skills all` found no agent directory; name one, or install that agent.                |
| A command is missing from the `/` menu           | The skills are not installed: `.\install.ps1 -Skills zed` (or `claude`).                |

## See also

- [INSTALLATION.md](INSTALLATION.md) — the macOS/Linux installer, and the shared details.
- [CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md) — the plugin package.
- [ZED-SKILLS.md](ZED-SKILLS.md) and [CLAUDE-SKILLS.md](CLAUDE-SKILLS.md) — the command surfaces.
- [../plans/PLAN-Phase-5.6-Windows-Distribution.md](../plans/PLAN-Phase-5.6-Windows-Distribution.md) — the work behind this page.
