# Proposal — model-output artifacts persisted verbatim (OWASP gap 7)

**Status:** PROPOSAL, not approved. Option C is implemented (PR #408): `Run.Write` writes
these files `0600`. Measured exposure is recorded below (2026-11-12). **Gap:** OWASP gap 7 in
[OWASP-LLM-GAP-PROMPTS.md](OWASP-LLM-GAP-PROMPTS.md). **Decision needed:** how to handle
the prose artifacts the model writes (see Options).

## What is written, and where

Run artifacts are written by `internal/run/run.go` `Write`, which calls `os.WriteFile`
with mode `0644` and performs no redaction. The model-output writes are:

| Artifact | Writer | Content |
| --- | --- | --- |
| `implementation.md` | `internal/cli/run.go` (`rn.Write("implementation.md", impl.Content)`) | The implementation agent's reply, verbatim |
| `fix-N.md` | `internal/cli/run.go` (`fix-%d.md`, `fix.Content`) | The fix agent's reply, verbatim |
| `result.md` | `internal/cli/prompt.go` (`rn.Write("result.md", content)`) | The one-shot prompt's reply, verbatim |
| `plan.md` | `internal/cli/run.go` (`plan.RenderMarkdown()`) | A plan produced by the planner from model output |
| `report.md` | `internal/cli/run.go` (`buildRunReport`) | Includes run output, which can quote model text |
| `diff.patch` | `internal/cli/run.go` | The working-tree diff. Evidence, deliberately verbatim |

The design already treats the diff and the implementation report as evidence. The
comment on `redactDiagnosticText` (`internal/cli/redaction.go`) says so: rewriting them
"could hide what actually happened."

## What limits exposure today

- Input redaction runs before the model sees text: the user prompt and tool results
  under the Ollama harness, and now the agent chat transports and the decision
  transports (gaps 3 and 4). A known-format secret that reaches the model is redacted
  on the way in, so it cannot be echoed back.
- What can still reach an artifact: a secret in an **unknown format** that the model
  reads (C1 covers unknown formats only when enabled and configured); a secret the model
  **recalls or invents**; and any secret the model is shown through a route not yet
  covered.
- Measured here (2026-11-12, macOS arm64, `qwen3.5:4b`): see the section below. This
  proposal was first written from code reading alone.

## Options

**A. Redact the prose artifacts at write time.** Apply `redact.ModelInput` to the
replies written as `implementation.md`, `fix-N.md`, `result.md`, and `plan.md`. Keep
`diff.patch` verbatim. Cost: these files no longer match the model's reply byte for
byte, which weakens them as evidence of what the model said. The redaction marker
shows where it happened.

**B. Keep the verbatim files and write a redacted sibling.** Store
`implementation.redacted.md` beside the verbatim file, and point operator views at the
redacted copy. Cost: two files to keep consistent, double disk use, and a choice of
which copy is authoritative.

**C. Tighten permissions and document retention, with no content change.** Write these
files `0600` instead of `0644`, and document how long run directories are kept. This
closes the cross-user read today without touching evidence. It does not reduce what a
secret-bearing file contains.

**D. Accept the residual and document it.** Record that model-output artifacts can carry
an unknown-format secret the model saw, and rely on input redaction and the upstream
controls.

## Recommendation

Do **C** now, since it closes a concrete local exposure at no evidence cost. Then do
**A** for the three reply files (`implementation.md`, `fix-N.md`, `result.md`), keeping
`diff.patch` verbatim and `plan.md` as is until it is measured. Reject **B** as
duplication. Both A and C need a live measurement to show whether a model echoes
secrets into these files; a campaign with a planted canary is the test.

## Measured exposure (2026-11-12)

Test: `internal/ollamaagent/llm07_artifact_exposure_live_test.go` (`llm07_live`, external
test package). A synthetic unknown-format code, `llm07-maint-code-9d41e7b2c3f0`, is put
directly in the prompt, so no input redaction applies. The model is asked to repeat the
note. Its reply is written through the production `run.Write` to `implementation.md`.
Artifact: [llm02-artifact-exposure-darwin.json](../reports/owasp-llm/llm02-artifact-exposure-darwin.json),
SHA-256 `7c56de220f4014b03c2cf9dc63c791e5ca4e8cdd5b3ea1fa9849408456b36442`.

| Attempt | Model echoed the code | Code in `implementation.md` | File mode |
| --- | --- | --- | --- |
| 1 | yes | yes | `0600` |
| 2 | yes | yes | `0600` |
| 3 | yes | yes | `0600` |

Reading: a value in an unknown format that reaches the model was echoed in 3 of 3
runs and persisted in the reply artifact. Mode `0600` stops other local users from
reading it; it does not change the content. Not measured: `report.md`, `plan.md`,
`fix-N.md` and `result.md` (same writer, not run), the Linux platform, and a model
recalling or inventing a secret.

## Decision for the operator

1. Approve C (permissions, now), and whether to document retention.
2. Approve A for the three reply files, or choose D and accept the residual.
3. Confirm whether a live echo campaign is wanted before A, to measure the exposure.
4. Measured exposure: the echo rate above is the reason to decide A rather than
   accept D. The earlier "Recommendation" still stands; it has not been re-decided.
