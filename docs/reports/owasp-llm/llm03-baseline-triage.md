# LLM03 baseline triage for the required-categories decision (OWASP gap 9, item 3)

**Status:** triage recorded; **the decision is not made here.** Item 3 of
[PROPOSAL-SCANNER-PIN-POLICY.md](../../plans/PROPOSAL-SCANNER-PIN-POLICY.md) asks
which categories become required. This page gives the operator the baseline's facts.

**Baseline:** the 2026-11-12 scan on darwin arm64 in
[llm03-scanner-binary-pins.md](llm03-scanner-binary-pins.md). Gitleaks 8.30.1 and
OSV-Scanner 2.7.0 were the pinned builds; Semgrep 1.183.0 came from Homebrew. All three
adapters completed with full coverage. 101 finding records: 40 suppressed by the
reviewed baseline, 61 unsuppressed. Verdict FAIL in report-only mode. No finding text
or matched value is recorded here.

## Unsuppressed findings by adapter and class

| Adapter | Severity | Count | Where | Class |
| --- | --- | --- | --- | --- |
| Gitleaks | HIGH | 27 | 9 files: redaction unit tests (`internal/redact/*_test.go`, `internal/notify/`, `internal/cli/` LLM08 tests), the seven-format live test, and two LLM02 reports and a proposal (credential-shaped strings) | **Fixture by design.** The redaction tests need credential shapes as input. Not individually confirmed synthetic: most lines carry no synthetic marker. |
| Semgrep | HIGH | 13 | production: `dangerous-exec-command` at 13 sites (below) | **Needs per-site review.** |
| Semgrep | HIGH | 5 | test files: `toolharness/isolation*_test.go`, `dqharness/replay_exec_test.go` | Test-only exec; expected. |
| Semgrep | MEDIUM | 16 | all test files: ResponseWriter writes, cookie flags, `string-formatted-query`, `unsafe-reflect-by-name` | Test-only; the rules target HTTP handlers and query code, so these are mostly not in production paths. |

### Production `dangerous-exec-command` sites (13)

Five run a command string through `sh -c`. The string's origin decides whether a
finding is a real exposure, so each needs a per-site review before required mode:

- `sh -c`: `internal/agent/command.go:104`, `internal/handoff/caveman/caveman.go:90`,
  `internal/hipcompare/command.go:402`, `internal/review/ocr.go:57`,
  `internal/testrunner/command.go:53`.
- argv or executable, not a shell: `internal/cli/outcome.go:779`,
  `internal/decision/command/command.go:128`,
  `internal/decisionadapter/providers/julia/runner.go:232`,
  `internal/dqharness/replay_exec.go:275`, `internal/github/command.go:33`,
  `internal/ragclient/exec.go:19`, `internal/toolharness/harness.go:920`, and
  `internal/hipcompare/command.go:430`.

`internal/toolharness/harness.go:920` is the agent's `run_command` path, governed by the
command policy and isolation, not by this rule.

## What the required-categories choice would do today

- Making **secrets** required would block on the 27 Gitleaks hits unless the fixture
  files get reviewed, expiring baseline entries or a path exclusion. The repo's
  existing mechanism is the baseline file (`.agent-sdlc/security-baseline.json`).
- Making **sast** required would block on the 18 HIGH Semgrep findings, including the
  13 production exec sites, until each is reviewed.
- **dependencies** (OSV-Scanner) had no findings in the baseline.

## Recommendation

Do not switch to required categories on this baseline. Order:

1. Review the 13 production exec sites, starting with the five `sh -c` sites.
2. Record reviewed, expiring baseline entries for the Gitleaks fixture files and the
   test-only Semgrep findings, so they stop counting, with reasons.
3. Then choose the required categories against the reduced unsuppressed set. Secrets
   and dependencies are the lower-risk starting points; sast follows once the exec
   sites are reviewed.

The baseline entries and the path choices are the operator's to approve.
