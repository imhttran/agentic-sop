# PE-3 live acceptance — round 1 (partial)

Evidence from the first live planner-continuation acceptance attempt for
`plan-executore-integration` PE-3. **The round did not complete**: retrieval and
submission were proven live, durable resume was blocked by a fixture defect, and the
blocker is recorded here rather than worked around.

Produced `2026-10-26` against merged `main` at `7f11871d` (PR #130).

## Environment

- Planner binary: built from `7f11871d` (includes the PE-3 decision-context fields).
- Transport: OpenAI Secure MCP Tunnel, tunnel id `tunnel_6ad00777bf3c8191a88091a07a0617b3`,
  stdio command `.run/tunnel/sop-mcp-planner.sh`, seven-tool planner surface.
- Fixture: an isolated disposable SOP project outside the checkout, with its own git
  repository and its own question store, so the checkout's completed PE-2 records were
  never touched. Paths below are written as `<FIXTURE>`.
- Plan: `plan@cc6002dcedc09c9e8e6e06357d70f3403a8a565c512862d1d267ab9f582e158f`.
- Task: `PA-1`, run epoch `2026-10-26T16:14:47.062264Z`.

## Files

| File | Contents |
| --- | --- |
| `questions.json` | the fixture's question store: `q-1`, `a-2`, outcomes `o-3`/`o-4`, continuation `cont:q-1:` |
| `run-state.json` | `PA-1` run state at the point the round stopped |
| `plan.meta.json` | the plan revision every decision was bound to |
| `PLAN.md` | the fixture plan, including the acceptance check that caused the blocker |
| `manifest.sha256` | SHA-256 of each file above |

## Proven live

1. **Decision context retrieval over the connector.** The external client read
   `sop_task_result` and `sop_decision_list` for `PA-1` and reported that every binding and
   every new decision-context field matched, with no `question_stale` line:
   `actions`, `question_body`, `question_options`, `question_return_stage`, plus
   `state=PENDING revision=1 attempt=0 plan_revision=plan@cc6002dc…`.
2. **Bound decision submission and executor consumption.** `a-2` was recorded against
   `q-1` (`action: continue`, `body: accepted`), outcome `o-3 ACCEPTED`, then consumed by
   `sop planner continue --apply` with outcome `o-4 APPLIED`, leaving the durable
   continuation `cont:q-1:` `PENDING` with `return_stage: IMPLEMENTING`.

`answered_by: "chatgpt"` on `a-2` is caller-supplied metadata, per SPIKE-006. It is not
cryptographic provenance; the operator reported the submission came from the external
client through the connector.

## Blocked: durable resume

`sop run` in the fixture stopped at preflight, before any model call and before
`plannerResume` could claim the continuation:

```
run: setup blocker (command-policy): test -f accepted.txt: not authorized on the
acceptance path: acceptance check "test -f accepted.txt" is not authorized
(REQUIRES_APPROVAL)
setup blocker: resolve the declared prerequisite before the model is invoked
```

**Cause.** The fixture plan declared `### Acceptance Checks` as `test -f accepted.txt`.
`internal/commandpolicy` classifies only `go` (`test`, `build`, `vet`, `fmt`, `list`),
read-only `git`, and permissioned `cargo` as `SAFE`; everything else is
`REQUIRES_APPROVAL`, and `internal/cli/preflight.go` refuses to invoke the model on a
command that is not `SAFE`. There is no approval escape and no configuration key that
disables or allowlists a plan acceptance check. The refusal is correct behaviour; the
fixture was wrong.

**Why it cannot be repaired in place.** Rewriting the acceptance check changes `PLAN.md`,
which changes the plan source fingerprint and therefore the live plan revision. Every
decision is bound to the plan revision it was recorded against, so the recorded decision
would immediately be refused as stale — which is exactly the PE-3 requirement that a plan
revision invalidates the decisions bound to it. A corrected fixture therefore needs a
**fresh** planner decision; the existing `a-2` cannot be replayed against it.

The continuation `cont:q-1:` is left `PENDING` in the fixture. Nothing was marked complete,
no task was transitioned, and no implementation was replayed.

## What a corrected round needs

1. A fixture plan whose acceptance checks are `SAFE` — for example `go test ./...` backed by
   a Go test that asserts the marker file, instead of `test -f`.
2. A fresh question on the corrected plan and one external submission bound to the new
   revision and epoch.

## Unmet criteria

- PE-3: *The workflow resumes from durable state after a new ChatGPT interaction.*
  Not demonstrated live. The mechanism is unchanged and covered by the planner integration
  suite, but this round did not claim the continuation.

All other PE-3 criteria were satisfied before this round: decision-context retrieval,
bound submission and consumption, refusal of a superseded plan revision (focused test),
and the documented interactive-only limitation (`docs/specs/EXECUTION.md` §12).
