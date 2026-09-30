# Development

This guide covers building and testing the `agentic-sop` repository itself, and
the development principles the codebase is organized around. For using SOP on a
project, see [GETTING-STARTED.md](GETTING-STARTED.md).

## Building

```bash
go build ./...          # build everything (make build)
go install ./cmd/sop    # install the CLI
```

The `Makefile` wraps the common tasks: `make fmt`, `make vet`, `make test`,
`make build`, `make check`, and `make install` (which runs
`./scripts/install.sh`, installing the CLI and the bundled `sop-end-to-end`
skill).

## Testing

```bash
go test ./...                # run the test suite
go test -race ./...          # with race detection
CGO_ENABLED=0 go test ./...  # verify operation without CGO
make check                   # format, vet, test, build
```

AI boundaries are replaced with deterministic fakes in unit tests. The tests do
not require a running LLM, API keys, network access, or cloud services, so they
run fully offline.

## Development principles

### Deterministic control, agentic execution

Use AI where reasoning is useful; use ordinary software where correctness and
state management matter. Good AI responsibilities:

```text
Planning · Coding · Review · Failure Analysis · Documentation
```

Good deterministic responsibilities:

```text
State Transitions · Dependency Validation · Scheduling · Persistence
Retry Limits · Git State · CI State · Merge Gates
```

### Machine state is not markdown

Markdown is for people; structured data is for executable state. `PRD.md` and
`PLAN.md` are the human side; `plan.json`, tasks, dependencies, attempts, and
SQLite are the machine side. The orchestrator does not depend on parsing human
documentation to understand its own state.

### Bounded autonomy

Agents do not retry indefinitely. A task that keeps failing reaches a bounded
limit and becomes `BLOCKED` rather than looping; the exact behaviour depends on
the workflow stage, but retries carry explicit limits.

### Recoverability

After a crash, process restart, agent failure, CI failure, or human interruption,
the system answers: what happened, what state are we in, and what should happen
next.

### Local first

The orchestrator runs locally. The architecture supports local models, cloud
models, local Git, local development environments, local SQLite state, and
external CI where appropriate; a cloud service is not required simply to run the
orchestrator.

### Provider independence

Application components depend on small interfaces rather than model-provider
SDKs:

```go
type Agent interface {
    Generate(ctx context.Context, request Request) (Response, error)
}
```

This lets model providers and agent harnesses change without rewriting the
planning or orchestration logic. See
[../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) for how harness,
provider, and model fit together.

## The repository's own state

The repository dogfoods SOP: it keeps its own SOP state under `.agent-sdlc/`
(which ignores itself for Git) and its own work items under `docs/tasks/`. Task
specifications, plans, and history live under `docs/`.
