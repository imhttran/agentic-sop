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

## Starting work

- Apply these writing rules without asking.
- Starting a plan still goes through SOP's normal gates. A reviewed plan starts only on a human go-ahead.
