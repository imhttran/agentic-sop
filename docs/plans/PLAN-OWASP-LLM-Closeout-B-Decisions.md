# Plan B — OWASP LLM gap closeout: work gated on operator decisions

Status: BLOCKED on operator authorization, stage by stage; not executed.
Order: 2 of 2. Independent of [Plan A](PLAN-OWASP-LLM-Closeout-A-Unblocked.md).
Suggested branch: one branch per stage (`security/mandatory-isolation`, `security/clean-filter-fingerprint`, `security/scanner-required-mode`, `security/llm02-defaults`, `docs/owasp-residual-acceptance`).
Dependency: each stage starts only after the operator records the decision named in it.

## Project

Plan B — OWASP LLM gap closeout: work gated on operator decisions

## Summary

Implement the three decisions in
[DECISION-PACKET-OWASP-LLM-W6-W8.md](DECISION-PACKET-OWASP-LLM-W6-W8.md) (W6–W8 of
[PLAN-OWASP-LLM-E2E-REMEDIATION.md](PLAN-OWASP-LLM-E2E-REMEDIATION.md)), then the
LLM02 opt-in defaults and the residual-acceptance record. This plan adds no new
analysis: the options, measurements, and recommendations are in the packet and
the proposals it cites. It turns them into ordered stages with checks.

**Nothing here is authorized by this document.** Each stage states the decision
it needs. A stage whose decision is declined is recorded as declined and skipped;
it does not block stages that do not depend on it.

## OWASPB-001 — Mandatory isolation for model-reachable execution (M1 + G3 + G4)

Priority: P1. Closes the LLM03/LLM06 `BLOCKED` criterion. Today `isolate()` has
one caller (`run_command`); the git tools and the per-turn fingerprints run on
the host, and the file tools can write `.git/config`.

**Decision needed:** the authorization text in §11 of
[PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md](PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md),
with the packet's amendment: keep the PR #491 git hardening flags.

Scope is §6 of the proposal: make the isolation predicate mandatory and remove
the `SOP_AGENT_RUN_COMMAND_ISOLATION` / `..._UNSANDBOXED_ACK` override and its
notice; route `git`, `execFingerprintCmd`, and `WorkingTreeChanged` through one
isolated-exec helper; deny file-tool writes under `.git/`. Known cost:
`git_status` and `git_diff` fail closed on Windows and on Linux hosts where
`bwrap` cannot start.

### Dependencies

None

### Deliverables

- `internal/toolharness/isolation.go`, `harness.go`, `files.go` — as scoped above.
- The tests named in §8 of the proposal, including `TestGitStatusIgnoresRepositoryFsmonitor` and `TestGitMetadataIsReadOnlyToModelTools`.
- `docs/reference/` and `docs/reports/owasp-llm/llm03-agency.md`, `git-output-integrity.md` — override removal, platform impact, and the remaining exposure from §9.

### Acceptance Criteria

- Every model-reachable exec site runs sandboxed or refuses; an architecture test fails if a new one does not.
- The removed override variables have no effect when set.
- `write_file` and `create_file` under `.git/` are refused with a message that says git metadata is read-only to the model.
- The PR #491 flags and their canary tests are still present and passing.
- The scan still shows the `run_command` finding unsuppressed; no baseline entry is added.

### Acceptance Checks

- `go build ./...`
- `go vet ./internal/toolharness/...`
- `go test ./internal/toolharness/ -count=1`
- `go test ./internal/archtest/ -count=1`
- `go test ./...`

## OWASPB-002 — Clean-filter fingerprint policy (Option C)

Priority: P2. A clean filter still runs on `status` and `diff` and can
under-report changes, so a working-tree fingerprint can miss an edit.

**Decision needed:** Decision 2 of the packet. Recommended Option C: treat
`git-lfs` (standard invocation, resolved outside the repository) as expected; any
other active filter driver makes the fingerprint inconclusive, which SOP reads as
"changed". Options A (label only) and B (fail closed on any filter) are the
alternatives.

### Dependencies

None. If OWASPB-001 has landed, the filter also runs contained; this stage is
still needed because containment does not make the reported output complete.

### Deliverables

- `internal/toolharness/git_integrity.go` and the fingerprint callers in `harness.go`.
- Tests: an LFS-only repository keeps normal change detection; a non-LFS driver makes the fingerprint report changed; a driver named `lfs` that resolves inside the repository is not trusted.
- `docs/reports/owasp-llm/git-output-integrity.md` — residual 1 updated.

### Acceptance Criteria

- No configured non-allowlisted filter can produce an `IMPLEMENT_NO_CHANGES` result after a real edit.
- The label from PRs #491/#494 is unchanged.
- A probe failure is treated as inconclusive, not as "no filter".

