# T008 --- Configurable Test Runner

## Status

DONE

## Objective

Implement SOP's deterministic project verification boundary.

T008 must allow SOP to run project-defined verification commands without
hard-coding Go, Java, JavaScript, Python, Rust, or any other project
language into the orchestrator.

The runner executes configured commands, captures structured results,
and lets later workflow stages decide pass/fail without asking an LLM to
interpret normal command execution.

``` text
Project Configuration
        │
        ▼
    Test Runner
        │
        ├─ build
        ├─ unit
        ├─ integration
        ├─ lint/static checks
        └─ docker build
        │
        ▼
Structured Results
  exit code
  stdout
  stderr
  duration
  status
```

The project being managed may be written in any language. SOP itself
being written in Go must not make the verification layer Go-specific.

------------------------------------------------------------------------

## Dependencies

-   T003 --- CLI Foundation

T007 is already complete and may be used by later orchestration, but the
test runner itself must remain independent of Git workflow behavior.

------------------------------------------------------------------------

## Design Rule

Preserve the control boundary:

``` text
Agents do the work.
SOP controls the process.
```

Verification is workflow control.

The test runner determines what happened when a configured command ran.
An LLM must not decide whether a successful exit code is a pass or
whether a non-zero exit code is a failure.

For V1:

``` text
exit code 0     → PASS
non-zero        → FAIL
timeout/cancel  → ERROR/CANCELED
cannot execute  → ERROR
```

The exact result enum may vary, but the classification must be
deterministic.

------------------------------------------------------------------------

## Language Independence

Do not hard-code commands such as:

``` text
go test ./...
go build ./...
mvn test
./gradlew test
npm test
pytest
cargo test
```

inside the runner.

Instead, projects define the commands SOP should execute.

Conceptual configuration:

``` yaml
commands:
  build: "./gradlew build"
  unit_test: "./gradlew test"
  integration_test: "./gradlew integrationTest"
  lint: "./gradlew check"
  docker_build: "docker build ."
```

Another project may use:

``` yaml
commands:
  build: "go build ./..."
  unit_test: "go test ./..."
  lint: "go vet ./..."
```

Or:

``` yaml
commands:
  build: "npm run build"
  unit_test: "npm test"
  lint: "npm run lint"
```

The runner executes configured behavior; it does not infer project
language.

Automatic project detection may be added later to propose configuration,
but configuration remains authoritative.

------------------------------------------------------------------------

## Configuration Scope

T008 needs a small project verification configuration model.

Do not build a large configuration framework.

A reasonable initial representation is conceptually:

``` go
type Commands struct {
    Build           string
    UnitTest        string
    IntegrationTest string
    Lint            string
    DockerBuild     string
}
```

or an equivalent map/category representation if that produces a smaller
and more extensible implementation.

The implementation should support absent optional commands.

Example:

``` yaml
commands:
  unit_test: "go test ./..."
```

must be valid even when no integration or Docker command is configured.

Do not treat an unconfigured optional command as a failed test.

------------------------------------------------------------------------

## Verification Categories

T008 should support these categories:

``` text
BUILD
UNIT_TEST
INTEGRATION_TEST
LINT
DOCKER_BUILD
```

The representation should make it possible to add future categories such
as:

``` text
FORMAT
STATIC_ANALYSIS
SECURITY_SCAN
E2E
```

without redesigning the execution engine.

Avoid unnecessary abstraction in V1.

------------------------------------------------------------------------

## Runner Boundary

Add:

``` text
internal/test/
```

Suggested structure:

``` text
internal/test/
├── runner.go
├── command.go
└── runner_test.go
```

If `internal/test` creates confusing naming with Go's testing
terminology, `internal/testrunner` is also acceptable.

Keep it independent of CLI formatting, scheduler state, Git state, agent
providers, GitHub, and SQLite.

A possible API is:

``` go
type Runner struct {
    dir string
}

func (r *Runner) Run(
    ctx context.Context,
    check Check,
) Result
```

where `Check` contains at least:

``` text
category
command
```

The exact API may differ if a smaller shape is clearer.

------------------------------------------------------------------------

## Command Execution

