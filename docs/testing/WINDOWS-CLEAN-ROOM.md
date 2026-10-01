# Windows clean-room test

A checklist and results log for installing SOP on a **real, clean Windows machine**, as
distinct from CI. CI proves the installer's behavior against temporary directories on
`windows-latest`; it cannot prove what a developer's actual machine does — a real
`%USERPROFILE%`, a real PATH, a real Zed or Claude Code installation, and a real
`sop.exe` that a shell actually finds.

Results are recorded below exactly as observed. Anything not yet performed stays
**NOT YET RUN**; nothing here is inferred from CI.

## Environment to record

| Item | Value |
| ---- | ----- |
| Windows version | NOT YET RUN |
| PowerShell version (`$PSVersionTable.PSVersion`) | NOT YET RUN |
| Go version (`go version`) | NOT YET RUN |
| Claude Code version (`claude --version`), if tested | NOT YET RUN |
| Zed version, if tested | NOT YET RUN |

## One-shot transcript (Windows PowerShell)

The whole checklist in one paste, so a run is a single copy. Substitute `<REPO>` with the
clone URL. Go is required; Zed and Claude Code are only needed for the steps that test
them. The installer needs no administrator rights and never edits PATH or a profile.

This transcript was authored without a Windows machine available; if a line errors, paste
the error under [Issues discovered](#issues-discovered) rather than silently continuing.

```powershell
function Section($t) { Write-Host ""; Write-Host "===== $t =====" }

Section "0. Environment"
"Windows     : $([System.Environment]::OSVersion.VersionString)"
"PowerShell  : $($PSVersionTable.PSVersion)"
"USERPROFILE : $env:USERPROFILE"
"go          : $((Get-Command go -ErrorAction SilentlyContinue | Select-Object -First 1).Source)"
go version
$claude = Get-Command claude -ErrorAction SilentlyContinue | Select-Object -First 1
"claude      : $(if ($claude) { $claude.Source } else { '<absent>' })"
if ($claude) { claude --version }

Section "1. Clone + install the CLI"
git clone <REPO> agentic-sop
Set-Location agentic-sop
.\install.ps1
$env:Path = "$env:USERPROFILE\.local\bin;$env:Path"   # session-only; the installer never edits PATH
"resolves to : $((Get-Command sop -ErrorAction SilentlyContinue | Select-Object -First 1).Source)"
where.exe sop
sop version
sop --help

Section "2. Zed skills"
.\install.ps1 -Skills zed
Get-ChildItem "$env:USERPROFILE\.agents\skills" -Directory | Where-Object { $_.Name -like 'sop*' } |
  ForEach-Object { "  $($_.Name): marker=$(Test-Path (Join-Path $_.FullName '.sop-managed'))" }

Section "3. Claude Code skills"
.\install.ps1 -Skills claude
Get-ChildItem "$env:USERPROFILE\.claude\skills" -Directory | Where-Object { $_.Name -like 'sop*' } |
  ForEach-Object { "  $($_.Name): marker=$(Test-Path (Join-Path $_.FullName '.sop-managed'))" }

Section "4. Claude Code plugin package"
.\install.ps1 -Plugin claude

Section "5. Idempotency + refresh"
.\install.ps1 -Skills claude           # a second run reports each copy as current
.\install.ps1 -Skills claude -Force    # refreshes SOP-owned copies

Section "6. Foreign-skill safety"
.\install.ps1 -UninstallSkills claude  # removes only SOP-owned claude skills
$foreign = "$env:USERPROFILE\.claude\skills\sop-review"
New-Item -ItemType Directory -Force -Path $foreign | Out-Null
Set-Content -Path (Join-Path $foreign 'SKILL.md') -Value 'not managed by SOP'
.\install.ps1 -Skills claude           # skips sop-review: an existing non-SOP skill uses this name
"foreign marker?  $(Test-Path (Join-Path $foreign '.sop-managed'))"   # expect False
.\install.ps1 -UninstallSkills claude  # leaves sop-review: is not SOP-managed; leaving it
"foreign survived? $(Test-Path $foreign)"                             # expect True
Remove-Item -Recurse -Force $foreign
```

Steps 5–6 below (the two read-only commands and the mutating command) are typed in the
agent panel and are **not** scriptable — do those by hand after the transcript.

## Checklist

Run each step and record the result. Use a disposable checkout and a disposable project
for the mutating command.

### 1. Install the CLI

```powershell
git clone <repo>
cd agentic-sop
.\install.ps1
```

Record: the installation command, the installation directory it chose, and the PATH
behavior it reported (whether the directory was already on PATH, and whether a different
`sop` resolved first).

```powershell
sop version
sop --help
```

### 2. Zed skills

```powershell
.\install.ps1 -Skills zed
```

Open Zed and check that `/sop`, `/sop-plan`, `/sop-review`, `/sop-diagnose`, `/sop-test`,
and `/sop-implement` are discoverable in the agent panel (type `/` in the message
editor). Confirm `%USERPROFILE%\.agents\skills\<name>\.sop-managed` exists for each.

### 3. Claude Code skills

```powershell
.\install.ps1 -Skills claude
```

Confirm the same commands are discoverable in Claude Code, and that
`%USERPROFILE%\.claude\skills\<name>\.sop-managed` exists for each.

### 4. Claude Code plugin

```powershell
.\install.ps1 -Plugin claude
```

Then follow the printed steps (one of them, not both):

```text
claude --plugin-dir <checkout>\integrations\claude
claude plugin marketplace add <checkout>
claude plugin install sop@agentic-sop
```

Record the `claude plugin validate` result if the CLI was present, then
`claude plugin details sop` and the commands it exposes.

### 5. Read-only commands

Type `/` in the agent to confirm which names are offered, then run:

```text
/sop-plan explain how you would add caching to this project
/sop-review review the current repository for architectural problems
```

The name depends on how the integration was installed. The **skills** install
(`-Skills zed`, `-Skills claude`) exposes the unprefixed `/sop-plan`, `/sop-review`, …
The **plugin** (`-Plugin claude`) namespaces them as `/sop:sop-plan`,
`/sop:sop-review`, …, and also registers the unprefixed names when no other skill
claims them, so in a session with the plugin loaded expect `/sop:sop-plan`.

Record what each returned and where the run was recorded
(`.agent-sdlc\runs\prompts\<run-id>\`).

### 6. The mutating command, in a disposable repository

```text
/sop-implement add a small documented test change
```

(With the plugin loaded, this is `/sop:sop-implement`.) Confirm it enters SOP's
governed implementation lifecycle rather than editing the repository directly, and
that the change is validated and reviewed.

### 7. Update and uninstall

```powershell
.\install.ps1 -Skills claude            # a second run reports the copies as current
.\install.ps1 -Skills claude -Force     # refreshes them
.\install.ps1 -UninstallSkills claude   # removes only SOP-managed skills
```

Create a skill folder of the same name that is *not* SOP's first, and confirm both the
install and the uninstall leave it alone.

## Results

| Step | Result | Notes |
| ---- | ------ | ----- |
| 1. Install the CLI | NOT YET RUN | |
| 1. `sop version` / `sop --help` | NOT YET RUN | |
| 2. Zed skills discoverable | NOT YET RUN | |
| 3. Claude Code skills discoverable | NOT YET RUN | |
| 4. Plugin installed from Claude Code | NOT YET RUN | |
| 5. `/sop-plan`, `/sop-review` | NOT YET RUN | |
| 6. `/sop-implement` (disposable repo) | NOT YET RUN | |
| 7. Update, `-Force`, uninstall | NOT YET RUN | |

### Issues discovered

None recorded yet.

## What CI does cover

`windows-latest` runs `.github/workflows/ci.yml`'s `windows` job: `go build ./...`,
`go vet ./...`, a PowerShell language-parser check on `install.ps1`, the distribution
test packages, and a smoke run of `install.ps1` (`-Help`, `-DryRun`, a real install into
a path containing spaces, `sop.exe version`, and `-Skills claude` into a scratch
`USERPROFILE` asserting all seven skills and their ownership markers). The Windows tests
in [`internal/dist`](../../internal/dist) exercise the same surface under both Windows
PowerShell and `pwsh`. CI does not exercise Zed, Claude Code, a real PATH, or a real
user profile.

## See also

- [../guides/WINDOWS-INSTALLATION.md](../guides/WINDOWS-INSTALLATION.md)
- [../plans/PLAN-Phase-5.6-Windows-Distribution.md](../plans/PLAN-Phase-5.6-Windows-Distribution.md)
- [../guides/CLAUDE-PLUGIN.md](../guides/CLAUDE-PLUGIN.md)
