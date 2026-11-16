# OWASP LLM Top 10 — acceptance report (2026-11-16)

**Repository:** `imhttran/agentic-sop`
**Revision:** `origin/main` at `c55b42ff` (2026-11-16), darwin arm64, Go 1.27.1,
Git 2.50.1.
**Scope of this report:** a verification-and-remediation pass over all ten OWASP
2025 LLM categories. It records what was verified from source and tests, what was
implemented in this pass, and what remains a governance decision.

This is **not** a compliance certification and does not claim complete protection
against any risk. Coverage classifications are:

- `VERIFIED` — implemented and tested within the stated scope.
- `PARTIAL` — controls exist but coverage is incomplete.
- `BLOCKED` — an unresolved decision prevents the stated acceptance criterion.
- `NOT_APPLICABLE` — no corresponding SOP-owned capability.
- `NOT_RUN` — required verification has not occurred.

## Summary

| OWASP 2025                             | Repo plan | Classification            | Change in this pass                               | Open decision                   |
| -------------------------------------- | --------- | ------------------------- | ------------------------------------------------- | ------------------------------- |
| LLM01 Prompt Injection                 | Plan 02   | `VERIFIED` (bounded)      | none                                              | none                            |
| LLM02 Sensitive Information Disclosure | Plan 03   | `PARTIAL`                 | none (reviewed)                                   | C1 enablement, residuals        |
| LLM03 Supply Chain                     | Plan 04   | `PARTIAL` + `BLOCKED`     | scanners re-run; git output integrity implemented | M1+G3+G4; baseline/enforcement  |
| LLM04 Data and Model Poisoning         | Plan 05   | `PARTIAL`                 | #488/#489 verified; index guard verified          | none                            |
| LLM05 Improper Output Handling         | Plan 10   | `VERIFIED` (SOP sinks)    | none                                              | none                            |
| LLM06 Excessive Agency                 | Plan 01   | `PARTIAL` + `BLOCKED`     | git invocation hardening                          | M1+G3+G4                        |
| LLM07 System Prompt Leakage            | Plan 08   | `PARTIAL` (by design)     | none                                              | none                            |
| LLM08 Vector and Embedding Weaknesses  | Plan 09   | `VERIFIED` (shipped path) | none                                              | none                            |
| LLM09 Misinformation                   | Plan 07   | `PARTIAL` (by scope)      | git output labelled                               | clean-filter fingerprint policy |
| LLM10 Unbounded Consumption            | Plan 06   | `VERIFIED` (bounded)      | none                                              | none                            |

## Verification performed in this pass

