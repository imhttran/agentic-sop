# LLM07 state-write detection — Phase 1 investigation (NO-GO)

**Status:** investigation complete; operator decision **NO-GO** (2026-11-09).
No application code was changed. No evidence state in the matrix changes.

**Provenance:** this is a corrected reconstruction. The original report was
produced by an SOP-governed read-only stage (plan `7-llm07-state-write-detection`,
model `deepseek-v4-pro:cloud`) on 2026-11-09 and was never committed; its worktree
was later removed. This version keeps its conclusion and writer inventory, re-checks
each writer against `main`, and corrects two points:

1. The original said a subprocess "cannot write" `state.db` and its side files
   because they are blocked. That is wrong: only the model's *file tools* refuse
   `.agent-sdlc/`. A `run_command` subprocess can write anywhere the operator's
   user can, `state.db` included. That is the gap this investigation is about.
2. The original's reason for NO-GO ("the exclusion set cannot be proven
   complete") was weaker than the real one, which is in §2.

## 1. Question

A SAFE `run_command` (for example `go test ./...`) can write into `.agent-sdlc/`
as a side effect. `TestSubprocessStateWriteIsOutsideFileToolBoundary`
(`internal/toolharness/state_dir_write_test.go`) pins this, and
[llm07-misinformation.md](llm07-misinformation.md) records it. Phase 1 asked:
can a before/after fingerprint of `.agent-sdlc/` around each `run_command`
reliably detect a write made by the subprocess, without also flagging SOP's own
concurrent writes?

## 2. Decision: NO-GO

A fingerprint check would have to ignore the paths SOP itself writes while a
command runs. Those paths include the most valuable targets, so the check would
miss the writes that matter most:

- **`state.db` must be excluded, and it is the main target.** SOP's store writes
  the SQLite task graph and its `-wal`/`-shm`/journal files on its own schedule.
  A subprocess that edits `state.db` directly can change task status or attempts.
  `commandpolicy` does not class `sqlite3` as SAFE, but a SAFE `go test ./...`
  runs whatever test code the model wrote, and that code can open the file. A check that excludes `state.db` cannot see
  that; a check that includes it fails on SOP's own writes.
- **SOP writes into the same run directory as the forgeable artifacts.** When
  activity reporting is on, SOP appends `runs/<task>/activity.jsonl` live while
  the task runs. The authoritative artifacts a forgery would target
  (`outcome.json`, `acceptance_decisions.jsonl`, `waivers.jsonl`,
  `changed-files.json`) live in that same `runs/<task>/` directory. Excluding the
  directory hides forgeries; excluding files by name is a list that must track
  every artifact SOP may write during a task, now and later.
- **The window can be escaped.** The harness kills a command's process group
  only on timeout or cancellation (`internal/toolharness/harness.go`,
  `execCaptured`). A descendant left running after a normal exit, or one that
  left the group with `setsid`, can write after the "after" fingerprint is taken.

A check with these holes would report protection it does not provide. The
investigation therefore stops here rather than shipping it.

## 3. Writer inventory (`.agent-sdlc/` during one agent invocation)

Re-checked against `main` at `54b8cb4c`.

| Writer | Paths under `.agent-sdlc/` | When it writes | Code |
| --- | --- | --- | --- |
| Task store (SQLite) | `state.db`, `state.db-wal`, `state.db-shm`, `state.db-journal` | Any task save or graph change, independent of `run_command` | `internal/store/sqlite.go` (`Open`) |
| Project lock | `state.lock` | Acquire/release around plan-state changes | `internal/projectlock/lock.go` (`FileName`) |
| Execution lease | `state.exec.lock`; `state.exec.lock.recover.*` during stale-lease recovery | Heartbeat refresh for the whole run (`HeartbeatEvery`) | `internal/executionlease/lease.go`; `internal/cli/execution_lease.go` |
| Activity artifact | `runs/<task>/activity.jsonl` | Appended live while the task runs, when activity reporting is enabled | `internal/cli/activity.go` (`taskActivityContext`) |
| Run artifacts | other files under `runs/<task>/` (`state.json`, `attempt.txt`, routing, JEV, outcome, review) | On run transitions around the agent invocation (whether any is written while it runs was not checked) | `internal/run` |
| Operator sinks | any path set by `SOP_TOOL_AUDIT_LOG`, `SOP_OLLAMA_TRACE_LOG`, `SOP_COMMAND_EVIDENCE_LOG` (not restricted, so possibly inside `.agent-sdlc/`) | Appended by the agent harness during the invocation | `internal/ollamaagent/harness.go`, `config.go` |
| Command agent | none directly; it talks over stdin/stdout/stderr only | — | `internal/agent/command.go` |

## 4. What would close the gap

Detection is the wrong layer. The gap closes only if the `run_command`
subprocess cannot write `.agent-sdlc/` at all, while SOP's own process still can.
That is OS isolation of the subprocess: a sandbox profile that keeps the
repository writable but denies writes under `.agent-sdlc/`. The existing Seatbelt
and Bubblewrap backends (`internal/evaltarget`, `internal/dqharness`) are the
starting point. Normal agent execution is not OS-sandboxed today (row 1 of
[../security-shift-left/execution-path-matrix.md](../security-shift-left/execution-path-matrix.md)).

**Update (LLM03):** the opt-in `run_command` sandbox now implements this on
macOS: with `SOP_AGENT_RUN_COMMAND_ISOLATION=1` a subprocess write under
`.agent-sdlc/` is denied by Seatbelt (`TestRunCommandIsolationDeniesStateDirWrite`).
With the flag off, or on Linux, the residual risk below still applies.

## 5. Decision record

- **Operator decision:** NO-GO, 2026-11-09. Phase 2 (the fingerprint check) was
  not implemented.
- **SOP lifecycle:** the plan was archived with disposition `SUPERSEDED`
  (explicitly abandoned, not completed). Its archive was damaged by a lifecycle
  defect: an accidental historicalize followed by re-activation caused the final
  historicalize to clear the executed stage records without archiving them. That
  defect is fixed in PR #322 (each plan activation now gets its own archive slot);
  the lost task records are not recoverable.
- **Accepted residual risk:** a `run_command` subprocess can still write under
  `.agent-sdlc/`, including `state.db` and run artifacts, and SOP neither prevents
  nor detects it. This report makes no detection or prevention claim, and
  `TestSubprocessStateWriteIsOutsideFileToolBoundary` still pins that boundary.