Commands are project configuration, so T008 must explicitly define how
they are executed.

Unlike the Git adapter, verification commands commonly require shell
syntax:

``` text
go test ./...
npm test -- --runInBand
./gradlew test
docker compose -f docker-compose.test.yml up
```

V1 may execute configured verification commands through a shell if
needed to support normal project command syntax.

If a shell is used:

-   treat the command as trusted project configuration, not model
    output;
-   do not concatenate task titles, PR text, model responses, or other
    untrusted values into the command;
-   document the trust boundary;
-   pass the working directory explicitly;
-   preserve context cancellation and timeout behavior.

On Unix-like systems an implementation may use:

``` text
sh -c <configured-command>
```

behind a small command-execution boundary.

Do not reuse the Git adapter's no-shell rule blindly: Git arguments are
structured by SOP, while project verification commands are intentionally
user/project-defined command lines.

------------------------------------------------------------------------

## Working Directory

Every verification command must run against an explicit project
directory.

Do not rely on the SOP process's accidental current working directory.

Example:

``` text
Runner{
    dir: "/workspace/project",
}
```

All commands for that runner execute there.

A missing or invalid directory must produce a clear execution error.

------------------------------------------------------------------------

## Structured Result

Every configured command execution should produce a structured result.

At minimum capture:

``` go
type Result struct {
    Category  Category
    Command   string
    ExitCode  int
    Stdout    string
    Stderr    string
    Duration  time.Duration
    Status    Status
}
```

Exact names may differ.

The result should preserve enough diagnostic output for later TDD,
review, and CI remediation stages.

Do not require an LLM to convert raw command execution into pass/fail.

------------------------------------------------------------------------

## Result Status

Use deterministic statuses such as:

``` text
PASS
FAIL
ERROR
CANCELED
```

Suggested semantics:

### PASS

The command started and exited with code `0`.

### FAIL

The command started and exited normally with a non-zero exit code.

Examples:

``` text
unit test assertion failure
compiler error
lint violation
Docker build failure
```

These are verification failures, not runner infrastructure failures.

### ERROR

The runner could not execute the check normally.

Examples:

``` text
invalid working directory
shell/executable unavailable
process could not start
invalid runner configuration
```

### CANCELED

The supplied context was canceled or its deadline expired.

If distinguishing `TIMEOUT` from `CANCELED` is inexpensive and clearly
useful, it may be represented separately. Do not introduce complex retry
policy in T008.

------------------------------------------------------------------------

## Exit Codes

Capture the actual exit code whenever the command process starts and
exits.

Examples:

``` text
0 → PASS
1 → FAIL
2 → FAIL
```

Do not assume only exit code `1` represents a verification failure.

For execution errors where no child exit code exists, use a clearly
documented sentinel such as:

``` text
-1
```

or an optional field.

Avoid ambiguous `0` for commands that never successfully executed.

------------------------------------------------------------------------

## stdout and stderr

Capture stdout and stderr separately.

Do not merge them in the core result.

Later workflow stages may need to distinguish normal test output from
compiler or diagnostic output.

Preserve command output on both success and failure.

Example:

``` text
Result
├─ stdout: test progress/results
└─ stderr: compiler/runtime diagnostics
```

T008 does not need sophisticated log parsing.

------------------------------------------------------------------------

## Duration

Measure wall-clock execution duration.

Use Go's monotonic time support through normal `time.Now()` /
`time.Since()` behavior.

Duration must be populated for:

-   passing commands;
-   failing commands;
-   execution errors where timing began;
-   canceled/timed-out commands.

Do not use duration to classify success.

------------------------------------------------------------------------

## Context Cancellation and Timeouts

All command execution must honor `context.Context`.

Conceptually:

``` text
context
   │
   ▼
command starts
   │
   ├─ completes → PASS/FAIL
   │
   └─ context canceled/deadline → CANCELED
```

Use process execution that is tied to the context.

T008 does not need to define workflow retry behavior. It only reports
the result.

Later tasks decide whether a canceled or failed check is retried.

------------------------------------------------------------------------

## Sequential Execution

V1 runs verification checks sequentially.

Given:

