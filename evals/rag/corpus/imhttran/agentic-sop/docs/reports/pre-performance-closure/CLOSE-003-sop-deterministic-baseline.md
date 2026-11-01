# CLOSE-003 — SOP Deterministic Baseline

**Type:** Normative-recorded closure deliverable (CLOSE-003 of `docs/plans/PLAN-Pre-Performance-Closure.md`).

## 0. Provenance

> **How this report was produced.** CLOSE-003's own invocation (provider `ollama`,
> `deepseek-v4.1-flash:cloud`) reported that it had created this file, but the
> harness reconciled `changes_expected` to repository reality — `changes_expected:false`
> and the repository was **not changed** (`.agent-sdlc/runs/CLOSE-003/implementation.md`).
> The stage still passed on its recorded deterministic validation
> (`.agent-sdlc/runs/CLOSE-003/report.md`: `PASS BUILD`, `PASS UNIT_TEST`, `PASS LINT`).
> This document was therefore produced during the post-closure documentation
> reconciliation by **actually running** each command below at the recorded revision
> and capturing the real stdout/stderr and exit code. Every value here is captured
> output, not narration; where nothing was printed the cell says so.

## 1. Environment

| Field | Value |
| --- | --- |
| Repository | `agentic-sop` |
| Working directory (cwd) | `/Users/imhttran/agentic-workspace/agentic-sop` |
| Branch | `fix/verify-repository-mutations` |
| Revision | `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc` |
| Toolchain | `go version go1.27.1 darwin/arm64` |
| Module / Go directive | `github.com/imhttran/agentic-sop` / `go 1.27.1` (`go.mod`) |
| Captured (UTC) | `2026-10-06T01:29:59Z` |

## 2. Results summary

| # | Command | Exit | Status |
| --- | --- | --- | --- |
| 1 | `gofmt -l .` | 0 | PASS |
| 2 | `go vet ./...` | 0 | PASS |
| 3 | `go build ./...` | 0 | PASS |
| 4 | `go test ./...` | 0 | PASS |
| 5 | `go test -race ./...` | 0 | PASS |
| 6 | `bash scripts/checks/check-doc-links.sh` | 0 | PASS |
| 7 | `scripts/packaging/build-claude-plugin.sh --check` | 0 | PASS |
| 8 | `go test -v ./internal/dist/... ./internal/skill/...` | 0 | PASS |
| 9 | `./install.sh --help` | 0 | PASS |
| 10 | `./install.sh --dry-run` | 0 | PASS |
| 11 | `./install.sh --skills zed --dry-run` | 0 | PASS |
| 12 | `./install.sh --skills claude --dry-run` | 0 | PASS |
| 13 | `./install.sh --plugin claude --dry-run` | 0 | PASS |

All commands above exited `0`.

## 3. Raw command evidence

### `gofmt -l .`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
(no output)
```

### `go vet ./...`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
(no output)
```

