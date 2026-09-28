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

## Persisted state must describe what actually happened

A local-only run never opens a PR or executes CI, so it must not advance a task
through `PR_OPEN`/`CI_RUNNING`/`CI_PASS`/`MERGED` as if it had. Synthesizing those
states would make the state store lie to every reader — a dashboard, a resume, or
a dependency check. The fix was a distinct terminal state (`LOCAL_DONE`) for local
completion, with the remote path left intact for when a real PR exists. The same
principle governs plan preparation: a stale machine plan is reconciled explicitly
(or stopped with `NEEDS_HUMAN`), never silently rebuilt over existing history.

## A retry budget measures progress, not attempts

Bounding a `needs_human` requeue by `max_attempts` stopped an infinite loop but
still counted retries that changed nothing — a harness that kept asking for the
same authorization burned the budget and blocked a task no human had even been
given the chance to fix. The budget should measure progress: a retry that
reproduces the previous outcome spends no attempt and leaves the task runnable,
while a retry that moves the outcome forward spends one. That required comparing
the outcome (its decisive reasons) against the previous attempt, persisted beside
the run so it survives a restart — a durable signature, not a timestamp.

## Recovery reuses the reason it already recorded

When a task stopped at a human boundary, the cheapest way to help the retry was
to hand it the outcome SOP had already persisted — decision and reasons — rather
than re-derive intent from run logs or prose. The same recorded signature that
detects a no-progress repeat doubles as the retry's context, so budgeting retries
and explaining them stay one artifact. Recovery also has to be one command: making
the operator requeue blocked tasks one id at a time turns a bounded step into a
manual chore, which is why `sop retry --all` exists.

## Spend the agent only where intelligence is required

The command agent is the most expensive component, so the workflow should not
reach for it by default. A verification-only task was paying for an implementation
agent before SOP ran its own validation, on work that needed no repository change.
Making the deterministic validation _first_ for a task that opts into it (through
explicit plan metadata, never by guessing from the title) turns that task into a
validation pass, and the agent is invoked only when the validation actually fails
-- handed the failure instead of rediscovering it. The optimisation is safe because
it reorders work, it does not skip a gate: the task still passes only when its
configured checks pass.

## A budget is not a completion path

A bounded loop can still have no way to finish. IMPLEMENT was capped at 24 turns,
but a productive model read and searched for all of them and then failed with
`iteration_limit` -- SOP validation never ran, and raising the number would only
have deferred the same ending. The cap was doing the job of a _safety bound_, not a
_completion mechanism_. Giving the capability explicit phases fixed it: a few
turns of discovery, a soft nudge to start implementing, a hard transition that
withdraws the repository tools before the ceiling, then a small finalization
allowance in which the model must return the structured outcome. The agent does not
own validation -- SOP runs the deterministic gates after it returns -- so a
capability needs a way to _hand back control_, and a ceiling alone never provides
one.

## A phase transition needs a precondition, not just a counter

The first IMPLEMENT phases had the same failure in a new shape. A model gathered
enough context to know exactly what to change, crossed the finalize threshold with
no repository change yet, and was forced to finalize -- so it returned "no
repository changes were made" about work it had never been allowed to do. The
threshold was being treated as _sufficient reason_ to finish when it was only a
_maximum_. The fix was to give the transition a precondition: finalization also
requires an observed mutation. Below it the tools stay enabled and the model is
told to implement; a no-mutation run ends truthfully (`mutation_observed=false`)
rather than claiming success. When a counter drives a one-way transition, ask what
else must be true -- a count is a bound, not a decision.

The precondition also has to be _current_, not merely ever-true. Requiring _a_
mutation still finalized a model that had written once and then kept writing: an
earlier change was read as a finished change, the tools were withdrawn mid-edit,
and the run died with `validation_runs: 0` all over again. The predicate that
matters is not "has this invocation mutated" but "is it still mutating" -- so
finalization waits for the writer to stop, and a write offered during finalization
resumes the change instead of being refused. A state that was true once is not the
state you are in now.

## A bootstrap tool is an adapter, not a second engine

When SOP needed to drive a plan with a local model instead of a hosted coding
agent, the tempting shape was a new runner. Keeping it an _adapter_ — the same
command-agent protocol, SOP still the only authority — meant it could borrow the
request/response contract and the shared command policy, add nothing to the
workflow, and be deleted once the real harness lands. The boundary that made it
safe was small and testable: canonicalize every path so a symlink cannot leave the
repository, refuse writes to the state directory, tokenize commands instead of
trusting a shell, and bound the loop. One real-run surprise was worth keeping:
cloud models occasionally answer with reasoning and no content, so an empty turn
is re-requested a bounded number of times rather than treated as a hard failure.

## One question, one answer — even for "is a task in flight"

`sop run` refused to continue an interrupted task, printing `no runnable task
(ACTIVE_TASK)`, while `sop resume` answered the same state correctly with
`CREATE_BRANCH`. The two commands had grown separate opinions: the scheduler treated
`READY` as "in flight" (it sets that status for the task it just selected), and the
run loop had no case for it, so it fell to the default and stopped; the resumption
rule lived only in `resume`. The fix was not a second state machine but exporting
the two rules that already existed — `scheduler.IsActive` for "occupies the
execution slot" and `resume.ActionFor` for "next legal action" — and having `sop
run` consult them. A guard that blocks (`ACTIVE_TASK`) should ask the _same_
question as the command that recovers from it, or the two will disagree at exactly
the moment they must agree.

## A tool that can break itself needs a pinned fallback

The bootstrap agent edits the same repository that builds it. When a change to
`internal/ollamaagent` leaves a compile error, SOP cannot build the harness from the
broken tree — so the agent needed to repair it cannot start, and recovery falls to
hand-editing the tool or to another agent entirely.

Self-hosting is worth it, but the way back must not run from the working copy. A
tool allowed to modify its own implementation should be installable and runnable as
a pinned, prebuilt known-good binary, outside the tree under edit, so a broken
change is always recoverable. "The agent can fix anything" holds only while the
agent can start.
