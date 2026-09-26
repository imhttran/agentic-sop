# T019A --- Task Context Compaction and Handoff

## Status

DONE

## Objective

Add a bounded context-management layer between completed SOP tasks.
After a task finishes, SOP produces a small structured handoff capsule
containing information useful to later tasks while SQLite, Git,
verification, PR, and CI records remain authoritative.

Optionally, bulky supporting material such as logs, diffs, tool output,
and diagnostic transcripts may be compressed by a pluggable context
compressor.

```text
Completed Task
      ↓
Collect durable facts
      ↓
Create Task Handoff Capsule
      ↓
Optionally compress bulky context
      ↓
Persist handoff metadata
      ↓
Next Task / Resume
```

T019A follows T019 because reliable resume/recovery should exist before
context compaction becomes part of automated multi-task execution.

## Dependencies

- T018 --- Completion Loop
- T019 --- Resume and Recovery

T019A must not change the authoritative task state machine.

## Core Principle

```text
Compression is an optimization.
It is never workflow truth.
```

Authoritative sources remain SQLite workflow state, Git history,
deterministic verification results, and remote PR/CI state when
available.

## Architecture

Introduce a provider-independent boundary:

```go
type ContextCompressor interface {
    Compress(ctx context.Context, input ContextBundle) (CompressedContext, error)
}
```

Possible implementations:

```text
NoOpCompressor
CavemanCompressor
future local/custom compressor
```

Caveman is optional. Core orchestration must not depend directly on it.

Prefer a dedicated package such as `internal/handoff/`:

```text
internal/handoff/
├── capsule.go
├── builder.go
├── compressor.go
├── noop.go
└── builder_test.go
```

Provider adapters may live below it, for example
`internal/handoff/caveman/`.

## Two Context Layers

### SOP Task Handoff Capsule

SOP owns a small deterministic schema containing the meaning of the
completed task.

```yaml
task: T010
result: LOCAL_TESTS_PASS

changes:
  - added task runner
  - added bounded RED/GREEN workflow

decisions:
  - only FAIL establishes RED
  - ERROR and CANCELED do not establish RED

files:
  - internal/taskrunner/runner.go
  - internal/taskrunner/runner_test.go

verification:
  focused_test: PASS
  required_suite: PASS

carry_forward:
  - process-tree cancellation remains a follow-up
```

A reasonable model is:

```go
type Capsule struct {
    TaskID       string
    Result       domain.TaskStatus
    Summary      string
    Changes      []string
    Decisions    []string
    Files        []string
    Verification []VerificationSummary
    CarryForward []string
}
```

Keep it small. Do not serialize the entire task history.

### Mechanical Compression

Mechanical compression is for bulky supporting context:

```text
large test logs
CI logs
diffs
tool transcripts
agent transcripts
structured JSON
diagnostics
```

Where supported, a compressor may retain references that allow recovery
of originals.

## Capsule Inputs

Build deterministic fields from known workflow facts where possible:

```text
task metadata/status
attempt history
Git diff/history
verification results
review results
PR/CI results
explicit workflow decisions
```

Do not ask an LLM to invent facts SOP already knows. An LLM may
summarize human-readable details, but deterministic fields come from
authoritative sources.

Carry-forward information may include architecture decisions, new
interfaces, known limitations, follow-up work, compatibility behavior,
important files, and testing conventions.

Do not automatically carry arbitrary conversation history forward.

## Compression Contract

A generic bundle may look like:

```go
type ContextBundle struct {
    Capsule   Capsule
    Artifacts []Artifact
}
```

Artifacts can have generic kinds such as:

```text
DIFF
TEST_LOG
CI_LOG
AGENT_OUTPUT
REVIEW_OUTPUT
DIAGNOSTIC
```

A compressed result may be:

```go
type CompressedContext struct {
    Content    string
    References []Reference
}
```

Do not assume every provider supports retrieval. Recoverability should
be explicit.

## NoOp Compressor

Provide a deterministic no-op implementation so SOP works normally with
compression disabled and tests require no external service.

## Optional Caveman Adapter

Support Caveman only as an adapter:

```text
ContextCompressor
        │
        ├── NoOp
        └── Caveman
```

