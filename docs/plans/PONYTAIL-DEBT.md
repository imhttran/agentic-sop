# Ponytail Debt — Adopt Concrete Revisit Triggers

**Status:** ACTIVE.
**Baseline:** `main` at `f4610c0e`.

## Project

agentic-sop

## Summary

The ponytail debt scan found seven `ponytail:` markers. Six of them name no
revisit trigger, so nothing calls them back. This plan adopts one concrete
trigger per marker in the source comment. It keeps the `ponytail:` prefix so the
scan still finds every marker.

The work changes comment text only. It changes no executable behavior. The
underlying ceilings stay as they are: this plan makes each one reviewable, not
fixed.

The ledger snapshot at the end of this document records the state before these
tasks. Rerun `grep -rnE '(#|//) ?ponytail:' .` after the tasks land to refresh
it.

Out of scope: fixing the ceilings themselves, changing sandbox behavior,
changing the token-budget semantics, and changing CI timeouts. Those are
separate plans.

## PD-001 — Adopt a trigger for the process-group marker

Replace the vague "move them here when next touched" note in
`internal/procgroup/procgroup.go` with a named trigger.

Keep the `ponytail:` prefix. Keep the marker where it is. Write this comment,
with `Revisit when a change to waitDelay` on one physical line:

```go
// ponytail: the exec/process-group binding is copied into security, toolharness,
// agent, evaltarget, controller/sopclient and decision/commandprofile; each copy
// can drift. Revisit when a change to waitDelay, SysProcAttr, killGroup or
// cmd.Cancel has to land in more than one copy: consolidate here first.
```

Change no executable code.

### Deliverables

- internal/procgroup/procgroup.go

### Dependencies

- None

### Acceptance Criteria

- The procgroup marker names the trigger `Revisit when a change to waitDelay`.
- The comment keeps the `ponytail:` prefix.
- The package still builds.

### Acceptance Checks

- `git grep -q "Revisit when a change to waitDelay" -- internal/procgroup/procgroup.go`
- `git grep -q "ponytail: " -- internal/procgroup/procgroup.go`
- `go build ./internal/procgroup`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3

## PD-002 — Adopt triggers for the four isolation markers

Replace the four `ponytail:` notes in `internal/toolharness/isolation.go` with
named triggers. Keep each `ponytail:` prefix and each marker's position.

Write these comments. Keep each quoted trigger phrase on one physical line.

The build-cache marker:

```go
// ponytail: every sandboxed build starts with a cold build cache; seeding it from
// a trusted snapshot would make it warm again. Revisit when a sandboxed build takes
// longer than twice an unsandboxed build of the same commit, or when a race shard
// approaches SOP_TEST_TIMEOUT.
```

The home-credential marker:

```go
// ponytail: the denies match paths, so a credential inside a writable dir (the
// root or temp, for example KUBECONFIG pointing into the repo) can be renamed, or
// its parent dir moved, and then read. Revisit when a new credential path is added
// under a writable root, or a credential is read after a rename or move: fail
// closed on those paths then.
```

The home-socket marker:

```go
// ponytail: a socket created after the scan, or deeper than the depth bound, is not
// masked. Revisit when a sandboxed tool creates sockets at runtime, or a socket is
// observed beyond the depth bound: mask at connect time, or hide the home tree, then.
```

The `/tmp` socket marker:

```go
// ponytail: a Unix socket outside /tmp and /run that the bounded home scan does not
// reach (too deep, or outside $HOME) stays connectable. Revisit when the sandbox
// moves to a deny-default view, or a socket connect escapes the sandbox; a
// deny-default view closes both.
```

Change no executable code.

### Deliverables

- internal/toolharness/isolation.go

### Dependencies

- None

### Acceptance Criteria

- The build-cache marker names the trigger `Revisit when a sandboxed build takes`.
- The home-credential marker names the trigger `Revisit when a new credential path is added`.
- The home-socket marker names the trigger `Revisit when a sandboxed tool creates sockets`.
- The `/tmp` socket marker names the trigger `Revisit when the sandbox moves to a deny-default view`.
- Every comment keeps the `ponytail:` prefix.
- The package still builds.

### Acceptance Checks

- `git grep -q "Revisit when a sandboxed build takes" -- internal/toolharness/isolation.go`
- `git grep -q "Revisit when a new credential path is added" -- internal/toolharness/isolation.go`
- `git grep -q "Revisit when a sandboxed tool creates sockets" -- internal/toolharness/isolation.go`
- `git grep -q "Revisit when the sandbox moves to a deny-default view" -- internal/toolharness/isolation.go`
- `git grep -q "ponytail: " -- internal/toolharness/isolation.go`
- `go build ./internal/toolharness`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3
- criterion 4: check 4
- criterion 5: check 5
- criterion 6: check 6

