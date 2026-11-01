# Writing Plans and Agent Prompts

How to write implementation plans, prompts for coding agents, SOP workflows, and procedures for this repository. These rules apply to any author: a human, a planner model, or a coding agent writing a follow-up plan.

## Scope

- Plans, prompts for coding agents, SOP workflows, and procedures follow the STE-style rules below.
- Explanations, reports, and chat replies use plain, natural prose.

## STE-style rules

These follow the intent of ASD-STE100 (Simplified Technical English). They aren't a formal STE implementation.

- One instruction per sentence. Imperative, active voice.
- Procedural sentences: 20 words or fewer. Descriptive sentences: 25 or fewer.
- One term per concept. Don't switch synonyms.
- Keep commands, paths, config keys, and IDs exactly as written.
- Don't claim STE compliance. No formal audit has been done.

## Plan structure

- Split work into phases. Each phase has acceptance criteria, acceptance checks, and a stop condition.
- Reuse a passing check when its inputs haven't changed. After a change, rerun the affected checks. Run the full suite at integration milestones. Never weaken a gate to pass.
- Never invent results or experiences.

## Prompts for coding agents

- Let the agent work end to end. It stops only at a real blocker:
  - a gate that still fails after the allowed fix attempts;
  - a missing credential, service, or file it can't create;
  - a decision that needs a human (a SOP approval gate or `needs_human`; see [specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md));
  - anything destructive or irreversible.
- Don't ask the agent for manual spot checks. Automated acceptance checks, required integration tests (see [specs/RAG-POSTGRES-VALIDATION-POLICY.md](../specs/RAG-POSTGRES-VALIDATION-POLICY.md)), and CI always run. Final review happens in the PR.

## SOP plan format

`sop run` validates a plan before it runs anything. A plan that breaks these rules is refused with "No work was executed".

### Acceptance Mapping

- Write one line per criterion, in one of two forms: `- criterion N: check M` or `- criterion N: manual: <evidence>`.
- Count from 1. N is the bullet number under `### Acceptance Criteria`. M is the bullet number under `### Acceptance Checks`.
- Map every criterion exactly once. A check can serve more than one criterion, or none.
- Use `manual` only for a judgment that no command can prove, such as the quality of a report.
- Free text such as "X → test Y" fails validation. See `internal/planner/plan.go`.

### Acceptance Checks

- Use only commands that `internal/commandpolicy` classifies as SAFE:
  - `go test`, `go build`, `go vet`, `go fmt`, `go list`;
  - read-only git: `git diff`, `git status`, `git log`, `git show`, `git rev-parse`, `git ls-files`, `git describe`, `git blame`, `git shortlog`, `git cat-file`, `git grep`.
- Any other command is REQUIRES_APPROVAL. It blocks the task before the model runs. This includes `go run`, `grep`, `jq`, `shasum`, `cd … &&`, and every `cargo` command.
- To check a file's text, use `git grep -q "<text>" -- <path>`.
- To check data or CLI behavior, require a named Go test in the deliverables. Run it with `go test -run <Name>`. Keep the gate as strict as the command it replaces.
- To prove that protected files are unchanged, use `git diff --quiet main -- <paths>`.

### Dependencies

- Write `None` when a task has no dependencies. Otherwise write one task ID per bullet.
- Put no commentary in this section. Put prerequisites in the plan body.

### Changing a plan after a start

- After SOP creates the task graph, any edit to the plan makes `sop run` stop with NEEDS_HUMAN.
- Commit the edit, then run `sop reconcile <plan>` before `sop run`.
- Validate the plan before the first start to avoid this step.

## Starting work

- Apply these writing rules without asking.
- Starting a plan still goes through SOP's normal gates. A reviewed plan starts only on a human go-ahead.
