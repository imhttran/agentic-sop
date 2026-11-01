# PE-3 live acceptance — round 2 (corrected fixture, complete)

Second live planner-continuation acceptance round for `plan-executore-integration` PE-3,
run on a corrected isolated fixture after round 1 was blocked by a non-executable
acceptance command (see `../pe3-acceptance/README.md`).

Produced `2026-10-26` against merged `main` at `7f11871d` (PR #130).

## Environment

- Planner binary built from `7f11871d` (includes the PE-3 decision-context fields).
- Transport: OpenAI Secure MCP Tunnel, tunnel id `tunnel_6ad00777bf3c8191a88091a07a0617b3`,
  stdio command `.run/tunnel/sop-mcp-planner.sh`, seven-tool planner surface.
- Fixture: an isolated disposable SOP project outside the checkout, with its own git
  repository and its own question store. Paths below are written as `<FIXTURE>`.
- Plan `plan@fb1fff09d4097054c54b39af6bd18885b3de9678a5cf591bd0a0109890fc0c94`,
  task `PA-1`.

## Corrected design, and why round 1 failed

Round 1 declared `test -f accepted.txt` as the plan acceptance check.
`internal/commandpolicy` classifies only `go` (`test`/`build`/`vet`/`fmt`/`list`),
read-only `git`, and permissioned `cargo` as `SAFE`; everything else is
`REQUIRES_APPROVAL`, and `internal/cli/preflight.go` refuses to invoke the model on a
non-`SAFE` command with no approval escape and no allowlist key.

This round uses `go test ./...`, backed by an operator-owned Go test
(`acceptance_test.go`) that asserts `accepted.txt` contains exactly `accepted`. The new
design was **proven in a disposable probe before any question was raised**: preflight
passed with no setup blocker, the model implemented, validation and the check passed, and
the probe task reached `LOCAL_DONE` / `VERIFIED`. Only then was the acceptance fixture
created and a question raised.

## Binding — as actually recorded

| Field | Value |
| --- | --- |
| task | `PA-1`, attempt `0` |
| question | `q-1`, revision `1` |
| plan revision | `plan@fb1fff09d4097054c54b39af6bd18885b3de9678a5cf591bd0a0109890fc0c94` |
| return stage | `IMPLEMENTING` |
| **question run-epoch binding** | **empty (none)** |
| live run epoch | `2026-10-26T16:39:10.077218Z` |

### Limitation: the question carries no run-epoch binding

`runPlannerAsk` assigns `RunEpoch: currentRunEpoch(dir, taskID)` **before** calling
`runpkg.SetStageOnExisting`, which is what creates the run's `state.json`. On a task with
no prior run, the question therefore binds an empty epoch, and the run epoch exists only
afterwards.

Confirmed empirically: no `.agent-sdlc/runs/PA-1/` existed before `planner ask`, the
question recorded `run_epoch: null`, and the run state appeared immediately after with
`created_at = 2026-10-26T16:39:10.077218Z`.

**This round did not verify epoch-bound resume.** The continuation identity is
`cont:q-1:` with an empty epoch segment; the resume was permitted because the stale-binding
rule only fires when both sides are non-empty. Recorded here as a **PE-4 hardening item**:
a newly raised question should bind the run epoch it is created in, and the read surface
should expose the absence rather than showing only the live epoch in `sop_task_result`.

## Sequence

1. **Retrieval over the connector.** The external client read `sop_task_result` and
   `sop_decision_list` for `PA-1` and confirmed the binding plus the PE-3 decision-context
   fields (`actions`, `question_body`, `question_options`, `question_return_stage`) with no
   `question_stale` line.
2. **Submission.** `a-2` was recorded against `q-1`: action `continue`, body `accepted`,
   outcome `o-3 ACCEPTED`.
3. **Consumption.** `sop planner continue --apply` → `PA-1 APPLIED q-1 (continuation
   pending execution)`, outcome `o-4 APPLIED`, continuation `cont:q-1:` `PENDING` at
   `return_stage IMPLEMENTING`.
4. **Durable resume.** `sop run` in the fixture printed
   `PA-1 planner: resumed from IMPLEMENTING and completed (generation 1)`. The run resumed
   the **same** run epoch (`created_at` unchanged, `16:39:10.077218Z`), it did not restart.

## Verified outcome

| Condition | Result |
| --- | --- |
| `accepted.txt` content | `accepted` (9 bytes: `accepted\n`) |
| `go test ./...` | `ok pe3probe` |
| continuation | `cont:q-1:` **`COMPLETED`**, generation 1, attempts 1 |
| continuation `completed_at` | `2026-10-26T16:42:30.625525Z` |
| task | `PA-1` `LOCAL_DONE` |
| run stage | `PASSED` |
| outcome | `VERIFIED`, validation passed |
| history | `q-1` `APPLIED`; `a-2`, `o-3`, `o-4` unchanged |

`answered_by: "chatgpt"` on `a-2` is caller-supplied metadata (SPIKE-006), not
cryptographic provenance. The operator reported the submission came from the external
client through the connector.

## Files

| File | Contents |
| --- | --- |
| `questions.json` | `q-1`, `a-2`, outcomes, and the completed continuation (path-sanitized) |
| `plan.meta.json` | the plan revision every decision was bound to |
| `run-state.json` | run stage `PASSED`, original `created_at` preserved |
| `PLAN.md` | the fixture plan with the corrected `go test ./...` acceptance check |
| `acceptance_test.go` | the operator-owned check asserting the exact marker |
| `accepted.txt` | the produced deliverable |
| `outcome.json`, `validation.json`, `gate.json`, `report.md` | the run's verdict and evidence |
| `manifest.sha256` | SHA-256 of each file above |

Sanitized: no credentials, no local paths.