``` text
build
unit
lint
```

execute in deterministic configured order.

Do not add concurrent command execution in T008.

Parallel test execution inside the project's own test command is fine
because that is controlled by the project command itself.

------------------------------------------------------------------------

## Suite Execution

In addition to executing one check, provide a small way to run an
ordered set of checks.

Conceptually:

``` go
func (r *Runner) RunAll(
    ctx context.Context,
    checks []Check,
) SuiteResult
```

A suite should preserve the ordered individual results.

For V1, fail-fast behavior should be explicit rather than accidental.

Preferred default:

``` text
run configured checks in order
stop after the first FAIL/ERROR/CANCELED
```

This avoids wasting time on later gates when an earlier required gate
already failed.

If the implementation supports continue-on-failure, it must be an
explicit policy rather than implicit behavior.

Do not add a large policy engine.

------------------------------------------------------------------------

## Unconfigured Checks

An absent command is not a failure.

Example:

``` yaml
commands:
  unit_test: "go test ./..."
  integration_test: ""
```

The runner should either:

-   omit the unconfigured check from the suite; or
-   represent it explicitly as `SKIPPED`.

Choose one simple consistent behavior.

Do not execute an empty shell command and call it a pass.

------------------------------------------------------------------------

## Environment

By default, child verification commands should inherit the current
environment.

T008 may support explicit environment overrides if it remains small:

``` go
map[string]string
```

Do not add secret management.

Do not print or persist the full environment in results.

Later project/bootstrap work can define environment provisioning.

------------------------------------------------------------------------

## No Workflow State Mutation

The test runner does not own task state.

It must not transition:

``` text
TESTS_WRITTEN
RED_VERIFIED
IMPLEMENTING
LOCAL_TESTS_PASS
FIX_REQUIRED
BLOCKED
```

It reports verification facts.

Later orchestration/TDD logic decides legal state transitions.

``` text
Test Runner
    ↓
Result
    ↓
Task Runner / Orchestrator
    ↓
State transition
```

------------------------------------------------------------------------

## No LLM Boundary

T008 requires:

``` text
no model
no agent harness
no API key
no network service
```

A configured command itself may access resources if the project
intentionally does so, but the runner must not invoke an LLM to
interpret results.

Tests for T008 must not require network access.

------------------------------------------------------------------------

## Security Boundary

Configured project commands are executable code.

Document this clearly:

``` text
SOP verification commands are trusted project configuration.
```

SOP must not automatically build shell commands from:

-   LLM output;
-   task descriptions;
-   acceptance criteria;
-   branch names;
-   PR text;
-   issue text;
-   arbitrary external input.

This boundary becomes especially important when agent-generated
configuration is introduced later.

T008 should execute only commands already accepted as project
configuration by the caller.

------------------------------------------------------------------------

## Tests

Write tests before or alongside implementation.

Use temporary directories and small commands/scripts. Tests must not
depend on Go being the language of the target project.

At minimum cover the following.

### Passing Command

Configured command exits `0`.

Expected:

``` text
Status   PASS
ExitCode 0
stdout   captured
duration populated
```

### Failing Command

Configured command exits non-zero.

Expected:

``` text
Status   FAIL
ExitCode preserved
stdout/stderr captured
```

### stderr Capture

Command writes distinct values to stdout and stderr.

Expected:

``` text
stdout != stderr
both preserved
```

### Invalid Working Directory

Runner points at a missing directory.

Expected:

``` text
ERROR
```

with a useful diagnostic.

### Context Cancellation

Cancel the context before or during execution.

Expected:

``` text
CANCELED
```

and command does not continue indefinitely.

### Timeout

Use a short deadline against a deliberately slow local command.

Expected deterministic cancellation/timeout behavior.

Do not make the test materially slow.

### Duration

Run a small local command and verify duration is non-negative/populated.

Do not assert fragile exact timings.

### Ordered Suite

Given:

``` text
BUILD
UNIT_TEST
LINT
```

verify results occur in that order.

### Fail Fast

Given:

``` text
BUILD → PASS
UNIT_TEST → FAIL
LINT → would PASS
```

expected:

