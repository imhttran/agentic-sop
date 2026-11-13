# LLM03 D4 reconciliation — independent reproduction of the 45 blocking findings

**Status:** evidence report, read-only. **No decision is made here, no baseline entry is
added, and no enforcement setting is changed.** This report independently reproduces and
reconciles the D4 packet. Where it differs from the merged packet, the difference is
stated as a finding for the operator, not applied as a change.

> **Superseded detail (2026-11-13).** The reproduction below is preserved as recorded at
> `bd2e0d45`. Later merged work moved the residual from `harness.go:920` to `:926`, so the
> current blocking remainder is that line with fingerprint
> `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771`. The current
> full-workspace report-only baseline is 101 findings, 40 suppressed and 45 blocking. The
> five additional Gitleaks findings that the decision documents introduced by quoting
> placeholder shapes were redacted, so they no longer exist. Use the current values, not
> the `:920` references below, for any approval.

**Inputs reconciled:**

- [PROPOSAL-LLM03-D4-ENFORCEMENT.md](../../plans/PROPOSAL-LLM03-D4-ENFORCEMENT.md) (PR #467)
- [PROPOSAL-LLM03-D4-DECISION-MATRIX.md](../../plans/PROPOSAL-LLM03-D4-DECISION-MATRIX.md) (PR #468)
- [llm03-baseline-triage.md](llm03-baseline-triage.md) (PR #464)
- [llm03-exec-site-review.md](llm03-exec-site-review.md) (PR #466, historical; six-site count supersedes its five)

**Revision:** clean `origin/main` at `bd2e0d4514d72e775f23da0d28c7d13bb677838e`
(2026-11-12), darwin arm64, Go 1.27.1, pinned Gitleaks 8.30.1 and OSV-Scanner 2.7.0,
Semgrep 1.183.0, rules pin `d90ff1b1729b740b1754f6b6ba8d1c9da8525136aa4a2345797f43e770fc5958`.

## 1. Reproduction

The recorded baseline reproduces exactly. Two preconditions fail on a machine that looks
ready, and both are the controls working as intended rather than defects:

| Attempt | Condition                                                                  | Result                                                                                                                                                          |
| ------- | -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1       | Homebrew Gitleaks 8.30.1 on `PATH`                                         | Gitleaks `FAILED`: `binary pin mismatch: … sha256 f414bc2f…`. OSV-Scanner had no offline database in the fresh worktree. Verdict `FAIL`, coverage `UNKNOWN`.    |
| 2       | Release-archive Gitleaks extracted, passed via `SOP_SECURITY_GITLEAKS_BIN` | Still `FAILED`: that variable is consumed by the integration tests only; the CLI resolves `command: [gitleaks]` through `PATH`.                                 |
| 3       | Pinned release binary first on `PATH`, rules tree and OSV database staged  | **Reproduced:** 101 findings, 40 suppressed, 61 unsuppressed, **45 blocking**, verdict `FAIL` in report-only mode; all three adapters `COMPLETED` / `COMPLETE`. |

Verified release pins: archive `sha256 b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5`
matches `scripts/checks/security-integration.sh`; extracted binary
`sha256 ba52fb1bfabbcde42f032afad3d6e0b19dff8ed105229a16e7caa338bbc0e84f` matches the committed
`darwin/arm64` pin. The Homebrew build has a different digest, so a developer with
`brew install gitleaks` gets a fail-closed `FAILED`, not a silent pass.

Reproduction (not committed): `.agent-sdlc/runs/gap9-d4-verify3/security/scan.json`.

## 2. The 45 blocking findings reconcile exactly

Mechanical comparison of the scan against decision-matrix §1:

| Check                                                               | Result                                       |
| ------------------------------------------------------------------- | -------------------------------------------- |
| Declared rows parsed                                                | 45                                           |
| Declared fingerprints absent from the scan                          | 0                                            |
| Scan blocking fingerprints absent from the matrix                   | 0                                            |
| Duplicate declared fingerprints                                     | 0                                            |
| Fingerprints recomputed from `(provider, rule, path, line, column)` | 45/45 match                                  |
| `rule_id`, adapter, and `path:line` agree                           | 45/45 match                                  |
| Adapter split                                                       | 27 Gitleaks HIGH, 18 Semgrep HIGH            |
| Decisions                                                           | 44 **B**, 1 **R**                            |
| Expiries                                                            | 32 at 2027-06-30, 12 at 2027-03-31, 1 `OPEN` |

Severity census: 101 total (71 HIGH, 30 MEDIUM), 40 suppressed (all existing entries,
expiry 2027-06-30), 61 unsuppressed, 45 at a blocking severity. The 16 unsuppressed MEDIUM
findings are all in `_test.go` files and do not block under `fail_on: [critical, high]`.

The 5 test-only Semgrep HIGH findings are in the functions the matrix names
(`probeIsolation`, `TestLinuxIsolationCancellationKillsChild`, `needSandbox`,
`sandboxedOutput`, `TestIsolateScrubsSecretEnv`).

## 3. Corrected shell-execution inventory: six sites, confirmed

Every production site was read at the flagged line. The six `sh -c` sites are:

1. `internal/agent/command.go:104`
2. `internal/handoff/caveman/caveman.go:90`
3. `internal/hipcompare/command.go:402`
4. `internal/hipcompare/command.go:430` — the site #466 classified as argv; it is `sh -c`
5. `internal/review/ocr.go:57`
6. `internal/testrunner/command.go:53`

The remaining seven sites build a structured argv:
`internal/cli/outcome.go:779`, `internal/decision/command/command.go:128`,
`internal/decisionadapter/providers/julia/runner.go:232`, `internal/dqharness/replay_exec.go:275`,
`internal/github/command.go:33`, `internal/ragclient/exec.go:19`,
`internal/toolharness/harness.go:920`.

The six-site count in #467 and #468 is correct. The five-site count in #466 is superseded
and is preserved as history.

## 4. The 27 Gitleaks findings are individually synthetic

The triage recorded that most fixture lines carry no synthetic marker. All 27 matched
values were inspected and classified by value shape without reproducing any value:

- **One shared AWS-shaped constant appears in 6 findings** (`ASIA…`, 20 characters, one
  fixed digest across the proposal, the campaign JSON, the Linux live test, and two unit
  test files). It is a sequential placeholder, not a credential.
- **One shared Stripe-shaped constant appears in 6 findings** (`sk_test_…`; the fixture is
  labelled `stripe-test`).
- **Sequential placeholders:** a GitLab-PAT shape (3 findings), a Stripe live-key shape
  (1), and the four placeholders in the proposal's "Fixture" column (an AWS key shape, a
  Stripe webhook shape, a Docker token shape, and an age key shape). The literal values are
  not reproduced: the report describes the shape and points at the proposal, which is where
  the reviewed fixtures live.
- **Explicitly synthetic canaries:** `internal/notify/llm08_notification_secrets_test.go`
  and `internal/cli/llm08_notification_secrets_test.go`; the former's comment states the
  value "is not a real secret".
- **`internal/redact/scrub_test.go:38` is a false positive, not a fixture:** the matched
  text is a natural-language phrase (`secretary=…`) with no credential shape.
- The redaction files flag input rows of `[]struct{ name, in, want string }` tables whose
  `want` is `[redacted]`; they are the input side of the redaction policy.

No matched value is a live credential, and none is reproduced in this report.

## 5. Finding not in the merged packet: 16 baseline entries are stale

The committed baseline has 56 entries. **16 of them match no current finding.** They name
the 13 production exec sites (three sites carry two entries each) under the older
fingerprints from the 2026-11-03 bulk triage and the 2026-11-08 individual reviews:

| Observation                                                                                                                     | Consequence                                                                                                                                        |
| ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| The production sites were **already reviewed and suppressed** under the prior baseline regime.                                  | The 12 production **B** entries are a re-affirmation of prior dispositions with refreshed fingerprints, not a first acceptance of unreviewed risk. |
| `Baseline.Apply` diagnoses _expired_ entries but is silent about an entry that matches nothing.                                 | An entry becomes inert without notice whenever a fingerprint drifts. That is why 13 reviewed sites reappeared as blocking.                         |
| The stale entries expire 2027-06-30; the matrix proposes 2027-03-31 for the refreshed production entries.                       | The proposal tightens the prior window by three months. This is a tightening, and should be approved as one.                                       |
| The bulk reason ("argv is a constant, operator configuration, or a validated payload") is inaccurate for the six `sh -c` sites. | The per-site reasons in matrix §2 are a substantive correction and should replace the bulk text.                                                   |

This is a baseline-hygiene gap, not a security regression: a drifted entry fails **open to
the scanner** (the finding reappears and blocks), never closed to the gate.

One reference defect was also found while verifying the residual's test coverage: matrix §5
cites `TestRunCommandIsolationUnsupportedPlatforms`, which does not exist. The behavior is
covered by `TestIsolationDecisionUnsupportedPlatforms` and
`TestRunCommandIsolationOnFailsClosedWithoutBackend`. The claim holds; the test name does not.

## 6. Enforcement options, measured

The projection below uses the real `security.EvaluatePolicyWithDetection` with the recorded
findings and the policy this repository declares (`fail_on: [critical, high]`,
`unknown_severity: indeterminate`, workspace scope). The harness was temporary and is not
part of this change.

| Configuration                                                         | Verdict | Blocking | Reading                                                                                                                                                   |
| --------------------------------------------------------------------- | ------- | -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Report-only, as declared                                              | `FAIL`  | 45       | Delivery is not blocked; findings are visible. This is today.                                                                                             |
| **Option B** — required `secrets` + `dependencies`                    | `FAIL`  | **45**   | **Identical to C without dispositions.** Required categories add coverage obligations; they do not narrow which findings the enabled adapters contribute. |
| Option C — required `secrets` + `dependencies` + `sast`               | `FAIL`  | 45       | Blocks until the dispositions are approved.                                                                                                               |
| Option C + the 44 proposed entries                                    | `FAIL`  | **1**    | The single remainder is `internal/toolharness/harness.go:920`.                                                                                            |
| Option C + 44 entries + residual entry (R1)                           | `PASS`  | 0        | `COMPLETE` coverage, `no unsuppressed blocking findings`.                                                                                                 |
| Positive control: previous row + one new synthetic HIGH at a new path | `FAIL`  | 1        | A genuine new finding still blocks. The baseline is not a blanket.                                                                                        |

**Option B is dominated by Option C.** B blocks on the same 45 findings; making B pass
requires either the same 44 dispositions (so the extra `sast` obligation costs nothing) or
disabling the `sast` category, which removes coverage of the very exec, subprocess, and
file-handling code this gap exists to review. There is no configuration in which B is
stronger than C.

## 7. Where the gate actually binds

`security.mode: required` is enforced by the SOP **quality stage**
(`internal/quality/quality.go`: `PASS` passes, `FAIL` and `INDETERMINATE` fail closed), fed
by `internal/cli/security_gate.go`. **GitHub Actions does not run `sop security scan` on the
repository.** `.github/workflows/ci.yml` runs `scripts/checks/security-integration.sh`,
which provisions the pinned scanners and runs the scanner integration tests.

So the "CI impact" column in the merged packet should be read as **SOP lifecycle impact**.
Enabling Option C gates `sop run`; it does **not** add a GitHub CI gate. Adding one is a
separate change (a workflow step invoking `sop security scan --required`), and it would
inherit the D5 version drift below.

## 8. D5 now matters for D4

CI provisions Semgrep **1.136.0** (`scripts/checks/security-integration.sh`,
`scripts/security/semgrep-requirements.lock`), while the tracked config declares **1.183.0**.
The declared version is descriptive for Semgrep (there is no `binary_sha256`), so this does
not fail today.

The 45 fingerprints were produced by 1.183.0. Fingerprints are `(path, line, column)`
based, so an engine or rule-evaluation difference between the two versions can change the
set. **If a CI or lifecycle host runs 1.136.0 against a baseline approved for 1.183.0, the
gate can block on findings the operator already reviewed, or fail to match entries they
approved.** D5 is therefore a stability prerequisite for the Option C gate, not an
independent nicety.

## 9. One production entry has a non-suppression alternative

`internal/handoff/caveman/caveman.go:90` is entry 7 of the 12. The `caveman` package is
imported by no production file and no other test:

- `handoff.SelectCompressor` accepts a `Candidate`, but nothing constructs the Caveman
  adapter (`handoff/caveman` has no importer outside its own directory).
- `caveman.New` therefore has no non-test caller, and `ShellRunner` — the `sh -c` site — is
  unreachable from any shipped path.

Matrix §4 states that no narrow, policy-neutral fix removes any of the 45 findings. That is
correct for the other 44 but not for this one: **deleting an unimported package** removes
the finding without suppressing it, without weakening a control, and without changing any
reachable behavior. It is still a capability removal at the API and mode
(`handoff.ModeCaveman`) level, so it needs an operator choice, not an automatic fix. It is
offered as an alternative to a 12th baseline entry, not applied here.

## 10. What this report does not establish

- No Windows scan evidence exists; the pins exclude Windows and `run_command` refuses there.
  This assessment ran on darwin arm64.
- The `noexec` temp-mount path was not reproduced; it is expected to surface as
  `ErrUnavailable` and therefore `INDETERMINATE`, not a bypass.
- No baseline entry, `security.mode`, `required_categories`, scanner pin, or exclusion was
  changed by this report.
- No approval, waiver, or residual acceptance is recorded here.
