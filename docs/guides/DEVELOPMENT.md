# Development

This guide covers building and testing the `agentic-sop` repository itself, and
the development principles the codebase is organized around. For using SOP on a
project, see [GETTING-STARTED.md](GETTING-STARTED.md).

## Building

```bash
go build ./...          # build everything (make build)
./install.sh            # install the CLI (make install; add --all for every integration)
```

The `Makefile` wraps the common tasks: `make fmt`, `make vet`, `make test`,
`make test-fast`, `make check`, `make check-fast`, `make build`, and `make install`
(which delegate to `./install.sh`, the documented installation entry point; see
[INSTALLATION.md](INSTALLATION.md)).

## Testing

```bash
go test ./...                # run the test suite
make test                    # the same ordinary, disk-backed suite
go test -race ./...          # with race detection
CGO_ENABLED=0 go test ./...  # verify operation without CGO
make test-fast               # the full suite with TMPDIR on RAM-backed storage
make check                   # format, vet, test, build, plugin package current
make check-fast              # the same gates, with the fast test runner
```

AI boundaries are replaced with deterministic fakes in unit tests. The tests do
not require a running LLM, API keys, network access, or cloud services, so they
run fully offline.

### Fast test runs (`make test-fast`)

On Linux the full suite exercises SQLite heavily. When every test's temporary
database lands on a disk-backed `TMPDIR`, that file I/O can dominate the run and
cause timeouts on some machines (for example, Linux Mint). `make test-fast`
runs the same suite with `TMPDIR` pointed at RAM-backed temporary storage, which
keeps those temporary databases in memory and removes the bottleneck.

**When to use it.** Use `make test-fast` for the local edit / test loop when the
ordinary suite is slow or times out because of temporary-storage I/O. It runs the
same tests with the same assertions — it is a faster place to put their temporary
files, never a reduced or skipped suite, and a failure still exits non-zero.

**How the RAM-backed temporary directory works.** `make test-fast` delegates to
`scripts/checks/test-fast.sh`. Each invocation creates its *own* directory with
`mktemp -d` (default `/dev/shm/sop-test-fast.XXXXXX` on Linux), sets `TMPDIR` to
that directory only for the `go test` child process, and removes exactly that
directory on exit. The shared base (`/dev/shm`, `$TMPDIR`, `/tmp`) is never
removed, so concurrent runs cannot delete each other's files. Only temporary
test files live in RAM: no persistent SOP state, no repository checkout, and no
`.agent-sdlc/` lifecycle state is ever placed there, and no SQLite durability
setting is changed.

**Configuration.** Override any input on the command line or via the
environment:

| Setting             | Make variable              | Environment            | Default |
| ------------------- | -------------------------- | ---------------------- | ------- |
| Parallelism         | `TEST_FAST_PARALLEL`       | `SOP_TEST_PARALLEL`    | `2`     |
| Timeout             | `TEST_FAST_TIMEOUT`        | `SOP_TEST_TIMEOUT`     | `3m`    |
| Temp base directory | `TEST_FAST_TMPDIR`         | `SOP_TEST_TMPDIR`      | auto    |
| Min free RAM (MiB)  | `TEST_FAST_MIN_FREE_MB`    | `SOP_TEST_MIN_FREE_MB` | `512`   |

For example: `make test-fast TEST_FAST_PARALLEL=4 TEST_FAST_TIMEOUT=5m`.

**Platform behavior.**

- **Linux:** uses `/dev/shm` when it exists, is writable, permits executing
  files, and has at least `MIN_FREE_MB` free; otherwise it falls back to the
  system temp directory and prints a notice. A `noexec` mount (Ubuntu's default
  for `/dev/shm` in containers) is detected and skipped, because `go test` runs
  its test binaries from the temporary work directory. No `sudo`, mount, or
  elevated privilege is required.
- **macOS:** uses the normal system temporary directory (there is no `/dev/shm`);
  the run logs `source=system` so the disk-backed fallback is visible.
- **Other platforms:** fall back to the normal system temporary directory.

**Why the ordinary disk-backed tests remain necessary.** RAM-backed storage is a
local convenience, not a replacement for `make test`. A tmpfs can behave
differently from a real disk — capacity is bounded, and durability/`fsync`
semantics differ — so `make test` stays the authoritative local gate and the
race-detector job in `.github/workflows/ci.yml` stays disk-backed
(`go test -race ./...`). The regular Linux CI test job runs the complete
`go test ./...` suite through the fast runner (with `-p 4 -count=1 -timeout 10m`),
so CI still exercises every test; when `/dev/shm` is unavailable, not executable,
or too small, the runner falls back to the disk-backed system temp directory and
logs why. Keep `make test-fast` as the inner-loop accelerator and
`make test`/`make check` as the disk-backed correctness gate.

### The Ollama agent installation gate

`make install-ollama-agent` runs the repository gates before installing the pinned
known-good `sop-ollama-agent` binary outside the tree under edit. Its default test
mode is the ordinary, disk-backed suite (`make check`). To run the same gates with
the fast test runner, opt in explicitly:

```bash
make install-ollama-agent TEST_MODE=fast
```

`TEST_MODE` accepts `ordinary` (the default) or `fast`. Any other value fails before
a gate or the installer runs, so a typo cannot install a binary. In both modes the
formatting, vet, build, and plugin-package gates are unchanged; a failing gate
stops the installation.

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
