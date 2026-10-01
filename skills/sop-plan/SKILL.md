---
name: sop-plan
description: |-
  Create a governed plan through Agentic SOP. Use when an operator asks for a plan,
  a design, an approach, or a comparison ("plan this", "how would you add caching",
  "design the migration") and should get SOP's deterministic routing, provider
  validation, capability guard, and run artifacts.
disable-model-invocation: false
---

# /sop-plan — planning through SOP

Capability: `plan` (read-only). This is a thin alias: it fixes the capability and
delegates to the canonical SOP skill, which owns the contract.

```bash
sop prompt --capability plan "<the operator's request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Read-only: planning MUST NOT modify the repository. A plan is not an implementation;
  do not follow it with an edit here.
- SOP picks the model class and provider. Do not choose either.
- If `sop` is not on PATH, stop and tell the operator to install the SOP CLI (see the
  project README); do not produce the plan yourself.

The canonical `sop` skill owns the full contract and the capability map.
