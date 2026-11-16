# Git output integrity — repository-configured helpers and manipulated output

**Status:** `IMPLEMENTED` / `VERIFIED` locally on `darwin arm64` for the mechanisms
listed below; **NOT MERGED**. The clean/smudge-filter residual is `OPEN` and is
**not** accepted. The wider containment proposal (M1 + G3 + G4) is **not approved**
and not implemented here.

**Revision:** `origin/main` at `c55b42ff` (2026-11-16), darwin arm64, Go 1.27.1,
Git 2.50.1 (Apple Git-155).

This report addresses the review finding recorded against
[PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md](../../plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md):
_"sandboxing contains configured Git helpers but does not guarantee unmodified Git
output."_ Sandbox containment bounds what a helper can reach; it does not stop a
helper from returning whatever it chooses, and it never runs at all on the
operator override. This change does not rely on containment for output integrity.

## 1. What was wrong

Git executes programs named by repository configuration and by the ambient
environment whenever it reads the working tree. SOP reaches git on two paths, both
of which inherited SOP's full environment and passed no hardening options:

| Path                 | Caller                                                         | Subcommand                    |
| -------------------- | -------------------------------------------------------------- | ----------------------------- |
| model-facing tools   | `ToolGitStatus`, `ToolGitDiff`                                 | `git status`, `git diff`      |
| per-turn observation | `WorkingTreeFingerprint`, `ChangedSince`, `WorkingTreeChanged` | `git status`, `git diff HEAD` |

The model can write `.git/config` and `.gitattributes`, and the operator override
(`SOP_AGENT_RUN_COMMAND_ISOLATION=off` + acknowledgement) runs `run_command`
unsandboxed. Either way, a `SAFE`, fixed-argv git inspection could run a program
of the repository's choosing with SOP's credentials in its environment.

## 2. Measured vectors and their switches

Each row was measured on Git 2.50.1 with a harmless `#!/bin/sh` canary that appends
to a marker file. "Bare" is `git status`/`git diff` with no options; "hardened" is
the argv SOP now builds.

| Vector               | Where it is configured                        | Bare                 | Hardened           | Switch                    |
| -------------------- | --------------------------------------------- | -------------------- | ------------------ | ------------------------- |
| external diff driver | `diff.external`                               | executes             | blocked            | `--no-ext-diff`           |
| external diff driver | `GIT_EXTERNAL_DIFF` env                       | executes             | blocked            | `--no-ext-diff`           |
| fsmonitor hook       | `core.fsmonitor`                              | executes on `status` | blocked            | `-c core.fsmonitor=false` |
| textconv driver      | `diff.<driver>.textconv` + `diff=` attribute  | executes             | blocked            | `--no-textconv`           |
| **clean filter**     | `filter.<driver>.clean` + `filter=` attribute | executes             | **still executes** | none exists               |

The last row is the residual: git exposes no global switch to disable a filter
driver, because the driver name is chosen by the repository. `-c
filter.<name>.clean=` cannot be applied in advance. This matches the earlier
measurement in the D4-5 packet and is why G2 (per-invocation flags) was never a
boundary.

## 3. What the change does

`internal/toolharness/git_integrity.go`:

- `hardenedGitArgs` builds the argv for every SOP-run git command: `-c
core.fsmonitor=false` globally, plus `--no-ext-diff --no-textconv` for `diff`
  (`git status` rejects those two as unknown options — measured). The leading
  `git` is omitted so each caller passes the literal program name and the harness
  keeps one policy-reviewed `exec.CommandContext` site instead of multiplying it.
- `gitChildEnv` drops the variables that inject configuration or an executable
  (`GIT_EXTERNAL_DIFF`, `GIT_CONFIG`, `GIT_CONFIG_COUNT`,
  `GIT_CONFIG_PARAMETERS`, `GIT_CONFIG_KEY_*`, `GIT_CONFIG_VALUE_*`,
  `GIT_ASKPASS`, `SSH_ASKPASS`, `GIT_PAGER`, `PAGER`). This is defence in depth
  for the flags: an ambient profile or CI variable cannot re-add a mechanism the
  flags do not know about. It is **not** a credential scrub for git children and
  does not claim to be one.
- `activeGitFilters` probes `git check-attr -z filter -- <paths>` for the paths a
  result named, and `filterOutputNote` appends a labelled line to the tool result
  when a driver is applied: the content is repository-transformed and "is not
  guaranteed to match the files on disk". The label is a statement of reduced
  trust, not a fix.

Applied at `git`, `WorkingTreeChanged`, and `execFingerprintCmd`.

## 4. What this change does not do

- It does **not** sandbox git. G3 (sandbox every model-reachable git subprocess)
  is part of the unapproved M1 + G3 + G4 change.
- It does **not** deny model writes under `.git/` (G4, also unapproved). A model
  can still add a filter driver; the result is now labelled, not prevented.
- It does **not** disable clean filters, because git offers no switch.
- It does **not** change the operator override, `security.mode`,
  `required_categories`, or any baseline entry. The scan is unchanged.
- It does **not** make `git` output trustworthy in general. Section 2 is the
  complete list of what is now prevented; everything else is unchanged.