### `go build ./...`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
(no output)
```

### `go test ./...`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
?   	github.com/imhttran/agentic-sop/cmd/sop	[no test files]
?   	github.com/imhttran/agentic-sop/cmd/sop-ollama-agent	[no test files]
ok  	github.com/imhttran/agentic-sop/internal/activity	0.167s
ok  	github.com/imhttran/agentic-sop/internal/agent	0.319s
ok  	github.com/imhttran/agentic-sop/internal/agentbin	0.363s
ok  	github.com/imhttran/agentic-sop/internal/approval	0.516s
ok  	github.com/imhttran/agentic-sop/internal/autonomy	0.768s
ok  	github.com/imhttran/agentic-sop/internal/bootstrap	0.627s
ok  	github.com/imhttran/agentic-sop/internal/ci	0.890s
ok  	github.com/imhttran/agentic-sop/internal/ciremediation	1.017s
ok  	github.com/imhttran/agentic-sop/internal/cli	8.527s
ok  	github.com/imhttran/agentic-sop/internal/commandpolicy	0.655s
ok  	github.com/imhttran/agentic-sop/internal/commitgate	0.775s
ok  	github.com/imhttran/agentic-sop/internal/completion	0.896s
ok  	github.com/imhttran/agentic-sop/internal/config	1.039s
ok  	github.com/imhttran/agentic-sop/internal/decision	1.152s
ok  	github.com/imhttran/agentic-sop/internal/dist	9.178s
ok  	github.com/imhttran/agentic-sop/internal/domain	1.357s
ok  	github.com/imhttran/agentic-sop/internal/dotenv	1.489s
ok  	github.com/imhttran/agentic-sop/internal/e2e	1.685s
ok  	github.com/imhttran/agentic-sop/internal/e2e/harness	1.963s
ok  	github.com/imhttran/agentic-sop/internal/e2e/lifecycle	2.792s
ok  	github.com/imhttran/agentic-sop/internal/eval	2.000s
ok  	github.com/imhttran/agentic-sop/internal/failure	2.016s
ok  	github.com/imhttran/agentic-sop/internal/git	5.253s
ok  	github.com/imhttran/agentic-sop/internal/github	2.045s
ok  	github.com/imhttran/agentic-sop/internal/handoff	2.046s
ok  	github.com/imhttran/agentic-sop/internal/handoff/caveman	2.046s
ok  	github.com/imhttran/agentic-sop/internal/jev	2.139s
ok  	github.com/imhttran/agentic-sop/internal/mcp	2.003s
ok  	github.com/imhttran/agentic-sop/internal/mergegate	1.998s
ok  	github.com/imhttran/agentic-sop/internal/model	1.931s
ok  	github.com/imhttran/agentic-sop/internal/ollamaagent	23.590s
ok  	github.com/imhttran/agentic-sop/internal/parallel	1.883s
ok  	github.com/imhttran/agentic-sop/internal/perf	1.827s
ok  	github.com/imhttran/agentic-sop/internal/planflow	1.779s
ok  	github.com/imhttran/agentic-sop/internal/planner	1.738s
ok  	github.com/imhttran/agentic-sop/internal/provider	1.753s
ok  	github.com/imhttran/agentic-sop/internal/provider/command	1.862s
ok  	github.com/imhttran/agentic-sop/internal/provider/httpx	1.822s
ok  	github.com/imhttran/agentic-sop/internal/provider/llamacpp	1.887s
ok  	github.com/imhttran/agentic-sop/internal/provider/mlx	1.905s
ok  	github.com/imhttran/agentic-sop/internal/provider/ollama	1.938s
ok  	github.com/imhttran/agentic-sop/internal/provider/openai	1.509s
ok  	github.com/imhttran/agentic-sop/internal/quality	1.563s
ok  	github.com/imhttran/agentic-sop/internal/recovery	1.598s
ok  	github.com/imhttran/agentic-sop/internal/resume	1.667s
ok  	github.com/imhttran/agentic-sop/internal/review	1.703s
ok  	github.com/imhttran/agentic-sop/internal/router	1.692s
ok  	github.com/imhttran/agentic-sop/internal/run	1.696s
ok  	github.com/imhttran/agentic-sop/internal/scheduler	1.637s
ok  	github.com/imhttran/agentic-sop/internal/skill	2.164s
ok  	github.com/imhttran/agentic-sop/internal/store	1.801s
ok  	github.com/imhttran/agentic-sop/internal/taskbuilder	1.606s
ok  	github.com/imhttran/agentic-sop/internal/taskfile	1.598s
ok  	github.com/imhttran/agentic-sop/internal/testrunner	1.827s
ok  	github.com/imhttran/agentic-sop/internal/toolharness	2.644s
ok  	github.com/imhttran/agentic-sop/internal/validate	1.603s
ok  	github.com/imhttran/agentic-sop/internal/workitem	1.723s
```