All commands run from a worktree at `origin/main` `c55b42ff` on darwin arm64.
Focused packages were run locally, and **full CI passed on the pull-request head**
(`f0d8d399`, 13 of 13 checks: build/test, race detector, real
Semgrep+Gitleaks+OSV integration, installer packaging, Windows, tagged
acceptance, CodeQL; runs
[43924409662](https://github.com/imhttran/agentic-sop/actions/runs/43924409662),
[43924403486](https://github.com/imhttran/agentic-sop/actions/runs/43924403486)).
The change is **unmerged**.

```
go build ./...                                                             # ok
go vet ./internal/toolharness/                                             # ok
go test ./internal/toolharness/ -count=1                                   # ok  33.5s
go test ./internal/ollamaagent/ -count=1                                   # ok  25.5s
go test ./internal/agent/ ./internal/commandpolicy/ ./internal/security/ -count=1  # ok
go test ./internal/redact/ ./internal/repoindex/ ./internal/decisionmemory/ \
        ./internal/context/ ./internal/ragclient/ ./internal/notify/ \
        ./internal/procgroup/ ./internal/testrunner/ -count=1              # ok
go test ./internal/archtest/ -count=1                                      # ok
go test ./internal/cli/ -run 'Memory|RAG|Unverified|Isolation|TokenBudget' -count=1  # ok
```

Scanner sweep with the pinned adapters (Gitleaks 8.30.1 pinned binary
`ba52fb1b…`, Semgrep 1.183.0, OSV-Scanner 2.7.0):

```
verdict FAIL, coverage COMPLETE
findings 101, suppressed 84, unsuppressed 17, blocking 1
```

- The single blocking finding is `dangerous-exec-command` at the harness's one
  `exec.CommandContext` call site. It is the deliberately unaccepted D4-5
  residual and remains visible.
- The 16 non-blocking unsuppressed findings are `MEDIUM` semgrep rules in
  `_test.go` files; they do not meet `fail_on` (`critical`, `high`).
- Baseline integrity: 84 entries, 84 unique fingerprints, 84 matched, 0 expired,
  earliest expiry `2027-03-31`. No entry was added, edited, or suppressed.
- Fail-closed was demonstrated, not assumed: with the Homebrew Gitleaks binary
  (sha256 `f414bc2f…`, not the pin) the `secrets` category reported `FAILED`,
  overall `verdict` stayed `FAIL` and coverage `UNKNOWN`. It never produced a
  `PASS`. Re-running with the pinned binary restored `COMPLETE`.

## Per category

### LLM01 — Prompt Injection · `VERIFIED` (bounded)

- **Controls:** tool authorization, command policy (`SAFE` /
  `REQUIRES_APPROVAL` / `DENIED`), and the default-on `run_command` sandbox.
  `REQUIRES_APPROVAL` is refused, never escalated.
- **Evidence:** bounded live campaigns recorded in
  [llm01-injection.md](llm01-injection.md) (macOS 4/4 denied + positive control
  allowed; Linux `qwen3.5:4b` 4/4; macOS `llama3.2:3b` 4/4). No change in this
  pass; `internal/agent` and `internal/toolharness` tests pass.
- **Remaining gaps:** two models on macOS, one on Linux, one run per case. Not a
  claim about other models or providers.
- **Classification:** `VERIFIED` within the recorded scope.

### LLM02 — Sensitive Information Disclosure · `PARTIAL`

- **Controls:** model-bound tool results and errors are redacted
  (`redact.ModelInput`); credential path denial for `read_file`, `search_files`,
  `copy_file`; the rendered user prompt is redacted; Option C Phase 1 denies
  hardlink aliases of protected workspace credentials on Linux and macOS; C1
  unknown-format redaction is implemented but **opt-in and disabled**.
- **Evidence:** Plan 03 acceptance 16/16 (2026-11-12); Linux/macOS live campaigns;
  `internal/redact` and `internal/toolharness` disclosure tests pass.
- **Remaining gaps / residuals:** not general DLP; unknown-format values in
  ordinary files, command output, and diffs; outside-workspace hardlink origins;
  Windows; artifact persistence; provider retention. R2 and R4 are bounded
  accepted residuals; the Option D subprocess hardlink alias is accepted within
  its scope. The context-free unknown-format residual is `NOT_APPROVED`.
- **Change in this pass:** none. No evidence justified widening pattern matching;
  source-level isolation remains preferred. C1 enablement is unchanged.
- **Classification:** `PARTIAL`.

### LLM03 — Supply Chain · `PARTIAL` + `BLOCKED`

- **Controls verified:** SHA-256 pins for scanner archives; execution-time
  `binary_sha256` / `rules_sha256` checks; verified private-copy execution closing
  the hash-then-exec race; `stageRules` staging verified rules bytes. Semgrep
  reported `rules_identity d90ff1b1…`, matching the committed rules pin.
- **Verified in this pass:** pinned scanners re-run to `COMPLETE` coverage; the
  binary pin refused an unpinned binary (fail-closed); a scanner failure produced
  `verdict FAIL`, never `PASS`; baseline fingerprints are unique and unexpired.
- **Implemented in this pass (local, unmerged):** git invocation hardening and a
  labelled clean-filter residual — see
  [git-output-integrity.md](git-output-integrity.md).
- **Remaining gaps:** no committed execution-time pin for cargo-audit, Semgrep,
  or Windows; no publisher signatures; provider model provenance is outside SOP.
  The `run_command` finding stays blocking and unsuppressed.
- **Blocked:** the D4-5 remediation needs operator authorization (M1+G3+G4).
- **Classification:** `PARTIAL`; the D4-5 acceptance criterion is `BLOCKED`.

### LLM04 — Data and Model Poisoning · `PARTIAL`

- **Controls verified:**
  - PR #488 labels RAG evidence that carries no `chunk_hash` as
    `chunk=unverified (no chunk_hash)` in the model context and counts it in
    `rag.json`. Re-verified in source and by
    `TestUnverifiedEvidenceLabelled` (`internal/cli`).
  - PR #489 makes a full decision store refuse `memory add` instead of evicting
    the smallest-ID record, and lists stale ids so they can be retired
    (`TestMemoryAddRefusesWhenStoreFull`, `TestMemoryListShowsStaleIDs`).
  - Per-row index digests are guarded by
    `TestPersistedRepoIndexReadersVerifyRows` (`internal/archtest`), so a future
    reader that skips `repoindex.VerifyRows` fails the build.
  - Provenance on decision memory; recovery by reindex/remove.
- **Remaining gaps:** a digest proves bytes, not benign content; `document_hash`
  stays unverified until the producer emits `chunk_hash`; provider-side training
  is external.
- **Classification:** `PARTIAL`. No unaccepted defect was found in the two
  recently merged integrity PRs.

### LLM05 — Improper Output Handling · `VERIFIED` (SOP sinks)

- **Controls:** terminal output and the Slack payload encoder are escaped;
  loopback transport verified; live Slack transport delivered once.
- **Remaining gaps:** third-party terminal/IDE rendering, persisted Markdown, and
  Slack unfurl are external. Escaped-markup rendering is `NOT_OBSERVED`.
- **Classification:** `VERIFIED` for SOP-owned sinks; external rendering is out of
  scope.

### LLM06 — Excessive Agency · `PARTIAL` + `BLOCKED`

- **Controls verified:** root/command policy with read-only roots; default-on OS
  sandbox for `run_command` on macOS/Linux (Seatbelt / bubblewrap) with
  fail-closed refusal where no backend exists (Windows); the override requires
  `SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK`; control variables are dropped from
  model-controlled child environments; a per-run throwaway build cache.
- **Implemented in this pass (PR #491, unmerged):** every SOP-run git command now
  carries `-c core.fsmonitor=false` and, for `diff`,
  `--no-ext-diff --no-textconv`, and inherits a filtered environment. Measured:
  this blocks `diff.external`, `GIT_EXTERNAL_DIFF`, `core.fsmonitor`, and
  textconv.
- **Remaining gaps:** `run_command` is the only sandboxed path. Validation
  commands, scanners, and the `git`/`gh` sinks stay unsandboxed; a clean filter
  still executes and can transform git output; pre-existing hardlink aliases, the
  macOS `kern.procargs2` parent-environment read, and host-to-sandbox Unix sockets
  remain recorded residuals.
- **Blocked:** making sandboxing mandatory for every model-reachable exec and
  denying `.git` writes (M1+G3+G4) changes the approved override and removes
  `git_status`/`git_diff` where no backend exists. It needs operator
  authorization; exact wording in the proposal §11.
- **Classification:** `PARTIAL`; the mandatory-isolation acceptance criterion is
  `BLOCKED`.

### LLM07 — System Prompt Leakage · `PARTIAL` (by design)

- **Controls:** hidden context is not echoed into notifications or diagnostics;
  descriptive fields are redacted; model input is redacted at the tool-result
  boundary.
- **Remaining gaps:** SOP does not treat its prompts as secret and does not rely
  on prompt secrecy, so prompt secrecy is not claimed. Unknown-format secrets in
  command output, provider retention, and persisted model output remain bounded.
- **Classification:** `PARTIAL` by design.

### LLM08 — Vector and Embedding Weaknesses · `VERIFIED` (shipped path)

- **Controls:** retrieval isolation and authorization; bounded
  cancellation/deadline handling; advisory retrieval disabled by default.
  `internal/ragclient` and `internal/context` tests pass.
- **Live evidence:** one bounded run against disposable PostgreSQL + pgvector with
  a local embedding model: cross-corpus isolation held, an unauthorized corpus was
  rejected, positive controls returned their documents, and an embedded
  instruction arrived as untrusted evidence text.
- **Remaining gaps:** one producer, one embedding model, one run, synthetic data;
  `TIMEOUT`/cancellation not observed live; semantic poisoning inside an
  authorized repository is not prevented (see LLM04).
- **Classification:** `VERIFIED` for the shipped, disabled-by-default path.

### LLM09 — Misinformation · `PARTIAL` (by scope)

- **Controls:** decision evidence is bound to real command output and reviewed
  repository content; a `go test -run` selection that matched no tests is flagged
  so exit 0 is not read as passing. Git output that a repository-configured filter
  transformed is now labelled rather than presented as raw.
- **Remaining gaps:** SOP does not check that model prose is factually correct. A
  hostile clean filter could still under-report changes to a working-tree
  fingerprint — see the clean-filter residual in
  [git-output-integrity.md](git-output-integrity.md) — which needs an operator
  decision because closing it would disable mutation detection on `git-lfs`
  repositories.
- **Classification:** `PARTIAL` by scope.

### LLM10 — Unbounded Consumption · `VERIFIED` (bounded)

- **Controls:** read/output bounds; default provider output-token cap;
  per-command timeouts; MCP frame bounds; atomic continuation accounting;
  process-group termination on the guarded subprocess paths; a guard test that
  fails when a new `exec.CommandContext` file binds no process group; an opt-in
  per-run token budget that fails closed when usage is unmeasurable.
- **Remaining gaps:** the token budget is off by default and is a token cap, not a
  dollar cap; the request that crosses the budget still completes (bounded by one
  request).
- **Classification:** `VERIFIED` for time/output/process bounds.

## Implemented in this pass

| Change                                            | Files                                                                          | Status                                |
| ------------------------------------------------- | ------------------------------------------------------------------------------ | ------------------------------------- |
| Git invocation hardening + clean-filter labelling | `internal/toolharness/git_integrity.go`, `harness.go`, `git_integrity_test.go` | PR #491; CI 13/13 green; **unmerged** |
| Evidence: git output integrity                    | `docs/reports/owasp-llm/git-output-integrity.md`                               | PR #491; **unmerged**                 |
| This acceptance report                            | `docs/reports/owasp-llm/acceptance-report-2026-11-16.md`                       | PR #492 (chained); **unmerged**       |
| Plan of record                                    | `docs/plans/PLAN-OWASP-LLM-E2E-REMEDIATION.md`                                 | PR #492 (chained); **unmerged**       |

## Decisions requiring operator approval

1. **M1 + G3 + G4** (mandatory sandbox for every model-reachable exec; sandbox
   git; deny `.git` writes). Wording is in
   [PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md](../../plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md)
   §11. Not implied by anything here.
2. **Clean-filter fingerprint policy.** Fail closed when a repository configures a
   filter (safer, but disables mutation detection on `git-lfs` repositories), or
   keep the label-only behaviour recorded here.
3. **Scanner enforcement.** Whether to move off `report-only` and set
   `required_categories`. Unchanged in this pass.
4. **Baseline / residual acceptance.** No entry was added, suppressed, or removed.
   Accepting the D4-5 residual instead of fixing it remains `Reject` per the
   packet.

## Honest limitations of this report

- Nothing is merged. The git hardening is PR #491 (CI green) and this
  reconciliation is PR #492, chained on #491. Both await review and a merge
  decision.
- Full CI was not run locally; it passed on the PR head for #491. The docs PR
  itself is documentation-only.
- Live-provider campaigns were not re-run; the campaign evidence cited is the
  recorded historical evidence, not a fresh observation.
- The git-helper measurements were taken on Git 2.50.1 on darwin arm64 only.
  Linux and Windows were not measured for this change; Windows CI passed, which
  exercises the build and tests, not the helper measurements.
- The scan's `secrets` coverage depends on the pinned Gitleaks binary being on
  `PATH`; with an unpinned binary the scan fails closed rather than scanning.
