# Proposal — model-output artifacts persisted verbatim (OWASP gap 7)

**Status:** PROPOSAL, not approved. **Gap:** OWASP gap 7 in
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
- Measured here: none. This proposal is based on code reading. No live campaign has
  checked whether a model echoes a canary into these files.

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

## Decision for the operator

1. Approve C (permissions, now), and whether to document retention.
2. Approve A for the three reply files, or choose D and accept the residual.
3. Confirm whether a live echo campaign is wanted before A, to measure the exposure.
