# T067 --- Persist a Failed Run's Trace to an Operator Sink

> SOP's command provider discards the harness's stderr, so the per-turn trace that
> explains a failure is lost. Give it a durable, operator-selected sink.

## Status

DONE

## Objective

The bootstrap harness writes a safe per-turn trace (capability, phase, iteration,
tool, request, progress, recovery, termination) to stderr on a failed run. But
`CommandAgent.Generate` captures stderr and discards it on a successful subprocess
exit (`command.go`, "diagnostic only") — and a harness capability failure exits 0
with a `failed` outcome on stdout. So every IMPLEMENT/FIX failure was undiagnosable
without reproducing it by hand. Dogfooding AHV2009 needed exactly this trace and had
to tee stderr manually to get it.

## Scope

- `internal/ollamaagent/config.go`: `envToolTraceLog = "SOP_OLLAMA_TRACE_LOG"`.
- `internal/ollamaagent/harness.go`: `New` reads the sink; `FlushTrace` appends the
  trace to the file (in addition to stderr) when it is set.
- `internal/ollamaagent/harness_test.go`: a test that a failed run's trace reaches
  the sink.
- `README.md`: document the trace and audit sinks.

## Rules

- **Operator-set, like the audit sink.** The path comes from the environment,
  mirroring `SOP_TOOL_AUDIT_LOG`; it is never SOP's state database.
- **Diagnostics only.** The trace stays content-free (no prompts, file contents, or
  secrets), and a missing/unwritable sink never fails the run.
- **No new required configuration.** Unset, behavior is unchanged: the trace goes
  to stderr as before.

## Tests

`TestRunPersistsFailedTraceToSink`: with `SOP_OLLAMA_TRACE_LOG` set, a run that
fails writes its per-turn trace to the named file.

## Acceptance Criteria

- [x] A failed run appends its trace to the configured sink.
- [x] Without the variable, behavior is unchanged.
- [x] The trace contains no prompts, file contents, or secrets.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T066,T067): FIX shares the phased loop; persist a failed-run trace`

## Out of Scope

Changing the audit sink; writing directly into `.agent-sdlc`; surfacing stderr
through the `Agent` interface; committing AHV2009's implementation.