## PD-003 — Adopt a trigger for the token-budget marker

Replace the note in `internal/perf/budget.go` with a named trigger. Keep the
`ponytail:` prefix and the marker's position.

Write this comment, with `Revisit when pricing data becomes available` on one
physical line:

```go
// ponytail: checked before each request, so the request that crosses the budget
// still completes; the overshoot is bounded by one request's input plus
// SOP_AGENT_MAX_OUTPUT_TOKENS. It is a token cap, not a dollar cap: SOP has no
// pricing data. Revisit when pricing data becomes available (switch to a dollar
// cap), or when one request overshoots by more than SOP_AGENT_MAX_OUTPUT_TOKENS.
```

Change no executable code.

### Deliverables

- internal/perf/budget.go

### Dependencies

- None

### Acceptance Criteria

- The budget marker names the trigger `Revisit when pricing data becomes available`.
- The comment keeps the `ponytail:` prefix.
- The package still builds.

### Acceptance Checks

- `git grep -q "Revisit when pricing data becomes available" -- internal/perf/budget.go`
- `git grep -q "ponytail: " -- internal/perf/budget.go`
- `go build ./internal/perf`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3

## PD-004 — Adopt a trigger for the CI timeout marker

Replace the note in `.github/workflows/ci.yml` with a named trigger. Keep the
`ponytail:` prefix, the marker's position, and the `SOP_TEST_TIMEOUT: "30m"`
value.

Write this comment, with `Revisit when a race shard hits` on one physical line:

```yaml
# ponytail: the package suite exceeds the default 10 m per-package alarm on the
# runner under -race. Revisit when a race shard hits the raised 30 m timeout, or
# when shards are split; cut test cost before raising the ceiling again.
```

Change no timeout value and no workflow step.

### Deliverables

- .github/workflows/ci.yml

### Dependencies

- None

### Acceptance Criteria

- The CI marker names the trigger `Revisit when a race shard hits`.
- The comment keeps the `ponytail:` prefix.
- The `SOP_TEST_TIMEOUT` value is unchanged.

### Acceptance Checks

- `git grep -q "Revisit when a race shard hits" -- .github/workflows/ci.yml`
- `git grep -q "ponytail: " -- .github/workflows/ci.yml`
- `git grep -q "SOP_TEST_TIMEOUT" -- .github/workflows/ci.yml`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3

## Ledger Snapshot

State before PD-001 to PD-004. Regenerate after they land.

Total: 7 markers, 6 with no trigger.

### internal/procgroup/procgroup.go:17

Simplified: exec / process-group binding is duplicated in six packages
(`security`, `toolharness`, `agent`, `evaltarget`, `controller/sopclient`,
`decision/commandprofile`). Ceiling: six private copies drift. Upgrade as
written: "move them here when next touched" (weak). Owner: Tom Tran
(f4198741f, 2026-11-09).

### internal/toolharness/isolation.go:139

Simplified: every sandboxed build starts with a cold build cache. Ceiling: builds
pay full cache cost. Upgrade as written: none named. `no-trigger`. Owner: Tom
Tran (be77f8b85, 2026-11-09).

### internal/toolharness/isolation.go:268

Simplified: credential denies match paths, so a credential inside a writable dir
can be renamed or moved and then read. Ceiling: escape path stays open. Upgrade
as written: "if it ever matters" (not a trigger). `no-trigger`. Owner: Tom Tran
(c125fc121, 2026-11-10).

### internal/toolharness/isolation.go:332

Simplified: a socket created after the scan, or deeper than the depth bound, is
not masked. Ceiling: bounded-depth masking can be bypassed. Upgrade as written:
a fix, not a trigger. `no-trigger`. Owner: Tom Tran (83ebcef84, 2026-11-11).

### internal/toolharness/isolation.go:408

Simplified: a Unix socket outside `/tmp` and `/run` that the bounded home scan
misses stays connectable. Ceiling: only a deny-default view closes it. Upgrade as
written: none named. `no-trigger`. Owner: Tom Tran (83ebcef84, 2026-11-11).

### internal/perf/budget.go:25

Simplified: the token budget is checked before each request. Ceiling: one
request's overshoot, and a token cap rather than a dollar cap. Upgrade as
written: none named. `no-trigger`. Owner: Tom Tran (115ad0e8e, 2026-11-09).

### .github/workflows/ci.yml:177

Simplified: the package suite exceeds the default 10 m per-package alarm under
`-race`, so `SOP_TEST_TIMEOUT` is raised to `30m`. Ceiling: slow CI masked by a
bigger timeout. Upgrade as written: "cut test cost later" (not a trigger).
`no-trigger`. Owner: Tom Tran (3bd7fd273, 2026-11-19).