### `go test -race ./...`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
?   	github.com/imhttran/agentic-sop/cmd/sop	[no test files]
?   	github.com/imhttran/agentic-sop/cmd/sop-ollama-agent	[no test files]
ok  	github.com/imhttran/agentic-sop/internal/activity	1.205s
ok  	github.com/imhttran/agentic-sop/internal/agent	1.421s
ok  	github.com/imhttran/agentic-sop/internal/agentbin	1.481s
ok  	github.com/imhttran/agentic-sop/internal/approval	1.678s
ok  	github.com/imhttran/agentic-sop/internal/autonomy	1.828s
ok  	github.com/imhttran/agentic-sop/internal/bootstrap	1.992s
ok  	github.com/imhttran/agentic-sop/internal/ci	2.136s
ok  	github.com/imhttran/agentic-sop/internal/ciremediation	2.290s
ok  	github.com/imhttran/agentic-sop/internal/cli	15.799s
ok  	github.com/imhttran/agentic-sop/internal/commandpolicy	1.899s
ok  	github.com/imhttran/agentic-sop/internal/commitgate	2.046s
ok  	github.com/imhttran/agentic-sop/internal/completion	2.200s
ok  	github.com/imhttran/agentic-sop/internal/config	2.402s
ok  	github.com/imhttran/agentic-sop/internal/decision	2.530s
ok  	github.com/imhttran/agentic-sop/internal/dist	10.355s
ok  	github.com/imhttran/agentic-sop/internal/domain	2.394s
ok  	github.com/imhttran/agentic-sop/internal/dotenv	2.494s
ok  	github.com/imhttran/agentic-sop/internal/e2e	2.625s
ok  	github.com/imhttran/agentic-sop/internal/e2e/harness	2.754s
ok  	github.com/imhttran/agentic-sop/internal/e2e/lifecycle	3.395s
ok  	github.com/imhttran/agentic-sop/internal/eval	2.583s
ok  	github.com/imhttran/agentic-sop/internal/failure	2.608s
ok  	github.com/imhttran/agentic-sop/internal/git	5.969s
ok  	github.com/imhttran/agentic-sop/internal/github	2.634s
ok  	github.com/imhttran/agentic-sop/internal/handoff	2.711s
ok  	github.com/imhttran/agentic-sop/internal/handoff/caveman	2.682s
ok  	github.com/imhttran/agentic-sop/internal/jev	2.838s
ok  	github.com/imhttran/agentic-sop/internal/mcp	2.544s
ok  	github.com/imhttran/agentic-sop/internal/mergegate	2.548s
ok  	github.com/imhttran/agentic-sop/internal/model	2.354s
ok  	github.com/imhttran/agentic-sop/internal/ollamaagent	24.850s
ok  	github.com/imhttran/agentic-sop/internal/parallel	2.114s
ok  	github.com/imhttran/agentic-sop/internal/perf	2.274s
ok  	github.com/imhttran/agentic-sop/internal/planflow	2.233s
ok  	github.com/imhttran/agentic-sop/internal/planner	2.239s
ok  	github.com/imhttran/agentic-sop/internal/provider	2.258s
ok  	github.com/imhttran/agentic-sop/internal/provider/command	2.247s
ok  	github.com/imhttran/agentic-sop/internal/provider/httpx	2.185s
ok  	github.com/imhttran/agentic-sop/internal/provider/llamacpp	2.061s
ok  	github.com/imhttran/agentic-sop/internal/provider/mlx	2.247s
ok  	github.com/imhttran/agentic-sop/internal/provider/ollama	2.296s
ok  	github.com/imhttran/agentic-sop/internal/provider/openai	2.293s
ok  	github.com/imhttran/agentic-sop/internal/quality	2.135s
ok  	github.com/imhttran/agentic-sop/internal/recovery	1.978s
ok  	github.com/imhttran/agentic-sop/internal/resume	2.032s
ok  	github.com/imhttran/agentic-sop/internal/review	1.996s
ok  	github.com/imhttran/agentic-sop/internal/router	2.159s
ok  	github.com/imhttran/agentic-sop/internal/run	2.148s
ok  	github.com/imhttran/agentic-sop/internal/scheduler	2.131s
ok  	github.com/imhttran/agentic-sop/internal/skill	2.722s
ok  	github.com/imhttran/agentic-sop/internal/store	2.774s
ok  	github.com/imhttran/agentic-sop/internal/taskbuilder	2.113s
ok  	github.com/imhttran/agentic-sop/internal/taskfile	2.077s
ok  	github.com/imhttran/agentic-sop/internal/testrunner	2.318s
ok  	github.com/imhttran/agentic-sop/internal/toolharness	3.384s
ok  	github.com/imhttran/agentic-sop/internal/validate	2.193s
ok  	github.com/imhttran/agentic-sop/internal/workitem	2.149s
```

### `bash scripts/checks/check-doc-links.sh`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
broken links: 0
```

