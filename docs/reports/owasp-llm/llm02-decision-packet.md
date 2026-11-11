# LLM02 operator decision packet (OWASP gap 8)

**As of:** `main` at the commit this file was added to. **Status:** decision packet.
This document records no decision, acceptance, waiver, memory entry, or configuration
change. It lists each open LLM02 decision with its evidence, a recommendation, and the
command or change that would record it. The
[coverage statement](coverage-statement.md) and
[gap register](gap-register-20261109.md) stay authoritative for status.

Commands below were checked against the source in `internal/cli/historical_acceptance.go`
(`historicalOutcomeUsage`, `historicalVerifyUsage`) and `internal/config/redaction.go`.

## 1. Plan 03 per-criterion acceptance (criteria 1–4 and 6)

- **Evidence:** [llm02-acceptance-request.md](llm02-acceptance-request.md); the
  Plan 03 closeout matrix; the bounded campaign artifacts
  (`llm02-campaign.json`, `llm02-linux-campaign.json`, `llm02-c1-linux-campaign.json`,
  `llm02-c1-hardlink-campaign.json`).
- **Current state:** bounded evidence acceptance is recorded (decision memory
  `59d986984142bef314ea784a401b204cd57b7ca1664f570b4763c13a5f580c98`). Per-criterion
  acceptance is not recorded, so full acceptance stays `NEEDS_HUMAN`.
- **Recommendation:** review the evidence, then accept or reject each criterion
  individually. Do not treat the historicalized `COMPLETE` as acceptance.
- **Command (designated approver only):**
  ```bash
  sop verify --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure
  sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md --archive <archive-dir> --accept <criterion-id> --evidence "<text>" --evidence-ref "path=<file>,sha256=<digest>" --by <name>
  ```
  Use `--reject` or `--annotate` instead of `--accept` where that fits. `--reason` is
  required with `--annotate`.

## 2. Enable C1 in repository configuration

- **Evidence:** [llm02-disclosure.md](llm02-disclosure.md) (verification and scope);
  the C1 campaigns above, all of which loaded the inventory through the external loader.
  The 16-case campaign and the `config.RedactionPolicy` composition are still
  unit-tested only.
- **Current state:** `redaction.enabled` is `false`. Enabling it needs an external
  inventory file, which must be outside the repository and not a symlink.
- **Recommendation:** enable it only after a campaign that goes through
  `config.RedactionPolicy` on this repository, and only with an inventory the operator
  owns. C1 is not general DLP.
- **Change (after the decision):**
  ```yaml
  redaction:
    enabled: true
    inventory_path: /absolute/path/outside/the/repository/sensitive-values.txt
  ```
  The file must be `0600`, outside the repository, and not a symlink. Optional bounds:
  `max_values`, `min_value_bytes`, `max_value_bytes`.

## 3. Unknown-format residual (context-free values)

- **Evidence:** [PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md](../../plans/PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md):
  35-case corpus, 27 redacted, 1 context-free residual, 7 ordinary controls; entropy
  rejected.
- **Current state:** status `NOT_APPROVED`. The residual is open.
- **Recommendation:** `ACCEPT_PROPOSED` for context-free values, with the stated
  consequence that a secret with no key or known shape can reach the model.
- **Record:** the proposal's own disposition, which this packet does not record. The
  decision-memory command is `sop memory add`. Its usage, from `memoryUsage` in
  `internal/cli/memory.go`, is:
  ```bash
  sop memory add --decision "<text>" --reason "<text>" [--scope repository|project|architecture] [--source <id>] [--evidence "<text>"]
  ```
  `--help` is not a flag this command accepts; it fails with `needs a value`. The command
  records an advisory decision only. It does not change formal acceptance or lifecycle,
  so the Plan 03 acceptance in section 1 is still needed for that.

## 4. Remaining residual dispositions (from the six-item matrix)

| Item | Recommended disposition | Evidence | Note |
| --- | --- | --- | --- |
| Model-output artifacts | `INVESTIGATE` (gap 7) | `PROPOSAL-LLM02-MODEL-OUTPUT-ARTIFACTS.md` (PR #407, not yet merged); option C implemented in PR #408 | Option A (redact the reply files) still needs a decision |
| Notification prose | `MITIGATE` | Gap register, notification row | Not yet tracked as a separate item |
| Retrieval and handoff inputs | `INVESTIGATE` / `ACCEPT_PROPOSED` | Gap register | Retrieval sinks are `NOT_RUN` on Linux |
| Provider retention | `TRANSFER_PROPOSED` | Gap register | Outside SOP's control |
| Parent-environment read on macOS (`kern.procargs2`) | Accept as known limit, or design a sysctl allow-list | [seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md) (PR #406) | Narrow denies do not work; a full sysctl deny breaks the Go runtime |
| Hardlink alias to protected credentials, macOS | Verify the existing registry on macOS (option B) | [PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md](../../plans/PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md) (PR #404) | First macOS run is recorded as not passed; a rerun or a prompt change needs a decision |

## 5. Already decided (no action needed)

- Bounded subprocess hardlink alias: `RISK_ACCEPTED` (Option D, PR #387).
- Per-row index digests, RAG `chunk_hash`, and scanner pins: decided and implemented.
- Model-input credential policy A+B: approved and implemented.

## What this packet does not decide

Whether to change any default, record any acceptance, or edit any archive. Each of
those stays with the designated approver.
