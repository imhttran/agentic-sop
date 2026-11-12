# LLM02 operator decision packet (OWASP gap 8)

**As of:** `main` `138a149e` (2026-11-12, after PR #450 merged; acceptance recorded against plan
identity `10e84b69…` and the evidence revisions verified in this checkout).
**Status:** **PLAN 03 ACCEPTANCE RECORDED — 16/16.** On 2026-11-12 the
designated approver (`Tom Tran`) accepted the 13 manual criteria, and the 3
check-mapped criteria are check-passed. The acceptances are scoped to the
verified evidence below. This record does **not** accept any LLM02 residual
risk, change C1 defaults, or declare LLM02 fully mitigated. The pre-acceptance
decision-preparation state is preserved in §A.1 and the acceptance request.

**Verified in this checkout**

- `sop verify --plan … --archive … --json` reports `accepted: 16`, `mapped: 16`,
  `manual: 13`, `check: 3`, `manual_accepted: 13`, `check_passed: 3`,
  `unverified: 0`, `stale: 0`, `rejected: 0`. The `decision_eligible` counter
  remains `13` because it is the static count of manual mappings, not a
  remaining-work counter; the per-criterion `decision-eligible` status count is
  `0`.
- `sop plan historicalize --check --json …` reports `state: NONE`,
  `disposition: COMPLETE`, `eligible: false`, `reason: no active plan is recorded`.
  `COMPLETE` is lifecycle state, not acceptance.
- The acceptance ledger is
  `.agent-sdlc/acceptance/plan-03-sop-llm02-sensitive-information-disclosure/`
  (`decisions.jsonl` 13 records; `checks.jsonl` 4 records). It is Git-ignored
  local evidence.
- The preserved archive has `runs/DISCLOSE-001/outcome.json` and
  `runs/DISCLOSE-002/outcome.json` (both `PARTIAL`), and only
  `runs/DISCLOSE-003/external-completion.json`. The Plan 03 per-criterion
  decisions are in the separate historical acceptance ledger above.
- `human.designated_approvers` names `Tom Tran` in the preserved configuration.
- The CLI syntax below was checked against `internal/cli/historical_acceptance.go`
  (`historicalOutcomeUsage`, `historicalVerifyUsage`) and the validation in
  `internal/planacceptance/record.go`, `internal/planacceptance/evaluate.go`, and
  `internal/planacceptance/evidence.go`.

**How to read this packet**

- **Verified fact** — reproduced from source, tests, artifacts, or CLI output.
- **Existing recorded decision** — an operator decision already documented. It is
  not reopened here.
- **Proposed decision** — a recommendation that only the approver can record.
- **Unresolved evidence** — a gap that limits a claim. It is not acceptance.
- **Human authority** — an action this packet must not perform.

The [coverage statement](coverage-statement.md) and
[gap register](gap-register-20261109.md) remain authoritative for status. This
packet is the single-sitting decision view.

**Local-evidence caveat.** The archive under
`.agent-sdlc/archive/worktree-evidence/` and the acceptance ledger under
`.agent-sdlc/acceptance/` are Git-ignored, local evidence. The recorded commands
were executed in the checkout that contains that archive (the main checkout used
for the Plan 03 closeout). The decision-memory IDs cited below are preserved in
committed reports; the main checkout used for the closeout reports `0
applicable` and `31 stale`, so the entries are not currently applied as run
context there. Staleness is not revocation, but it is a reproducibility
limitation.

---

## A. Plan 03 acceptance

### A.1 Verified state

Plan 03 is historicalized `COMPLETE` (external completion `d3830e35`,
historicalized `2026-11-09T17:19:47Z`). Lifecycle state is separate from
acceptance. On 2026-11-12 the designated approver recorded 13 manual accept
decisions; the 3 check-mapped criteria are check-passed. The resulting
acceptance is 16/16. The separate bounded evidence acceptance (decision-memory
ID `59d98698…`) remains a separate scoped decision and did not substitute for
these per-criterion decisions.

| Item                    | Current value                                                                                    |
| ----------------------- | ------------------------------------------------------------------------------------------------ |
| Manual criteria         | 13 (`001.1`–`001.5`, `002.4`, `002.5`, `003.1`–`003.6`)                                          |
| Check-mapped criteria   | 3 (`002.1`, `002.2`, `002.3`)                                                                    |
| Accepted                | **16 / 16**                                                                                      |
| Manual accepted         | **13 / 13**                                                                                      |
| Check passed            | **3 / 3**                                                                                        |
| Plan identity           | `10e84b69258c63f02c702aed7e8ce7e7b3c2f7431a2b65f93ec819e9696835de`                               |
| Classification          | All 16 accepted 2026-11-12; no criterion remains `decision-eligible`                            |
| Blockers                | None for acceptance; Linux live retrieval remains `NOT_RUN` and limits any live-retrieval claim |

### A.2 Criterion matrix

> **Post-decision note (2026-11-12):** every row below is now accepted — 13
> manual decisions recorded by `Tom Tran` and 3 check-passed criteria. The
> `Classification` and `Recommendation` columns preserve the pre-decision
> assessment; the acceptance ledger is authoritative for the outcome.

