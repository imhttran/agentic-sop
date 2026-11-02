# T000 — Bootstrap Go Project and CI

## Status

DONE

## Goal

Create the minimum working foundation for `agentic-sdlc`.

When complete:

- the project is a valid Go module
- the CLI runs
- application code is separated from the CLI entry point
- at least one automated test exists
- local verification runs through one command
- GitHub Actions runs the same basic checks
- the work is delivered through a task branch and PR

Do not implement SDLC orchestration, state management, SQLite, scheduling,
agent execution, Git automation, or review logic in this task.

---

## Starting State

Repository:

`imhttran/agentic-sdlc`

Expected initial repository contains at least:

- `LICENSE`

Work must not be performed directly on `main`.

---

## Step 1 — Create Task Branch

Create:

`task/T000-bootstrap`

Verify the active branch before making changes.

### Acceptance

- branch exists
- current branch is `task/T000-bootstrap`
- branch is based on current `main`

---

## Step 2 — Initialize Go Module

Initialize the module as:

`github.com/imhttran/agentic-sdlc`

### Expected artifact

`go.mod`

### Verification

Run:

`go mod tidy`

It must complete successfully.

---

## Step 3 — Create CLI Entry Point

Create:

`cmd/agent-sdlc/main.go`

The executable must print:

`agent-sdlc`

Keep `main.go` thin. Application behavior belongs under `internal/`.

### Verification

Run:

`go run ./cmd/agent-sdlc`

Expected output:

`agent-sdlc`

---

## Step 4 — Create Minimal Application Package

Create:

`internal/app/app.go`

Expose:

`Name() string`

It must return:

`agent-sdlc`

Change the CLI entry point to use this package rather than hardcoding
the application name in `main.go`.

### Architecture

Terminal
    |
    v
cmd/agent-sdlc
    |
    v
internal/app

---

## Step 5 — Add Automated Test

Create:

`internal/app/app_test.go`

Test that:

`app.Name() == "agent-sdlc"`

### Verification

Run:

`go test ./...`

Expected result:

PASS

---

## Step 6 — Establish Local Quality Gate

The project must successfully run:

`go fmt ./...`
`go vet ./...`
`go test ./...`
`go build ./...`

Do not continue if any command fails.

Fix the failure and rerun the complete gate.

---

## Step 7 — Add Makefile

Create a `Makefile` supporting:

- `make fmt`
- `make vet`
- `make test`
- `make build`
- `make check`

`make check` must execute the complete local quality gate.

### Verification

Run:

`make check`

Expected result:

all checks pass.

---

## Step 8 — Add .gitignore

Create/update `.gitignore`.

Ignore at minimum:

- `bin/`
- `dist/`
- `.agent-sdlc/`
- `.env`
- `.DS_Store`
- `.idea/`
- `.vscode/`

Reserve `.agent-sdlc/` for future runtime state.

Do not implement runtime state in T000.

---

## Step 9 — Add GitHub Actions

Create:

`.github/workflows/ci.yml`

CI must run for:

- pull requests
- pushes to `main`

CI must perform:

1. checkout
2. Go setup using `go.mod`
3. formatting verification
4. `go vet ./...`
5. `go test ./...`
6. `go build ./...`

CI should reproduce the local quality gate as closely as practical.

---

## Step 10 — Final Local Verification

Run:

`make check`

Then run:

`go run ./cmd/agent-sdlc`

Expected CLI output:

`agent-sdlc`

Inspect:

`git status`

No generated binaries, temporary files, environment files, or runtime
state should be staged.

---

## Step 11 — Review

Perform a lightweight self-review before committing.

Check:

- T000 acceptance criteria are satisfied
- `main.go` contains no business logic
- unnecessary abstractions were not introduced
- no future T001+ functionality was implemented
- tests pass
- build passes
- vet passes
- formatting passes
- CI and local checks are consistent

Fix any problems and rerun:

`make check`

---

## Step 12 — Commit

Stage the T000 changes.

Commit using:

`task(T000): bootstrap Go project and CI`

Do not include unrelated changes.

---

## Step 13 — Push

Push:

`task/T000-bootstrap`

to `origin`.

---

## Step 14 — Open Pull Request

Create PR:

`[Task T000] Bootstrap Go project and CI`

Target:

`main`

Source:

`task/T000-bootstrap`

The PR description should contain:

- purpose
- major files added
- tests performed
- acceptance criteria
- known limitations, if any

---

## Step 15 — Validate CI

Wait for GitHub Actions.

If CI fails:

1. inspect failing job
2. identify root cause
3. make smallest appropriate fix
4. run `make check` locally
5. commit fix
6. push
7. wait for CI again

Maximum automatic remediation attempts:

3

If CI still fails after 3 attempts, stop and mark T000 blocked rather
than continuing indefinitely.

---

## Step 16 — Merge

Merge only when:

- local checks pass
- CI passes
- review passes
- acceptance criteria are satisfied

After merge:

1. update local `main`
2. verify T000 exists on `main`
3. mark T000 DONE
4. proceed to T001

---

# Expected Final Structure

agentic-sdlc/
├── .github/
│   └── workflows/
│       └── ci.yml
├── cmd/
│   └── agent-sdlc/
│       └── main.go
├── internal/
│   └── app/
│       ├── app.go
│       └── app_test.go
├── .gitignore
├── LICENSE
├── Makefile
└── go.mod

Documentation files may also exist and should not be removed.

---

# Definition of Done

T000 is DONE only when:

- [x] task branch was used
- [x] Go module exists
- [x] CLI executes successfully
- [x] CLI uses `internal/app`
- [x] automated test exists
- [x] `make check` passes
- [x] GitHub Actions workflow exists
- [x] PR was created
- [x] CI passes
- [x] changes were merged into `main`

---

# Explicitly Out of Scope

Do not implement:

- workflow state machine
- SQLite
- task DAG
- scheduler
- Git adapter
- GitHub adapter
- agent harness
- review agent
- Open Code Review
- worktrees
- parallel execution
- retry engine
- Docker services

Those belong to later tasks.