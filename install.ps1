<#
.SYNOPSIS
  Installs the Agentic SOP CLI (sop.exe) and, optionally, the agent integrations.

.DESCRIPTION
  The Windows counterpart of ./install.sh. It builds cmd/sop from this checkout into
  sop.exe, and can install the SOP agent skills for Zed and Claude Code and prepare
  the Claude Code plugin package.

  No administrator rights are required and nothing outside the target directories is
  written. The skills are *copied* rather than symlinked, because symlinks on Windows
  need Developer Mode or elevation; each installed copy carries a marker that records
  that agentic-sop manages it. This installer never edits a PowerShell profile and
  never changes PATH: a missing PATH entry is reported with the command to add it.

.PARAMETER Skills
  none | zed | claude | all - install the SOP skills for those agents (default: none).

.PARAMETER Plugin
  none | claude - prepare the Claude Code plugin package (default: none).

.PARAMETER All
  Same as -Skills all -Plugin claude.

.PARAMETER BinDir
  Where to install sop.exe. Default: $env:SOP_BIN_DIR, else <home>/.local/bin.

.PARAMETER DryRun
  Print what would happen and change nothing.

.PARAMETER UninstallSkills
  none | zed | claude | all - remove the SOP-managed skills for those agents.

.PARAMETER Force
  Refresh SOP-owned skill copies even when they are already current. A skill that is
  not SOP-owned is never touched, with or without -Force.

.PARAMETER Help
  Show the usage.

.EXAMPLE
  .\install.ps1

.EXAMPLE
  .\install.ps1 -Skills claude -Plugin claude

.EXAMPLE
  .\install.ps1 -DryRun -All
#>
[CmdletBinding()]
param(
    [ValidateSet('none', 'zed', 'claude', 'all')][string]$Skills = 'none',
    [ValidateSet('none', 'claude')][string]$Plugin = 'none',
    [switch]$All,
    [string]$BinDir,
    [switch]$DryRun,
    [ValidateSet('none', 'zed', 'claude', 'all')][string]$UninstallSkills = 'none',
    [switch]$Force,
    [switch]$Help
)

$ErrorActionPreference = 'Stop'

# --- Context -----------------------------------------------------------------

$script:RepoDir = $PSScriptRoot
if (-not $script:RepoDir) { $script:RepoDir = (Get-Location).Path }
$script:DryRun = [bool]$DryRun
$script:Force = [bool]$Force
$script:Failed = $false

# The ownership marker. A directory under an agent's skill root is SOP-managed only
# when it carries this file with a matching identity line; the name of the directory is
# never enough, so a skill that happens to be called "sop-something" is safe.
$script:MarkerName = '.sop-managed'
$script:MarkerId = 'agentic-sop'

# The agents this installer understands, in report order.
$script:AgentOrder = @('zed', 'claude')

function Write-Note([string]$Text) {
    Write-Host $Text
}

function Write-Fail([string]$Text) {
    [Console]::Error.WriteLine("install: $Text")
    $script:Failed = $true
}

# Get-UserHome returns the Windows home directory. The environment variable comes first
# because it is the documented Windows home (%USERPROFILE%, which is what Zed's and
# Claude Code's "~" means) and because a caller can redirect it for a sandboxed or
# automated install; the shell API is the fallback when it is unset.
function Get-UserHome {
    if ($env:USERPROFILE) { return $env:USERPROFILE }
    if ($HOME) { return $HOME }
    return [Environment]::GetFolderPath('UserProfile')
}

# Agent skill roots. Zed documents the global root as ~/.agents/skills/ and Claude Code
# documents the personal one as ~/.claude/skills/; on Windows "~" is %USERPROFILE%, which
# is what the two Join-Path calls below build. The parent directory is the agent's own
# directory: `-Skills all` uses it to decide whether that agent is present.
function Get-SkillRoots {
    $home_dir = Get-UserHome
    $agents = Join-Path $home_dir '.agents'
    $claude = Join-Path $home_dir '.claude'
    return @{
        zed    = @{ Root = (Join-Path $agents 'skills'); Marker = $agents }
        claude = @{ Root = (Join-Path $claude 'skills'); Marker = $claude }
    }
}

$script:SkillRoots = Get-SkillRoots