### `scripts/packaging/build-claude-plugin.sh --check`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
build-claude-plugin: the plugin package is current
```

### `go test -v ./internal/dist/... ./internal/skill/...`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
=== RUN   TestSkillTreeMatchesTheCommandSurface
--- PASS: TestSkillTreeMatchesTheCommandSurface (0.00s)
=== RUN   TestInstallersShareTheDistributionContract
--- PASS: TestInstallersShareTheDistributionContract (0.00s)
=== RUN   TestInstallerHelp
--- PASS: TestInstallerHelp (0.02s)
=== RUN   TestInstallerRejectsBadArguments
--- PASS: TestInstallerRejectsBadArguments (0.07s)
=== RUN   TestInstallerDryRunChangesNothing
--- PASS: TestInstallerDryRunChangesNothing (0.04s)
=== RUN   TestInstallerInstallsTheCLI
--- PASS: TestInstallerInstallsTheCLI (1.80s)
=== RUN   TestInstallerBinDirWithSpaces
--- PASS: TestInstallerBinDirWithSpaces (0.49s)
=== RUN   TestInstallerDelegatesSkillInstallation
=== RUN   TestInstallerDelegatesSkillInstallation/zed
=== RUN   TestInstallerDelegatesSkillInstallation/claude
=== RUN   TestInstallerDelegatesSkillInstallation/all
--- PASS: TestInstallerDelegatesSkillInstallation (1.59s)
    --- PASS: TestInstallerDelegatesSkillInstallation/zed (0.52s)
    --- PASS: TestInstallerDelegatesSkillInstallation/claude (0.52s)
    --- PASS: TestInstallerDelegatesSkillInstallation/all (0.55s)
=== RUN   TestInstallerSkillsAllWithoutAnAgentFailsLoudly
--- PASS: TestInstallerSkillsAllWithoutAnAgentFailsLoudly (0.49s)
=== RUN   TestInstallerPreservesUnrelatedSkills
--- PASS: TestInstallerPreservesUnrelatedSkills (0.54s)
=== RUN   TestInstallerRequiresGo
--- PASS: TestInstallerRequiresGo (0.02s)
=== RUN   TestInstallerPluginStepIsSafeInEveryMode
--- PASS: TestInstallerPluginStepIsSafeInEveryMode (0.70s)
=== RUN   TestClaudePluginManifestIsValid
--- PASS: TestClaudePluginManifestIsValid (0.00s)
=== RUN   TestClaudeMarketplaceListsThePlugin
--- PASS: TestClaudeMarketplaceListsThePlugin (0.00s)
=== RUN   TestClaudePluginMirrorsTheCanonicalSkills
--- PASS: TestClaudePluginMirrorsTheCanonicalSkills (0.02s)
=== RUN   TestClaudePluginExposesTheCommandSurface
--- PASS: TestClaudePluginExposesTheCommandSurface (0.00s)
=== RUN   TestClaudePluginCarriesNoPolicyOrModelAccess
--- PASS: TestClaudePluginCarriesNoPolicyOrModelAccess (0.00s)
=== RUN   TestClaudePluginImplementIsGoverned
--- PASS: TestClaudePluginImplementIsGoverned (0.00s)
=== RUN   TestClaudePluginFailsClosedWithoutSOP
--- PASS: TestClaudePluginFailsClosedWithoutSOP (0.00s)
PASS
ok  	github.com/imhttran/agentic-sop/internal/dist	6.119s
=== RUN   TestEndToEndSkillContract
--- PASS: TestEndToEndSkillContract (0.00s)
=== RUN   TestSOPEndToEndAliasesDelegateToRunSkill
--- PASS: TestSOPEndToEndAliasesDelegateToRunSkill (0.00s)
=== RUN   TestInstallerZedIsIdempotent
--- PASS: TestInstallerZedIsIdempotent (0.06s)
=== RUN   TestInstallerClaudeIsIdempotent
--- PASS: TestInstallerClaudeIsIdempotent (0.05s)
=== RUN   TestInstallerAllInstallsPresentAgents
--- PASS: TestInstallerAllInstallsPresentAgents (0.05s)
=== RUN   TestInstallerAllSkipsAbsentAgent
--- PASS: TestInstallerAllSkipsAbsentAgent (0.03s)
=== RUN   TestInstallerExplicitTargetWithNoMarker
--- PASS: TestInstallerExplicitTargetWithNoMarker (0.03s)
=== RUN   TestInstallerNoSupportedAgentFails
--- PASS: TestInstallerNoSupportedAgentFails (0.01s)
=== RUN   TestInstallerUnknownTargetFails
--- PASS: TestInstallerUnknownTargetFails (0.01s)
=== RUN   TestInstallerDryRunChangesNothing
--- PASS: TestInstallerDryRunChangesNothing (0.01s)
=== RUN   TestInstallerUninstallIsScoped
--- PASS: TestInstallerUninstallIsScoped (0.07s)
=== RUN   TestInstallerPreservesForeignEntryAtSOPName
--- PASS: TestInstallerPreservesForeignEntryAtSOPName (0.08s)
=== RUN   TestInstallerProjectScope
--- PASS: TestInstallerProjectScope (0.03s)
=== RUN   TestInstallerForwardersDelegate
--- PASS: TestInstallerForwardersDelegate (0.04s)
=== RUN   TestInstallerKnowsEveryTargetRoot
--- PASS: TestInstallerKnowsEveryTargetRoot (0.00s)
=== RUN   TestInstallScriptsAreThinAndSafe
--- PASS: TestInstallScriptsAreThinAndSafe (0.00s)
=== RUN   TestSkillExists
--- PASS: TestSkillExists (0.00s)
=== RUN   TestSkillExamplesExist
--- PASS: TestSkillExamplesExist (0.00s)
=== RUN   TestSkillUsesCanonicalCapabilities
--- PASS: TestSkillUsesCanonicalCapabilities (0.00s)
=== RUN   TestSkillBashBlocksInvokeSOPOnly
--- PASS: TestSkillBashBlocksInvokeSOPOnly (0.00s)
=== RUN   TestSkillDoesNotRestatePolicyOrMutate
--- PASS: TestSkillDoesNotRestatePolicyOrMutate (0.00s)
=== RUN   TestZedSkillCommandsExist
--- PASS: TestZedSkillCommandsExist (0.00s)
=== RUN   TestZedSkillCapabilityMapping
--- PASS: TestZedSkillCapabilityMapping (0.00s)
=== RUN   TestSOPEntryPointListsAliases
--- PASS: TestSOPEntryPointListsAliases (0.00s)
=== RUN   TestZedSkillsFailClosedWithoutSOP
--- PASS: TestZedSkillsFailClosedWithoutSOP (0.00s)
=== RUN   TestImplementCommandDelegatesToGovernedPrompt
--- PASS: TestImplementCommandDelegatesToGovernedPrompt (0.00s)
=== RUN   TestInstallerKnowsEverySkill
--- PASS: TestInstallerKnowsEverySkill (0.00s)
PASS
ok  	github.com/imhttran/agentic-sop/internal/skill	0.641s
```

