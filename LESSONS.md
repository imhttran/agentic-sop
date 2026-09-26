# Lessons

Engineering lessons from building SOP V1. These are working notes, not rules;
follow them unless a task gives a concrete reason not to.

## Keep the control plane deterministic

The most valuable decision was drawing a hard line between reasoning and control.
State transitions, scheduling, retry bounds, merge eligibility, and CI-failure
classification are all deterministic and model-free. Models only produce content
(tests, implementation, review findings, plans). Every time a question like "is
this failure actionable?" came up, answering it in code instead of asking a model
made the system testable and predictable.

## Ports with fakes, adapters at the edge

Everything that touches the network, a model, Git, GitHub, or a shell sits behind
a narrow interface. Tests use fakes; the real adapters live at the composition
root (the CLI) or in small packages. This kept the whole suite green without
network access and made every feature unit-testable. It also forced call sites to
be explicit about what they actually need from a dependency.

## Trusted commands, structured arguments

Two different boundaries need two different treatments:

- configured verification/agent commands are trusted configuration and run via
  `sh -c` (like a Makefile);
- Git and GitHub operations use structured argument lists, never a shell.

Mixing these up is how accidental injection happens. Keep them separate and
document which is which.

## The staged-copy pattern for persistence

Mutating a domain object and then saving can leave in-memory state that was never
persisted if the save fails. The pattern that worked everywhere: copy the object,
mutate the copy, save it, and only then publish the change back. It makes
atomicity a property of the shape of the code rather than something to remember.

## Dependencies are complete only when merged

Treating local test pass, review pass, or CI pass as "complete" is wrong: that
work may still live on an unmerged branch. A dependency is satisfied only at
`MERGED`/`DONE`. This one rule prevents a whole class of ordering bugs.

## Recovery is a first-class feature

Processes get interrupted between a side effect and its state write. Resume must
reconcile persisted state with the resources that actually exist and reuse (never
duplicate) a branch or PR. Missing required resources should be a loud
consistency error, not a silent re-create.

## Optional features must never gate core work

Compression, external review, and parallelism are optimizations. A failure in any
of them must not turn completed work into `BLOCKED` or block a merge. Model them
as adapters with a no-op fallback, record the failure as metadata, and keep going.

## Small, inspectable commits and specs

One branch and one reviewable commit per task, with a short spec stating
objective, scope, rules, tests, and acceptance criteria, kept the history
readable and the diffs honest. Specifications that grew beyond their task were the
clearest signal that a change needed to be split.

## Test the boundaries, not the framework

The highest-value tests were at the seams: state transitions, selection logic,
recovery decisions, budget thresholds, and adapter argument construction. Golden
tests of framework behavior added noise; boundary tests caught real bugs.

## Decline defensive noise deliberately

Automated review reliably suggests nil-guards, error wrapping, and speculative
generalization. Some of it is worth taking; most of it is not. Decide once, state
the rationale, and stay consistent — otherwise the codebase fills with checks that
never fire and options nobody uses.

## Configuration is policy, not state (and never secrets)

A single `.agent-sdlc/config.yaml` describes policy — providers, validation
commands, review engine, quality thresholds, human gates — and nothing mutable.
Strict decoding rejects unknown keys, so a secret cannot be committed by accident
and the schema stays honest about what it supports. Defaults are applied on load,
and an invalid file fails loudly instead of silently falling back. When a field's
zero value is also meaningful (an explicit `false`), use a pointer so "omitted"
and "false" stay distinguishable.

The environment overrides configuration where it matters, so operators can
override a committed default without editing it.

## One rule decides what blocks

Review and the quality gate must agree on what stops a run. Exporting a single
`BlockingFindings(fail_on, findings)` and calling it from both kept them from
drifting: the model produces findings, but the same deterministic rule classifies
severity in exactly one place.

## Runs are files, not a database row

Persisting a run as a directory of artifacts (`task.md`, `plan.md`, `diff.patch`,
`validation.json`, `review.json`, `report.md`, `state.json`) means a terminated
run is still readable without the tool. Durable control state belongs in SQLite;
the human-inspectable record belongs on disk.

## A document that is a copy should be recognized as one

`PLAN-wrapup.md` was byte-identical to `PLAN-JEV.md`. Diffing before implementing
saved building the same thing twice and clarified that the two names described
one roadmap.