function Show-Usage {
    Write-Note @'
Usage: .\install.ps1 [-Skills <none|zed|claude|all>] [-Plugin <none|claude>] [-All]
                     [-BinDir <path>] [-DryRun] [-UninstallSkills <none|zed|claude|all>]
                     [-Force] [-Help]

Installs the sop CLI (sop.exe), and optionally the agent integrations. No
administrator rights are required, and no PATH or profile setting is changed for you.

  -Skills <none|zed|claude|all>   install the SOP skills for those agents
                                  (default: none)
  -Plugin <none|claude>           prepare the Claude Code plugin package
                                  (default: none)
  -All                            same as -Skills all -Plugin claude
  -BinDir <path>                  where to install sop.exe
                                  (default: $env:SOP_BIN_DIR, else <home>/.local/bin)
  -UninstallSkills <...>          remove the SOP-managed skills for those agents
  -Force                          refresh SOP-owned skill copies even when current
  -DryRun                         print what would happen, change nothing
  -Help                           show this help

Examples:
  .\install.ps1
  .\install.ps1 -Skills zed
  .\install.ps1 -Skills claude -Plugin claude
  .\install.ps1 -All -DryRun
  .\install.ps1 -UninstallSkills all

After installing, run `sop version`, and type "/" in your agent to find
/sop, /sop-plan, /sop-review, /sop-diagnose, /sop-test, /sop-implement.
'@
}

# Resolve-BinDir returns the user-owned directory sop.exe is installed into, following
# the documented precedence: -BinDir, then SOP_BIN_DIR, then <home>/.local/bin.
function Resolve-BinDir {
    $dir = $BinDir
    if (-not $dir) { $dir = $env:SOP_BIN_DIR }
    if (-not $dir) { $dir = Join-Path (Get-UserHome) '.local' | Join-Path -ChildPath 'bin' }
    # A leading "~" is expanded, because PowerShell does not expand it inside an
    # argument, and GetFullPath normalizes separators and relative segments.
    if ($dir.StartsWith('~')) { $dir = (Get-UserHome) + $dir.Substring(1) }
    return [System.IO.Path]::GetFullPath($dir)
}

# Test-OnPath reports whether dir is one of PATH's entries.
function Test-OnPath([string]$Dir) {
    $want = $Dir.Replace('/', '\').TrimEnd('\')
    foreach ($entry in ($env:Path -split ';')) {
        if (-not $entry) { continue }
        $e = $entry.Replace('/', '\').TrimEnd('\')
        if ([string]::Equals($e, $want, [System.StringComparison]::OrdinalIgnoreCase)) { return $true }
    }
    return $false
}

# Test-IsReparsePoint reports whether Path is a link or junction. A link is never
# written through and never removed: its target is somebody else's directory, and
# Windows PowerShell's Remove-Item -Recurse on one deletes the target's contents.
function Test-IsReparsePoint([string]$Path) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) { return $false }
    return (([int]$item.Attributes) -band ([int][System.IO.FileAttributes]::ReparsePoint)) -ne 0
}

# --- The sop CLI -------------------------------------------------------------