### `./install.sh --help`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
Usage: ./install.sh [OPTIONS]

Installs the sop CLI, and optionally the agent integrations. No root is required,
and no shell startup file is modified.

Options:
  --skills <none|zed|claude|all>   link the SOP commands for those agents
                                   (default: none)
  --plugin <none|claude>           prepare the Claude Code plugin package
                                   (default: none)
  --all                            --skills all --plugin claude
  --bin-dir <path>                 where to install the sop binary
                                   (default: $SOP_BIN_DIR, else $GOBIN, else
                                   `go env GOPATH`/bin)
  --dry-run                        print what would happen, change nothing
  -h, --help                       show this help

Examples:
  ./install.sh
  ./install.sh --skills zed
  ./install.sh --skills claude --plugin claude
  ./install.sh --all --dry-run

Next steps after installing:
  sop version                      confirm the CLI runs
  In Zed or Claude Code, type "/" in the agent editor to find /sop, /sop-plan,
  /sop-review, /sop-diagnose, /sop-test, /sop-implement.
```

### `./install.sh --dry-run`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
SOP installer
  repo:     /Users/imhttran/agentic-workspace/agentic-sop
  bin dir:  /tmp/close003/home/go/bin
  skills:   none
  plugin:   none
  dry run:  yes (nothing will be changed)

Installing the sop CLI:
  from: /Users/imhttran/agentic-workspace/agentic-sop
  to:   /tmp/close003/home/go/bin/sop
would: mkdir -p /tmp/close003/home/go/bin
would: (cd /Users/imhttran/agentic-workspace/agentic-sop && go build -o /tmp/close003/home/go/bin/sop ./cmd/sop)

SOP installed to:

  /tmp/close003/home/go/bin/sop

Add this directory to PATH:

  export PATH="/tmp/close003/home/go/bin:$PATH"

Add that line to your shell profile (~/.zshrc, ~/.bashrc, or ~/.profile) to keep it.

Done.
```

