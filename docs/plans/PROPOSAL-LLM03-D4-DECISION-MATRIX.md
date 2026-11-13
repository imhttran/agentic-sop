# D4 operator decision matrix — LLM03 scanner enforcement (OWASP gap 9)

**Status:** decision matrix, read-only. **Option C (`secrets` + `dependencies` +
`sast`) is the preferred target but is not approved for activation.** The current
configuration remains report-only. No baseline entry is added or approved, and the
`internal/toolharness/harness.go:926` residual is **not accepted**.

**Authoritative inventory:** [PROPOSAL-LLM03-D4-ENFORCEMENT.md](PROPOSAL-LLM03-D4-ENFORCEMENT.md)
(PR #467), including its correction that there are **six** production `sh -c` sites.
[llm03-exec-site-review.md](../reports/owasp-llm/llm03-exec-site-review.md) (PR #466) is
preserved as historical evidence; its five-site count is superseded.

**Baseline:** clean `origin/main` at `da033bcd` (2026-11-12), darwin arm64, pinned
Gitleaks 8.30.1 and OSV-Scanner 2.7.0, Semgrep 1.183.0, pinned rules `d90ff1b1729b740b1754f6b6ba8d1c9da8525136aa4a2345797f43e770fc5958`.
The scan is identical to the `6ff4ee4b` baseline: **101 findings, 40 suppressed, 61
unsuppressed, 45 blocking**; all three adapters `COMPLETED` / `COMPLETE`; verdict
`FAIL` in report-only mode. Local artifact:
`.agent-sdlc/runs/gap9-d4-matrix/security/scan.json` (not committed).

> **Revision (2026-11-13, verified at `main` `986b9132`).** The residual moved from
> `harness.go:920` to `:926` when M1 added the environment scrub above `execCaptured`, so
> its fingerprint is now `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771` (was `semgrep:2e3c98676c0b89b137c13339ae43b6a5928dae9e7ff2fcf893de77038f473847`, preserved here as historical
> evidence). The other 73 Semgrep fingerprints are unchanged. The full-workspace
> report-only baseline is now **106 findings, 40 suppressed, 66 unsuppressed, 50
> blocking**: the residual's move did not change the count, and the five additional
> Gitleaks fixture findings are the decision documents' own quoted placeholders, pending
> disposition. Only the current fingerprint may be used for a new approval.


## Decision legend

- **B** — proposed fingerprint-level baseline entry. Not added; requires operator
  approval. The entry names the exact finding fingerprint, never a path or rule.
- **R** — residual-risk decision. Not accepted; requires an explicit operator decision.
- **A** — keep report-only. The fallback if the disposition package is not approved.
- **Expiry** — the date after which the entry stops suppressing. Proposed values:
  2027-06-30 for fixtures/test-only, 2027-03-31 for production exec sites.

## 1. The 45 blocking findings

All 45 are unsuppressed findings at `critical`/`high` under the current
`fail_on: [critical, high]`. The 16 MEDIUM findings are non-blocking and are listed in
§3.4.

| # | Adapter | Rule | Location | Fingerprint | Decision | Expiry |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | gitleaks | `aws-access-token` | `docs/plans/PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md:34` | `gitleaks:542e8c41d5552a91a81d7070b25f4978c67e8c9ccde6d9d194818ef2b7e2d09a` | **B** | 2027-06-30 |
| 2 | gitleaks | `stripe-access-token` | `docs/plans/PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md:35` | `gitleaks:9bce24de528ad1aac6f0b7be3c2ec32290e8254146b8e48727cd99be16576a2a` | **B** | 2027-06-30 |
| 3 | gitleaks | `aws-access-token` | `docs/reports/owasp-llm/llm02-seven-format-campaign.json:83` | `gitleaks:332c11915593ec84a395c2cd64eaa59eb77545cd4d4af9448a6b6f6e2a1eabd4` | **B** | 2027-06-30 |
| 4 | gitleaks | `aws-access-token` | `docs/reports/owasp-llm/llm02-seven-format-campaign.json:86` | `gitleaks:f32006839b09007ede952842800cf6e5e3438e8d2c404936e403df23e591e5e5` | **B** | 2027-06-30 |
| 5 | gitleaks | `stripe-access-token` | `docs/reports/owasp-llm/llm02-seven-format-campaign.json:102` | `gitleaks:b1a51e668cf2e25a1b4a46fb3127b73d0d0d5ca113aa33e7ad942d4ee86234ea` | **B** | 2027-06-30 |
| 6 | gitleaks | `stripe-access-token` | `docs/reports/owasp-llm/llm02-seven-format-campaign.json:105` | `gitleaks:c31d51667dc25b605d12faebb0a2b9afb0e8f7b46cd40d0ab4c91e0955dd8d1d` | **B** | 2027-06-30 |
| 7 | gitleaks | `generic-api-key` | `internal/cli/llm08_notification_secrets_test.go:23` | `gitleaks:c68e84fd911325959deb53720896e44ecf6a2a3164dc470b7a68da69a2392e4c` | **B** | 2027-06-30 |
| 8 | gitleaks | `generic-api-key` | `internal/notify/llm08_notification_secrets_test.go:29` | `gitleaks:db3bc94721d39f463b41ec350303a3a3bd47e4fafa91d8ca723298bd23c5aa98` | **B** | 2027-06-30 |
| 9 | gitleaks | `aws-access-token` | `internal/ollamaagent/llm02_seven_format_linux_live_test.go:33` | `gitleaks:35daf7cd697b192e38c7bb0ff5d82dd592b0c2b6e361a466f2b9bf8197af88a9` | **B** | 2027-06-30 |
| 10 | gitleaks | `stripe-access-token` | `internal/ollamaagent/llm02_seven_format_linux_live_test.go:34` | `gitleaks:5024394e9c2eab930589a60820e675678826502a3f04b5924612851e2acf340b` | **B** | 2027-06-30 |
| 11 | gitleaks | `aws-access-token` | `internal/redact/known_format_enhancement_test.go:20` | `gitleaks:1a6feaa4d53f118595851afd37311b9498aeb77b10f7b628f3246dcf9a47fecf` | **B** | 2027-06-30 |
| 12 | gitleaks | `stripe-access-token` | `internal/redact/known_format_enhancement_test.go:21` | `gitleaks:018670038beb44e4d94058bcc9a894ecabc6a70c2add7e15cd2a804126e054f7` | **B** | 2027-06-30 |
| 13 | gitleaks | `generic-api-key` | `internal/redact/policy_test.go:12` | `gitleaks:9a6496cc947aa36105cff143d189ef0f5df1a6ba345a07623a185cb791ce319c` | **B** | 2027-06-30 |
| 14 | gitleaks | `generic-api-key` | `internal/redact/policy_test.go:13` | `gitleaks:1bd172eb44cf97b69bdd68d57e1e1065e86cff947b8fe73fcbb9d19f18e5c5d1` | **B** | 2027-06-30 |
| 15 | gitleaks | `gitlab-pat` | `internal/redact/policy_test.go:76` | `gitleaks:9910e1b901cbbe5cd43ecf1262e0bda8b834a04246bdfa7c2ee941e93e99561b` | **B** | 2027-06-30 |
| 16 | gitleaks | `stripe-access-token` | `internal/redact/policy_test.go:84` | `gitleaks:6278e72f9d4d7130e78109d480ecc6c0993403d84bb176fdf06cd4e799e3bc8f` | **B** | 2027-06-30 |
| 17 | gitleaks | `stripe-access-token` | `internal/redact/policy_test.go:85` | `gitleaks:72987dfb22a55d6f24b93f98a3a36ffc7410c06443e223f320af33a295425862` | **B** | 2027-06-30 |
| 18 | gitleaks | `generic-api-key` | `internal/redact/scrub_test.go:38` | `gitleaks:fd2fd2e2a251b47fce4948677f9f77dc237e0af1d63031f71d4ab6087bed0b6d` | **B** | 2027-06-30 |
| 19 | gitleaks | `gitlab-pat` | `internal/redact/unknown_format_assessment_test.go:25` | `gitleaks:bf5aa4c0629a2eb4d05a03a059370c7b64a25b2bb74aaf39f462bac123802b57` | **B** | 2027-06-30 |
| 20 | gitleaks | `stripe-access-token` | `internal/redact/unknown_format_assessment_test.go:33` | `gitleaks:95a03c2ff32f3a935b4c07f194f26a5730181c036db84e0e57e9ba62ff502bdc` | **B** | 2027-06-30 |
| 21 | gitleaks | `aws-access-token` | `internal/redact/unknown_format_assessment_test.go:39` | `gitleaks:9fee8d1fea5eaa6bf431c1aa807463e37d526af2e82c56a9cd1dbc517f8c43dd` | **B** | 2027-06-30 |
| 22 | gitleaks | `jwt` | `internal/redact/unknown_format_assessment_test.go:40` | `gitleaks:8ba804fb22f81445bcda29f9901a5e87870a43a0694aa2ed8fd7017b2dabb74a` | **B** | 2027-06-30 |
| 23 | gitleaks | `generic-api-key` | `internal/redact/unknown_format_assessment_test.go:47` | `gitleaks:51df64d7eaa35e4f60faedc832ef84c835119afe4e24ad879b0b4651e604b85c` | **B** | 2027-06-30 |
| 24 | gitleaks | `aws-access-token` | `internal/redact/unknown_format_assessment_test.go:52` | `gitleaks:aa717d45db7a1d5e66910d7071b67e3612fa4cad1854c559480bbd23320044e0` | **B** | 2027-06-30 |
| 25 | gitleaks | `stripe-access-token` | `internal/redact/unknown_format_assessment_test.go:53` | `gitleaks:279f4fc447993b022ddd16668b3310e912e5fe821ea34bdb0689edc1ffc55eae` | **B** | 2027-06-30 |
| 26 | gitleaks | `gitlab-pat` | `internal/redact/unknown_format_assessment_test.go:116` | `gitleaks:9c1a8a1484b137ecb944c88c75296d103776d8331ffb6c170a7a4f2dfe27b385` | **B** | 2027-06-30 |
| 27 | gitleaks | `generic-api-key` | `internal/redact/unknown_format_assessment_test.go:149` | `gitleaks:92d6018e06ca21eb02731f20e69e2dfe2db00b0d2633aa209bf386b5945bc3fb` | **B** | 2027-06-30 |
| 28 | semgrep | `dangerous-exec-command` | `internal/agent/command.go:104` | `semgrep:60898e80cd5c2bc481c1929520ff2a58292a4bd4bc50d06cce1553701a9b7781` | **B** | 2027-03-31 |
| 29 | semgrep | `dangerous-exec-command` | `internal/cli/outcome.go:779` | `semgrep:4c5c524ae8329de9fb052b337310f9d8ed1156c34a96194668ea43e2654a7728` | **B** | 2027-03-31 |
| 30 | semgrep | `dangerous-exec-command` | `internal/decision/command/command.go:128` | `semgrep:bc6bd5cd04fa74b928093ebbe7c45d48447cd59af4a39cfaab0579fa4a73820e` | **B** | 2027-03-31 |
| 31 | semgrep | `dangerous-exec-command` | `internal/decisionadapter/providers/julia/runner.go:232` | `semgrep:049c3e59d957e1d4a28a4954b676810d7aa514eb22bda8aef587116f1188db3c` | **B** | 2027-03-31 |
| 32 | semgrep | `dangerous-exec-command` | `internal/dqharness/replay_exec.go:275` | `semgrep:63433ea4f23e9fd5bb730c3c0b26dedf34a3081dba435580b1bc301f8802fe6d` | **B** | 2027-03-31 |
| 33 | semgrep | `dangerous-exec-command` | `internal/dqharness/replay_exec_test.go:105` | `semgrep:ccb4bb42ca610fd2963268b516385bba795ee7c294c4ad0f7968bcd03d25725c` | **B** | 2027-06-30 |
| 34 | semgrep | `dangerous-exec-command` | `internal/github/command.go:33` | `semgrep:a057a8819f65f11b35ac5d11a41e7394f53b118e04c6073e90be61d48c4019f1` | **B** | 2027-03-31 |
| 35 | semgrep | `dangerous-exec-command` | `internal/handoff/caveman/caveman.go:90` | `semgrep:435864db8eb90bf0f49f6c97e8990a93e1bdb87230425c80b9bf6a9d200768bc` | **B** | 2027-03-31 |
| 36 | semgrep | `dangerous-exec-command` | `internal/hipcompare/command.go:402` | `semgrep:d982e8f12bb7c6597513193a4622edaf6a955a626feba0ed8f9198ec406560be` | **B** | 2027-03-31 |
| 37 | semgrep | `dangerous-exec-command` | `internal/hipcompare/command.go:430` | `semgrep:5933a67130ae763b9e6d30126cd43e84e2066fc2d2c218b85d32c57366905a16` | **B** | 2027-03-31 |
| 38 | semgrep | `dangerous-exec-command` | `internal/ragclient/exec.go:19` | `semgrep:f4506eb456705bd1fd134e7a2d1a79051205e570d70f62577be23702b2f8b71c` | **B** | 2027-03-31 |
| 39 | semgrep | `dangerous-exec-command` | `internal/review/ocr.go:57` | `semgrep:fc8287f18275bcdb937f8d3e12dab172076126d3045cc73a997cf5f523780b88` | **B** | 2027-03-31 |
| 40 | semgrep | `dangerous-exec-command` | `internal/testrunner/command.go:53` | `semgrep:0f959d4f3632e1c2244db5405f3f0309e58abbb00c08ab0d71b874d2c75ca344` | **B** | 2027-03-31 |
| 41 | semgrep | `dangerous-exec-command` | `internal/toolharness/harness.go:926` | `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771` | **R** | OPEN |
| 42 | semgrep | `dangerous-exec-command` | `internal/toolharness/isolation_credential_linux_test.go:113` | `semgrep:fab835e9941bb1ad0ab32bf7e51f0a1f30944f2d500ba43cf844b247c4c35718` | **B** | 2027-06-30 |
| 43 | semgrep | `dangerous-exec-command` | `internal/toolharness/isolation_test.go:32` | `semgrep:11ef53417d1b9eedc8e24e1593bdfa38b6d8df035e3265fce7c61bff2aa3e5c2` | **B** | 2027-06-30 |
| 44 | semgrep | `dangerous-exec-command` | `internal/toolharness/isolation_test.go:60` | `semgrep:fbedd5f6a2ed946cd639a20ce584f6ab65c3788767ea09cbb60e2c08294313b2` | **B** | 2027-06-30 |
| 45 | semgrep | `dangerous-exec-command` | `internal/toolharness/isolation_test.go:677` | `semgrep:b54aa7e631411a44e21ab9b190aac37cfd4148fde1f562bd8938897974cb88a5` | **B** | 2027-06-30 |

**Summary:** 44 proposed fingerprint entries (**B**) — 27 Gitleaks, 5 test-only
Semgrep HIGH, and 12 production Semgrep — and one residual decision (**R**,
`internal/toolharness/harness.go:926`). No finding is proposed as a code fix.

## 2. Production dispositions (13 sites)

| Site | Shell? | Source / trust | Decision | Expiry | Fingerprint |
| --- | --- | --- | --- | --- | --- |
| `internal/agent/command.go:104` | sh -c | operator env/config | **B** | 2027-03-31 | `semgrep:60898e80cd5c2bc481c1929520ff2a58292a4bd4bc50d06cce1553701a9b7781` |
| `internal/cli/outcome.go:779` | argv | operator task spec + command policy | **B** | 2027-03-31 | `semgrep:4c5c524ae8329de9fb052b337310f9d8ed1156c34a96194668ea43e2654a7728` |
| `internal/decision/command/command.go:128` | argv | operator config | **B** | 2027-03-31 | `semgrep:bc6bd5cd04fa74b928093ebbe7c45d48447cd59af4a39cfaab0579fa4a73820e` |
| `internal/decisionadapter/providers/julia/runner.go:232` | argv | operator config | **B** | 2027-03-31 | `semgrep:049c3e59d957e1d4a28a4954b676810d7aa514eb22bda8aef587116f1188db3c` |
| `internal/dqharness/replay_exec.go:275` | argv (sandbox launcher) | replay fixture + required sandbox | **B** | 2027-03-31 | `semgrep:63433ea4f23e9fd5bb730c3c0b26dedf34a3081dba435580b1bc301f8802fe6d` |
| `internal/github/command.go:33` | argv | internal fixed gh/git commands | **B** | 2027-03-31 | `semgrep:a057a8819f65f11b35ac5d11a41e7394f53b118e04c6073e90be61d48c4019f1` |
| `internal/handoff/caveman/caveman.go:90` | sh -c | operator config | **B** | 2027-03-31 | `semgrep:435864db8eb90bf0f49f6c97e8990a93e1bdb87230425c80b9bf6a9d200768bc` |
| `internal/hipcompare/command.go:402` | sh -c | operator executor command | **B** | 2027-03-31 | `semgrep:d982e8f12bb7c6597513193a4622edaf6a955a626feba0ed8f9198ec406560be` |
| `internal/hipcompare/command.go:430` | sh -c (corrected from #466) | manifest verification commands | **B** | 2027-03-31 | `semgrep:5933a67130ae763b9e6d30126cd43e84e2066fc2d2c218b85d32c57366905a16` |
| `internal/ragclient/exec.go:19` | argv | operator config absolute path | **B** | 2027-03-31 | `semgrep:f4506eb456705bd1fd134e7a2d1a79051205e570d70f62577be23702b2f8b71c` |
| `internal/review/ocr.go:57` | sh -c | operator env | **B** | 2027-03-31 | `semgrep:fc8287f18275bcdb937f8d3e12dab172076126d3045cc73a997cf5f523780b88` |
| `internal/testrunner/command.go:53` | sh -c | project validation config | **B** | 2027-03-31 | `semgrep:0f959d4f3632e1c2244db5405f3f0309e58abbb00c08ab0d71b874d2c75ca344` |
| `internal/toolharness/harness.go:926` | argv | model run_command + command policy + default-on sandbox | **R** | OPEN | `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771` |

The six `sh -c` sites are `internal/agent/command.go:104`,
`internal/handoff/caveman/caveman.go:90`, `internal/hipcompare/command.go:402`,
`internal/hipcompare/command.go:430`, `internal/review/ocr.go:57`, and
`internal/testrunner/command.go:53`. The other seven execute a structured argv.
`internal/toolharness/harness.go:926` is the only model-controlled exec path; it is
subject to `commandpolicy.Classify` and the default-on OS sandbox, and is marked
**R**, not **B**.

## 3. Proposed fingerprint-level baseline entries

These entries are **not added** by this change. Each is exact-fingerprint; no
path-wide or rule-wide suppression is proposed.

### 3.1 Gitleaks fixtures and report examples (27 entries, expiry 2027-06-30)

| File | Line | Rule | Fingerprint | Expiry |
| --- | --- | --- | --- | --- |
| `docs/plans/PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md` | 34 | `aws-access-token` | `gitleaks:542e8c41d5552a91a81d7070b25f4978c67e8c9ccde6d9d194818ef2b7e2d09a` | 2027-06-30 |
| `docs/plans/PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md` | 35 | `stripe-access-token` | `gitleaks:9bce24de528ad1aac6f0b7be3c2ec32290e8254146b8e48727cd99be16576a2a` | 2027-06-30 |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 83 | `aws-access-token` | `gitleaks:332c11915593ec84a395c2cd64eaa59eb77545cd4d4af9448a6b6f6e2a1eabd4` | 2027-06-30 |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 86 | `aws-access-token` | `gitleaks:f32006839b09007ede952842800cf6e5e3438e8d2c404936e403df23e591e5e5` | 2027-06-30 |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 102 | `stripe-access-token` | `gitleaks:b1a51e668cf2e25a1b4a46fb3127b73d0d0d5ca113aa33e7ad942d4ee86234ea` | 2027-06-30 |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json` | 105 | `stripe-access-token` | `gitleaks:c31d51667dc25b605d12faebb0a2b9afb0e8f7b46cd40d0ab4c91e0955dd8d1d` | 2027-06-30 |
| `internal/cli/llm08_notification_secrets_test.go` | 23 | `generic-api-key` | `gitleaks:c68e84fd911325959deb53720896e44ecf6a2a3164dc470b7a68da69a2392e4c` | 2027-06-30 |
| `internal/notify/llm08_notification_secrets_test.go` | 29 | `generic-api-key` | `gitleaks:db3bc94721d39f463b41ec350303a3a3bd47e4fafa91d8ca723298bd23c5aa98` | 2027-06-30 |
| `internal/ollamaagent/llm02_seven_format_linux_live_test.go` | 33 | `aws-access-token` | `gitleaks:35daf7cd697b192e38c7bb0ff5d82dd592b0c2b6e361a466f2b9bf8197af88a9` | 2027-06-30 |
| `internal/ollamaagent/llm02_seven_format_linux_live_test.go` | 34 | `stripe-access-token` | `gitleaks:5024394e9c2eab930589a60820e675678826502a3f04b5924612851e2acf340b` | 2027-06-30 |
| `internal/redact/known_format_enhancement_test.go` | 20 | `aws-access-token` | `gitleaks:1a6feaa4d53f118595851afd37311b9498aeb77b10f7b628f3246dcf9a47fecf` | 2027-06-30 |
| `internal/redact/known_format_enhancement_test.go` | 21 | `stripe-access-token` | `gitleaks:018670038beb44e4d94058bcc9a894ecabc6a70c2add7e15cd2a804126e054f7` | 2027-06-30 |
| `internal/redact/policy_test.go` | 12 | `generic-api-key` | `gitleaks:9a6496cc947aa36105cff143d189ef0f5df1a6ba345a07623a185cb791ce319c` | 2027-06-30 |
| `internal/redact/policy_test.go` | 13 | `generic-api-key` | `gitleaks:1bd172eb44cf97b69bdd68d57e1e1065e86cff947b8fe73fcbb9d19f18e5c5d1` | 2027-06-30 |
| `internal/redact/policy_test.go` | 76 | `gitlab-pat` | `gitleaks:9910e1b901cbbe5cd43ecf1262e0bda8b834a04246bdfa7c2ee941e93e99561b` | 2027-06-30 |
| `internal/redact/policy_test.go` | 84 | `stripe-access-token` | `gitleaks:6278e72f9d4d7130e78109d480ecc6c0993403d84bb176fdf06cd4e799e3bc8f` | 2027-06-30 |
| `internal/redact/policy_test.go` | 85 | `stripe-access-token` | `gitleaks:72987dfb22a55d6f24b93f98a3a36ffc7410c06443e223f320af33a295425862` | 2027-06-30 |
| `internal/redact/scrub_test.go` | 38 | `generic-api-key` | `gitleaks:fd2fd2e2a251b47fce4948677f9f77dc237e0af1d63031f71d4ab6087bed0b6d` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 25 | `gitlab-pat` | `gitleaks:bf5aa4c0629a2eb4d05a03a059370c7b64a25b2bb74aaf39f462bac123802b57` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 33 | `stripe-access-token` | `gitleaks:95a03c2ff32f3a935b4c07f194f26a5730181c036db84e0e57e9ba62ff502bdc` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 39 | `aws-access-token` | `gitleaks:9fee8d1fea5eaa6bf431c1aa807463e37d526af2e82c56a9cd1dbc517f8c43dd` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 40 | `jwt` | `gitleaks:8ba804fb22f81445bcda29f9901a5e87870a43a0694aa2ed8fd7017b2dabb74a` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 47 | `generic-api-key` | `gitleaks:51df64d7eaa35e4f60faedc832ef84c835119afe4e24ad879b0b4651e604b85c` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 52 | `aws-access-token` | `gitleaks:aa717d45db7a1d5e66910d7071b67e3612fa4cad1854c559480bbd23320044e0` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 53 | `stripe-access-token` | `gitleaks:279f4fc447993b022ddd16668b3310e912e5fe821ea34bdb0689edc1ffc55eae` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 116 | `gitlab-pat` | `gitleaks:9c1a8a1484b137ecb944c88c75296d103776d8331ffb6c170a7a4f2dfe27b385` | 2027-06-30 |
| `internal/redact/unknown_format_assessment_test.go` | 149 | `generic-api-key` | `gitleaks:92d6018e06ca21eb02731f20e69e2dfe2db00b0d2633aa209bf386b5945bc3fb` | 2027-06-30 |

**Why expected / evidence.** The redaction tests are the input side of the redaction
policy; `internal/notify/llm08_notification_secrets_test.go` explicitly calls its value
a synthetic secret with no credential shape and says it is not a real secret. The
campaign JSON stores planted `raw_canaries` and redacted `model_input`; the proposal is
a table of candidate rule fixtures. These are test or evidence fixtures.

**Revalidation.** On any change to the file, the redaction policy, the Gitleaks
version/rules, or when the file moves out of test/report scope.

**Risk if too broad.** A path- or rule-wide entry would hide a real credential later
added to the same report or test file. Use the exact fingerprints.

### 3.2 Test-only Semgrep HIGH (5 entries, expiry 2027-06-30)

| File | Line | Rule | Fingerprint | Expiry |
| --- | --- | --- | --- | --- |
| `internal/dqharness/replay_exec_test.go` | 105 | `dangerous-exec-command` | `semgrep:ccb4bb42ca610fd2963268b516385bba795ee7c294c4ad0f7968bcd03d25725c` | 2027-06-30 |
| `internal/toolharness/isolation_credential_linux_test.go` | 113 | `dangerous-exec-command` | `semgrep:fab835e9941bb1ad0ab32bf7e51f0a1f30944f2d500ba43cf844b247c4c35718` | 2027-06-30 |
| `internal/toolharness/isolation_test.go` | 32 | `dangerous-exec-command` | `semgrep:11ef53417d1b9eedc8e24e1593bdfa38b6d8df035e3265fce7c61bff2aa3e5c2` | 2027-06-30 |
| `internal/toolharness/isolation_test.go` | 60 | `dangerous-exec-command` | `semgrep:fbedd5f6a2ed946cd639a20ce584f6ab65c3788767ea09cbb60e2c08294313b2` | 2027-06-30 |
| `internal/toolharness/isolation_test.go` | 677 | `dangerous-exec-command` | `semgrep:b54aa7e631411a44e21ab9b190aac37cfd4148fde1f562bd8938897974cb88a5` | 2027-06-30 |

**Why expected / evidence.** All are `_test.go` helpers that start a sandbox or probe
isolation; they cannot run in production. **Revalidation.** On test-file or
isolation-helper changes; remove if a helper becomes production code. **Risk if too
broad.** A path-wide entry would hide a new production exec in the same file.

### 3.3 Production Semgrep baseline entries (12 entries, expiry 2027-03-31)

| Site | Shell? | Source / trust | Fingerprint | Expiry |
| --- | --- | --- | --- | --- |
| `internal/agent/command.go:104` | sh -c | operator env/config | `semgrep:60898e80cd5c2bc481c1929520ff2a58292a4bd4bc50d06cce1553701a9b7781` | 2027-03-31 |
| `internal/cli/outcome.go:779` | argv | operator task spec + command policy | `semgrep:4c5c524ae8329de9fb052b337310f9d8ed1156c34a96194668ea43e2654a7728` | 2027-03-31 |
| `internal/decision/command/command.go:128` | argv | operator config | `semgrep:bc6bd5cd04fa74b928093ebbe7c45d48447cd59af4a39cfaab0579fa4a73820e` | 2027-03-31 |
| `internal/decisionadapter/providers/julia/runner.go:232` | argv | operator config | `semgrep:049c3e59d957e1d4a28a4954b676810d7aa514eb22bda8aef587116f1188db3c` | 2027-03-31 |
| `internal/dqharness/replay_exec.go:275` | argv (sandbox launcher) | replay fixture + required sandbox | `semgrep:63433ea4f23e9fd5bb730c3c0b26dedf34a3081dba435580b1bc301f8802fe6d` | 2027-03-31 |
| `internal/github/command.go:33` | argv | internal fixed gh/git commands | `semgrep:a057a8819f65f11b35ac5d11a41e7394f53b118e04c6073e90be61d48c4019f1` | 2027-03-31 |
| `internal/handoff/caveman/caveman.go:90` | sh -c | operator config | `semgrep:435864db8eb90bf0f49f6c97e8990a93e1bdb87230425c80b9bf6a9d200768bc` | 2027-03-31 |
| `internal/hipcompare/command.go:402` | sh -c | operator executor command | `semgrep:d982e8f12bb7c6597513193a4622edaf6a955a626feba0ed8f9198ec406560be` | 2027-03-31 |
| `internal/hipcompare/command.go:430` | sh -c (corrected from #466) | manifest verification commands | `semgrep:5933a67130ae763b9e6d30126cd43e84e2066fc2d2c218b85d32c57366905a16` | 2027-03-31 |
| `internal/ragclient/exec.go:19` | argv | operator config absolute path | `semgrep:f4506eb456705bd1fd134e7a2d1a79051205e570d70f62577be23702b2f8b71c` | 2027-03-31 |
| `internal/review/ocr.go:57` | sh -c | operator env | `semgrep:fc8287f18275bcdb937f8d3e12dab172076126d3045cc73a997cf5f523780b88` | 2027-03-31 |
| `internal/testrunner/command.go:53` | sh -c | project validation config | `semgrep:0f959d4f3632e1c2244db5405f3f0309e58abbb00c08ab0d71b874d2c75ca344` | 2027-03-31 |

The thirteenth production site, `internal/toolharness/harness.go:926`, is not in this
table because it is the residual decision in §5.

### 3.4 Optional non-blocking Semgrep MEDIUM (16 entries)

These do not block under `fail_on: [critical, high]`. They are optional hygiene for a
clean report and are **not required** to activate Option C.

| File | Line | Rule | Fingerprint |
| --- | --- | --- | --- |
| `internal/agent/openaicompatible_execution_test.go` | 52 | `no-direct-write-to-responsewriter` | `semgrep:2b5a87b2ae0c39c8fef3e980ddd9a2220482dc2cbd66ab8ca29617e3686ba914` |
| `internal/agent/provider_test.go` | 199 | `no-direct-write-to-responsewriter` | `semgrep:6d67a58248b4b5fa1c6d79b902c58db109e05cd779510e9bd3946f6463a562f4` |
| `internal/agent/provider_test.go` | 291 | `no-direct-write-to-responsewriter` | `semgrep:4b55e86aabfbc3ef7a97493fcdc8d1210c78f7566ebbddc65eaa1835181b57b4` |
| `internal/agent/redacted_test.go` | 62 | `no-direct-write-to-responsewriter` | `semgrep:595e210e46b0ccf6536e2310aa905f4b04815b2e1b0689619c379abc6283d52d` |
| `internal/agent/truncation_test.go` | 19 | `no-direct-write-to-responsewriter` | `semgrep:039b18b89012f00823df209a800579666ca9cb4228314bdf37a000d274d2c019` |
| `internal/cli/ollama_e2e_test.go` | 41 | `no-direct-write-to-responsewriter` | `semgrep:a83133ce6a59c832a5b262c6c618c74190d57eed6d3703d9fa129bd5d76ca3c2` |
| `internal/cli/provider_validation_task_test.go` | 39 | `no-direct-write-to-responsewriter` | `semgrep:149068d334137adbb0e7759815f802fce6ff1ba79397deba9f7ef245aea1213a` |
| `internal/cli/zz_operator_acceptance_acc008_test.go` | 158 | `unsafe-reflect-by-name` | `semgrep:ee77e1747a4ebaca904aadaa06f4f0ceeb73425b745d92f6658caae70f530e73` |
| `internal/controller/web/recovery_test.go` | 474 | `cookie-missing-httponly` | `semgrep:44bb1ebabebf28e50622eefa37e94f734526cfcc9ba4ab23fd8b0d8909b57dcb` |
| `internal/controller/web/recovery_test.go` | 474 | `cookie-missing-secure` | `semgrep:7c3c5c9fa90365a786e3b59f9efad0a22fb55c5323c31ae62f7f4138999b9921` |
| `internal/decisionadapter/providers/clef/chat_test.go` | 39 | `no-fprintf-to-responsewriter` | `semgrep:57df50ac6f6f8010d5dd10cd10656bc6e66c440b1100f641146d1c6451947686` |
| `internal/decisionadapter/providers/nimble/score_test.go` | 77 | `no-io-writestring-to-responsewriter` | `semgrep:512a5c92c463843f83452d6a6c624aeb0f61013f098053d539b83142920df8d4` |
| `internal/ollamaagent/ollama_retry_test.go` | 70 | `no-io-writestring-to-responsewriter` | `semgrep:f882aaa11e4597979183b30a0151f680f7403d7c983a90dd0c7e32c94a05ecac` |
| `internal/ollamaagent/ollama_retry_test.go` | 73 | `no-io-writestring-to-responsewriter` | `semgrep:cbd49f285f948561b34651424e857ee0e3fa70d5750b966397faccaa5eaceeca` |
| `internal/provider/openai/provider_locality_test.go` | 24 | `no-io-writestring-to-responsewriter` | `semgrep:d04540b6ab8321b7dc68737c4cbf671a7f646771e044232e3a4432385c73da41` |
| `internal/store/notifications_test.go` | 40 | `string-formatted-query` | `semgrep:1b660db194071413c2c277c7add47c9d3d4004c36c357f56c54520995f479c5a` |

Proposed expiry if approved: 2027-06-30. Revalidate on test-file or rule-set changes.

## 4. Findings eliminable by narrow code fixes

**None of the 45 blocking findings can be eliminated by a narrow, policy-neutral code
fix that preserves behavior and does not suppress or evade the finding.** This is the
result of the review, not an assumption:

- **`sh -c` sites (6).** Converting to argv is possible only when the command is a
  simple argv. The operator/project commands rely on shell syntax (`go test ./...`,
  `docker compose …`, pipes), and the Hipcompare manifest stores verification commands
  as strings. A conversion would change behavior or require a manifest schema change.
- **Direct-argv sites (7).** The rule flags the non-static executable/argv, not a
  shell-injection path. Eliminating the finding would require hardcoding the command
  (breaking configuration), moving the feature out of scope, or adding an inline
  `nosemgrep` annotation (suppression, which the direction forbids).
- **Gitleaks fixtures (27).** Eliminating them would require removing or changing the
  test/evidence values (weakening redaction tests or historical evidence), moving the
  fixtures under an already-excluded `testdata` path (a scope change), or excluding the
  docs (a policy change). Generating the same values at runtime would be scanner
  evasion, not a code fix; it is not implemented.
- **Dead-code candidate.** #466 notes that `caveman.New` has no non-test caller. Removing
  the Caveman shell runner would eliminate one finding, but it is a feature removal,
  not a policy-neutral correction, and is not independently authorized.
- **`internal/toolharness/harness.go:926`.** The exec is the intended capability. There
  is no code fix that removes the finding without removing `run_command` or weakening
  the policy; the residual decision is §5.

Accordingly, this change is **documentation only**; it makes no code, configuration,
baseline, or enforcement change.

## 5. Remaining residual-risk decision: `internal/toolharness/harness.go:926`

This is the only blocking finding that is a **true model-controlled exec capability**
rather than a trusted-config or test-only site. It is constrained by:

- `SplitCommand` rejects shell metacharacters and globs; there is no shell.
- `checkCommandIn` requires `commandpolicy.Classify == SAFE` and confines path
  arguments to the authorized root.
- `isolationRequested` defaults on; Seatbelt on macOS and bubblewrap on Linux deny
  network and writes outside the root/temp, and Windows refuses `run_command`.
- Tests pin the default-on policy and the platform refusal
  (`TestRunCommandIsolationIsOnByDefault`, `TestIsolationPolicyNotice`,
  `TestIsolationDecisionUnsupportedPlatforms`, and the Windows runtime test).

The residual returns if the operator sets `SOP_AGENT_RUN_COMMAND_ISOLATION=off` on
macOS/Linux, or if a platform has no backend (Windows refuses by design).

**Decision: OPEN — not accepted.** The options are:

- **R1 — Accept the bounded residual** with a short expiry in the baseline and a
  revalidation trigger on any command-policy, isolation, or `run_command` change.
  This is the prerequisite for Option C as drafted.
- **R2 — Keep Option A (report-only)** until the residual is accepted or the
  `run_command` design changes. This is the safe fallback.
- **R3 — Remove or disable `run_command`.** A capability loss; needs a separate
  product/security decision.
- **R4 — Authorize a code change** that changes the exec mechanism or policy. This is
  outside the current authorization and is not proposed here.

I do not infer R1 from the existing mitigations, and this packet does not record it.

## 6. Smallest set of operator approvals to activate Option C safely

**Two approvals are the smallest coherent set:**

1. **Disposition package.** Approve the 44 fingerprint entries in §3 (27 Gitleaks,
   5 test-only HIGH, 12 production) with the stated expiries, **and** decide the
   `toolharness` residual in §5 (R1, R2, R3, or R4). The package cannot be approved
   without the residual decision; if the operator will not accept R1, C cannot be
   activated safely and A remains.
2. **Required-mode change.** After (1), approve `security.mode: required`,
   `required_categories: [secrets, dependencies, sast]`, and full workspace scope.
   This is the only enforcement change and is not made by this PR.

**Optional third approval:** the 16 MEDIUM entries in §3.4. They do not block; omit
them to keep the gate minimal, or approve them for a clean report.

**Not required for C:** D3 (rules source) stays as-is; D5 (CI Semgrep version
alignment) is recommended but not a prerequisite for the gate. D1 and D2 are already
recorded.

If the operator wants a **staged rollout** instead, the smallest alternative is:
approve A now, defer the disposition package, and revisit after the `run_command`
residual decision. Do **not** choose B as an interim step: B still blocks on the 18
Semgrep HIGH findings and would require either the same dispositions or disabling SAST
coverage.

## 7. Remaining platform and staging gaps

- **Windows:** no committed binary pins, no scanner provisioning, and `run_command`
  refuses (B1). No Windows scan evidence for Gitleaks, OSV-Scanner, or Semgrep. The
  fingerprints are platform-independent, but this assessment ran on darwin arm64.
- **noexec temp:** `stageVerified` and `stageRules` stage into the system temp. A
  noexec temp mount makes the scanner fail closed with `ErrUnavailable`; required mode
  would report `INDETERMINATE`, not a bypass. Not reproduced here.
- **Platform coverage:** darwin arm64 for this scan; Linux arm64 container tests for
  the focused suites; CI's scanner-integration job covers linux/amd64. No new platform
  claim is made.

## What this change does not do

- It does not add a baseline entry, change `security.mode`, set
  `required_categories`, or change a scanner command, pin, or exclusion.
- It does not accept the `toolharness` residual.
- It does not modify #466; it preserves it as historical evidence and uses #467's
  six-site correction as authoritative.
- It does not merge or modify unrelated OWASP work.