Do not expose Caveman-specific concepts throughout SOP. Do not make SOP
startup fail because Caveman is unavailable when compression is
optional.

If configured compression fails, retain the uncompressed capsule and
report the compression failure.

## Compressor Selection and Availability Check

SOP must not assume that Caveman is usable merely because it may be
installed.

Support three configuration modes:

```yaml
context:
  compression: none # always use NoOp
```

```yaml
context:
  compression: auto # use Caveman when available and healthy
```

```yaml
context:
  compression: caveman # explicitly prefer Caveman
```

Resolve the compressor deterministically:

```text
none
  ↓
NoOpCompressor

auto
  ↓
Caveman executable/config available?
  ├─ no  → NoOpCompressor
  └─ yes
       ↓
     health check succeeds?
       ├─ no  → NoOpCompressor + diagnostic
       └─ yes → CavemanCompressor

caveman
  ↓
Caveman executable/config available?
  ├─ no  → preserve capsule + report unavailable
  └─ yes
       ↓
     health check succeeds?
       ├─ no  → preserve capsule + report unhealthy
       └─ yes → CavemanCompressor
```

Explicit `caveman` configuration must still not make successful task
completion depend on compression. If Caveman is unavailable or
unhealthy, retain the uncompressed handoff capsule and surface a clear
diagnostic.

### Health Check Boundary

Keep health checking separate from the core compression interface where
useful:

```go
type HealthChecker interface {
    Check(ctx context.Context) error
}
```

`CavemanCompressor` may implement both:

```go
ContextCompressor
HealthChecker
```

The check must be cheap, bounded by `context.Context`, and
non-destructive.

Prefer checking:

```text
executable availability
required configuration
adapter/CLI readiness
```

Do not perform real task compression merely to test availability unless
the Caveman integration provides no safer readiness mechanism.

Do not repeatedly probe Caveman before every artifact. Resolve
compressor availability once per run/session where practical, while
allowing a later run or resume to re-evaluate availability.

### Auto-Detection Is Convenience, Not Truth

`auto` means:

```text
use Caveman if SOP can prove it is currently usable;
otherwise continue with NoOp compression.
```

Do not infer health solely from:

```text
binary exists
package installed
configuration file exists
```

Installation and readiness are separate facts.

## Failure Policy

Compaction must never invalidate completed work.

```text
Task DONE
   ↓
compression fails
   ↓
Task stays DONE
capsule remains available
compression status = FAILED
workflow may continue with uncompressed capsule
```

Do not turn a completed task into `BLOCKED` because context compression
failed.

## Persistence

Persist enough metadata for handoffs to survive restart:

```text
task ID
capsule
compression status
compressed content/reference when available
creation timestamp
```

Prefer a dedicated persistence concept rather than placing handoffs in
`Task.Attempt.Output`.

Do not duplicate authoritative task status in a way that can drift from
the task table.

A small metadata status is enough:

```text
NOT_REQUESTED
COMPRESSED
FAILED
```

This is metadata, not another workflow state machine.

## Context Budget

Support a configurable budget, for example:

```yaml
context:
  max_handoff_bytes: 50000
  compression: caveman
```

The semantic capsule should always remain intentionally small. The
budget determines whether bulky artifacts need compression.

## When Compaction Runs

Run at the task boundary:

```text
task reaches DONE
      ↓
build capsule
      ↓
compact supporting context
      ↓
persist handoff
      ↓
select/start next task
```

Do not compact between RED → IMPLEMENT, IMPLEMENT → GREEN, FIX → RETEST,
or REVIEW → FIX.

## Next-Task Context

A new task may receive:

```text
current task objective
acceptance criteria
relevant project files
relevant prior handoff capsules
selected compressed artifacts
```

Do not inject every prior capsule.

V1 should prefer direct dependency capsules plus a small set of
project-level carry-forward notes.

For example, if T011 depends on T010, prefer T010's capsule. If a task
depends on T007 and T009, load those handoffs before unrelated
historical tasks.

Avoid vector/semantic retrieval in T019A.

## Recovery and Idempotency

Because T019 defines resume behavior, handoff generation must be
idempotent.