| ID      | Requirement                                                                                                                          | Verified evidence                                                                                                                                                                                                                                                                                                   | Limit / remaining exposure                                                                                                                                                                                                                                    | Classification               | Recommendation                                                                                                                                                                             |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `001.1` | Distinguish provider authentication headers from model-visible context.                                                              | §Provider authentication is not model content; §Data-flow trace provider row; `TestOpenAICompatibleExecutionReusesAuthorization`.                                                                                                                                                                                   | The test covers the OpenAI-compatible provider; no live provider request was captured.                                                                                                                                                                        | Ready for human decision     | Accept the documented header/body separation and the deterministic test.                                                                                                                   |
| `001.2` | Identify where redaction occurs before sending, before persistence, and before display.                                              | §Where redaction happens; `redact.ModelInput`; `redactDiagnosticText`; handoff, outcome, and notification tests.                                                                                                                                                                                                    | Pattern-based, not general DLP; unknown-format prose remains outside.                                                                                                                                                                                         | Ready for human decision     | Accept the documented redaction points.                                                                                                                                                    |
| `001.3` | Record scope checks before retrieval and response validation after retrieval.                                                        | §Authorization boundaries; retrieval row; `TestSearchRejectsUnrelatedEvidence`; `TestRankerAdapterRejectsScopeItCannotHonor`; `TestLocalRetrieverRequiresCurrent`.                                                                                                                                                  | Live retrieval is `NOT_RUN`; the deterministic contract is the evidence.                                                                                                                                                                                      | Ready for human decision     | Accept the deterministic scope/validation contract with live retrieval explicitly excluded.                                                                                                |
| `001.4` | Distinguish credentials from other confidential content; pattern matching is not general DLP.                                        | §Residual risks and §Pattern matching; `TestModelInputToolBoundariesResidualLimits`; Linux residual cases; C1 hardlink unlisted-value residual (`ed1ba11`).                                                                                                                                                         | The bounded claim does not mitigate or accept any residual.                                                                                                                                                                                                   | Ready for human decision     | Accept the bounded-claim documentation only.                                                                                                                                               |
| `001.5` | Do not read or print actual secret values during assessment.                                                                         | §Method; all campaign artifacts use synthetic canaries.                                                                                                                                                                                                                                                             | Synthetic-only scope.                                                                                                                                                                                                                                         | Ready for human decision     | Accept.                                                                                                                                                                                    |
| `002.1` | Protected canaries do not appear in unauthorized model messages, persisted diagnostics, or outgoing payloads at in-scope boundaries. | Archived check: `go test ./internal/cli ./internal/agent -count=1`. Supporting live evidence: 7 protected Linux cases, 4 C1 hardlink cases, task-input re-runs.                                                                                                                                                     | Check-mapped; a passing check is evidence for its tested revision, not human acceptance. Unknown-format ordinary-file/diff/SAFE-output, hardlink path bypass, unlisted C1 residual, and verbatim model-output artifacts remain outside the protected classes. | Ready for human decision (check-mapped) | Run `--run-check 002.1` and accept the recorded check evidence only for the declared protected classes.                                                                                    |
| `002.2` | Cross-scope retrieval content is rejected before it can become model context under the current retrieval contract.                   | Archived check: `go test ./internal/retrievalboundary ./internal/memindex ./internal/handoff ./internal/notify ./internal/acp ./internal/controller/sopclient ./internal/outcomefile -count=1`. Supporting deterministic tests: `TestSearchRejectsUnrelatedEvidence`, `TestRankerAdapterRejectsScopeItCannotHonor`. | Check-mapped; live retrieval is `NOT_RUN`; no live-retrieval claim.                                                                                                                                                                                           | Ready for human decision (check-mapped) | Run `--run-check 002.2`; do not extend the result to live retrieval.                                                                                                                       |
| `002.3` | Legitimate non-sensitive content and required authentication still work.                                                             | Archived check: `go test ./internal/cli ./internal/agent -count=1`. Supporting live evidence: 6 Linux positive controls, C1 hardlink positive control, `.env.example`/`.env.sample`, auth test.                                                                                                                     | Check-mapped; only tested paths and platform.                                                                                                                                                                                                                 | Ready for human decision (check-mapped) | Run `--run-check 002.3`.                                                                                                                                                                   |
| `002.4` | Each fix reuses the shared redaction or established authorization boundary where appropriate.                                        | §Reproduced gaps and fixes; `redact.ModelInput`; `sensitivepath`; `toolharness`; `userPrompt`.                                                                                                                                                                                                                      | None material.                                                                                                                                                                                                                                                | Ready for human decision     | Accept.                                                                                                                                                                                    |
| `002.5` | Unknown formats and arbitrary confidential text remain explicitly bounded claims.                                                    | §Residual risks and §Residual investigation status; residual tests; C1 hardlink unlisted residual.                                                                                                                                                                                                                  | Does not mitigate or accept the residual.                                                                                                                                                                                                                     | Ready for human decision     | Accept the bounded-claim documentation only.                                                                                                                                               |
| `003.1` | Direct extraction and an indirect injected extraction case have recorded outcomes.                                                   | `llm02-campaign.json` (`9c32e3d2`): direct context disclosure observed, indirect retrieval refusal observed; Linux tool-path campaign.                                                                                                                                                                              | Direct disclosure observed is not protection; a refusal is not credited.                                                                                                                                                                                      | Ready for human decision     | Accept that outcomes are recorded.                                                                                                                                                         |
| `003.2` | Unauthorized tool effects remain denied even if the model proposes disclosure.                                                       | macOS effects denied by `authorization`/`policy`, `executed` empty; Linux `unauthorized_action=false`.                                                                                                                                                                                                              | One model and platform for the recorded cases; no universal claim.                                                                                                                                                                                            | Ready for human decision     | Accept for the recorded cases.                                                                                                                                                             |
| `003.3` | No real credential or external receiver is used.                                                                                     | Synthetic canaries; inert `.invalid`; loopback; 5/5 confinement probes.                                                                                                                                                                                                                                             | None material.                                                                                                                                                                                                                                                | Ready for human decision     | Accept.                                                                                                                                                                                    |
| `003.4` | Report separates prompt exposure, generated disclosure, artifact leakage, and attempted exfiltration.                                | Four-way artifact flags; Linux artifact separation; §Live extraction.                                                                                                                                                                                                                                               | None material.                                                                                                                                                                                                                                                | Ready for human decision     | Accept.                                                                                                                                                                                    |
| `003.5` | Unavailable live prerequisites produce `NOT_RUN` and leave the relevant acceptance incomplete.                                       | Historical `NOT_RUN` record preserved; Linux live retrieval remains `NOT_RUN`; task-input re-runs and the notification-sink run are now recorded.                                                                                                                                                                   | This is a reporting criterion. Accepting it does not claim live retrieval and does not complete live acceptance.                                                                                                                                              | Ready for human decision     | Accept the `NOT_RUN` reporting behavior while explicitly recording that Linux live retrieval remains incomplete; alternatively reject/defer if the approver requires live retrieval first. |
| `003.6` | Required verification passes and a focused PR records changed paths and residual risks.                                              | CI on PRs #337, #341, #349, #350, #351, #352; focused PRs; residual sections.                                                                                                                                                                                                                                       | Does not accept residual risks.                                                                                                                                                                                                                               | Ready for human decision     | Accept.                                                                                                                                                                                    |

