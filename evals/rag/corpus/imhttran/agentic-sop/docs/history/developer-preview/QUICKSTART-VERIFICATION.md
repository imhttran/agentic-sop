# Quickstart verification

**Task:** PREVIEW-002 (Install and quickstart verification)
**Provenance:** operator-executed. The governed tool harness denies `./install.sh`
(`REQUIRES_APPROVAL`), so the governed PREVIEW-002 run cannot execute the install or the
quickstart; it recorded the block and left its deliverable unfilled
(`.agent-sdlc/runs/PREVIEW-002/`). The results below were produced by running the exact
commands on the operator side and are recorded verbatim. No step is fabricated.

**Environment**

- Date (UTC): 2026-10-09
- OS: macOS (darwin), arm64
- `go version go1.27.1 darwin/arm64`
- Checkout: `sop-preview-001` (branch `preview-001`), revision `d9f7722`
- Installed binary: `/Users/imhttran/go/bin/sop` (on `PATH`)

## Install

```sh
./install.sh
```

Observed (exit 0):

```text
SOP installer
  repo:     /Users/imhttran/agentic-workspace/agentic-sop/sop-preview-001
  bin dir:  /Users/imhttran/go/bin
  skills:   none
  plugin:   none

Installing the sop CLI:
  from: /Users/imhttran/agentic-workspace/agentic-sop/sop-preview-001
  to:   /Users/imhttran/go/bin/sop
  installed: /Users/imhttran/go/bin/sop

  /Users/imhttran/go/bin is on PATH.

Done.
```

Result: **PASS** — the CLI built from the checkout and was installed to a user-writable
bin directory. (It also replaced a stale installed binary; see
[KNOWN-ISSUES.md](KNOWN-ISSUES.md).)

## Version

```sh
sop version
```

Observed:

```text
sop dev
```

Result: **PASS** — the installed binary reports a version (`sop dev`; the development
build stamp). Every later command below resolved to this installed binary.

## Disposable project

A throwaway project was created outside the repository:

```sh
mkdir -p /tmp/sop-qs-001
cd /tmp/sop-qs-001
git init -q .
printf '# T001 --- Add a greeting file\n\n## Objective\n\nCreate a file named greeting.txt containing the single line: hello.\n' > TASK.md
git add TASK.md && git commit -qm init
```

### `sop init`

```sh
sop init
```

Observed:

```text
initialized .agent-sdlc/state.db
wrote .agent-sdlc/config.yaml
```

Result: **PASS** — project state and default configuration were created.

### `sop status`

```sh
sop status
```

Observed: (no output)

Result: **PASS (expected empty)** — no persisted tasks exist before a plan is compiled
or a task is run; the command exits cleanly.

### `sop run --task`

```sh
sop run --task TASK.md
```

Observed (exit 0):

```text
Project: sop-qs-001
Harness: tool
Provider: ollama
Model: deepseek-v4.1-flash:cloud
Provider source: configuration

[00:00] START      Add a greeting file
[00:00] PLAN       generating plan
[00:00] DISCOVER   listing
[00:00] DISCOVER   reading TASK.md
[00:00] DISCOVER   listing .agent-sdlc
[00:01] DISCOVER   inspecting git status
[00:04] IMPLEMENT  implementing
[00:04] DISCOVER   listing .
[00:04] CHANGE     creating greeting.txt
[00:05] DISCOVER   reading greeting.txt
[00:05] VALIDATE   true
[00:05] VALIDATE   true
[00:05] VALIDATE   true
[00:05] REVIEW     reviewing changes
[00:05] QUALITY    PASS
[00:06] COMPLETE   PASS
run T001: PASS
  - all required checks passed
fix cycles: 0/1
performance: 6.0s total (agent 5.5s, validation 13ms, review 443ms) | agent calls 2, validation runs 1, fix cycles 0
report: .agent-sdlc/runs/T001/report.md
human approval required before commit; completed locally without committing.
```

Result: **PASS** — a bounded, single-task governed lifecycle (START → PLAN → IMPLEMENT →
VALIDATE → REVIEW → QUALITY → COMPLETE) ran to `run T001: PASS` with gate `PASS`,
created `greeting.txt` containing `hello`, and stopped at the commit human boundary
without committing (as designed).

## Summary of observed results

| Step      | Command                  | Result                      |
| --------- | ------------------------ | --------------------------- |
| Install   | `./install.sh`           | **PASS** (exit 0)           |
| Version   | `sop version`            | **PASS** (`sop dev`)        |
| Init      | `sop init`               | **PASS**                    |
| Status    | `sop status`             | **PASS** (empty, expected)  |
| First run | `sop run --task TASK.md` | **PASS** (`run T001: PASS`) |

No step is UNAVAILABLE: every command above was executed and its result recorded.

## Notes for the preview

- The model used for the first run was the project's configured
  `deepseek-v4.1-flash:cloud` (provider `ollama`). This exercises the real agent harness
  end to end, unlike the deterministic fixtures used by the ETOE reliability series.
- `sop run --task` is single-task mode and writes **no** persisted task status; the
  persisted-completion path is the plan/graph runner (`sop run PLAN.md`). See
  [KNOWN-ISSUES.md](KNOWN-ISSUES.md).
