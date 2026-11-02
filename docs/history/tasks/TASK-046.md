# T046 --- Provider Visibility, Capability Guard, Untracked Files

> Three wrap-up adjustments: show the effective provider, reject a provider that
> cannot implement, and detect new (untracked) files.

## Status

DONE

## Objective

1. **Provider resolution visibility** — the startup summary prints the effective
   provider and whether configuration or the environment selected it:
   `Provider: command (environment)`.
2. **Capability guard** — a run rejects a provider that cannot `IMPLEMENT` before
   attempting any task.
3. **Untracked-file detection** — working-tree change detection includes untracked
   files, so a new file the agent creates counts.

## Dependencies

- T024/T026/T035 (providers, selection, capability guard), T001 (`git` adapter)

## Scope

- `internal/agent/provider.go`: `EffectiveProvider(configured)` returning the name
  and its `ProviderSource` (`environment` | `configuration` | `default`);
  `FromConfig` reuses it.
- `internal/agent/capabilities.go`: `Capabilities.String()` for messages.
- `internal/git/git.go`: `Untracked(ctx, excludes…)` and
  `DiffAll(ctx, excludes…)` (tracked diff + synthetic additions for untracked
  files), plus a synthetic new-file diff renderer.
- `internal/cli`: `guardCapability` used by `run` (graph) and `--task`;
  `deps.readDiff` now uses `DiffAll` excluding `.agent-sdlc` and `docs/reports`;
  the provider line is printed at startup.
- Tests and docs.

## Rules

- The environment (`SOP_AGENT_PROVIDER`) wins over `agent.provider`, which wins
  over the default command provider; the source is reported so a surprising
  provider is visible.
- The guard is a pre-flight check: it fails with an actionable message naming the
  capability and the supported set, before any task runs, and never overrides a
  provider that does support it.
- Change detection widens Git's tracked diff to include untracked files but
  excludes SOP's own output (`.agent-sdlc/`, `docs/reports/`), so runtime state
  and generated reports never masquerade as the task's change. Git remains
  authoritative; unreadable (binary/symlink) untracked files are skipped.

## Tests

agent: `EffectiveProvider` (default, configuration, environment override).
git: `DiffAll` includes an untracked file's synthetic addition and excludes SOP
output prefixes. CLI: a run with a provider lacking `IMPLEMENT` fails before
executing; the startup prints `Provider: ollama (configuration)`.

## Acceptance Criteria

- [x] The startup summary prints the effective provider and its source.
- [x] A provider that cannot `IMPLEMENT` is rejected before any task is attempted.
- [x] Untracked files the agent creates count as changes.
- [x] SOP's own output is excluded from change detection.
- [x] `make check` passes.

## Git

Branch: `task/T046-provider-guard-untracked`
Commit: `task(T046): provider visibility, capability guard, untracked files`
PR: `[Task T046] Provider visibility, capability guard, untracked files`

## Out of Scope

A baseline delta (only files created during this run) — untracked files are
included wholesale, minus SOP output, so pre-existing untracked files also count.