**Classification summary**

- **Already accepted:** 16/16 on 2026-11-12 (13 manual accepted, 3 check-passed).
- **Ready for human decision:** none.
- **Blocked by missing evidence:** none for the criterion text as written. A
  claim that live retrieval is verified remains blocked; live retrieval is
  `NOT_RUN`.
- **Deferred under an existing recorded decision:** none.

### A.3 Executed commands (retained for audit)

The 13 manual commands in A.3.2 were executed on 2026-11-12; the 3 check runs in
A.3.3 are check-passed. Do not re-run them; a re-run would append duplicate
decisions or checks. They are retained so the acceptance can be audited against
the exact command and evidence text. The approver name is `Tom Tran` (the
preserved `human.designated_approvers` entry).

**A.3.1 Read-only verification**

```bash
sop verify --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --json
```

**A.3.2 Manual criteria (13 commands; executed 2026-11-12)**

Each command requires non-empty `--evidence` and at least one
`--evidence-ref path=...,sha256=...`. The digest below is the SHA-256 of
`docs/reports/owasp-llm/llm02-disclosure.md` at this revision. If that file
changes before the decision, recompute it with
`shasum -a 256 docs/reports/owasp-llm/llm02-disclosure.md`.

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 001.1 \
  --evidence "Provider authentication is an Authorization header separate from model-visible context; TestOpenAICompatibleExecutionReusesAuthorization passes." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 001.2 \
  --evidence "Redaction points are documented and implemented at userPrompt, toolResultMessage, redactDiagnosticText, handoff, outcome, and notification boundaries." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 001.3 \
  --evidence "Accepted for the deterministic scope: retrieval scope checks and post-retrieval validation are covered by TestSearchRejectsUnrelatedEvidence, TestRankerAdapterRejectsScopeItCannotHonor, and TestLocalRetrieverRequiresCurrent; live retrieval is NOT_RUN and excluded." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 001.4 \
  --evidence "Accepted as a bounded claim: credentials are distinguished from other confidential content and pattern matching is not general DLP; residual risks remain unaccepted." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 001.5 \
  --evidence "The assessment used synthetic canaries only; the campaign artifacts contain no real secret values." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-campaign.json,sha256=0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 002.4 \
  --evidence "Fixes reuse shared redaction and established authorization boundaries: redact.ModelInput, sensitivepath, toolharness, and userPrompt." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 002.5 \
  --evidence "Accepted as a bounded claim: unknown formats and arbitrary confidential text remain explicitly outside C1 and general DLP; residual risks remain unaccepted." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 003.1 \
  --evidence "The macOS artifact records a direct context disclosure and an indirect retrieval refusal; the Linux tool-path campaign records the five scoped tool paths." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-campaign.json,sha256=0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06" \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-linux-campaign.json,sha256=31c059b28590eb8e39e6c00e298f624b20f8a51fbefafb6fd885e5a2ffbc16a9" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 003.2 \
  --evidence "For the recorded cases, unauthorized effects were denied by authorization and policy; the executed set is empty and the Linux artifact records unauthorized_action=false." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-campaign.json,sha256=0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06" \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-linux-campaign.json,sha256=31c059b28590eb8e39e6c00e298f624b20f8a51fbefafb6fd885e5a2ffbc16a9" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 003.3 \
  --evidence "Synthetic canaries, inert .invalid targets, and loopback only; the confinement probes passed and no real credential or receiver was used." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-campaign.json,sha256=0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 003.4 \
  --evidence "Artifact flags and the report separate prompt exposure, generated disclosure, artifact leakage, and attempted exfiltration." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-campaign.json,sha256=0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06" \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 003.5 \
  --evidence "The historical NOT_RUN record is preserved, and Linux live retrieval remains NOT_RUN, leaving live acceptance incomplete; this accepts the reporting behavior only and claims no live retrieval." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --accept 003.6 \
  --evidence "Required checks and focused PRs are recorded for PRs #337, #341, #349, #350, #351, and #352; the residual sections are present." \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

**A.3.3 Check-mapped criteria (3 commands; check-passed 2026-11-12)**

`--run-check` executes the archived `go test` command, requires a clean tree,
records the tested revision, exit status, and log digest, and refuses to count a
check if the source changes during execution. Run these after the manual
decisions, or before them; the order does not matter. A passing check is
evidence for the tested revision, not human acceptance.

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --run-check 002.1 --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --run-check 002.2 --by "Tom Tran"
```

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --run-check 002.3 --by "Tom Tran"
```

**A.3.4 Rejection and annotation alternatives (not used)**

No rejection or annotation was recorded for Plan 03. The alternatives are
retained for audit. Use `--reject` instead of `--accept` with the same
`--evidence` and `--evidence-ref` requirements. Use `--annotate` to record reviewed manual
evidence without accepting it; it requires `--reviewed-evidence`, `--reason`, and
at least one verified `--evidence-ref`. `--accept`, `--reject`, and `--annotate`
are refused for the three check-mapped criteria; use `--run-check` for those.

