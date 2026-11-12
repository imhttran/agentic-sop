# D4 enforcement readiness — LLM03 scanner categories (OWASP gap 9)

**Status:** PROPOSAL, read-only. **No decision is made here, and no configuration,
baseline entry, or enforcement default is changed.** D1 is approved and implemented
(PR #461); the rules-staging follow-up is merged (PRs #462/#463); D2 is deferred
(PR #465); D3 and D5 are unchanged. This packet gives the operator the facts needed to
decide **D4**: keep report-only, or require security categories.

**Baseline:** clean `origin/main` at `6ff4ee4b` (2026-11-12), darwin arm64, pinned
Gitleaks 8.30.1 and OSV-Scanner 2.7.0, Semgrep 1.183.0 (Homebrew, not pinned), pinned
Semgrep rules `d90ff1b1…`, freshly provisioned Go and crates.io OSV databases. The scan
reproduced the recorded baseline exactly: **101 findings, 40 suppressed, 61
unsuppressed, 45 unsuppressed at a blocking severity**, verdict `FAIL` in report-only
mode. All three adapters were `COMPLETED` with `COMPLETE` coverage.

Reproduction: with the pinned Gitleaks 8.30.1 and OSV-Scanner 2.7.0 first on `PATH`,
Semgrep 1.183.0, and the pinned rules, run
`sop security scan --report-only --run-id gap9-d4-baseline` on clean `origin/main`.
Local evidence: `.agent-sdlc/runs/gap9-d4-baseline/security/scan.json` (not committed).
Related records: [llm03-baseline-triage.md](../reports/owasp-llm/llm03-baseline-triage.md),
[llm03-scanner-binary-pins.md](../reports/owasp-llm/llm03-scanner-binary-pins.md),
[PROPOSAL-SCANNER-PIN-POLICY.md](PROPOSAL-SCANNER-PIN-POLICY.md).

## 1. Verified current scanner state

- `security.mode: report-only`, `security.required_categories: []`.
- Enabled adapters: Gitleaks (secrets, repository-wide), Semgrep (sast, Go/Rust rules),
  OSV-Scanner (dependencies, Go/Rust).
- Blocking policy: `fail_on: [critical, high]`, `unknown_severity: indeterminate`.
- Required-mode readiness is `false` because no required category is configured.
- **Important policy fact:** required categories add coverage requirements; they do
  not limit which findings are evaluated. While the Semgrep adapter is enabled, its
  HIGH findings count toward `FAIL` even under Option B (`secrets` + `dependencies`).
  Option B does not bypass the SAST findings unless the SAST category is disabled or
  those findings are baselined.
- **Correction to the earlier triage:** the triage says five production sites use
  `sh -c`. There are **six**: `internal/hipcompare/command.go:430` is also
  `exec.CommandContext(ctx, "sh", "-c", vc)`. The remaining seven production sites
  execute a structured argv with no shell.

## 2. Review of all 13 production `dangerous-exec-command` findings

The rule flags any non-static command, so a finding is not by itself an injection
defect. Each row states why the code is intentional and whether the shell-injection
vector is actually present. "Justified" below means the site is intentional and the
untrusted-input-to-shell path is absent; it does not mean the exec capability is
harmless.

| # | Site | Construction | Source / trust | Shell? | Validation / isolation | Assessment | Disposition |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | `internal/agent/command.go:104` | `sh -c a.command`; request JSON on stdin | Operator-configured agent command (`SOP_AGENT_COMMAND`/config) | Yes | Request validation; timeout; process-group kill. Normal agent execution is not sandboxed. | Justified use; no model data is interpolated into the command string. The exec capability is the operator's chosen agent. | Fingerprint baseline, expiry 2027-03-31; revalidate if the command source or any interpolation changes. |
| 2 | `internal/cli/outcome.go:779` | `argv[0] argv[1:]…` | Operator-declared acceptance checks | No | `commandpolicy.Classify` must be `SAFE`; `rejectDangerousForm` rejects tooling-injection flags, `go fmt`, and `git -c`/`--config-env`; process group; 30-minute bound | Justified use; no shell. A malicious spec could run an authorized command, but the spec is operator/task input and is policy-checked before execution. | Fingerprint baseline, expiry 2027-03-31; revalidate on acceptance-policy changes. |
| 3 | `internal/decision/command/command.go:128` | `a.argv[0] a.argv[1:]…`; request JSON on stdin | Operator-configured decision provider | No | Argument vector validated at construction; timeout; process group | Justified use; no shell; no untrusted interpolation. | Fingerprint baseline, expiry 2027-03-31. |
| 4 | `internal/decisionadapter/providers/julia/runner.go:232` | `r.Command[0] r.Command[1:]…`; input JSON on stdin | Operator-configured provider command | No | Non-empty command check; timeout; process group; scrubbed child environment | Justified use; no shell; no untrusted command construction. | Fingerprint baseline, expiry 2027-03-31. |
| 5 | `internal/dqharness/replay_exec.go:275` | `iso.Tool` with sandbox arguments; fixture verification argv is embedded | Replay fixture (operator/repo-authored) + selected sandbox backend | No (launcher is argv) | Seatbelt or bubblewrap required; unknown backend fails closed; scrubbed environment; bounded timeout | Justified use; the finding is the sandbox launcher, and the fixture command runs inside it. Not a shell-injection path. | Fingerprint baseline, expiry 2027-03-31; revalidate if a non-sandboxed replay path is added. |
| 6 | `internal/github/command.go:33` | `gh`/`git` with structured args | Internal callers (PR/issue numbers, repository paths) | No | `procgroup.Bind`; command-specific argument construction; prompt disabled | Justified use; no shell. Injection would require a caller to pass attacker-controlled program/args, which the adapter does not do. | Fingerprint baseline, expiry 2027-03-31. |
| 7 | `internal/handoff/caveman/caveman.go:90` | `sh -c compressCommand`; JSON input on stdin | Operator-configured compression command | Yes | Process group; input on stdin | Justified use; shell is required by the operator command syntax; no untrusted interpolation. | Fingerprint baseline, expiry 2027-03-31. |
| 8 | `internal/hipcompare/command.go:402` | `sh -c c.opts.ExecutorCommand` | Operator-provided executor command | Yes | Worktree confinement for instruction files; process group; bounded output; environment | Justified use; the executor command is operator configuration, not model text. | Fingerprint baseline, expiry 2027-03-31. |
| 9 | `internal/hipcompare/command.go:430` | `sh -c vc` | Manifest's trusted verification commands, identical for both arms | **Yes** (the triage misclassified this as argv) | Manifest is the reviewed input; worktree; process group; bounded output; acceptance requires them | Justified use; shell is required for common verification syntax (`go test ./...`); no untrusted interpolation. | Fingerprint baseline, expiry 2027-03-31; keep the manifest review as the control. |
| 10 | `internal/ragclient/exec.go:19` | `executable` with no args; request JSON on stdin | Operator-configured absolute executable | No | Process group; bounded stdin/stdout/stderr | Justified use; no shell; the executable is operator configuration. | Fingerprint baseline, expiry 2027-03-31. |
| 11 | `internal/review/ocr.go:57` | `sh -c p.command`; request JSON on stdin | `SOP_REVIEW_COMMAND` environment | Yes | Process group; input on stdin; bounded output | Justified use; operator environment is trusted; no untrusted interpolation. | Fingerprint baseline, expiry 2027-03-31. |
| 12 | `internal/testrunner/command.go:53` | `sh -c command` | Project validation commands (`build`/`test`/`lint`) | Yes | Command is trusted project configuration; process group; timeout; bounded output | Justified use; shell is required for common validation syntax; the command is never built from model output. | Fingerprint baseline, expiry 2027-03-31. |
| 13 | `internal/toolharness/harness.go:920` | Direct argv from `SplitCommand`; no shell | **Model-controlled `run_command`**, constrained by command policy + sandbox | No | Shell metacharacters/globs rejected; `commandpolicy.Classify` must be `SAFE`; default-on Seatbelt/bubblewrap; Windows refuses; audit trail | **True exec capability with strong mitigations.** This is not a false positive. The residual returns if the operator disables isolation on macOS/Linux, or if a platform has no backend (Windows refuses by design). | **Do not blanket-baseline.** Either keep SAST report-only (Option A), or add one fingerprint entry with a short expiry (2027-03-31) whose reason cites the default-on isolation and command-policy tests; revalidate on any `commandpolicy`, isolation, or `run_command` change. |

### Fingerprints for the 13 production sites

| Site | Fingerprint |
| --- | --- |
| `internal/agent/command.go:104` | `semgrep:60898e80cd5c2bc481c1929520ff2a58292a4bd4bc50d06cce1553701a9b7781` |
| `internal/cli/outcome.go:779` | `semgrep:4c5c524ae8329de9fb052b337310f9d8ed1156c34a96194668ea43e2654a7728` |
| `internal/decision/command/command.go:128` | `semgrep:bc6bd5cd04fa74b928093ebbe7c45d48447cd59af4a39cfaab0579fa4a73820e` |
| `internal/decisionadapter/providers/julia/runner.go:232` | `semgrep:049c3e59d957e1d4a28a4954b676810d7aa514eb22bda8aef587116f1188db3c` |
| `internal/dqharness/replay_exec.go:275` | `semgrep:63433ea4f23e9fd5bb730c3c0b26dedf34a3081dba435580b1bc301f8802fe6d` |
| `internal/github/command.go:33` | `semgrep:a057a8819f65f11b35ac5d11a41e7394f53b118e04c6073e90be61d48c4019f1` |
| `internal/handoff/caveman/caveman.go:90` | `semgrep:435864db8eb90bf0f49f6c97e8990a93e1bdb87230425c80b9bf6a9d200768bc` |
| `internal/hipcompare/command.go:402` | `semgrep:d982e8f12bb7c6597513193a4622edaf6a955a626feba0ed8f9198ec406560be` |
| `internal/hipcompare/command.go:430` | `semgrep:5933a67130ae763b9e6d30126cd43e84e2066fc2d2c218b85d32c57366905a16` |
| `internal/ragclient/exec.go:19` | `semgrep:f4506eb456705bd1fd134e7a2d1a79051205e570d70f62577be23702b2f8b71c` |
| `internal/review/ocr.go:57` | `semgrep:fc8287f18275bcdb937f8d3e12dab172076126d3045cc73a997cf5f523780b88` |
| `internal/testrunner/command.go:53` | `semgrep:0f959d4f3632e1c2244db5405f3f0309e58abbb00c08ab0d71b874d2c75ca344` |
| `internal/toolharness/harness.go:920` | `semgrep:2e3c98676c0b89b137c13339ae43b6a5928dae9e7ff2fcf893de77038f473847` |

## 3. Proposed baseline entries

All entries below are **fingerprint-exact**. No path-wide, directory-wide, or
rule-wide suppression is proposed. The existing baseline schema is unchanged
(`fingerprint`, `rule_id`, `path`, `reason`, `owner`, `expires_at`). Nothing is added
in this change.

### 3.1 Gitleaks fixtures and report examples (27 entries)

These files carry credential-shaped strings by design: redaction test inputs, a
seven-format live-campaign artifact with planted canaries, and an LLM02 proposal table
whose "Fixture" column lists example shapes. The scan's `message` fields describe the
rule, not a live credential; no value is reproduced here.

| File | Line | Rule | Fingerprint |
| --- | --- | --- | --- |
| `docs/plans/PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md` | 34 | `aws-access-token` | `gitleaks:542e8c41d5552a91a81d7070b25f4978c67e8c9ccde6d9d194818ef2b7e2d09a` |
| `docs/plans/PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md` | 35 | `stripe-access-token` | `gitleaks:9bce24de528ad1aac6f0b7be3c2ec32290e8254146b8e48727cd99be16576a2a` |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 83 | `aws-access-token` | `gitleaks:332c11915593ec84a395c2cd64eaa59eb77545cd4d4af9448a6b6f6e2a1eabd4` |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 86 | `aws-access-token` | `gitleaks:f32006839b09007ede952842800cf6e5e3438e8d2c404936e403df23e591e5e5` |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 102 | `stripe-access-token` | `gitleaks:b1a51e668cf2e25a1b4a46fb3127b73d0d0d5ca113aa33e7ad942d4ee86234ea` |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 105 | `stripe-access-token` | `gitleaks:c31d51667dc25b605d12faebb0a2b9afb0e8f7b46cd40d0ab4c91e0955dd8d1d` |
| `internal/cli/llm08_notification_secrets_test.go` | 23 | `generic-api-key` | `gitleaks:c68e84fd911325959deb53720896e44ecf6a2a3164dc470b7a68da69a2392e4c` |
| `internal/notify/llm08_notification_secrets_test.go` | 29 | `generic-api-key` | `gitleaks:db3bc94721d39f463b41ec350303a3a3bd47e4fafa91d8ca723298bd23c5aa98` |
| `internal/ollamaagent/llm02_seven_format_linux_live_test.go` | 33 | `aws-access-token` | `gitleaks:35daf7cd697b192e38c7bb0ff5d82dd592b0c2b6e361a466f2b9bf8197af88a9` |
| `internal/ollamaagent/llm02_seven_format_linux_live_test.go` | 34 | `stripe-access-token` | `gitleaks:5024394e9c2eab930589a60820e675678826502a3f04b5924612851e2acf340b` |
| `internal/redact/known_format_enhancement_test.go` | 20 | `aws-access-token` | `gitleaks:1a6feaa4d53f118595851afd37311b9498aeb77b10f7b628f3246dcf9a47fecf` |
| `internal/redact/known_format_enhancement_test.go` | 21 | `stripe-access-token` | `gitleaks:018670038beb44e4d94058bcc9a894ecabc6a70c2add7e15cd2a804126e054f7` |
| `internal/redact/policy_test.go` | 12 | `generic-api-key` | `gitleaks:9a6496cc947aa36105cff143d189ef0f5df1a6ba345a07623a185cb791ce319c` |
| `internal/redact/policy_test.go` | 13 | `generic-api-key` | `gitleaks:1bd172eb44cf97b69bdd68d57e1e1065e86cff947b8fe73fcbb9d19f18e5c5d1` |
| `internal/redact/policy_test.go` | 76 | `gitlab-pat` | `gitleaks:9910e1b901cbbe5cd43ecf1262e0bda8b834a04246bdfa7c2ee941e93e99561b` |
| `internal/redact/policy_test.go` | 84 | `stripe-access-token` | `gitleaks:6278e72f9d4d7130e78109d480ecc6c0993403d84bb176fdf06cd4e799e3bc8f` |
| `internal/redact/policy_test.go` | 85 | `stripe-access-token` | `gitleaks:72987dfb22a55d6f24b93f98a3a36ffc7410c06443e223f320af33a295425862` |
| `internal/redact/scrub_test.go` | 38 | `generic-api-key` | `gitleaks:fd2fd2e2a251b47fce4948677f9f77dc237e0af1d63031f71d4ab6087bed0b6d` |
| `internal/redact/unknown_format_assessment_test.go` | 25 | `gitlab-pat` | `gitleaks:bf5aa4c0629a2eb4d05a03a059370c7b64a25b2bb74aaf39f462bac123802b57` |
| `internal/redact/unknown_format_assessment_test.go` | 33 | `stripe-access-token` | `gitleaks:95a03c2ff32f3a935b4c07f194f26a5730181c036db84e0e57e9ba62ff502bdc` |
| `internal/redact/unknown_format_assessment_test.go` | 39 | `aws-access-token` | `gitleaks:9fee8d1fea5eaa6bf431c1aa807463e37d526af2e82c56a9cd1dbc517f8c43dd` |
| `internal/redact/unknown_format_assessment_test.go` | 40 | `jwt` | `gitleaks:8ba804fb22f81445bcda29f9901a5e87870a43a0694aa2ed8fd7017b2dabb74a` |
| `internal/redact/unknown_format_assessment_test.go` | 47 | `generic-api-key` | `gitleaks:51df64d7eaa35e4f60faedc832ef84c835119afe4e24ad879b0b4651e604b85c` |
| `internal/redact/unknown_format_assessment_test.go` | 52 | `aws-access-token` | `gitleaks:aa717d45db7a1d5e66910d7071b67e3612fa4cad1854c559480bbd23320044e0` |
| `internal/redact/unknown_format_assessment_test.go` | 53 | `stripe-access-token` | `gitleaks:279f4fc447993b022ddd16668b3310e912e5fe821ea34bdb0689edc1ffc55eae` |
| `internal/redact/unknown_format_assessment_test.go` | 116 | `gitlab-pat` | `gitleaks:9c1a8a1484b137ecb944c88c75296d103776d8331ffb6c170a7a4f2dfe27b385` |
| `internal/redact/unknown_format_assessment_test.go` | 149 | `generic-api-key` | `gitleaks:92d6018e06ca21eb02731f20e69e2dfe2db00b0d2633aa209bf386b5945bc3fb` |

**Why expected / evidence.** The redaction tests are the input side of the redaction
policy (`internal/redact`, `internal/notify`, `internal/cli`); `llm08_notification_secrets_test.go`
explicitly calls its value "a synthetic secret value with no credential shape. It is
not a real secret." The campaign JSON stores planted `raw_canaries` and redacted
`model_input`; the proposal is a table of candidate rule *fixtures*. These are test or
evidence fixtures, not shipped credentials.

**Expiry:** 2027-06-30 (matching the existing baseline). **Revalidation:** on any
change to the file, the redaction policy, the Gitleaks version/rules, or when the file
moves out of test/report scope. **Risk if too broad:** a directory- or rule-wide entry
would hide a real credential later pasted into the same report or test file. Use the
exact fingerprints above.

### 3.2 Test-only Semgrep HIGH findings (5 entries)

| File | Line | Rule | Test function | Fingerprint |
| --- | --- | --- | --- | --- |
| `internal/dqharness/replay_exec_test.go` | 105 | `dangerous-exec-command` | `probeIsolation` | `semgrep:ccb4bb42ca610fd2963268b516385bba795ee7c294c4ad0f7968bcd03d25725c` |
| `internal/toolharness/isolation_credential_linux_test.go` | 113 | `dangerous-exec-command` | `TestLinuxIsolationCancellationKillsChild` | `semgrep:fab835e9941bb1ad0ab32bf7e51f0a1f30944f2d500ba43cf844b247c4c35718` |
| `internal/toolharness/isolation_test.go` | 32 | `dangerous-exec-command` | `needSandbox` | `semgrep:11ef53417d1b9eedc8e24e1593bdfa38b6d8df035e3265fce7c61bff2aa3e5c2` |
| `internal/toolharness/isolation_test.go` | 60 | `dangerous-exec-command` | `sandboxedOutput` | `semgrep:fbedd5f6a2ed946cd639a20ce584f6ab65c3788767ea09cbb60e2c08294313b2` |
| `internal/toolharness/isolation_test.go` | 677 | `dangerous-exec-command` | `TestIsolateScrubsSecretEnv` | `semgrep:b54aa7e631411a44e21ab9b190aac37cfd4148fde1f562bd8938897974cb88a5` |

**Why expected / evidence.** All are `_test.go` helpers that start a sandbox or probe
isolation; they cannot run in production. **Expiry:** 2027-06-30. **Revalidation:** on
test-file or isolation-helper changes; remove if a helper becomes production code.
**Risk if too broad:** a path-wide entry would hide a new production exec added to the
same test helper file; use fingerprints.

### 3.3 Test-only Semgrep MEDIUM findings (16 entries, non-blocking)

These do not block under `fail_on: [critical, high]`. They are listed for hygiene and
for a clean required-mode report.

| File | Line | Rule | Function | Fingerprint |
| --- | --- | --- | --- | --- |
| `internal/agent/openaicompatible_execution_test.go` | 52 | `no-direct-write-to-responsewriter` | `captureOpenAICompatibleServer` | `semgrep:2b5a87b2ae0c39c8fef3e980ddd9a2220482dc2cbd66ab8ca29617e3686ba914` |
| `internal/agent/provider_test.go` | 199 | `no-direct-write-to-responsewriter` | `TestOllamaGenerateErrors` | `semgrep:6d67a58248b4b5fa1c6d79b902c58db109e05cd779510e9bd3946f6463a562f4` |
| `internal/agent/provider_test.go` | 291 | `no-direct-write-to-responsewriter` | `TestLlamaCppGenerateErrors` | `semgrep:4b55e86aabfbc3ef7a97493fcdc8d1210c78f7566ebbddc65eaa1835181b57b4` |
| `internal/agent/redacted_test.go` | 62 | `no-direct-write-to-responsewriter` | `TestRedactedBeforeChatTransports` | `semgrep:595e210e46b0ccf6536e2310aa905f4b04815b2e1b0689619c379abc6283d52d` |
| `internal/agent/truncation_test.go` | 19 | `no-direct-write-to-responsewriter` | `truncationServer` | `semgrep:039b18b89012f00823df209a800579666ca9cb4228314bdf37a000d274d2c019` |
| `internal/cli/ollama_e2e_test.go` | 41 | `no-direct-write-to-responsewriter` | `scriptedOllama` | `semgrep:a83133ce6a59c832a5b262c6c618c74190d57eed6d3703d9fa129bd5d76ca3c2` |
| `internal/cli/provider_validation_task_test.go` | 39 | `no-direct-write-to-responsewriter` | `ollamaTagsServer` | `semgrep:149068d334137adbb0e7759815f802fce6ff1ba79397deba9f7ef245aea1213a` |
| `internal/cli/zz_operator_acceptance_acc008_test.go` | 158 | `unsafe-reflect-by-name` | `accOp008Field` | `semgrep:ee77e1747a4ebaca904aadaa06f4f0ceeb73425b745d92f6658caae70f530e73` |
| `internal/controller/web/recovery_test.go` | 474 | `cookie-missing-httponly` | `postWithCSRF` | `semgrep:44bb1ebabebf28e50622eefa37e94f734526cfcc9ba4ab23fd8b0d8909b57dcb` |
| `internal/controller/web/recovery_test.go` | 474 | `cookie-missing-secure` | `postWithCSRF` | `semgrep:7c3c5c9fa90365a786e3b59f9efad0a22fb55c5323c31ae62f7f4138999b9921` |
| `internal/decisionadapter/providers/clef/chat_test.go` | 39 | `no-fprintf-to-responsewriter` | `chatServer` | `semgrep:57df50ac6f6f8010d5dd10cd10656bc6e66c440b1100f641146d1c6451947686` |
| `internal/decisionadapter/providers/nimble/score_test.go` | 77 | `no-io-writestring-to-responsewriter` | `TestScoreTransportMalformed` | `semgrep:512a5c92c463843f83452d6a6c624aeb0f61013f098053d539b83142920df8d4` |
| `internal/ollamaagent/ollama_retry_test.go` | 70 | `no-io-writestring-to-responsewriter` | `newRetryFixture` | `semgrep:f882aaa11e4597979183b30a0151f680f7403d7c983a90dd0c7e32c94a05ecac` |
| `internal/ollamaagent/ollama_retry_test.go` | 73 | `no-io-writestring-to-responsewriter` | `newRetryFixture` | `semgrep:cbd49f285f948561b34651424e857ee0e3fa70d5750b966397faccaa5eaceeca` |
| `internal/provider/openai/provider_locality_test.go` | 24 | `no-io-writestring-to-responsewriter` | `bareModelsServer` | `semgrep:d04540b6ab8321b7dc68737c4cbf671a7f646771e044232e3a4432385c73da41` |
| `internal/store/notifications_test.go` | 40 | `string-formatted-query` | `countRows` | `semgrep:1b660db194071413c2c277c7add47c9d3d4004c36c357f56c54520995f479c5a` |

**Why expected / evidence.** All are test-only HTTP/server or SQL test helpers.
**Expiry:** 2027-06-30. **Revalidation:** on test-file or rule-set changes; remove if a
helper becomes production code. **Risk if too broad:** hides a production HTTP/SQL
defect introduced into the same test package; use fingerprints.

## 4. Expected CI impact

The current scan has 45 unsuppressed blocking findings: 27 Gitleaks HIGH and 18
Semgrep HIGH (13 production + 5 test-only). The 16 MEDIUM findings do not block under
`fail_on: [critical, high]`.

| Option | Required categories | Blocking findings today | CI impact |
| --- | --- | --- | --- |
| **A — report-only** | none | none | No block. The scan still records `FAIL` and 45 blocking findings; the operator sees them but delivery is not blocked. |
| **B — secrets + dependencies** | `secrets`, `dependencies` | **45** (27 Gitleaks + 18 Semgrep) | Blocks on the Semgrep HIGH findings even though `sast` is not required, because the enabled adapter's findings are still evaluated. To make B pass without baselining SAST, the operator must disable the `sast` category (a coverage reduction) — then 27 Gitleaks block until the 27 entries are approved. |
| **C — secrets + dependencies + sast** | `secrets`, `dependencies`, `sast` | **45** until the 27 Gitleaks and 18 Semgrep HIGH entries are approved; then 0 | The strongest gate: all three adapters must be `COMPLETED`/`COMPLETE`, and all 45 blocking findings must be reviewed/baselined. Semgrep coverage is `COMPLETE` for the detected Go source; the rules also declare Rust. The 16 MEDIUM findings remain visible but do not block. |

**Coverage readiness.** On this host all three adapters are `COMPLETED`/`COMPLETE`.
OSV-Scanner needs its offline database; the baseline used a freshly provisioned Go and
crates.io DB and reported no findings. A platform without the pinned binaries or the
DB will fail closed, which is correct but affects CI/local operability.

## 5. Recommendation

**Adopt Option C as the target, after the 45 blocking findings are dispositioned.** The
evidence favors C over B:

- B does not reduce the current blocking surface: the enabled Semgrep adapter still
  produces 18 HIGH findings. Making B pass without baselining SAST would require
  disabling the SAST category, which loses coverage for the very code paths (exec,
  subprocess, file handling) that the repository most needs reviewed.
- The 18 Semgrep HIGH findings are a bounded, reviewable set: 13 production exec sites
  with trusted configuration or a sandboxed model path, and 5 test-only helpers. The
  27 Gitleaks findings are all test/report fixtures.
- C gives a single, coherent gate: secrets, dependencies, and SAST coverage, with
  expiring, fingerprint-exact exceptions where the code is intentionally executing.

**Sequencing (recommended):**

1. Approve the 27 Gitleaks fixture entries and the 5 test-only Semgrep HIGH entries
   (expiry 2027-06-30).
2. Review and approve the 13 production dispositions. Prefer fingerprint entries with
   the reasons in §2; for `internal/toolharness/harness.go:920`, either keep SAST
   report-only until the isolation/policy evidence is accepted, or add a short-expiry
   entry and explicitly accept the residual.
3. Decide whether to also record the 16 MEDIUM test-only entries (not required for the
   gate; recommended for a clean report).
4. Enable Option C with `security.mode: required`,
   `required_categories: [sast, secrets, dependencies]`, and a full workspace scope.
5. Re-run the clean scan and `sop security inspect`; record the required-mode evidence.

If the operator wants an immediate gate before the 13 production sites are reviewed,
choose **Option A** (keep report-only), not B: B blocks on the same Semgrep findings
while creating the impression of a narrower gate.

## 6. Required operator approvals

- **D4 option:** A, B, or C. Recommendation: C after the dispositions above.
- **Baseline entries:** approve the exact fingerprint sets in §3.1 and §3.2, and the
  production dispositions in §2. No entry is added by this proposal.
- **Expiry:** approve 2027-06-30 for fixtures/test-only and 2027-03-31 for production
  exec sites, or set a shorter period.
- **`run_command` residual:** for `internal/toolharness/harness.go:920`, explicitly
  accept the bounded residual (default-on sandbox + command policy; the override on
  macOS/Linux and the Windows refusal are the boundaries).
- **Enforcement change:** approve `security.mode: required` and the exact
  `required_categories`; this proposal changes neither.

## 7. Remaining Windows and noexec staging gaps

- **Windows:** the committed binary pins exclude Windows; the scanner fetch scripts
  do not provision these binaries there; `run_command` refuses on Windows (B1). No
  Windows scan evidence exists for Gitleaks, OSV-Scanner, or Semgrep. The proposed
  fingerprints are platform-independent, but this assessment ran on darwin arm64.
- **noexec temp:** `stageVerified` and `stageRules` stage into the system temp
  directory. On a host with a noexec temp mount, the scanner copy cannot start and the
  adapter reports `ErrUnavailable` (fail-closed). Not reproduced here. Required mode
  would make that an `INDETERMINATE`/blocking condition, not a bypass.
- **linux/amd64 and darwin/amd64:** this assessment used darwin arm64 plus the Linux
  arm64 container tests; CI's scanner-integration job covers linux/amd64. No new
  platform claim is made.

## What this proposal does not do

- It does not add a baseline entry, change `security.mode`, set
  `required_categories`, or change any scanner command, pin, or exclusion.
- It does not classify a finding as a false positive merely because the command is
  intentional; each production row states the trust boundary and the residual.
- It does not merge or modify unrelated OWASP work.