### Residuals that remain open

1. **A clean filter runs on `status` and `diff`.** It can transform the reported
   content, and a hostile filter can under-report changes. That would make a
   working-tree fingerprint blind to a modification. The fingerprint is not
   changed by this work: making it fail closed whenever a filter is configured
   would disable mutation detection on every `git-lfs` repository, which is a
   behaviour change to an approved control and needs an operator decision.
2. **`git diff`/`git status` output is still repository-shaped**, not raw file
   bytes. Only the label tells the model so.
3. Detection is bounded (100 paths per probe, only the paths git already
   reported), best-effort (a probe failure reports no driver and must not be read
   as "no filter"), and covers `filter` only — not a future git execution
   mechanism.
4. The OS sandbox residuals recorded elsewhere (pre-existing hardlink aliases,
   the macOS `kern.procargs2` parent-environment read, host-to-sandbox Unix
   sockets) are unchanged.

## 5. Verification

Focused tests in `internal/toolharness/git_integrity_test.go`. Every execution test
pairs a **positive control** — the canary is proven to run under plain `git` — with
the assertion that the harness path does not run it. A "no marker" assertion alone
would pass on a broken canary and is not used.

| Test                                             | Asserts                                                                |
| ------------------------------------------------ | ---------------------------------------------------------------------- |
| `TestHardenedGitArgsNeutralisesHelperMechanisms` | argv shape for `status`, `diff`, `diff HEAD -- .`                      |
| `TestGitChildEnvDropsInjectionVariables`         | injection variables dropped, unrelated ones kept                       |
| `TestParseFilterAttrs`                           | `check-attr -z` parsing; "unspecified"/"unset" ignored                 |
| `TestGitTouchedPathsHandlesStatusAndDiff`        | strict path extraction from porcelain and diff headers                 |
| `TestGitStatusDoesNotRunRepositoryFsmonitor`     | `core.fsmonitor` canary: plain `status` runs it, `git_status` does not |
| `TestGitDiffDoesNotRunExternalDiffDriver`        | `diff.external` canary                                                 |
| `TestGitDiffDoesNotRunTextconvDriver`            | textconv canary                                                        |
| `TestGitDiffDropsExternalDiffEnvironment`        | `GIT_EXTERNAL_DIFF` canary                                             |
| `TestGitOutputLabelsActiveCleanFilter`           | the residual is labelled on `status` and `diff`                        |
| `TestGitOutputUnlabelledWithoutFilters`          | negative control: no notice on an ordinary repository                  |
| `TestFingerprintStillObservableWithHardening`    | fingerprints still detect a real change                                |

Positive control for the change itself: with `hardenedGitArgs` reverted to a
pass-through, `TestGitStatusDoesNotRunRepositoryFsmonitor`,
`TestGitDiffDoesNotRunExternalDiffDriver`, and `TestGitDiffDoesNotRunTextconvDriver`
all fail with "executed the repository-configured …". They pass with the change.

Commands and results (this revision, darwin arm64):

```
go build ./...                                                  # ok
go vet ./internal/toolharness/                                  # ok
go test ./internal/toolharness/ -count=1                        # ok  33.5s
go test ./internal/ollamaagent/ -count=1                        # ok  25.5s
go test ./internal/agent/ ./internal/commandpolicy/ ./internal/security/ -count=1   # ok
```

Full CI on the pull-request head `f0d8d399` passed **13 of 13** checks: the
`fmt, vet, build, test` job, the race detector, the real Semgrep + Gitleaks +
cargo-audit + OSV integration job, installer/plugin packaging, the Windows
`install.ps1` job, the tagged evidence and operator acceptance checks, and the
CodeQL analyses (runs
[43924409662](https://github.com/imhttran/agentic-sop/actions/runs/43924409662),
[43924403486](https://github.com/imhttran/agentic-sop/actions/runs/43924403486)).
That makes the CI evidence real rather than local-only; the change is still
**unmerged**.

Scanner, pinned adapters, after the change:

```
Gitleaks 8.30.1 (pinned binary ba52fb1b...), Semgrep 1.183.0, OSV-Scanner 2.7.0
verdict FAIL, coverage COMPLETE
findings 101, suppressed 84, unsuppressed 17, blocking 1
```

The single blocking finding is still `dangerous-exec-command` at the harness's one
`exec.CommandContext` call site, on the reviewed baseline: this change adds no
call site and no suppression.

## 6. Operator decisions this work surfaces

| #   | Decision                                                                      | Why it cannot be settled here                                                                                                                                         |
| --- | ----------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Approve M1 + G3 + G4 (sandbox every model-reachable exec; deny `.git` writes) | It changes the approved override and removes `git_status`/`git_diff` where no backend exists. Exact wording is in the proposal §11.                                   |
| 2   | Decide the clean-filter fingerprint policy                                    | Making fingerprints fail closed on a configured filter would disable mutation detection on `git-lfs` repositories. That is a behaviour change to an approved control. |

Until item 1 is approved, `git` remains unsandboxed and the Semgrep finding remains
blocking. Until item 2 is decided, residual 1 above stays open and unaccepted.