### Acceptance Checks

- `go build ./...`
- `go test ./internal/toolharness/ -count=1`

## OWASPB-003 — Scanner required mode

Priority: P2. `security.mode` is `report-only` with no `required_categories`.

**Decision needed:** Decision 3 of the packet, plus its item 5: whether the
`run_command` fingerprint stays blocking or gets a justified, expiring baseline
entry after OWASPB-001. Requiring `sast` while that finding is blocking and
unbaselined makes every run fail.

Sequence: re-scan on a clean tree after OWASPB-001; settle the fingerprint;
require `secrets` and `sast`; add `dependencies` only once offline OSV database
provisioning is stable locally and in CI.

### Dependencies

- OWASPB-001

### Deliverables

- `.agent-sdlc/config.yaml` — `mode` and `required_categories`, exactly as authorized.
- `docs/reports/owasp-llm/llm04-supply-chain.md` — the clean-tree scan, the fingerprint decision, and the suppression expiry policy.

### Acceptance Criteria

- A missing or failed required scanner fails the run; it never yields `PASS`.
- The gate is green on `main` at activation; it is not activated red.
- Every baseline change carries its own recorded approval.

### Acceptance Checks

- `go build ./...`
- `go test ./internal/security/... -count=1`

## OWASPB-004 — Turn on the LLM02 and LLM10 opt-in controls in this repository

Priority: P3. C1 unknown-format redaction and the per-run token budget are
implemented, tested, and off.

**Decision needed:** (a) enable C1 in repository configuration (gap-register
decision 2); (b) set a per-run token budget value for this repository. The
dollar cap stays unbuilt, as decided in
[PROPOSAL-LLM10-DOLLAR-BUDGET.md](PROPOSAL-LLM10-DOLLAR-BUDGET.md).

### Dependencies

None

### Deliverables

- Repository configuration only; no code change expected.
- `docs/reports/owasp-llm/llm02-disclosure.md` and `llm06-consumption.md` — the enabled state and one run that shows each control active.

### Acceptance Criteria

- The installation default for both controls is unchanged; only this repository's configuration changes.
- One recorded run shows a redacted unknown-format canary and a budget that is enforced.

### Acceptance Checks

- `go test ./internal/redact/ ./internal/perf/ -count=1`

## OWASPB-005 — Record the bounded residual dispositions

Priority: P3. Documentation and decision memory only. Several residuals are
external or by design and have a recommendation but no recorded disposition:
the context-free unknown-format value, provider retention, verbatim model-output
artifacts, notification prose, retrieval/handoff prose (LLM02); content that is
intact but hostile, and producer-side `chunk_hash` (LLM04); third-party
rendering (LLM05); prompt secrecy (LLM07); factual correctness (LLM09).

**Decision needed:** accept, mitigate, or transfer, per residual. The
recommendations are in
[llm02-decision-packet.md](../reports/owasp-llm/llm02-decision-packet.md) and the
coverage statement.

### Dependencies

None

### Deliverables

- One `sop memory add` record per decision, with its id cited in the gap register.
- `docs/reports/owasp-llm/gap-register-20261109.md` and `coverage-statement.md` — each residual marked with its recorded disposition.
- A follow-up issue in the RAG producer repository for `chunk_hash` emission, if the operator wants it tracked there.

### Acceptance Criteria

- No residual is marked accepted without a decision-memory id.
- Residuals the operator does not decide stay open and say so.

### Acceptance Checks

- `go build ./...`

## Risks and boundaries

OWASPB-001 removes an approved operator override and removes git tools on hosts
with no sandbox backend; that is the reason it needs authorization and not a
side effect to minimise. Containment is not output trust: OWASPB-001 and
OWASPB-002 answer different questions and neither replaces the other. Publisher
signatures (pin-policy item 2), hardlink aliases from outside the workspace, the
macOS `kern.procargs2` read, and Windows stay outside this plan.

## Completion

Complete when every stage is either merged or recorded as declined, and the
coverage statement reflects the result.

## Execution contract

- Planning deliverable only. Creating this plan executed nothing and authorizes nothing.
- Quote the operator's own words when recording an authorization; a merged proposal or this plan is not one.
- Refresh `origin/main` before each stage and work in a new worktree; the planning baseline is `6c9500e4`.
- Reuse the designs in the cited proposals; do not re-open options they already rejected.
- Run `go build ./...`, `go vet ./...`, and `go test ./...` once before each PR. Record commands, exit status, commit, and platform.
- Never hand-edit `.agent-sdlc/state.db`. No baseline entry is added, edited, or removed without its own approval.
- One focused PR per stage. The operator merges.