function Install-Cli([string]$TargetDir) {
    $dest = Join-Path $TargetDir 'sop.exe'
    $goCmds = @(Get-Command go -CommandType Application -ErrorAction SilentlyContinue)
    if ($goCmds.Count -eq 0) {
        Write-Fail "the Go toolchain ('go') is required to build sop and was not found on PATH"
        Write-Fail "install Go (https://go.dev/dl/), then re-run .\install.ps1"
        return
    }
    # More than one installation on PATH is normal; the first is the one a shell would
    # run, so use that. ($goCmds[0].Source is a single path: taking the whole collection
    # would join several paths into one unusable command.)
    $go = $goCmds[0].Source

    Write-Note "Installing the sop CLI:"
    Write-Note "  from: $($script:RepoDir)"
    Write-Note "  to:   $dest"

    if ($script:DryRun) {
        Write-Note "would: create $TargetDir"
        Write-Note "would: go build -o $dest ./cmd/sop"
        return
    }

    if (-not (Test-Path -LiteralPath $TargetDir -PathType Container)) {
        New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
    }

    # Build beside the destination and move it into place, so a failed build never
    # truncates a working sop.exe. An existing sop.exe is deleted immediately before the
    # move rather than overwritten by it: Windows PowerShell 5.1 cannot move onto an
    # existing file, and deleting first also means a sop.exe that is running fails here
    # with the old binary still in place instead of being half-replaced.
    $tmp = Join-Path $TargetDir ('.sop.build.' + [Guid]::NewGuid().ToString('N') + '.exe')
    try {
        Push-Location -LiteralPath $script:RepoDir
        try {
            & $go build -o $tmp ./cmd/sop
            if ($LASTEXITCODE -ne 0) {
                Write-Fail "go build failed (exit $LASTEXITCODE); any existing sop.exe was left untouched"
                return
            }
            if (-not (Test-Path -LiteralPath $tmp -PathType Leaf)) {
                Write-Fail "go build reported success but produced no binary"
                return
            }
        }
        finally {
            Pop-Location
        }
    }
    catch {
        Write-Fail "go build could not run: $($_.Exception.Message)"
        return
    }

    try {
        if (Test-Path -LiteralPath $dest) {
            # Delete first, then move. Move-Item's overwrite behaviour differs between
            # Windows PowerShell 5.1 and PowerShell 7, so the .NET file APIs are used
            # here: they behave identically on both, and a sop.exe that is running fails
            # at the delete with the old binary still in place rather than half-replaced.
            [System.IO.File]::Delete($dest)
        }
        [System.IO.File]::Move($tmp, $dest)
        Write-Note "  installed: $dest"
    }
    catch {
        Write-Fail "could not replace $dest : $($_.Exception.Message)"
        Write-Fail "if sop.exe is running or open elsewhere, close it and re-run .\install.ps1"
    }
    finally {
        if (Test-Path -LiteralPath $tmp -PathType Leaf) {
            Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
        }
    }
}

# --- Skills ------------------------------------------------------------------

# Get-SkillNames returns the canonical skills shipped in skills/, in a deterministic
# order. The tree is the single source of truth: a folder without a SKILL.md is not a
# skill and is ignored.
function Get-SkillNames {
    $skillsDir = Join-Path $script:RepoDir 'skills'
    if (-not (Test-Path -LiteralPath $skillsDir -PathType Container)) {
        Write-Fail "the canonical skills tree is missing: $skillsDir"
        return @()
    }
    $names = @()
    foreach ($dir in (Get-ChildItem -LiteralPath $skillsDir -Directory | Sort-Object -Property Name)) {
        if (Test-Path -LiteralPath (Join-Path $dir.FullName 'SKILL.md') -PathType Leaf) {
            $names += $dir.Name
        }
    }
    return $names
}

# Get-FileSha256 returns a file's SHA-256 as lowercase hex. It uses the .NET hash API
# rather than the Get-FileHash cmdlet, which is not resolvable in every Windows
# PowerShell environment (CI reported CommandNotFoundException for it while the
# surrounding cmdlets worked), so this has no module-autoloading dependency.
function Get-FileSha256([string]$Path) {
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $stream = [System.IO.File]::OpenRead($Path)
        try {
            $digest = $sha.ComputeHash($stream)
        }
        finally {
            $stream.Dispose()
        }
    }
    finally {
        $sha.Dispose()
    }
    return ([System.BitConverter]::ToString($digest) -replace '-', '').ToLowerInvariant()
}

# Read-Marker returns the marker's fields for a SOP-managed skill directory, or $null
# when the directory is not SOP's.
function Read-Marker([string]$SkillDir) {
    $path = Join-Path $SkillDir $script:MarkerName
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { return $null }
    $fields = @{}
    foreach ($line in (Get-Content -LiteralPath $path)) {
        if ($line -match '^([A-Za-z_]+)=(.*)$') { $fields[$Matches[1]] = $Matches[2] }
    }
    if ($fields['marker'] -ne $script:MarkerId) { return $null }
    return $fields
}

# Write-Marker records that this installer owns the skill copy.
function Write-Marker([string]$SkillDir, [string]$Skill) {
    $text = @(
        "marker=$($script:MarkerId)",
        'version=1',
        "skill=$Skill",
        "source=skills/$Skill"
    ) -join "`n"
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText((Join-Path $SkillDir $script:MarkerName), $text + "`n", $encoding)
}

