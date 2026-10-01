---
name: sop-implement
description: |-
  Run a repository-changing request through Agentic SOP's governed implementation
  lifecycle. Use ONLY when an operator explicitly asks to change the repository
  ("implement this", "add caching", "fix this bug"). Read-only requests must use
  /sop-plan, /sop-review, /sop-diagnose, or /sop-test instead.
disable-model-invocation: true
---

# /sop-implement — governed implementation through SOP

Capability: `implement` (**mutating**). This is a thin alias: it fixes the capability
and delegates to the canonical SOP skill, which owns the contract.

```bash
sop prompt --capability implement "<the operator's request>"
```

The request then runs SOP's **governed implementation lifecycle** — planning,
implementation, deterministic validation, review, quality gate, bounded fix, and the
human approval boundary — exactly as a planned task does. SOP decides what may happen;
this command never does.

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Do NOT satisfy this request by editing files, running git, or applying a patch
  yourself. Invoke SOP and return SOP's result; the repository mutation goes through
  SOP's tool/harness authorization path.
- Do NOT upgrade a read-only request into an implementation. If the operator asked for
  a review or a plan, use the read-only alias.
- `implement` cannot run on a provider that does not declare `IMPLEMENT`. SOP fails
  clearly rather than sending an edit-style request to a text-only model — surface that
  error instead of working around it.
- SOP picks the model class and provider. Do not choose either.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see the
  project README). Failing closed matters most here: do not implement the change
  yourself.

This skill is hidden from the agent's autonomous catalog (`disable-model-invocation`)
because repository mutation is an explicit operator decision.

The canonical `sop` skill owns the full contract and the capability map.
