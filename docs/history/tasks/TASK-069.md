# T069 --- Give the Agent a Way to Undo, and Trace Its Reported Failures

> An agent that can overwrite a file must be able to recover one; and a failure the
> model reports is as worth tracing as one the harness raises.

## Status

DONE

## Objective

Dogfooding AHV2010 exposed two gaps. First, the model overwrote `README.md` with a
placeholder via `write_file` and could not recover it — `git checkout` is
(correctly) refused, and there was no other tool — so it had to stop and ask a
human (`AHV2010 BLOCKED`). Second, because it returned a structured **`failed`**
outcome rather than a harness error, `FlushTrace` never ran and there was no
per-turn trace of the turns that clobbered the file.

## Scope

- `internal/toolharness/harness.go`: new `restore_file` tool (a single tracked path,
  restored from HEAD, worktree only) and its registration; `ToolDeleteFile`,
  `ToolRestoreFile` constants; dispatch.
- `internal/toolharness/files.go`: new `delete_file` tool (refuses directories and
  the state database).
- `internal/ollamaagent/harness.go`: `Run` also flushes the trace when the model
  returns a non-completed outcome, so a reported failure is diagnosable.
- Tests in both packages; `README.md`, `LESSONS.md`, `docs/PLAN.md`.

## Rules

- **Recovery is scoped.** `restore_file` restores exactly one path from HEAD, in
  the worktree — no history, branch, or commit change, and it cannot touch another
  path. `delete_file` removes one file and refuses a directory. Both enforce the
  same repository boundary and state-database protection as every other tool.
- **Recovery is not progress.** Neither tool counts as a mutation for the phased
  CHANGE transition: undoing a bad write or clearing a scratch file is
  housekeeping, not implementation.
- **Both are audited**, allowed or denied, like every other tool.
- **PLAN and REVIEW do not gain them.** They remain in the read-only tool sets.

## Tests

`TestDeleteFileRemovesFileAndRefusesDirectory` (removes a file; refuses a
directory; refuses `.agent-sdlc/state.db`), `TestRestoreFileRecoversCommittedContent`
(a clobbered tracked file is restored from HEAD; the state database is refused),
`TestToolsListsExactlyTheInitialTools` (registry updated), and
`TestRunTracesModelReportedFailure` (a model-reported `needs_human` still writes
the per-turn trace to `SOP_OLLAMA_TRACE_LOG`).

## Acceptance Criteria

- [x] A tracked file can be restored to its committed content from inside the
      harness, scoped to one path.
- [x] A file can be deleted; a directory and the state database cannot.
- [x] A model-reported failure (not just a harness error) writes the per-turn
      trace.
- [x] PLAN/REVIEW keep their read-only tool sets; command policy is unchanged.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T069): add scoped delete/restore tools; trace reported failures`

## Out of Scope

A pre-write snapshot/undo stack; allowing `git checkout -- .`/`git restore` via
`run_command`; changing the command policy; committing AHV2010's partial work.
