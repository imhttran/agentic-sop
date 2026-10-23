# HIP-005 — Pilot task selection and candidate revision

Recorded: 2026-10-23T16:00:00Z, before any candidate execution.

This selection is fixed for the HIP-006 pilot. The comparison harness may not
change the tasks, acceptance commands, or candidate after a run begins.

## 1. Selected tasks

All three task definitions come from the completed Promptfoo plan
(`docs/plans/SOP-Promptfoo-Implementation-Prompt.md`) and its preserved run
artifacts. They are small enough to replay in a disposable worktree, have
deterministic acceptance commands, and require no external write.

| Task | Pre-change commit | Implementation commit | Trusted acceptance command | Why selected |
| --- | --- | --- | --- | --- |
| S1 — Inspect repository and choose the smallest Promptfoo integration | `962a90cd` | `6da31939` | `go build ./... && go test ./internal/security ./internal/config -count=1` | Docs-first inspection with a concrete build/security check; no external writes. |
| S2 — Build the isolated evaluation target | `962a90cd` | `6da31939` | `go test ./internal/evaltarget -run TestAISEC -count=1` | Deterministic isolated-target behavior; the test must execute at least one case. |
| S3 — Implement the initial six-case behavioral suite | `962a90cd` | `6da31939` | `go test ./internal/evaltarget -run TestAISEC -count=1` | Six explicit assertions with a controlled failure path; no network or external writes. |

The task files are the archived `runs/S1/task.md`, `runs/S2/task.md`, and
`runs/S3/task.md` under `.sop-evidence/promptfoo-pr-113/`. Their content is copied
into the harness inputs, not read from the reference implementation patches. The
pre-change commit is the plan-only commit before the implementation commit; both
arms start there.

Reference solutions, implementation patches, and the merged PR diff are excluded
from both arm worktrees. Acceptance commands are run by the harness from the
manifest, not by the experimental agent.

## 2. Candidate instruction revision

Exactly one candidate revision is tested, and it is fixed for all three task
pairs.

- Allowed candidate path: `internal/ollamaagent/prompt.go`
- Baseline SHA-256: `cabbce12e75ba3cafbd3021bebad724d18a47dffd844b5df1dc75639729460c5`
- Candidate SHA-256: `00c4d2cf99b12f994630d190451bf522e356f8cc2f0d43b16c965528ab8f2dfe`
- Benchmark manifest hash: computed by `DeriveFingerprint` from the task inputs,
  starting commit `962a90cd`, these two instruction hashes, the executor/model
  settings, tool permissions, and the fixed verification commands.

Candidate diff (appended to the baseline prompt file):

```diff
+Before the first repository write, state the exact file paths and the smallest change; do not spend more than two discovery tool calls before the first write.
```

The baseline instruction set declares no file edits. The candidate instruction
set declares only the file above. The harness rejects any candidate edit outside
that path and records the rejection.

## 3. Fixed limits

- Three task pairs; six executions maximum for the initial comparison.
- One held-out pair only if the initial result meets the promotion criteria; eight
  executions maximum in total.
- Per-run time limit: 900 seconds (existing fixed pilot limit).
- Per-run token limit: 200000. Token accounting is recorded as UNAVAILABLE when
  the executor cannot report it; it is never replaced with zero.
- No automatic retries. Failed or timed-out runs are recorded as such.

## 4. Credential and executor availability probe

This probe launched no model call:

```text
/Users/imhttran/go/bin/sop present
ollama healthy (19 models)
configured model: deepseek-v4.1-flash:cloud
```

The executor command is available as `/Users/imhttran/go/bin/sop`; the model is
configured through the existing SOP configuration. No credential values were
read into the manifest, and the harness passes only a scrubbed environment to the
arm executor. The probe records availability only; it is not a pilot result.

## 5. Selection guard

- No candidate execution occurred before this record.
- No existing pilot implementation covered this work; the harness is the first
  implementation.
- The selected task inputs and acceptance commands are frozen for the comparison.
- A changed task input, commit, instruction hash, executor/model setting, tool
  permission, or verification command invalidates the resume fingerprint.
