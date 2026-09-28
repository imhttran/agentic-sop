# SOP Performance and Timing

## Project

agentic-sop

## Summary

Measure and improve SOP execution performance without weakening
correctness, validation, review, plan provenance, task isolation, or
human gates.

The plan is measurement-first: add stage-level timing and execution
metrics, establish a reproducible baseline, then optimize unnecessary
agent calls, repeated validation, review work, and safe execution
overhead based on measured evidence.

Existing `verify-first` behavior must be preserved and measured.

## Goal

Make SOP materially faster for real multi-task plans while retaining
deterministic workflow behavior.

The optimization loop is:

``` text
measure
  ↓
identify bottleneck
  ↓
optimize
  ↓
measure again
  ↓
prove no correctness regression
```

Do not optimize by skipping required gates or trusting an agent's
completion claim.

## PERF001 --- Define Performance Metrics

Define a small authoritative performance model for an SOP run.

Capture at minimum:

``` text
plan preparation
task selection
PLAN
IMPLEMENT
DESIGN_TESTS
FIX
validation build
validation test
validation lint
REVIEW
human-gate preparation
artifact/state persistence
total task time
total plan/run time
```

Also count:

``` text
agent calls
tool calls when available
validation executions
review executions
fix cycles
agent calls avoided by verify-first
tasks completed without an agent call
```

Use monotonic elapsed-time measurement for durations.

Do not make timing data part of workflow correctness decisions.

### Acceptance Criteria

-   stage-level duration metrics have a single documented representation
-   total task and total run durations are available
-   agent-call counts are available
-   validation/review/fix-cycle counts are available
-   verify-first avoided-agent calls can be measured
-   timing instrumentation does not alter lifecycle decisions
-   tests can use deterministic/fake timing where necessary

## PERF002 --- Add Stage-Level Timing Instrumentation

Instrument the existing execution lifecycle without creating a parallel
state machine.

Measure elapsed time around existing operations rather than duplicating
their control flow.

Example output:

``` text
Task: AHV2004 Implement Controlled Tool Harness

PLAN          2.1s
IMPLEMENT    18.7s
BUILD         3.2s
TEST          8.4s
LINT          2.0s
REVIEW        6.1s
----------------
TOTAL        40.5s

Agent calls: 2
Validation runs: 1
Fix cycles: 0
```

Keep normal output concise. Detailed timing may be surfaced at task
completion, report time, or behind an appropriate verbosity mechanism
consistent with existing CLI conventions.

### Acceptance Criteria

-   major lifecycle operations are timed
-   timing wraps existing operations rather than replacing lifecycle
    logic
-   task total equals or reasonably reconciles with measured stage
    durations
-   timing works for success, failure, blocked, and interrupted paths
-   instrumentation has negligible execution overhead
-   existing command output remains readable

## PERF003 --- Persist Performance Data

Persist useful performance metrics with existing run artifacts so
`sop report` can summarize completed work.

Use existing `.agent-sdlc/runs` conventions where practical.

Do not introduce a second workflow database solely for metrics.

Performance data must be diagnostic metadata, not authoritative workflow
state.

### Acceptance Criteria

-   completed task/run metrics survive process exit
-   interrupted runs preserve metrics collected before interruption
    where practical
-   persisted metrics do not control task state
-   old runs without timing data remain readable
-   metrics schema changes are backward compatible or safely optional
-   no secrets, prompts, or model credentials are stored as timing
    metadata

## PERF004 --- Add Performance Summary to `sop report`

Extend `sop report` to summarize where time was spent.

Example:

``` text
Performance

Tasks:       13
Total:       6m 42s

Agent:       4m 18s  (64%)
Validation:  1m 41s  (25%)
Review:        43s    (11%)

Agent calls:                    18
Agent calls avoided:             7
Validation runs:                21
Review runs:                    11
Fix cycles:                      3
```

If timing information is unavailable, report that clearly rather than
fabricating values.

Percentages should be derived from measured categories and should not
imply precision beyond the collected data.

### Acceptance Criteria

-   `sop report` shows total measured execution time
-   report identifies major measured time categories
-   agent/validation/review counts are shown when available
-   verify-first savings are visible
-   missing historical metrics are handled gracefully
-   report does not claim unmeasured time as measured

## PERF005 --- Establish Reproducible Baseline

Create a deterministic benchmark/evaluation fixture representative of a
small multi-task SOP plan.

The fixture should include examples of:

``` text
already-satisfied verification task
small implementation task
validation failure requiring correction
review path
dependency between tasks
```

Use fake/stub agents for deterministic benchmark tests where model
latency would make comparisons noisy.