``` text
BUILD executed
UNIT_TEST executed
LINT not executed
```

if fail-fast is the chosen V1 policy.

### Empty/Unconfigured Command

Verify empty configuration is omitted or `SKIPPED`, according to the
chosen design.

It must not be reported as a successful executed command.

### No Shell Data Injection From Structured Fields

If the runner has structured fields besides the trusted command itself,
verify they are not concatenated into executable command text.

------------------------------------------------------------------------

## Portability

SOP's verification model must remain language-independent.

T008 does not need full Windows shell support unless the current project
already requires it, but command execution should be isolated enough
that shell strategy can later be replaced per platform.

Do not spread `sh -c` calls throughout the package.

Keep process invocation behind one small boundary.

------------------------------------------------------------------------

## Documentation

Update architecture or README documentation only where needed to
explain:

-   project-defined verification commands;
-   language-independent runner behavior;
-   trusted-command security boundary;
-   deterministic result classification.

Do not document future auto-detection as if it is already implemented.

------------------------------------------------------------------------

## Acceptance Criteria

T008 is complete when:

1.  SOP can execute a configured project verification command;
2.  commands are not hard-coded to Go or any target language;
3.  build, unit, integration, lint, and Docker-build categories are
    representable;
4.  commands run in an explicit project working directory;
5.  exit code is captured;
6.  stdout is captured;
7.  stderr is captured separately;
8.  duration is captured;
9.  exit code `0` deterministically produces `PASS`;
10. a non-zero command exit deterministically produces `FAIL`;
11. runner infrastructure failures produce `ERROR`;
12. context cancellation/deadline is represented deterministically;
13. an ordered suite of checks can be executed;
14. V1 suite failure behavior is deterministic;
15. absent optional commands are not treated as failures;
16. the runner does not mutate workflow/task state;
17. the runner does not invoke an LLM;
18. tests require no network service;
19. trusted project-command boundaries are documented;
20. the existing repository test suite remains green.

------------------------------------------------------------------------

## Verification

Run:

``` bash
go test ./internal/test/...
```

or, if the package is named `testrunner`:

``` bash
go test ./internal/testrunner/...
```

Then:

``` bash
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check
```

------------------------------------------------------------------------

## Open Code Review

After local verification passes, run Open Code Review against the task
branch.

Review specifically for:

-   Go-specific behavior leaking into the generic runner;
-   non-zero exit codes incorrectly classified as infrastructure errors;
-   lost stdout/stderr on failure;
-   stdout and stderr being merged;
-   context cancellation not terminating the child command;
-   empty commands being treated as successful executions;
-   accidental dependence on process current directory;
-   nondeterministic suite ordering;
-   later checks running unexpectedly after fail-fast;
-   task-state mutation inside the runner;
-   LLM interpretation of ordinary pass/fail;
-   shell command construction from untrusted/model-generated values;
-   secrets/environment being unnecessarily logged;
-   unnecessary configuration framework complexity.

Fix correctness and safety findings.

Decline style suggestions that add abstraction without improving the
verification boundary.

------------------------------------------------------------------------

## Git

### Branch

``` text
task/T008-test-runner
```

### Commit

``` text
task(T008): add configurable test runner
```

### Pull Request

``` text
[Task T008] Add configurable test runner
```

------------------------------------------------------------------------

## Out of Scope

Do not add:

-   T009 agent harness behavior;
-   T010 TDD orchestration;
-   automatic test generation;
-   RED/GREEN state transitions;
-   retry/remediation loops;
-   Git operations;
-   GitHub Actions inspection;
-   PR creation;
-   CI monitoring;
-   review providers;
-   parallel check execution;
-   language-specific command hard-coding;
-   automatic project-language detection;
-   package-manager installation;
-   Docker environment provisioning;
-   secret management;
-   remote command execution.

------------------------------------------------------------------------

## Definition of Done

T008 is complete when SOP has a tested, deterministic,
language-independent verification boundary that executes trusted
project-defined commands, captures exit code/stdout/stderr/duration,
classifies results without LLM interpretation, supports an ordered V1
verification suite, honors cancellation, and leaves workflow state
ownership to later orchestration.