```bash
sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure \
  --annotate 001.1 --reviewed-evidence "<reviewed evidence>" --reason "<reason>" \
  --evidence-ref "path=docs/reports/owasp-llm/llm02-disclosure.md,sha256=e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb" \
  --by "Tom Tran"
```

**A.3.5 After recording**

```bash
sop verify --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md \
  --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure
```

`accepted` should equal the number of recorded manual accepts plus passed
checks. Full acceptance requires all 13 manual decisions and all 3 check runs
to count; any rejection or failed check leaves the plan partially accepted.
Do not reopen the plan or edit the archive to change a result. A recorded manual
decision is bound to the evidence-ref digest; editing a referenced evidence file
afterwards makes the decision `stale` in `sop verify`. Record the decisions only
after the evidence files are final.

### A.4 What acceptance would and would not mean

- It binds the named evidence to the named criteria for the archived plan
  identity.
- It does **not** create general DLP, prompt secrecy, provider-retention
  control, or coverage of another model, provider, platform, or route.
- It does **not** close any residual risk in section C.
- It does **not** change the separate bounded evidence acceptance or the C1
  authorization.

### A.5 Recorded acceptance (2026-11-12)

- **Approver:** `Tom Tran`.
- **Plan identity:** `10e84b69258c63f02c702aed7e8ce7e7b3c2f7431a2b65f93ec819e9696835de`.
- **Ledger:** `.agent-sdlc/acceptance/plan-03-sop-llm02-sensitive-information-disclosure/`
  (`decisions.jsonl` 13 accept records; `checks.jsonl` 4 records, latest per
  criterion passing).
- **Decisions:** `001.1`–`001.5`, `002.4`, `002.5`, `003.1`–`003.6` accepted by
  `Tom Tran`; `002.1`–`002.3` check-passed.
- **Verification:** `accepted: 16`; `manual_accepted: 13`; `check_passed: 3`;
  `unverified: 0`; `stale: 0`; `rejected: 0`.
- **Field note:** `decision_eligible: 13` in `sop verify --json` is the static
  count of manual mappings. Every criterion status is `manually-accepted` or
  `check-passed`; zero criteria remain `decision-eligible`.
- **Scope:** the acceptance is limited to the criterion-specific evidence and
  scopes in A.2/A.3.2. No LLM02 residual risk is accepted, and no C1 default
  changed.

---

## B. C1 default enablement

### B.1 Verified current state

**Verified fact.** C1 is implemented and disabled in the tracked configuration.
There is no `redaction` block in `.agent-sdlc/config.yaml`; the zero policy is
the default. The final C1 code is in `internal/redact/policy.go` and
`internal/config/redaction.go`, merged through PRs #361, #362, and #364.

**Verified fact.** Enabling C1 requires an external inventory file:

- `config.validateRedaction` requires `inventory_path` when `enabled` is true.
- `config.RedactionPolicy` refuses a missing, malformed, symlinked, non-regular,
  inside-repository, or non-`0600` file.
- `redact.LoadInventory` refuses Windows.
- One load happens at startup; values are never logged or persisted.
- Bounds: `max_values` default 256, `min_value_bytes` default 8,
  `max_value_bytes` default 4096.

### B.2 Protection by model-input route

| Route                                                                               | C1 policy applies?                                                                                | Evidence                                                                                |
| ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| Ollama native tool harness, user prompt (`userPromptWithPolicy`)                    | Yes: task, caller input, output requirements                                                      | `internal/ollamaagent/prompt.go`; `TestUserPromptRedactsTaskAndInput`                   |
| Ollama native tool harness, tool results and errors (`toolResultMessageWithPolicy`) | Yes                                                                                               | `internal/ollamaagent/prompt.go`; `TestModelInputToolBoundaries`                        |
| OpenAI-compatible execution (`agent.NewRedacted`)                                   | Yes: `Task`, `Input`, `OutputRequirements`; baseline pass plus C1                                 | `internal/cli/cli.go`; `internal/agent/redacted.go`; `TestRedactedBeforeChatTransports` |
| Ollama chat, llama.cpp, MLX, generic provider (`agent.NewRedacted`)                 | Yes: same three fields; baseline pass plus C1                                                     | `internal/cli/cli.go`; `internal/agent/redacted.go`                                     |
| CLEF decision transport                                                             | Baseline `redact.ModelInput` only; C1 inventory/contextual/known-format rules do not run          | `internal/decisionadapter/providers/clef/chat.go`                                       |
| Nimble SystemOne decision transport                                                 | Baseline only; choices and criteria are not scrubbed                                              | `internal/decisionadapter/providers/nimble/translate.go`                                |
| Decision command provider                                                           | Not redacted: the operator's program receives the request                                         | `llm08-hidden-context.md` route inventory                                               |
| Julia runner                                                                        | Not redacted: local non-chat inference runtime                                                    | `llm08-hidden-context.md` route inventory                                               |
| Ollama system prompt                                                                | Not redacted; static rules, tool reference, capability name, output contract only                 | `llm08-hidden-context.md` route inventory                                               |
| Retrieval                                                                           | No C1; retrieval is disabled by default and scope checks run before search                        | `retrievalboundary`, `memindex`                                                         |
| Handoff capsules                                                                    | Known secret-named env values redacted; shape/unknown-format persistence redaction is not applied | `internal/handoff`; `llm02-disclosure.md`                                               |
| Notifications                                                                       | `redact.Shapes` plus known env values; C1 rules do not run                                        | `internal/notify/event.go`; Linux notification-sink run                                 |
| Model-output artifacts                                                              | No redaction; files are now `0600`                                                                | `internal/run/run.go`; `TestWriteArtifactIsOwnerOnly`                                   |
| Provider-side retention                                                             | Outside SOP control                                                                               | §Provider retention in section C                                                        |

### B.3 Known-format, unknown-format, and false-positive behavior

