> **Historical note:** this run predates the approval-safe fix. It resumed after
> `NEEDS_HUMAN` because that branch requeued without recording an approval
> request. The bypass is fixed; the approval-safe rerun and final candidate
> evidence are in `../sop-006-rust-approval/` and
> `../SOP-RELIABILITY-REVIEW-REPORT.md`. The bounded Rust success itself remains
> valid; this file is preserved as historical evidence.

# SOP-006 — Disposable Rust Workflow Report

## Candidate binary

- Path: `/Users/imhttran/agentic-workspace/agentic-sop/.run/candidate/sop`
- SHA-256: `e2cd0917e0bf65d3d8c2ce2d0bd807a78d1c461808de44df66ce19116d66739b`
- Module: `github.com/imhttran/agentic-sop`
- VCS revision: `e99cde03aff969b458e6d23579d6b3bb3a6fbe56`
- VCS modified: `true`
- Installed binary was not replaced.

The candidate binary recorded its own provenance in each task's
`.agent-sdlc/runs/<task>/counts.json`.

## Policy probe (Cargo acceptance permission off)

Command:

```bash
SOP_AGENT_COMMAND='python3 .../agent.py' .../.run/candidate/sop run PLAN.md
```

Observed:

```text
Running: S001 Implement greet
run: setup blocker (command-policy): cargo test --quiet: not authorized on the acceptance path: acceptance check "cargo test --quiet" is not authorized (REQUIRES_APPROVAL)
setup blocker: resolve the declared prerequisite before the model is invoked
```

Then `sop status` showed `S001 READY`, `S002 PLANNED`, no `.agent-sdlc/runs`
directory existed, and the command-agent marker was absent. No model call, run,
or continuation-budget increment occurred.

## First workflow invocation (controlled acceptance failure)

`cargo.acceptance_check` was then enabled while `cargo.agent_tool` stayed off.

Observed:

```text
Running: S001 Implement greet
outcome: PARTIAL
  - required criterion criterion-1 not met: mapped check check-1 failed:
    acceptance check "cargo test --quiet" failed (exit 101)
  - required criterion criterion-2 not met: mapped check check-1 failed
S001 NEEDS_HUMAN (requeued)
```

S002 was not executed: its run directory/count artifact was not created during
this invocation. The failure was a controlled failing Rust test (`greet`
returned `hi` instead of `hello`).

## Supported recovery invocation

A second `sop run PLAN.md` used the same task and budgets; no state database was
edited and no budget was reset. The command agent wrote the fix.

Observed:

```text
S001 LOCAL_DONE
outcome: VERIFIED
  - every required criterion was independently met
Running: S002 Implement farewell
S002 LOCAL_DONE
outcome: VERIFIED
  - every required criterion was independently met
all tasks done (2 completed this run)
SOP COMPLETE
Tasks: 2/2 complete
Final gate: PASS
```

Final `sop status`:

```text
S001 LOCAL_DONE Implement greet
S002 LOCAL_DONE Implement farewell
```

`sop approvals --json` is empty.

## Counts evidence

S001 (two invocations: failed acceptance + recovery):

```json
{"approvals":0,"attempts":2,"continuations":0,"direct_interventions":0,"invocations":2,"model_calls":9}
```

S002 (one invocation: only after S001 advanced):

```json
{"approvals":0,"attempts":1,"continuations":0,"direct_interventions":0,"invocations":1,"model_calls":6}
```

The difference proves the dependent stage did not execute while S001 was unmet.

## Preserved artifacts

- `.agent-sdlc/runs/S001/report.md` — final VERIFIED outcome, acceptance check
  argv/workdir/exit/output.
- `.agent-sdlc/runs/S002/report.md` — dependent stage VERIFIED after S001.
- `.agent-sdlc/runs/S00{1,2}/counts.json` — lifecycle counts and candidate
  binary provenance.
- `.agent-sdlc/runs/S00{1,2}/outcome.json`, `report.json`, `trace.json`,
  `validation.json`.
- `/tmp/sop006-agent-requests.log` — exact scripted-agent requests.
- `src/lib.rs`, `Cargo.toml` — implemented Rust fixture.