Optionally support a separate real-provider benchmark that is not
required by the normal test suite.

Capture baseline metrics before optimization.

### Acceptance Criteria

-   a repeatable local performance fixture exists
-   deterministic benchmark does not require Ollama or Claude
-   baseline captures task count, stage durations/counts, validation
    runs, and agent calls
-   benchmark can compare before/after optimization behavior
-   normal correctness tests remain separate from performance assertions
    where appropriate

## PERF006 --- Measure and Harden Verify-First Savings

Measure the existing verify-first execution path and ensure it actually
avoids unnecessary model work.

For an already-satisfied verification task:

``` text
task
 ↓
deterministic validation
 ↓ PASS
complete task
 ↓
zero implementation-agent call
```

Do not broaden verify-first to tasks where deterministic validation
cannot prove completion.

### Acceptance Criteria

-   verify-first tasks report whether an agent call was avoided
-   already-satisfied fixture task completes without IMPLEMENT/FIX agent
    calls
-   failed verify-first validation invokes the normal required agent
    path
-   verify-first does not bypass review/human gates when those remain
    required by policy
-   metrics expose verify-first savings
-   regression tests protect the optimization

## PERF007 --- Eliminate Duplicate Validation Within a Task

Identify cases where identical validation commands run repeatedly
against an unchanged working tree during one task lifecycle.

Introduce safe reuse only when SOP can prove the relevant repository
state has not changed.

A validation result must become invalid when inputs that could affect it
change.

Possible identity inputs may include:

``` text
working-tree/repository content identity
validation configuration
command set
relevant execution context
```

Prefer correctness over cache hit rate.

Do not reuse validation merely because the task ID is the same.

### Acceptance Criteria

-   identical validation against provably unchanged state can be reused
    within the defined safe scope
-   any repository mutation invalidates affected cached validation
-   validation configuration changes invalidate reuse
-   failed and successful validation reuse semantics are explicitly
    defined
-   cache/reuse never crosses unsafe plan/project boundaries
-   metrics report executed versus reused validation
-   tests prove stale validation cannot incorrectly pass a changed
    repository

## PERF008 --- Add Targeted Validation Support

Allow plans/tasks or execution logic to use targeted validation where
the repository already provides a safe narrower command, while
preserving a final authoritative gate.

Example:

``` text
during implementation:
  go test ./internal/agent/...

final task/plan gate:
  go test ./...
  go vet ./...
  go build ./...
```

Do not infer unsafe test subsets from filenames without a defensible
repository-specific mechanism.

Keep existing configured validation authoritative unless explicit scoped
validation is configured.

### Acceptance Criteria

-   targeted validation can be represented explicitly
-   targeted checks do not replace required final authoritative
    validation
-   existing projects without targeted validation behave unchanged
-   metrics distinguish targeted and full validation
-   documentation explains when targeted validation is safe
-   tests ensure a targeted pass cannot bypass a required full final
    gate

## PERF009 --- Reduce Redundant Review Calls

Measure when REVIEW is invoked and identify duplicate reviews of an
unchanged diff.

If the exact review input and relevant policy are unchanged, safely
reuse an existing review result within the same task/run scope where
appropriate.

Any material diff change after FIX/IMPLEMENT must invalidate the prior
review.

Do not skip review merely because validation passes.

### Acceptance Criteria

-   review executions are counted and timed
-   unchanged review inputs may reuse a prior result only under explicit
    safe identity rules
-   repository/diff changes invalidate review reuse
-   review policy/config changes invalidate reuse
-   required review gates remain enforced
-   metrics report executed versus reused review operations
-   tests prevent stale review results from approving changed code

## PERF010 --- Reduce Agent Process Startup Overhead

Measure agent/provider startup separately from model execution where the
architecture exposes that distinction.

For command-based agents, avoid unnecessary repeated process launches
only if a safe persistent/session mechanism exists.

For the Agent Harness V2 path, design integration so a bounded harness
can perform the necessary tool loop without spawning a heavyweight
external coding-agent process for every individual tool action.

Do not introduce a daemon solely for performance unless lifecycle,
cleanup, isolation, and failure recovery are well-defined.

### Acceptance Criteria

-   agent startup overhead is measurable where applicable
-   no unnecessary process launch is introduced per tool call
-   persistent/session reuse, if implemented, has explicit task
    boundaries
-   context from one task cannot leak unsafely into another
-   interrupted sessions can be cleaned up deterministically
-   command-provider compatibility remains intact

## PERF011 --- Add Safe Intra-Task Context Reuse

Avoid making the model rediscover identical task context across
PLAN/IMPLEMENT/FIX/REVIEW operations when the provider/harness supports
safe context reuse.