# Test-SkillCurrent reports whether every canonical file is present in the installed
# copy with the same contents. Order does not matter, and files the user added are
# ignored, so a current copy is never needlessly rewritten.
function Test-SkillCurrent([string]$Source, [string]$Target) {
    foreach ($file in (Get-ChildItem -LiteralPath $Source -Recurse -File)) {
        $relative = $file.FullName.Substring($Source.Length).TrimStart('\', '/')
        $installed = Join-Path $Target $relative
        if (-not (Test-Path -LiteralPath $installed -PathType Leaf)) { return $false }
        $a = Get-FileSha256 $file.FullName
        $b = Get-FileSha256 $installed
        if ($a -ne $b) { return $false }
    }
    return $true
}

# Install-SkillRoot copies the canonical skills into one agent's root.
function Install-SkillRoot([string]$Agent) {
    if (-not $script:SkillRoots.ContainsKey($Agent)) {
        Write-Fail "unknown agent: $Agent (want zed or claude)"
        return
    }
    $root = $script:SkillRoots[$Agent].Root
    $names = Get-SkillNames

    Write-Note ""
    Write-Note "Installing the SOP skills ($Agent) into:"
    Write-Note "  $root"

    if ($names.Count -eq 0) {
        Write-Fail "no installable skills found under $(Join-Path $script:RepoDir 'skills')"
        return
    }

    if ($script:DryRun) {
        Write-Note "would: create $root"
        foreach ($name in $names) {
            Write-Note "would: copy $(Join-Path (Join-Path $script:RepoDir 'skills') $name) -> $(Join-Path $root $name)"
        }
        return
    }

    if (-not (Test-Path -LiteralPath $root -PathType Container)) {
        New-Item -ItemType Directory -Force -Path $root | Out-Null
    }

    foreach ($name in $names) {
        $source = Join-Path (Join-Path $script:RepoDir 'skills') $name
        $target = Join-Path $root $name
        if (-not (Test-Path -LiteralPath (Join-Path $source 'SKILL.md') -PathType Leaf)) {
            Write-Fail "the canonical skill is missing: $(Join-Path $source 'SKILL.md')"
            continue
        }

        if (Test-Path -LiteralPath $target) {
            if (Test-IsReparsePoint $target) {
                Write-Note "  skip: $target is a link, not a SOP-owned copy; leaving it"
                continue
            }
            if ($null -eq (Read-Marker $target)) {
                Write-Note "  skip: an existing non-SOP skill uses this name: $target"
                continue
            }
            if ((Test-SkillCurrent $source $target) -and (-not $script:Force)) {
                Write-Note "  ok: $target (current)"
                continue
            }
            # SOP-owned and out of date (or -Force): replace it from canonical. Removing
            # first means a file the canonical tree no longer ships does not survive the
            # update.
            try {
                Remove-Item -LiteralPath $target -Recurse -Force
            }
            catch {
                Write-Fail "could not replace $target : $($_.Exception.Message)"
                continue
            }
            Copy-Item -LiteralPath $source -Destination $target -Recurse -Force
            Write-Marker -SkillDir $target -Skill $name
            Write-Note "  updated: $target"
            continue
        }

        Copy-Item -LiteralPath $source -Destination $target -Recurse -Force
        Write-Marker -SkillDir $target -Skill $name
        Write-Note "  installed: $target"
    }

    # A SOP-managed copy of a skill this version no longer ships is reported, never
    # removed silently. `-UninstallSkills` is the way to remove it.
    foreach ($dir in (Get-ChildItem -LiteralPath $root -Directory -Force -ErrorAction SilentlyContinue)) {
        if (($names -contains $dir.Name)) { continue }
        if (Test-IsReparsePoint $dir.FullName) { continue }
        if ($null -ne (Read-Marker $dir.FullName)) {
            Write-Note "  note: $($dir.Name) is SOP-managed but no longer shipped; -UninstallSkills removes it"
        }
    }
}

# Uninstall-SkillRoot removes the SOP-managed skills under one agent's root. Only
# directories that carry the marker are removed; a skill of the same name that is not
# SOP's is reported and left where it is.
function Uninstall-SkillRoot([string]$Agent) {
    if (-not $script:SkillRoots.ContainsKey($Agent)) {
        Write-Fail "unknown agent: $Agent (want zed or claude)"
        return
    }
    $root = $script:SkillRoots[$Agent].Root

    Write-Note ""
    Write-Note "Removing the SOP skills ($Agent) from:"
    Write-Note "  $root"

    if (-not (Test-Path -LiteralPath $root -PathType Container)) {
        Write-Note "  nothing installed (no $root)"
        return
    }
    if ($script:DryRun) {
        Write-Note "would: remove the SOP-managed skills under $root"
        return
    }

    $removed = 0
    foreach ($dir in (Get-ChildItem -LiteralPath $root -Directory -Force)) {
        if (Test-IsReparsePoint $dir.FullName) {
            Write-Note "  skip: $($dir.FullName) is a link, not a SOP-owned copy; leaving it"
            continue
        }
        if ($null -eq (Read-Marker $dir.FullName)) {
            Write-Note "  skip: $($dir.FullName) is not SOP-managed; leaving it"
            continue
        }
        try {
            Remove-Item -LiteralPath $dir.FullName -Recurse -Force
            Write-Note "  removed: $($dir.FullName)"
            $removed++
        }
        catch {
            Write-Fail "could not remove $($dir.FullName) : $($_.Exception.Message)"
        }
    }
    Write-Note "  $removed SOP-managed skill(s) removed"
}

# Select-Agents turns a -Skills/-UninstallSkills value into the list of agents to act
# on. For `all`, installing only touches an agent that is present (so it never creates
# configuration for an agent the machine does not have); uninstalling considers every
# agent, because removing what SOP owns needs no environment.
function Select-Agents([string]$Requested, [bool]$ForInstall) {
    if ($Requested -eq 'none') { return @() }
    if ($Requested -eq 'all') {
        $agents = @()
        foreach ($agent in $script:AgentOrder) {
            if (-not $ForInstall) { $agents += $agent; continue }
            $marker = $script:SkillRoots[$agent].Marker
            if (Test-Path -LiteralPath $marker -PathType Container) {
                $agents += $agent
            }
            else {
                Write-Note ""
                Write-Note "skip: $agent (no $marker); install that agent, or run: .\install.ps1 -Skills $agent"
            }
        }
        if ($agents.Count -eq 0) {
            Write-Fail "no supported agent environment found (looked for: $($script:AgentOrder -join ', '))"
            Write-Fail "name one explicitly, e.g. .\install.ps1 -Skills zed"
        }
        return $agents
    }
    return @($Requested)
}

# --- The Claude Code plugin ---------------------------------------------------

# Prepare-Plugin verifies the committed plugin package and prints the commands that
# install it. Claude Code registers plugins from inside Claude Code, so this step never
# writes into Claude's configuration; it validates the package when the claude CLI is
# available and always prints the official steps.
function Prepare-Plugin([string]$Requested) {
    if ($Requested -eq 'none') { return }

    $pluginDir = Join-Path (Join-Path $script:RepoDir 'integrations') 'claude'
    $manifest = Join-Path (Join-Path $pluginDir '.claude-plugin') 'plugin.json'
    $marketplace = Join-Path (Join-Path $script:RepoDir '.claude-plugin') 'marketplace.json'

    Write-Note ""
    Write-Note "Preparing the Claude Code plugin:"
    if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) {
        Write-Fail "the plugin package is missing: $manifest"
        Write-Fail "regenerate it with scripts/build-claude-plugin.sh (or .\install.ps1 -DryRun to inspect)"
        return
    }
    if (-not (Test-Path -LiteralPath $marketplace -PathType Leaf)) {
        Write-Fail "the marketplace file is missing: $marketplace"
        return
    }
    Write-Note "  plugin:      $pluginDir"
    Write-Note "  marketplace: $marketplace"

    if (-not $script:DryRun) {
        $claudeCmds = @(Get-Command claude -ErrorAction SilentlyContinue)
        if ($claudeCmds.Count -gt 0) {
            Write-Note "  validating with the claude CLI:"
            $global:LASTEXITCODE = 0
            $validated = $false
            $ran = $true
            try {
                & $claudeCmds[0].Source plugin validate --strict $pluginDir
                $validated = ($LASTEXITCODE -eq 0)
            }
            catch {
                $ran = $false
                Write-Fail "'claude plugin validate' could not run: $($_.Exception.Message)"
            }
            if ($validated) {
                Write-Note "  validation passed"
            }
            elseif ($ran) {
                Write-Fail "'claude plugin validate' reported a problem (see above)"
            }
        }
        else {
            Write-Note "  note: the claude CLI is not on PATH, so validation was skipped;"
            Write-Note "        the package is still ready to install from Claude Code."
        }
    }

    $steps = @'
  Claude Code installs plugins from inside Claude Code, so run one of these:

    Load it for one session (no settings change):
      claude --plugin-dir "<PLUGIN>"

    Register this checkout as a marketplace, then install the plugin:
      claude plugin marketplace add "<REPO>"
      claude plugin install sop@agentic-sop

    The same two steps from inside a session:
      /plugin marketplace add <REPO>
      /plugin install sop@agentic-sop

  The plugin exposes the SOP commands as /sop:sop-plan, /sop:sop-review,
  /sop:sop-diagnose, /sop:sop-test, /sop:sop-implement (and /sop:sop), and the same
  names unprefixed (/sop-review, ...) when no other skill claims them. Every one of
  them calls sop prompt, so SOP keeps owning routing, providers, and approval.
'@
    Write-Note ""
    Write-Note ($steps.Replace('<PLUGIN>', $pluginDir).Replace('<REPO>', $script:RepoDir))
}