If SOP restarts after task completion but before handoff completion, it
must be safe to regenerate or complete the handoff without changing task
state or creating duplicate logical records.

Use a deterministic identity such as task ID plus completed
revision/commit where practical.

## Security

Do not blindly send environment variables, credentials, tokens, secret
files, or private keys to a compression provider.

Artifact collection must be explicit. If a compressor is remote, that
should be visible in configuration.

Do not assume a compressor is local merely because it is exposed through
a CLI.

## Tests

Use fake compressors and the no-op implementation. No external
compression service is required.

Cover at minimum:

- deterministic capsule construction;
- direct-dependency handoff selection;
- no-op compression;
- successful fake compression;
- compression failure retaining the capsule and completed task state;
- budget threshold behavior;
- idempotent resume without duplicate logical handoffs;
- secret/environment exclusion;
- cancellation without task-state corruption.

### Compressor Selection

Verify:

```text
none    → NoOp without probing Caveman
auto    + unavailable Caveman → NoOp
auto    + unhealthy Caveman   → NoOp
auto    + healthy Caveman     → Caveman
caveman + healthy Caveman     → Caveman
```

### Explicit Caveman Unavailable

With `compression: caveman`, simulate missing/unhealthy Caveman.

Expected:

```text
completed task remains completed
handoff capsule retained
clear diagnostic/status recorded
workflow correctness unaffected
```

### Health Check Cancellation

Cancel during the Caveman health check.

Expected:

```text
check stops promptly
no task-state corruption
capsule remains usable
```

## Acceptance Criteria

T019A is complete when:

- [x] SOP creates a structured handoff capsule after task completion.
- [x] The capsule distinguishes changes, decisions, verification, and carry-forward information.
- [x] The capsule is usable without a compression provider.
- [x] A provider-independent `ContextCompressor` boundary exists.
- [x] A no-op compressor exists.
- [x] Caveman can be an optional adapter without leaking into core orchestration.
- [x] Bulky artifacts can be compressed according to a configured budget.
- [x] Compression failure never changes completed task state.
- [x] Authoritative workflow sources remain authoritative.
- [x] Handoff metadata survives restart.
- [x] Handoff generation is idempotent across resume.
- [x] Direct-dependency handoffs can be selected for a new task.
- [x] SOP does not inject all historical context automatically.
- [x] Secrets/environment data are not blindly included.
- [x] Tests require no network or real compression service.
- [x] The full repository suite remains green.

## Verification

```bash
go test ./internal/handoff/...
go test ./internal/store/...
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check
```

## Open Code Review

Review specifically for:

- treating Caveman installation as proof of readiness;

- probing Caveman repeatedly for every artifact;

- health checks performing destructive or expensive compression work;

- `auto` mode failing workflow execution when Caveman is unavailable;

- explicit Caveman failure changing completed task state;

- compressed context becoming authoritative workflow state;

- task completion depending on compression success;

- Caveman concepts leaking into core interfaces;

- unbounded accumulation of prior handoffs;

- all historical capsules entering every prompt;

- secrets/environment variables entering compressor input;

- duplicate handoffs after resume;

- LLM-generated facts replacing deterministic facts;

- handoffs hidden inside attempt output;

- compression inside tight RED/GREEN/fix loops;

- unnecessary vector databases, embeddings, or RAG.

Fix correctness, security, recoverability, and bounded-context findings.
Decline complexity that turns T019A into a general knowledge-management
system.

## Git

Branch:

```text
task/T019A-context-handoff
```

Commit:

```text
task(T019A): add task context handoff
```

Pull request:

```text
[Task T019A] Add task context compaction and handoff
```

## Out of Scope

Do not add compression between every TDD/review step, a new
authoritative state store, vector search, embeddings, project-wide RAG,
arbitrary conversation-memory ingestion, mandatory Caveman installation,
replacement of SQLite/Git/test/CI truth, or unrestricted multi-agent
memory sharing.

## Definition of Done

T019A is complete when each completed task leaves a small,
deterministic, durable handoff capsule for later work, while optional
mechanical compression reduces bulky supporting context without becoming
a dependency for workflow correctness.

```text
Durable workflow truth
        ↓
SOP handoff capsule
        ↓
optional compression
        ↓
bounded context for next task
```
