# LLM03 production exec-site review (OWASP gap 9, item 3 input)

**Status:** review recorded; no decision made. This is an input to D4 (required
categories) in [PROPOSAL-SCANNER-PIN-POLICY.md](../../plans/PROPOSAL-SCANNER-PIN-POLICY.md).
It follows the Semgrep `dangerous-exec-command` findings in
[llm03-baseline-triage.md](llm03-baseline-triage.md) (PR #464). Read-only review of
`origin/main` on 2026-11-12; nothing was changed.

**Question:** can text written by the model reach a shell or an unchecked executable?

**Answer:** no, on the paths reviewed. The model reaches `exec` only through the
`run_command` allowlist. Every `sh -c` site takes an operator-supplied string.

## The five `sh -c` sites

| Site | Command string comes from | Model-reachable? |
| --- | --- | --- |
| `internal/agent/command.go:104` | `EnvAgentCommand`, the operator's environment | No |
| `internal/review/ocr.go:57` | `EnvReviewCommand`, the operator's environment | No |
| `internal/hipcompare/command.go:402` | `--executor-command`, the operator's CLI flag | No |
| `internal/testrunner/command.go:53` | `build`, `test` and `lint` in the repository's `.agent-sdlc/config.yaml` | No. The harness denies writes under `.agent-sdlc`, so a change needs a reviewed commit. |
| `internal/handoff/caveman/caveman.go:90` | `caveman.New`, which has no non-test caller | Not reachable from production code today |

## The eight argv sites

| Site | Argv comes from | Model-reachable? |
| --- | --- | --- |
| `internal/toolharness/harness.go:920` | The model's `run_command` call | Yes, but `commandpolicy.Classify` must allow it before `exec`, and the sandbox applies when isolation is on |
| `internal/cli/outcome.go:779` | Acceptance checks from the plan and configuration | No |
| `internal/decision/command/command.go:128` | `New(name, argv)` from configuration | No |
| `internal/decisionadapter/providers/julia/runner.go:232` | The Julia command from configuration or environment | No |
| `internal/ragclient/exec.go:19` | An absolute executable path from configuration | No |
| `internal/github/command.go:33` | Fixed `git` and `gh` commands with no shell. Arguments may include PR text. | No shell is involved, so no command injection |
| `internal/dqharness/replay_exec.go:275` | A fixed sandbox wrapper (`sandbox-exec` or `bwrap`). **Where the replay arguments come from was not traced.** | Not traced |
| `internal/hipcompare/command.go:430` | Operator-supplied, the same as the `sh -c` site above | No |

## Open gaps in this review

- The argument origin of `internal/dqharness/replay_exec.go` was not traced. Trace it
  before relying on this review for that site.
- The `run_command` allowlist was read, not tested against adversarial argv.

## Relation to D4

These sites do not block requiring `sast` on their own. The 18 HIGH Semgrep findings
still block it until the fixture and test-only hits are baselined, which the operator
decides under D4.