### `./install.sh --skills zed --dry-run`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
SOP installer
  repo:     /Users/imhttran/agentic-workspace/agentic-sop
  bin dir:  /tmp/close003/home/go/bin
  skills:   zed
  plugin:   none
  dry run:  yes (nothing will be changed)

Installing the sop CLI:
  from: /Users/imhttran/agentic-workspace/agentic-sop
  to:   /tmp/close003/home/go/bin/sop
would: mkdir -p /tmp/close003/home/go/bin
would: (cd /Users/imhttran/agentic-workspace/agentic-sop && go build -o /tmp/close003/home/go/bin/sop ./cmd/sop)

Installing the SOP skills (zed):
SOP skills -> zed
  scope: global
  root:  /tmp/close003/home/.agents/skills
would: mkdir -p /tmp/close003/home/.agents/skills
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop /tmp/close003/home/.agents/skills/sop
  linked /tmp/close003/home/.agents/skills/sop -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-prompt /tmp/close003/home/.agents/skills/sop-prompt
  linked /tmp/close003/home/.agents/skills/sop-prompt -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-prompt
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-plan /tmp/close003/home/.agents/skills/sop-plan
  linked /tmp/close003/home/.agents/skills/sop-plan -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-plan
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-review /tmp/close003/home/.agents/skills/sop-review
  linked /tmp/close003/home/.agents/skills/sop-review -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-review
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-diagnose /tmp/close003/home/.agents/skills/sop-diagnose
  linked /tmp/close003/home/.agents/skills/sop-diagnose -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-diagnose
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-test /tmp/close003/home/.agents/skills/sop-test
  linked /tmp/close003/home/.agents/skills/sop-test -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-test
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-implement /tmp/close003/home/.agents/skills/sop-implement
  linked /tmp/close003/home/.agents/skills/sop-implement -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-implement
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-end-to-end /tmp/close003/home/.agents/skills/sop-end-to-end
  linked /tmp/close003/home/.agents/skills/sop-end-to-end -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-end-to-end

done (8 changed, 0 skipped).
  in your agent, type "/" in the message editor to find the SOP commands.

SOP installed to:

  /tmp/close003/home/go/bin/sop

Add this directory to PATH:

  export PATH="/tmp/close003/home/go/bin:$PATH"

Add that line to your shell profile (~/.zshrc, ~/.bashrc, or ~/.profile) to keep it.

Done.
```

### `./install.sh --skills claude --dry-run`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
SOP installer
  repo:     /Users/imhttran/agentic-workspace/agentic-sop
  bin dir:  /tmp/close003/home/go/bin
  skills:   claude
  plugin:   none
  dry run:  yes (nothing will be changed)

Installing the sop CLI:
  from: /Users/imhttran/agentic-workspace/agentic-sop
  to:   /tmp/close003/home/go/bin/sop
would: mkdir -p /tmp/close003/home/go/bin
would: (cd /Users/imhttran/agentic-workspace/agentic-sop && go build -o /tmp/close003/home/go/bin/sop ./cmd/sop)

Installing the SOP skills (claude):
SOP skills -> claude
  scope: global
  root:  /tmp/close003/home/.claude/skills
would: mkdir -p /tmp/close003/home/.claude/skills
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop /tmp/close003/home/.claude/skills/sop
  linked /tmp/close003/home/.claude/skills/sop -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-prompt /tmp/close003/home/.claude/skills/sop-prompt
  linked /tmp/close003/home/.claude/skills/sop-prompt -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-prompt
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-plan /tmp/close003/home/.claude/skills/sop-plan
  linked /tmp/close003/home/.claude/skills/sop-plan -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-plan
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-review /tmp/close003/home/.claude/skills/sop-review
  linked /tmp/close003/home/.claude/skills/sop-review -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-review
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-diagnose /tmp/close003/home/.claude/skills/sop-diagnose
  linked /tmp/close003/home/.claude/skills/sop-diagnose -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-diagnose
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-test /tmp/close003/home/.claude/skills/sop-test
  linked /tmp/close003/home/.claude/skills/sop-test -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-test
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-implement /tmp/close003/home/.claude/skills/sop-implement
  linked /tmp/close003/home/.claude/skills/sop-implement -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-implement
would: ln -s /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-end-to-end /tmp/close003/home/.claude/skills/sop-end-to-end
  linked /tmp/close003/home/.claude/skills/sop-end-to-end -> /Users/imhttran/agentic-workspace/agentic-sop/skills/sop-end-to-end

done (8 changed, 0 skipped).
  in your agent, type "/" in the message editor to find the SOP commands.

SOP installed to:

  /tmp/close003/home/go/bin/sop

Add this directory to PATH:

  export PATH="/tmp/close003/home/go/bin:$PATH"

Add that line to your shell profile (~/.zshrc, ~/.bashrc, or ~/.profile) to keep it.

Done.
```