# --- Report -------------------------------------------------------------------

function Report-Path([string]$TargetDir) {
    $dest = Join-Path $TargetDir 'sop.exe'

    if (Test-OnPath $TargetDir) {
        Write-Note ""
        Write-Note "  $TargetDir is on PATH."
        $found = @(Get-Command sop.exe -CommandType Application -ErrorAction SilentlyContinue)
        if (($found.Count -gt 0) -and (-not ([string]::Equals($found[0].Source, $dest, [System.StringComparison]::OrdinalIgnoreCase)))) {
            Write-Note "  note: 'sop' currently resolves to $($found[0].Source)"
        }
        return
    }

    $guidance = @'
SOP installed to:

  <DEST>

Add this directory to PATH for the current session:

  $env:Path = "<BIN>;$env:Path"

To keep it, add "<BIN>" to your user PATH (Settings > System > About > Advanced
system settings > Environment Variables), or run this once — it appends to your user
PATH, so run it only once:

  [Environment]::SetEnvironmentVariable('Path', "<BIN>;" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')

Then open a new terminal. This installer never changes PATH or your PowerShell
profile for you.
'@
    Write-Note ""
    Write-Note ($guidance.Replace('<DEST>', $dest).Replace('<BIN>', $TargetDir))
}

# --- Go -----------------------------------------------------------------------

if ($Help) {
    Show-Usage
    exit 0
}
if ($All) {
    $Skills = 'all'
    $Plugin = 'claude'
}
if (($UninstallSkills -ne 'none') -and (($Skills -ne 'none') -or $All)) {
    [Console]::Error.WriteLine('install: -UninstallSkills cannot be combined with -Skills or -All')
    Show-Usage
    exit 2
}

if (-not (Test-Path -LiteralPath (Join-Path $script:RepoDir 'go.mod') -PathType Leaf) -or
    -not (Test-Path -LiteralPath (Join-Path (Join-Path $script:RepoDir 'cmd') 'sop') -PathType Container)) {
    Write-Fail "run this from the agentic-sop checkout (no cmd/sop under $($script:RepoDir))"
    exit 1
}

$targetDir = Resolve-BinDir
Write-Note "SOP installer (Windows)"
Write-Note "  repo:     $($script:RepoDir)"
Write-Note "  bin dir:  $targetDir"
Write-Note "  skills:   $Skills"
Write-Note "  plugin:   $Plugin"
if ($UninstallSkills -ne 'none') { Write-Note "  uninstall skills: $UninstallSkills" }
if ($script:DryRun) { Write-Note "  dry run:  yes (nothing will be changed)" }
Write-Note ""

if ($UninstallSkills -ne 'none') {
    # A removal run does not rebuild the CLI; it only removes what SOP installed.
    foreach ($agent in (Select-Agents $UninstallSkills $false)) {
        Uninstall-SkillRoot $agent
    }
}
else {
    Install-Cli $targetDir
    if ($Skills -ne 'none') {
        foreach ($agent in (Select-Agents $Skills $true)) {
            Install-SkillRoot $agent
        }
    }
}
Prepare-Plugin $Plugin
Report-Path $targetDir

if ($script:Failed) {
    Write-Note ""
    [Console]::Error.WriteLine('Finished with errors (see above).')
    exit 1
}
Write-Note ""
Write-Note "Done."
exit 0
