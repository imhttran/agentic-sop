# LLM02 operator decision packet (OWASP gap 8)

**As of:** `main` at the commit this file was added to. **Status:** decision packet.
This document records no decision, acceptance, waiver, memory entry, or configuration
change. It lists each open LLM02 decision with its evidence, a recommendation, and the
command or change that would record it. The
[coverage statement](coverage-statement.md) and
[gap register](gap-register-20261109.md) stay authoritative for status.

Commands below were checked against the source in `internal/cli/historical_acceptance.go`
(`historicalOutcomeUsage`, `historicalVerifyUsage`) and `internal/config/redaction.go`.

**Refreshed:** `main` at `4993cdb9` (merge of PR #429). This refresh corrects
merged-PR references, adds the check-mapped `--run-check` commands, and records
the Linux notification-sink evidence. It still records no decision, acceptance,
waiver, memory entry, or configuration change. **2026-11-12:** the gap 5 macOS
Option C row now records attempt 3 (`PASS`) from the dedicated branch; this
packet still records no decision, acceptance, waiver, memory entry, or
configuration change.

## 1. Plan 03 per-criterion acceptance (all 16 criteria)

- **Evidence:** [llm02-acceptance-request.md](llm02-acceptance-request.md) —
  §Plan 03 acceptance matrix and §Decision packet (2026-11-10; corrected) hold the
  per-criterion evidence, bounds, and proposed scope; the Plan 03 closeout matrix;
  the bounded campaign artifacts
  (`llm02-campaign.json`, `llm02-linux-campaign.json`, `llm02-c1-linux-campaign.json`,
  `llm02-c1-hardlink-campaign.json`).
- **Current state:** bounded evidence acceptance is recorded (decision memory
  `59d986984142bef314ea784a401b204cd57b7ca1664f570b4763c13a5f580c98`). Per-criterion
  acceptance is not recorded, so full acceptance stays `NEEDS_HUMAN`.
- **Recommendation:** review the evidence, then accept or reject each criterion
  individually. Do not treat the historicalized `COMPLETE` as acceptance.
- **Current verified state:** `sop verify --plan … --archive … --json` reports
  `accepted: 0`, `mapped: 16`, `manual: 13`, `check: 3`, `check_passed: 0`,
  `unverified: 3`. The 13 manual criteria are `decision-eligible`; the three
  check-mapped criteria (`DISCLOSE-002.1`, `002.2`, `002.3`) are
  `check-unverified`. No `acceptance_decisions.jsonl` exists for the plan.
- **Commands (designated approver only):**
  ```bash
  sop verify --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md --archive .agent-sdlc/archive/worktree-evidence/plan-03-sop-llm02-sensitive-information-disclosure
  sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md --archive <archive-dir> --accept <criterion-id> --evidence "<text>" --evidence-ref "path=<file>,sha256=<digest>" --by <name>
  sop outcome --plan docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md --archive <archive-dir> --run-check <criterion-id> [--tested-revision <commit>] --by <name>
  ```
  Use `--reject` instead of `--accept` to record a rejection, or
  `--annotate <criterion-id> --reviewed-evidence "<text>" --reason "<text>"` to record
  a manual-evidence annotation; `--reason` is required with `--annotate`.
- **Check-mapped criteria:** `DISCLOSE-002.1`, `DISCLOSE-002.2`, and `DISCLOSE-002.3`
  have archived check commands. `--accept`, `--reject`, and `--annotate` are refused
  for them (`criterion … is check-mapped; manual acceptance is not allowed`). Use
  `--run-check`; it requires a clean tree, records the tested revision, exit status,
  and log digest, and refuses to accept if the source changes during execution. A
  passing check is evidence for the tested revision, not human acceptance.

## 2. Enable C1 in repository configuration

- **Evidence:** [llm02-disclosure.md](llm02-disclosure.md) (verification and scope);
  the C1 campaigns above, all of which loaded the inventory through the external loader.
  The 16-case campaign and the `config.RedactionPolicy` composition are still
  unit-tested only.
- **Current state:** `redaction.enabled` is `false`. Enabling it needs an external
  inventory file, which must be outside the repository and not a symlink.
- **Protected routes (verified in source):** the Ollama native tool harness redacts
  `userPrompt` (task, caller input, output requirements) and every tool result and
  error in `toolResultMessage`. Since PR #400, the baseline pass and an enabled C1
  policy also run on every CLI-built chat transport through `agent.Redacted` — the
  OpenAI-compatible provider and the Ollama chat path — for `Task`, `Input`, and
  `OutputRequirements` (`TestRedactedBeforeChatTransports`, C1 on and off). The system
  prompt and non-native tool results are outside this boundary, and no live campaign
  exercises the chat transports.
- **A committed default would break other machines:** `config.validateRedaction`
  requires `inventory_path` whenever `enabled` is true, and `config.RedactionPolicy`
  fails closed when the policy loads if the file is missing, malformed, a symlink,
  not a regular file, inside the repository, or not `0600`; `LoadInventory` also
  refuses Windows. The tracked `.agent-sdlc/config.yaml` cannot name a path that
  exists on every clone, so a committed `enabled: true` would fail agent creation
  elsewhere. Keep C1 opt-in and enable it in a local, uncommitted configuration
  with an operator-owned external inventory.
- **Recommendation (preferred):** keep C1 opt-in and do not commit
  `enabled: true`; enable it locally, with an operator-owned external inventory,
  only after a campaign that goes through `config.RedactionPolicy` on the target
  repository. C1 is not general DLP.
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

| Item                                                | Recommended disposition                                     | Evidence                                                                                                                                       | Note                                                                                                                                                                                                                                                                                                                                                                                                                      |
| --------------------------------------------------- | ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Model-output artifacts                              | `INVESTIGATE` (gap 7)                                       | `PROPOSAL-LLM02-MODEL-OUTPUT-ARTIFACTS.md` (PR #407, merged); option C implemented in PR #408 (merged; run artifacts are now `0600`)           | Option A (redact the reply files) still needs a decision; the proposal records no measured echo campaign                                                                                                                                                                                                                                                                                                                  |
| Notification prose                                  | `MITIGATE`                                                  | Gap register row 6; Linux notification-sink run `llm02-campaign-linux-notify-sink.json` (PR #429)                                              | One Linux run is `protected` for shape and known-value canaries with a local receiver; Slack rendering and channel binding remain gap 12                                                                                                                                                                                                                                                                                  |
| Retrieval and handoff inputs                        | `INVESTIGATE` / `ACCEPT_PROPOSED`                           | Gap register                                                                                                                                   | The Linux live retrieval sink is `NOT_RUN`; the notification sink now has one Linux run (PR #429)                                                                                                                                                                                                                                                                                                                         |
| Provider retention                                  | `TRANSFER_PROPOSED`                                         | Gap register                                                                                                                                   | Outside SOP's control                                                                                                                                                                                                                                                                                                                                                                                                     |
| Parent-environment read on macOS (`kern.procargs2`) | Accept as known limit, or design a sysctl allow-list        | [seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md) (PR #406)                                                                  | Narrow denies do not work; a full sysctl deny breaks the Go runtime                                                                                                                                                                                                                                                                                                                                                       |
| Hardlink alias to protected credentials, macOS      | Verify the existing registry on macOS (option B) — complete | [PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md](../../plans/PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md) (PR #404; runs #405, #419, and attempt 3) | Attempt 3 `PASS`es: all five protected outcomes held (3 path-denied, 2 C1-redacted) and the positive control passed after a test-only prompt correction (artifact `llm02-option-c-phase1-macos-campaign-attempt3.json`, SHA-256 `50cc9e09…`). Outside-workspace origins, late external credentials, external removal before the first build, and Windows remain open; `Nlink > 1` is not implemented and is not approved. |

## 5. Already decided (no action needed)

- Bounded subprocess hardlink alias: `RISK_ACCEPTED` (Option D, PR #387).
- Option B macOS verification: `PASS` (attempt 3). This is evidence for the existing
  registry, not a new policy or acceptance.
- Per-row index digests, RAG `chunk_hash`, and scanner pins: decided and implemented.
- Model-input credential policy A+B: approved and implemented.

## What this packet does not decide

Whether to change any default, record any acceptance, or edit any archive. Each of
those stays with the designated approver.