Context reuse must be scoped and bounded.

Do not make correctness depend on an opaque model session.

Persisted SOP state remains authoritative.

If a provider does not support safe reuse, retain existing behavior.

### Acceptance Criteria

-   context reuse is optional and capability-aware
-   task boundaries isolate reused context
-   SOP can recover if reused model context disappears
-   persisted workflow state remains sufficient for resume
-   no provider is required to support session reuse
-   metrics can compare reused versus fresh agent interactions where
    available

## PERF012 --- Evaluate Safe Parallelism

Evaluate whether independent tasks can safely execute concurrently.

Do not enable parallel task mutation merely because dependency edges
permit it.

Consider:

``` text
shared working tree
Git branch ownership
validation interference
run artifact isolation
human gates
provider resource limits
state database concurrency
```

If safe implementation would require substantial architectural change,
document the result and defer parallel mutation rather than introducing
unsafe concurrency.

Read-only verification or independent non-mutating operations may be
considered separately.

### Acceptance Criteria

-   concurrency risks are explicitly analyzed
-   no unsafe shared-working-tree parallel mutation is introduced
-   any implemented parallelism has deterministic ownership/isolation
    rules
-   sequential execution remains available
-   provider/resource concurrency limits are respected
-   tests cover any concurrency behavior that is introduced
-   deferral is acceptable when supported by documented evidence

## PERF013 --- Add Performance Budgets and Regression Checks

Add coarse regression checks around deterministic benchmark behavior.

Avoid brittle wall-clock assertions in normal CI.

Prefer stable metrics such as:

``` text
number of agent calls
number of validation executions
number of review executions
number of redundant operations avoided
```

Use wall-clock benchmarks for observation rather than strict correctness
unless the environment is controlled.

### Acceptance Criteria

-   deterministic operation-count regressions can be detected
-   normal tests do not fail because a CI machine is temporarily slow
-   benchmark output includes elapsed timing for human comparison
-   optimization tests prove fewer redundant operations without
    weakening gates
-   baseline and optimized measurements are documented

## PERF014 --- End-to-End Performance Dogfood

Run a representative SOP plan before and after the optimizations.

Capture:

``` text
total elapsed time
per-stage time
agent calls
agent calls avoided
validation executions
validation reuse
review executions
review reuse
fix cycles
```

If Agent Harness V2 is available, include an opt-in run using:

``` text
Ollama
deepseek-v4.1-flash:cloud
```

Do not require a live external provider for the normal test suite.

### Acceptance Criteria

-   before/after measurements are captured
-   measured improvements are attributed to specific optimizations
-   correctness gates produce the same required outcome
-   no validation/review/human gate is removed merely to improve timing
-   real-provider benchmark is optional
-   dogfood results identify remaining dominant bottlenecks

## PERF015 --- Documentation and Performance Guide

Document:

``` text
verify-first
performance metrics
sop report timing
validation reuse
targeted validation
review reuse
agent/session reuse when supported
benchmark workflow
```

Include guidance for interpreting performance output.

Explain that faster execution must not come from weakening deterministic
gates.

Document any intentionally deferred optimization such as parallel
mutation.

### Acceptance Criteria

-   README or performance documentation explains the measurement model
-   users can identify where a slow run spends its time
-   verify-first savings are explained
-   validation/review reuse safety rules are documented
-   benchmark instructions are reproducible
-   deferred performance work is clearly identified

## Safety Constraints

Performance optimization must not:

-   trust an agent's completion claim instead of validating
-   bypass required validation
-   bypass required review
-   bypass human approval
-   reuse validation after relevant repository changes
-   reuse review after relevant diff changes
-   mix task context across unsafe boundaries
-   mix named-plan provenance
-   run concurrent mutations against an unsafe shared working tree
-   delete state to make execution appear faster
-   weaken retry/resume correctness
-   make timing metadata authoritative workflow state

SOP remains the deterministic workflow authority.

## Validation

Before completion run:

``` bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

All must pass.

Run the deterministic performance fixture and capture before/after
operation counts and timing.

## Definition of Done

A representative multi-task SOP run provides enough instrumentation to
answer:

``` text
Where did the time go?
How many agent calls were made?
How many were avoided?
How often did validation run?
How often was validation safely reused?
How often did review run?
How often was review safely reused?
Which stage is now the dominant bottleneck?
```

Measured redundant work is reduced without weakening SOP's validation,
review, provenance, state, or human-gate guarantees.

`verify-first` savings are visible, repeated validation/review of
unchanged state is safely reduced where possible, and remaining
performance bottlenecks are documented with evidence rather than
guessed.
