# SOP Performance

Measure first, optimize second. SOP records where a run spends its time so a slow
run can be diagnosed from evidence instead of guesswork. **No optimization in this
guide weakens a gate**: timing metadata never drives a workflow decision, and
faster execution never comes from skipping validation, review, provenance, or a
human approval boundary.

## The measurement model

One package, `internal/perf`, is the single representation of perf data. It records:

- **Stage durations** (milliseconds, from the monotonic clock): `plan`,
  `implement`, `validation` (with a `build`/`test`/`lint` breakdown), `review`,
  and `fix`, plus the task's total wall time.
- **Operation counts**: agent calls, agent calls avoided (verify-first),
  validation runs and safe reuses, review runs and safe reuses, and fix cycles.

Timing wraps existing operations in `runStages` (`internal/cli/run.go`); it does not
add a parallel state machine or change any lifecycle transition.

## Where it is persisted

Metrics are diagnostic metadata, never authoritative workflow state:

- per task: `.agent-sdlc/runs/<task-id>/metrics.json`, and a `performance` field in
  that run's `report.json`;
- per run: `.agent-sdlc/runs/<plan-id>/metrics.json` (the aggregate a full
  `sop run` leaves behind).

A missing or older artifact is handled gracefully — `sop report` says timing is
unavailable rather than inventing values.

## Reading the output

`sop run` prints a concise line per task:

```text
performance: 40.5s total (agent 20.8s, validation 13.6s, review 6.1s) | agent calls 2, validation runs 1, fix cycles 0
```

`sop report` prints the full picture — the run summary when one is available,
otherwise the reported task's own record:

```text
Performance

Tasks:       13
Total:       6m 42s

Agent:       4m 18s  (64%)
Validation:  1m 41s  (25%)
Review:        43s    (11%)

Agent calls:            18
Agent calls avoided:     7
Validation runs:        21
Validation reused:       3
Review runs:            11
Review reused:           0
Fix cycles:              3
```

Read it as: **agent** is `plan` + `implement` + `fix`; percentages are of the
measured stage time only (unmeasured overhead is excluded, so the three may not
sum to 100% of `Total`). To find the dominant bottleneck, compare the three
categories and the counts.

## verify-first savings

A `verify-first` task runs the configured validation before any agent. When it
passes, the implementation agent is never invoked — recorded as one **agent call
avoided**. Set `execution.mode: verify-first` on a task whose acceptance can be
proven by the configured commands. It is not broadened to tasks that deterministic
validation cannot prove complete; a failing verify-first validation still takes the
normal implementation path, and review/human gates still apply.

## Validation and review reuse (safety rules)

A single run may reuse a prior result, but only when SOP can *prove* the inputs are
identical. The identity is:

- validation: the exact command set **and** the working-tree change (`diff`), hashed;
- review: the task, the diff, and the review engine, hashed.

Rules, in one place (`internal/cli/session.go`):

- a **changed repository** changes the diff and therefore the identity — a stale
  result can never be reused;
- a **changed command set or config** changes the identity;
- only a **passing** validation or a **clean** review (no findings) is cached, so a
  transient or flaky failure is never replayed onto another task;
- reuse never crosses a run (the cache lives for one invocation) or a plan;
- a **miss is always safe** — correctness never depends on a hit.

In the common flow there is little to reuse *within* a task (a fix changes the
tree, so re-validation is genuinely needed). The measured win is across consecutive
tasks on an unchanged tree, such as a run of already-satisfied verify-first tasks.

## Benchmark workflow

The deterministic fixture is `TestPerformanceBenchmarkFixture` in
`internal/cli/perf_test.go`: a small plan with an already-satisfied verify-first
task, a small implementation task, and a dependency between them, driven by a
counting stub agent.

```bash
go test ./internal/cli/ -run 'TestPerformanceBenchmarkFixture|TestValidationReuse' -v
```

It asserts **operation counts** (stable on any machine) rather than wall-clock, so
a slow CI machine cannot make it fail. Baseline counts it protects:

```text
2 tasks (1 verify-first avoided agent call)
agent calls        2
validation runs    1   (+1 safely reused across the two-verify-first fixture)
review runs        1
fix cycles         0
```

A real-provider benchmark (Ollama / `deepseek-v4.1-flash:cloud`) is optional and
never required by the normal suite.

## Deferred optimizations (with evidence)

These were evaluated and deliberately deferred; none is a correctness shortcut.

- **Targeted validation (PERF008).** The lifecycle validates only at the task gate
  (and the verify-first pre-check); there is no "during implementation" validation
  point to target. Adding one is a new lifecycle stage whose cost/benefit must be
  measured first, and the plan itself requires that a targeted pass never replaces
  the authoritative final gate. Deferred pending that measurement.
- **Agent process startup (PERF010).** Startup is already inside the measured
  `implement`/`fix`/`plan` stage for the command provider. Avoiding a per-call
  process launch needs a persistent session/daemon, which the plan says not to
  introduce without well-defined lifecycle, cleanup, isolation, and recovery.
  Deferred.
- **Intra-task context reuse (PERF011).** The command provider runs a fresh process
  per call and exposes no session to reuse. SOP persisted state remains sufficient
  for resume, and no provider is required to support reuse. Deferred.
- **Parallel task mutation (PERF012).** The local run shares one working tree and
  one branch, so parallel *mutation* would race on both. A worktree-isolated
  executor exists (`internal/parallel`) but is not wired into the sequential local
  run; enabling it needs deterministic workspace ownership, per-worktree
  validation, and state-database concurrency rules. Per the plan, deferral with
  evidence is the correct outcome. Read-only verification could be parallelized
  later without these risks.
- **End-to-end dogfood (PERF014).** The deterministic fixture provides the
  reproducible before/after counts; a real-provider dogfood run is left to the
  operator and is not part of the suite.

## Guarantee

Wherever the two conflict, correctness wins over speed. Timing is metadata, reuse
requires an identical input identity, and verify-first only ever *skips work that
deterministic validation has already proven unnecessary*.
