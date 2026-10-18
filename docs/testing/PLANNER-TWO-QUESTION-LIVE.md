# Planner two-question live demo with a real executor

**Type:** Operator runbook (proposed; not yet run).
**Closes:** the evidence gap in
[SPIKE-006 §9](../plans/SPIKE-006-feasibility-report.md): a model-driven executor across
two planner questions in one task and run epoch, with SOP delivering the first decision to
the final round.

The earlier demos left two gaps. `planner-2q` ran two questions with a **scripted**
executor that kept the first decision in its own scratch file. `planner-chatgpt`
([findings](../plans/planner-live-test-findings.md)) used a **real** executor for one
question only. This run combines them: a real executor, two questions, and no scratch
state.

## What the run must show

1. The executor raises the ordering question, the run parks, ChatGPT answers, and SOP
   applies it.
2. The resumed executor raises the duplicates question. **Neither deliverable exists yet**,
   so the executor has stored the first decision nowhere.
3. ChatGPT answers; the final resume completes the task in the **same run epoch**, and
   `choices.txt` records **both** answers.

Because the executor keeps no state between rounds and wrote no file before step 3, the
first decision can reach the final round only through SOP's input. That is the
decision-history fix
([imhttran/agentic-sop#68](https://github.com/imhttran/agentic-sop/pull/68)).

## Prerequisites

- Ollama running locally with access to the executor model (default
  `deepseek-v4.1-flash:cloud`; override with `PLANNER2Q_MODEL`).
- `go` and `git` on `PATH`.
- The ChatGPT Secure MCP Tunnel set up as in
  [SPIKE-006 §1](../plans/SPIKE-006-feasibility-report.md#1-client-transport-and-setup).
- A dedicated SOP binary that **includes #68**. Build it from `main` once #68 is merged, or
  from the `claude/planner-decision-history` branch before then:

  ```bash
  go build -o "$TMPDIR/sop-2q" ./cmd/sop
  ```

  Do not use the installed `sop` unless it was built with #68.

## Setup

```bash
scripts/planner-2q-fixture-setup.sh "$TMPDIR/planner-2q-live" "$TMPDIR/sop-2q"
```

The script creates a disposable project under `$TMPDIR` (never inside this checkout) with:

- planner enabled (`round_limit: 3`, no deadline), the `ollama` tool harness, and the
  human commit gate on;
- `input.txt` with repeated, unsorted lines;
- one task, `T001`, whose ordering and duplicate handling must be clarified with the
  planner, one question at a time;
- an operator-owned acceptance test (`go test ./...`) that accepts any of the four answer
  combinations but checks `normalized.txt` against the choices in `choices.txt`.

It refuses an existing root unless `PLANNER2Q_FORCE_RECREATE=1`, and it checks that the
acceptance test fails before the task runs.

Point the tunnel's MCP command at the new binary and fixture: the wrapper must run
`"$TMPDIR/sop-2q" mcp --planner` with its working directory set to the fixture root. Keep
all logs **outside** the fixture root; a log inside it enters the task's change set (this
broke the earlier `planner-clarify` attempt).

## Run

Run every `sop` command from the fixture root with the dedicated binary. Capture output to
a log directory outside the fixture, for example `$TMPDIR/planner-2q-logs/`.

### Round 1: ordering

```bash
"$TMPDIR/sop-2q" run PLAN.md
```

Expected: `T001 PAUSED: WAITING_FOR_PLANNER`. Record:

- `"$TMPDIR/sop-2q" question list` shows one `PENDING` question about **ordering**.
- `ls choices.txt normalized.txt` fails: neither file exists.
- The question's run epoch (`"$TMPDIR/sop-2q" question <id>`).

In ChatGPT, have the planner read the question and submit an answer with an idempotency key
and the expected revision. Record the answer exactly.

### Round 2: duplicates

```bash
"$TMPDIR/sop-2q" run PLAN.md
```

Expected: SOP applies the first answer, resumes the same run, and parks again. Record:

- The first question is `APPLIED`; a second `PENDING` question asks about **duplicates**.
- **`ls choices.txt normalized.txt` still fails.** This is the key check: the executor has
  not written the first decision anywhere.
- `git status --short` in the fixture shows no new files besides SOP's own state.
- The run epoch is unchanged.

Answer the second question in ChatGPT and record the answer exactly.

### Round 3: completion

```bash
"$TMPDIR/sop-2q" run PLAN.md
```

Expected: the task completes. Record:

- `T001` is `LOCAL_DONE`, the run stage is `PASSED`, and the run epoch is unchanged.
- `choices.txt` matches **both** ChatGPT answers (`sort=...`, then `dedupe=...`).
- `.agent-sdlc/runs/T001/outcome.json` reports `status: "VERIFIED"` with the
  `go test ./...` acceptance check met.
- In `.agent-sdlc/questions.json`, the first question's continuation is `SUPERSEDED` and
  the second's is `COMPLETED`; none is `CLAIMED` or `PENDING`.
- `.agent-sdlc/runs/T001/implementation.md`, for any mention of the ordering decision.
  The run trace does not record prompt text, so this is supporting evidence only.

## Outcomes

| Result                                                                     | Classification                                                                                                                 |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| All round 3 checks pass, and no deliverable existed before round 3         | **GO** for the two-question loop with a real executor. Update SPIKE-006 §0, §8.1, and §9.                                      |
| `choices.txt` has the wrong ordering value, or the executor re-asks it     | **Fail**: the first decision did not reach the final round. Check the binary includes #68.                                     |
| The executor asks both questions at once, or returns `needs_human`         | **Fixture or model result, not an SOP failure.** Record it; adjust the task wording and rerun.                                 |
| A deliverable exists before round 3                                        | **Inconclusive**: the executor may have stored the first decision itself. Record it; the run does not prove decision delivery. |
| The task fails review or validation for reasons unrelated to the decisions | Record and diagnose as an ordinary run failure.                                                                                |

Record the result as a dated section in
[`planner-live-test-findings.md`](../plans/planner-live-test-findings.md) with the binary
fingerprint (`shasum -a 256 "$TMPDIR/sop-2q"`), the source commit, the exact questions and
answers, and the checks above. Keep the transcript sanitized: no credentials or environment
dumps.

## Cleanup

The fixture is disposable. Remove `$TMPDIR/planner-2q-live` once its evidence is recorded,
and point the tunnel wrapper back at its previous working directory.