- **Covered when enabled:** exact inventory values, contextual credential keys,
  and approved known formats, in addition to the always-on known env values and
  credential shapes.
- **Not covered:** a context-free value with no credential-like key, no approved
  known format, and no inventory entry. This is observed, not hypothetical:
  `READ-OPAQUE`, `COMMAND-OPAQUE`, and `DIFF-OPAQUE` reached the model in the
  Linux tool-path campaign; `READ-OPAQUE` reached it in the macOS task-input
  re-run. C1 does not close this residual.
- **False positives:** the approved corpus keeps ordinary words (`secretary`,
  `keyboard`, `monkey`), public-key names, and `.env.example`/`.env.sample`
  placeholders visible. The 35-case assessment records 27 redacted, 1
  context-free residual, and 7 ordinary controls. No false-positive measurement
  on a large real repository exists.
- **Entropy:** not implemented. The measured false positives at 3.5 and 4.0
  bits/char make it unsuitable; see
  [PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md](../../plans/PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md).

### B.4 Performance, compatibility, and isolation

- **Performance:** one bounded inventory load at startup; per-input string and
  regex passes. No benchmark is recorded. The cost is bounded by input size and
  inventory count (default 256 values).
- **Compatibility:** the external `0600` file and Unix-only loader mean a
  committed `enabled: true` would fail agent creation on clones that lack the
  file, and on Windows. The tracked `.agent-sdlc/config.yaml` cannot name a path
  that exists on every clone.
- **Isolation:** C1 is model-input redaction; the subprocess sandbox limits file
  reads. They are independent. C1 does not prevent a read, and the sandbox does
  not redact model input. Enabling C1 does not change the isolation default.
- **Local enablement and clean trees:** editing the tracked config to enable C1
  dirties the working tree, and `--run-check` refuses a dirty tree. Enable C1
  after the acceptance checks, or in a separate checkout used for C1-enabled
  operation.

### B.5 Recommendation

**Keep C1 opt-in. Do not commit `redaction.enabled: true`.**

Rationale:

1. A committed default fails closed on every clone that does not have the
   operator's external inventory, and on Windows. That would turn a security
   feature into an agent-startup outage.
2. The external inventory is operator-owned, rotating, and machine-specific; it
   is not repository configuration.
3. Enabling C1 does not close the demonstrated context-free residual, so the
   default change would not by itself justify the compatibility cost.
4. C1 is already available and verified for operators who can supply the
   inventory. The operational control is the inventory, not a repository
   default.

This is a recommendation, not a recorded decision. The approver may instead
choose to accept the residual and keep C1 disabled, or defer until a local
override mechanism and Windows support exist.

### B.6 Exact change if the approver enables C1 on a host

Do this in a local, uncommitted configuration (or a dedicated checkout):

```yaml
redaction:
  enabled: true
  inventory_path: /absolute/path/outside/the/repository/sensitive-values.txt
  # optional bounds
  max_values: 256
  min_value_bytes: 8
  max_value_bytes: 4096
```

Prerequisites: the file is a regular file, not a symlink, `0600`, outside the
repository, on a Unix-like filesystem; one value per line; each value at least 8
bytes. A malformed or insecure file fails agent creation closed. Do not commit
this block to the tracked config.

If the approver wants **default** enablement, the required changes are larger
and are **not recommended now**: provision a per-clone inventory (or change the
fail-closed contract), add Windows support, and provide a migration story. Those
are code/policy changes, not a configuration edit.

### B.7 C1 evidence identities

| Evidence                   | Identity                                                                                                                                                                                                |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C1 Linux live campaign     | `llm02-c1-linux-campaign.json`, SHA-256 `14e41eaf1a7f23ffad37cadc35e7f7e45910777d2fea64115455475e2ca16cfa`; 12 protected, 3 positive controls, 1 bounded residual                                       |
| C1 hardlink-alias campaign | `llm02-c1-hardlink-campaign.json`, SHA-256 `417599e2a561f202dc70ead1243ae70d3487f614d45c395bc1ba7a10f7cadfb3`; 4 protected, 1 positive control, 1 unlisted-value residual                               |
| Linux tool-path campaign   | `llm02-linux-campaign.json`, SHA-256 `31c059b28590eb8e39e6c00e298f624b20f8a51fbefafb6fd885e5a2ffbc16a9`; 17 requests, 7 protected, 6 positive controls, 4 residual exposures                            |
| macOS task-input re-run    | `llm02-campaign-darwin-task-input.json`, SHA-256 `ea42236caba243a7cdddf560133ca0171cf0255be1e9ddb3637ff68a2c3298c8`; 12 requests; `TASK-INPUT` protected, `READ-OPAQUE` residual                        |
| Linux task-input re-run    | `llm02-campaign-linux-task-input.json`, SHA-256 `a907298fd725b7fc254d67dbcd7eaf33afed54703f6fcd409ba58cd2277326e7`; 18 requests; `TASK-INPUT` protected, three opaque residuals                         |
| Linux notification sink    | `llm02-campaign-linux-notify-sink.json`, SHA-256 `7e0d235a92faf12657ec5b6849061d479c2be397d2f4e67151bb73bc96b5fab3`; one `protected` local-receiver run; does not cover Slack rendering/channel binding |
| macOS Option C attempt 3   | `llm02-option-c-phase1-macos-campaign-attempt3.json`, SHA-256 `50cc9e0960a260f911f7e5b9ea5fe6ead45dc30f3d3726c8a7d173775aa8062a`; 5 protected, 1 positive control, 0 residual                           |
| Seven-format enhancement   | `llm02-seven-format-campaign.json`, SHA-256 `316ac2e87f13719c16eaf79b3ceff83f5841b0f9e8033360a59d9f1a22078553`                                                                                          |

---

## C. Residual risks

All rows below are **proposed dispositions** unless the "Existing recorded
decision" column names an approval. No risk acceptance, transfer, waiver, or
rejection is recorded by this packet.