### `./install.sh --plugin claude --dry-run`

- cwd: `/Users/imhttran/agentic-workspace/agentic-sop`
- revision: `aa50cbbdc8fc47754e4d6f429a9d4accf39882cc`
- toolchain: `go version go1.27.1 darwin/arm64`
- exit code: `0`
- status: `PASS`

Raw output:

```text
SOP installer
  repo:     /Users/imhttran/agentic-workspace/agentic-sop
  bin dir:  /tmp/close003/home/go/bin
  skills:   none
  plugin:   claude
  dry run:  yes (nothing will be changed)

Installing the sop CLI:
  from: /Users/imhttran/agentic-workspace/agentic-sop
  to:   /tmp/close003/home/go/bin/sop
would: mkdir -p /tmp/close003/home/go/bin
would: (cd /Users/imhttran/agentic-workspace/agentic-sop && go build -o /tmp/close003/home/go/bin/sop ./cmd/sop)

Preparing the Claude Code plugin:
  plugin:      /Users/imhttran/agentic-workspace/agentic-sop/integrations/claude
  marketplace: /Users/imhttran/agentic-workspace/agentic-sop/.claude-plugin/marketplace.json

  Claude Code installs plugins from inside Claude Code, so run one of these:

    Load it for one session (no settings change):
      claude --plugin-dir /Users/imhttran/agentic-workspace/agentic-sop/integrations/claude

    Register this checkout as a marketplace, then install the plugin:
      claude plugin marketplace add /Users/imhttran/agentic-workspace/agentic-sop
      claude plugin install sop@agentic-sop

    The same two steps from inside a session:
      /plugin marketplace add /Users/imhttran/agentic-workspace/agentic-sop
      /plugin install sop@agentic-sop

  The plugin exposes the SOP commands as /sop:sop-plan, /sop:sop-review,
  /sop:sop-diagnose, /sop:sop-test, /sop:sop-implement (and /sop:sop), and the same
  names unprefixed (/sop-review, ...) when no other skill claims them. Every one of
  them calls sop prompt, so SOP keeps owning routing, providers, and approval.

SOP installed to:

  /tmp/close003/home/go/bin/sop

Add this directory to PATH:

  export PATH="/tmp/close003/home/go/bin:$PATH"

Add that line to your shell profile (~/.zshrc, ~/.bashrc, or ~/.profile) to keep it.

Done.
```

## 4. The twelve CLI/JEV no-change tests

`gofmt -l .` was empty and `go vet ./...`, `go build ./...`, `go test ./...` and
`go test -race ./...` all exited `0` (raw output in §3). The twelve CLI/JEV tests
that previously reported no-change expectation failures at `d0af5a4` / `1ce9bdb`
pass in the current gate; the CLOSE-003 run diagnosed them as stale test contracts
against the landed mutation-verification semantics, so no test remediation was
needed and no lifecycle safety was weakened.

## 5. Status

- Baseline: all five Go checks PASS; `gofmt -l .` empty; documentation links 0 broken;
  plugin package current; installer dry-runs and `internal/dist`/`internal/skill` tests PASS.
- Deterministic gate: **PASS**.
- No controller repository was read or modified by this record.

