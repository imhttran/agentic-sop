# SEC-LEFT-005 — Execution-path boundary matrix (S1)

Read-only inventory. Source: repository inspection only.

Each row states exactly one primary classification: **OS isolation**, **command policy only**, or **operator-trusted configuration**. A row with no runtime proof states **UNVERIFIED** and the reason.

## Matrix

| # | Execution path | Primary classification | Boundary-enforcing code (file / function) | Trust-boundary statement | Evidence status |
|---|---|---|---|---|---|
| 1 | Normal agent tool calls (coding-agent loop) | operator-trusted configuration | agent process: none (external to SOP). `run_command`: opt-in `internal/toolharness/isolation.go` `isolate` (Seatbelt on macOS, bubblewrap on Linux; `SOP_AGENT_RUN_COMMAND_ISOLATION`) | SOP does not sandbox the agent loop; with the flag on, each `run_command` subprocess runs under Seatbelt (no outbound network, writes limited to root/temp/Go caches, `.agent-sdlc/` denied), and fails closed without a backend | agent process UNVERIFIED; `run_command` sandbox VERIFIED on macOS arm64 and on Linux arm64 in a container with bwrap 0.12.0 (opt-in) |
| 2 | Configured agent commands | command policy only | `internal/commandpolicy` (deterministic allow/deny policy) | Command policy decides whether a command may run; it does not provide OS isolation | Command policy exists; no OS isolation is claimed for this path |
| 3 | Verification commands (as agent-side checks) | operator-trusted configuration | run by the agent or operator environment | No sandbox is enforced by SOP for agent-run verification commands | UNVERIFIED — runs in the agent/operator environment |
| 4 | Isolated replay (dqharness) | OS isolation | `internal/dqharness/replay.go` (`DetectIsolation`, `detectIsolationFor`, `BlockedSandbox.Replay`); `internal/dqharness/replay_exec.go` (`IsolationCommand`/`isolationCommand`, `bubblewrapCommand`, `seatbeltCommand`, `IsolatedReplay`) | `IsolatedReplay` refuses without an available backend and runs only through `bwrap` (Linux) or `sandbox-exec` (macOS) | macOS Seatbelt: runtime confinement verified on this host (see seatbelt-evidence.md). Linux Bubblewrap: NOT_RUN on this host, fixtures exist (see bubblewrap-verification.md) |
| 5 | Evaluation-target commands (evaltarget) | OS isolation | `internal/evaltarget/isolation.go` (`RequireIsolation`, `IsolateCommand`, `IsolationAvailable`); `internal/evaltarget/isolation_linux.go` (`isolationCommand`); `internal/evaltarget/isolation_darwin.go` | The runner refuses to execute without `SOP_EVAL_ISOLATION`; the command is wrapped in bwrap/sandbox-exec | macOS: runtime isolation verified by evaltarget tests on this host (see seatbelt-evidence.md). Linux bwrap: construction-only here; runtime NOT_RUN. Host Unix-socket denial: no fixture for this path (UNVERIFIED) |

## Scope disclaimer

Isolated replay (row 4) applies only to the dqharness replay entry point. It does not generalize to rows 1–3 (agent tool calls, configured agent commands, verification commands). No claim in this document states that all coding-agent execution, or all platforms, is OS-isolated.

## Fail-closed conditions cited by the matrix

- `isolationCommand` (dqharness) returns an error for an unknown backend and when canonicalization fails (`filepath.EvalSymlinks`), returning no arguments (test: `TestIsolationCommandUnknownBackendFailsClosed`, `TestBubblewrapCommandCanonicalizationFailsClosed`, `TestSeatbeltCommandCanonicalizationFailsClosed`).
- `bubblewrapCommand` returns an error and no arguments when `filepath.EvalSymlinks` fails.
- `IsolatedReplay` refuses with `replay isolation unavailable: <reason>` when `DetectIsolation().Available` is false.
- `evaltarget.RequireIsolation` fails closed unless `SOP_EVAL_ISOLATION` is set; `evaltarget.isolationCommand` returns `ErrIsolationUnavailable` when `bwrap` is missing.
