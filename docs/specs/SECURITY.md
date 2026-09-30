# Security

**Type:** Normative specification

## Purpose

This document owns SOP's repository-safety and command boundaries: the command
policy, the trust placed in configured verification and agent commands, the
prohibition on destructive or history-changing Git operations, state-database
ownership, and configuration as policy (never secret storage). The minimum-permission
principles are stated in [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md)
§14; this document builds on them and MUST NOT restate them.

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §14 Security and
  Permissions, §4 Command Policy, Git Adapter, State Store.
- [../PRD.md](../PRD.md) — §9 Safety and Idempotency, §15 Out of Scope.
- [EXECUTION.md](EXECUTION.md) — how configured commands run in the lifecycle.
- [AGENT-PROVIDER.md](AGENT-PROVIDER.md) — agent commands and providers.
- [OPENJEV.md](OPENJEV.md) — the read-only JEV boundary.
- [RECOVERY.md](RECOVERY.md) — plan provenance, retry budgets, non-destructive recovery.
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — configuration keys
  and validation.
- [../guides/LESSONS.md](../guides/LESSONS.md) — trusted commands, structured arguments.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. The Command Policy

- Before a command runs, it MUST be classified deterministically as `SAFE`,
  `REQUIRES_APPROVAL`, or `DENIED`.
- Read-only and verification commands MUST be `SAFE`; `git commit` and `git push`
  MUST require approval; a force-push MUST be `DENIED`.
- Project rules MAY only tighten the defaults — they MUST NOT loosen them, and a
  denied command MUST NOT be overridable by project configuration.

## 2. Command Execution

- Configured verification and agent commands are trusted configuration and MAY run
  via `sh -c`, like a Makefile; request data MUST pass through stdin, not command
  interpolation.
- Git and GitHub operations MUST use structured argument lists with no shell, and
  MUST NOT construct a shell command from agent or model output.
- The agent MUST NOT escape the repository root: out-of-repository absolute paths
  and location overrides (for example `--git-dir`, `--work-tree`, `-C`) MUST be refused.
- The agent MUST NOT read or modify SOP's runtime state, including
  `.agent-sdlc/state.db` and the run artifacts under `.agent-sdlc/runs/`.

## 3. Git Safety

- The agent MUST NOT run history-changing or destructive Git commands (for example
  `git reset`, `git clean`, force-push, or branch deletion).
- SOP MUST NOT disable tests or bypass repository protections to make a check pass.
- The permission and prohibition list in
  [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) §14 MUST continue to hold.

## 4. State Ownership

- The state database (`.agent-sdlc/state.db`) MUST be owned by SOP; only SOP writes
  persisted task state.
- A read-only analysis capability (for example JEV) MUST NOT mutate SOP persistence
  and MUST NOT manufacture a `PASS` — see [OPENJEV.md](OPENJEV.md).

## 5. Configuration Is Policy, Never Secrets

- Configuration MUST describe policy only; it MUST NOT contain secrets. Credentials
  MUST come from the environment (or GitHub secret management), never from project
  markdown or committed configuration.
- Unknown configuration keys MUST be rejected rather than silently ignored, so
  credentials cannot be committed by accident. An invalid severity, provider, or
  engine MUST fail with a clear message.
- SOP MUST NOT invent credentials or expose secrets to agent prompts unnecessarily.

## 6. Repository Safety Invariants

- SOP MUST NOT merge code that has failed required gates.
- Pre-existing working-tree changes MUST be preserved: recovery MUST NOT run a
  destructive Git operation or revert an unrelated dirty file.
- Plan provenance (source path, `source_sha256`, plan id) MUST be preserved so SOP
  never mixes tasks from two plans, and reconciliation MUST be non-destructive —
  it MUST NOT delete `.agent-sdlc/state.db` or discard run history (see
  [RECOVERY.md](RECOVERY.md)).
- Retry and fix budgets MUST be bounded, so no failure path loops indefinitely.