Default-on `run_command` isolation (OWASP Gap 13, PRs #441 and #444) moves R3's
baseline: the direct read is denied by default and reachable only through the
operator override on macOS/Linux. R2, R4 and the host-to-sandbox Unix socket
direction are assessed and recorded separately in
[PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md](../../plans/PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md):
R2 and R4 as bounded/known limits (gaps 5 and 6) and H2S as a bounded architectural
property (Option A, decision memory `a73749c2…`, 2026-11-12). None of them is a
risk acceptance. This packet does not dispose of them, and R3 is not closed.

| ID  | Residual                                                     | Verified current behavior                                                                                                                                                                                                                                                                                 | Existing mitigation and evidence                                                                                                                                                                                                                                        | Existing recorded decision                                                                                                                                                                                                                                                              | Remaining exposure                                                                                                                                                                    | Recommended disposition                                                                                                                                                   | Human approval required?                          |
| --- | ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| R1  | Unknown-format context-free values                           | A value with no key, shape, approved format, or inventory entry reaches the model. Observed in `READ-OPAQUE`, `COMMAND-OPAQUE`, `DIFF-OPAQUE` (Linux) and `READ-OPAQUE` (macOS).                                                                                                                          | A+B shape/path redaction; C1 exact inventory, contextual keys, known formats (opt-in). Evidence: C1 campaigns and seven-format campaign.                                                                                                                                | C1 authorization `e233daee7645d770d0813713b82523e4272d0b6ccb03c3e5974e5f434f0debe1` (opt-in). Unknown-format proposal is `NOT_APPROVED` and recommends `ACCEPT_PROPOSED`.                                                                                                               | A context-free secret in ordinary content, diff, SAFE output, or caller input can reach the model even with C1 enabled unless it is in the inventory.                                 | `ACCEPT_PROPOSED` the bounded residual; keep C1 opt-in and the inventory as the operational control; reject entropy.                                                      | Yes — risk acceptance and any policy change.      |
| R2  | File-tool hardlink/path alias                                | Hardlink aliases bypass the lexical credential-path check. Option C Phase 1 denies aliases to protected workspace credentials on Linux and macOS. Outside-workspace origins, late external credentials, external removal before the first build, and Windows remain open; `Nlink > 1` is not implemented. | `internal/toolharness/identity.go`; Option C Phase 1 campaigns (Linux `fcc621ef…`, macOS attempt 3 `50cc9e09…`); `TestIdentityEnforcementDirectAndLateAliases`.                                                                                                         | Option C Phase 1 approved and implemented (decision memory `a01d2007e600ff5b4fa76eebb0eb8a8e6644efaeb68183beb66c14d0a462a2db`, source `operator:option-c-phase1-2026-11-10`; PR #381; macOS verified PR #432).  Option A `Nlink > 1` not approved. The bounded limits this row recommends were recorded on 2026-11-12 as gap 5 option 1 (bounded residual, not accepted); see [PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md](../../plans/PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md).                                       | Aliases outside the workspace registry, late-created credentials, external removal, and Windows. Under C1, an unlisted opaque value read through an alias still reaches the model.    | Record the outside-workspace, late-credential, external-removal, and Windows cases as bounded residual limits; reject `Nlink > 1`; keep Phase 1.                          | Yes — residual acceptance or a new control.       |
| R3  | SAFE `run_command` direct credential read with isolation off | `go test` is `SAFE`, so model-written test code can read `$HOME` credentials or `os.Link` them into the repository. With isolation on (the default) the read is denied; it works only under the operator override on a platform that has a backend, and on a platform with none no command runs at all. Reproduced with synthetic canaries through the real harness on macOS arm64 and Linux arm64 (`TestDarwinIsolationSAFECommandCredentialRead`, `TestLinuxIsolationSAFECommandCredentialRead`). Isolation is now on by default, so the direct read is denied unless an operator sets `SOP_AGENT_RUN_COMMAND_ISOLATION=off`.                                                                                                                                | Opt-in sandbox read-deny for macOS and Linux; secret env scrubbing; `ln` refusal; Keychain mach lookup denied on macOS. Evidence: `llm02-sandbox-credential-linux-campaign.json` (SHA-256 `1baa836ba7511d73dc79c907e6e6af01d86ca62936d634d08c1687b8ddb661f3`), PR #384; `TestDarwinIsolationSAFECommandCredentialRead` (new) and `TestLinuxIsolationSAFECommandCredentialRead` for the SAFE-command vector; default-on feasibility in [llm06-sandbox-default-measurements.md](llm06-sandbox-default-measurements.md). | Direct reads with isolation off are **not accepted**. The bounded pre-existing subprocess hardlink alias is `RISK_ACCEPTED — SUBPROCESS HARDLINK ALIAS (BOUNDED)` under Option D (same decision-memory repository, source `operator:option-d-subprocess-hardlink-2026-11-10`; PR #387). | Under the operator override on macOS/Linux, direct reads and `os.Link` remain open; on a platform with no backend the command is refused (Windows B1), and no override value changes that. With isolation on, a pre-existing alias remains readable inside the sandbox on Linux (Option D) and on macOS (measured), and the macOS parent launch-time environment is readable through `kern.procargs2`, so a credential in SOP's environment survives the sandbox.                                   | **`MITIGATED` for the direct-read vector, bounded.** The operator approved default-on isolation with the operator override and Windows B1, and it is implemented and verified ([PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md](../../plans/PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md)); a missing backend refuses the command instead of running it unsandboxed. The hardlink alias (R2/Option D) and the macOS parent-environment read (R4) survive the sandbox and remain **open and unaccepted** — this row does not dispose of them. Do not reopen Option D. | Yes — risk acceptance or deferral.                |
| R4  | macOS parent-environment visibility (`kern.procargs2`)       | With the sandbox on, a sandboxed process can read the parent environment through `kern.procargs2`; narrow `sysctl` denies do not work and a full deny breaks the Go runtime.                                                                                                                              | Measured in [seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md) (PR #406).                                                                                                                                                                              | Recorded as a known limit for the opt-in sandbox on 2026-11-12 (gap 6, PR #447). Not accepted; no option implemented. | Parent environment values remain visible to a sandboxed process.                                                                                                                      | Accept the limit formally, or authorize an allow-list / execution-model design. Do not weaken the sandbox. | Yes — risk acceptance or design authorization.    |
| R5  | Model-output artifact persistence                            | `implementation.md`, `fix-N.md`, `result.md`, `plan.md`, `report.md`, and `diff.patch` are written from model output; `diff.patch` is deliberately verbatim evidence. Files are now `0600`.                                                                                                               | `internal/run/run.go` writes `0600`; `TestWriteArtifactIsOwnerOnly`; `TestRunArtifactPersistsModelOutputVerbatim`. Option C implemented in PR #408.                                                                                                                     | Option C (permissions) implemented in PR #408; Option A (`Run.WriteReply` redacts reply files) implemented and merged in PR #450, at the operator's chat instruction on 2026-11-12. No decision-memory entry located for either.                                                                    | Shaped secrets are redacted in `implementation.md`, `fix-N.md`, `result.md`; measured 0 of 3 persisted. An unknown-format value the model echoes is still persisted (3 of 3, darwin arm64, `llm02-artifact-after-option-a-darwin.json`). `diff.patch` and other evidence remain verbatim. `plan.md` and `report.md` are not measured. | Unknown-format residual for reply files: the gap index records `accepted residual (operator, 2026-11-12)`; no decision-memory entry. Recommend recording it, or authorizing an opt-in control. | Yes — record the unknown-format residual acceptance if the approver wants a decision-memory entry. |
| R6  | Caller-owned and notification prose                          | `notify.NewEvent` applies `redact.Shapes` plus known env values; C1 rules do not run. Linux notification-sink run is `protected` for shape and known-value canaries with a local receiver; an unshaped value not supplied as a known value would pass.                                                    | `TestLLM08NotificationRedactsCredentialShapes`; `TestLLM08OutboxRowOmitsSecret`; `llm02-campaign-linux-notify-sink.json`.                                                                                                                                               | No recorded decision. Recommendation in the six-item matrix: `MITIGATE`.                                                                                                                                                                                                                | Unknown-format caller/notification prose; Slack rendering and channel binding remain gap 12.                                                                                          | `MITIGATE` by extending the notification descriptive redaction to `redact.ModelInput` (baseline plus C1 when configured), or `ACCEPT_PROPOSED` caller-owned prose.        | Yes — code change or risk acceptance.             |
| R7  | Retrieval and handoff inputs                                 | Retrieval is disabled by default; `retrievalboundary` validates scope and `memindex` filters. Handoff redacts known secret-named env values; handoff capsule persistence lacks shape/unknown-format redaction. Linux live retrieval is `NOT_RUN`.                                                         | `TestSearchRejectsUnrelatedEvidence`; `TestRankerAdapterRejectsScopeItCannotHonor`; `TestLocalRetrieverRequiresCurrent`; handoff tests.                                                                                                                                 | No recorded decision. Recommendation: `INVESTIGATE` / `ACCEPT_PROPOSED`.                                                                                                                                                                                                                | A live-retrieval claim is unsupported. Cross-scope retrieval is rejected deterministically, but live behavior is not observed. Handoff persistence may retain unknown-format content. | `ACCEPT_PROPOSED` the disabled-by-default boundary, with live retrieval still `NOT_RUN`; or authorize a live-retrieval verification when a producer/index exists.         | Yes — accept the boundary or authorize live work. |
| R8  | Provider retention and general DLP                           | SOP does not control provider-side request/response retention. The API key is an Authorization header, not model content, and SOP does not persist the request body.                                                                                                                                      | Model-input redaction; auth-header separation; local-model option.                                                                                                                                                                                                      | No recorded decision. Recommendation: `TRANSFER_PROPOSED`.                                                                                                                                                                                                                              | Provider retention, data residency, and general DLP remain external.                                                                                                                  | `TRANSFER_PROPOSED` to the provider/compliance owner. No SOP code change.                                                                                                 | Yes — provider/compliance owner.                  |

### C.1 Existing recorded approvals (not reopened)

| Decision                          | Scope                                                                                                                      | Recorded identity                                                                                                                                |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| Bounded evidence acceptance       | Approved A+B implementation plus PR #349 user-prompt redaction, within documented scope; explicitly not Plan 03 acceptance | Decision memory `59d986984142bef314ea784a401b204cd57b7ca1664f570b4763c13a5f580c98`, source `operator:bounded-acceptance-2026-11-10`              |
| C1 authorization                  | Opt-in inventory plus contextual credential keys and approved known formats                                                | Decision memory `e233daee7645d770d0813713b82523e4272d0b6ccb03c3e5974e5f434f0debe1`, source `operator:c1-approval-2026-11-10`                     |
| Option C Phase 1                  | Hardlink/path identity for protected workspace credentials; Linux implemented and verified; macOS verified in attempt 3    | Decision memory `a01d2007e600ff5b4fa76eebb0eb8a8e6644efaeb68183beb66c14d0a462a2db`, source `operator:option-c-phase1-2026-11-10`; PRs #381, #432 |
| Option D                          | Bounded pre-existing subprocess hardlink alias inside the opt-in Bubblewrap sandbox                                        | Same decision-memory repository, source `operator:option-d-subprocess-hardlink-2026-11-10`; PR #387                                              |
| Seven-format enhancement          | Seven approved provider formats under C1                                                                                   | Decision memory `c7f143fe3b1f3e79308f6f9bcdaf7657a17cae36107739e54a1700d90a83366b`, source `operator:llm02-seven-format-2026-11-10`              |
| A+B model-input credential policy | Direct credential-path denial and model-bound tool-result/error redaction                                                  | Decision memory `f891d5cbe5dd578addbba83ee95b47c35752970219b4d0ad6d3fc2be254a741c`                                                               |
| Plan 03 per-criterion acceptance  | 13 manual accepted + 3 check-passed; scoped to the evidence in §A.3.2; no residual risk accepted                           | Historical acceptance ledger `.agent-sdlc/acceptance/plan-03-sop-llm02-sensitive-information-disclosure/`; approver `Tom Tran`; 2026-11-12        |

### C.2 Proposed versus existing

- **Existing recorded approvals:** the seven rows in C.1. They are preserved.
- **Proposed dispositions:** R1 `ACCEPT_PROPOSED`, R2 bounded residual limits,
  R3 `MITIGATED` for the direct-read vector (default-on approved and implemented), with R2 and R4
  still open, R4 accept as known limit or authorize a design, R5 Option A merged (PR #450) with the unknown-format residual open, R6 `MITIGATE`, R7 `ACCEPT_PROPOSED`, R8
  `TRANSFER_PROPOSED`.
- **Unresolved evidence:** Linux live retrieval; a Linux or `plan.md`/`report.md`
  echo campaign for model-output artifacts (the darwin campaign covers reply
  files only); Slack rendering/channel binding; Windows hardlink
  identity; `Nlink > 1`; a live chat-transport campaign.
- **Human authority:** every acceptance, rejection, waiver, risk acceptance,
  transfer, memory entry, and configuration default change.

### C.3 What this packet does not decide

It records Plan 03 acceptance only. It does not change any default, record any
residual-risk decision, edit the archive, reopen Plan 03, or claim full LLM02
coverage. LLM02 remains `OPEN`, and every residual risk in section C remains
unaccepted except the bounded Option D decision already recorded.

---

## D. Single-sitting decision list

1. **Plan 03:** **DONE (2026-11-12).** The 13 manual decisions and 3 check
   runs are recorded; `sop verify` reports `accepted: 16/16`. Do not re-record.
2. **C1:** keep C1 opt-in (recommended), or explicitly choose default
   enablement with the prerequisites in B.6.
3. **R1:** accept the context-free unknown-format residual, or keep it open and
   require a separate control.
4. **R2:** record the outside-workspace, late-credential, external-removal, and
   Windows cases as bounded limits, or authorize a new control.
5. **R3:** **DONE** — default-on isolation with the operator override and Windows B1
   is approved and implemented. R2's hardlink alias and R4's parent-environment
   read still need their own disposition; do not reopen Option D.
6. **R4:** accept the macOS parent-environment limit, or authorize a design.
7. **R5:** Option A is merged (PR #450). Record the unknown-format reply-artifact
   residual as an acceptance, or authorize a control for it. Do not re-approve Option A.
8. **R6:** approve the notification-prose extension, or accept caller-owned
   prose.
9. **R7:** accept the disabled-by-default retrieval/handoff boundary, or
   authorize live retrieval work.
10. **R8:** transfer provider retention to the provider/compliance owner.

## E. Evidence index

All paths are repository-relative. Digests are SHA-256 of the current files.

| Path                                                                        | SHA-256                                                            |
| --------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `docs/reports/owasp-llm/llm02-campaign.json`                                | `0c8cc0ff132482db7a531bd197473c11066ae28c6d4ba35a912aa25a7292ad06` |
| `docs/reports/owasp-llm/llm02-linux-campaign.json`                          | `31c059b28590eb8e39e6c00e298f624b20f8a51fbefafb6fd885e5a2ffbc16a9` |
| `docs/reports/owasp-llm/llm02-c1-linux-campaign.json`                       | `14e41eaf1a7f23ffad37cadc35e7f7e45910777d2fea64115455475e2ca16cfa` |
| `docs/reports/owasp-llm/llm02-c1-hardlink-campaign.json`                    | `417599e2a561f202dc70ead1243ae70d3487f614d45c395bc1ba7a10f7cadfb3` |
| `docs/reports/owasp-llm/llm02-campaign-darwin-task-input.json`              | `ea42236caba243a7cdddf560133ca0171cf0255be1e9ddb3637ff68a2c3298c8` |
| `docs/reports/owasp-llm/llm02-campaign-linux-task-input.json`               | `a907298fd725b7fc254d67dbcd7eaf33afed54703f6fcd409ba58cd2277326e7` |
| `docs/reports/owasp-llm/llm02-campaign-linux-notify-sink.json`              | `7e0d235a92faf12657ec5b6849061d479c2be397d2f4e67151bb73bc96b5fab3` |
| `docs/reports/owasp-llm/llm02-option-c-phase1-campaign.json`                | `fcc621ef272dbb12791facf9d2254c098816a1eb673176be5137fcac9f5bfc92` |
| `docs/reports/owasp-llm/llm02-option-c-phase1-macos-campaign-attempt3.json` | `50cc9e0960a260f911f7e5b9ea5fe6ead45dc30f3d3726c8a7d173775aa8062a` |
| `docs/reports/owasp-llm/llm02-seven-format-campaign.json`                   | `316ac2e87f13719c16eaf79b3ceff83f5841b0f9e8033360a59d9f1a22078553` |
| `docs/reports/owasp-llm/llm02-sandbox-credential-linux-campaign.json`       | `1baa836ba7511d73dc79c907e6e6af01d86ca62936d634d08c1687b8ddb661f3` |
| `docs/reports/owasp-llm/llm02-disclosure.md`                                | `e06325a8402f445b5707b434269b079849fa1132edf9592872ac1ae3c044e3bb` |

The acceptance ledger is local, Git-ignored evidence and is not committed:
`.agent-sdlc/acceptance/plan-03-sop-llm02-sensitive-information-disclosure/`
(`decisions.jsonl`, `checks.jsonl`, and the check logs).
