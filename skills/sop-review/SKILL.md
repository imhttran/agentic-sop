---
name: sop-review
description: |-
  Review code, architecture, security, or a design through Agentic SOP. Use when an
  operator asks for a review ("review this", "review internal/provider", "any layering
  problems?") and should get SOP's deterministic routing, provider validation,
  capability guard, and run artifacts.
disable-model-invocation: false
---

# /sop-review — review through SOP

Capability: `review` (read-only). This is a thin alias: it fixes the capability and
delegates to the canonical SOP skill, which owns the contract.

```bash
sop prompt --capability review "<the operator's request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Read-only: a review MUST NOT modify the repository. If the operator also wants the
  findings applied, that is a separate `/sop-implement` request, not something this
  command does.
- SOP picks the model class and provider. Do not choose either.
- A review can run on a text-only provider. If `sop` is not on PATH, stop and tell the
  operator to install the SOP CLI (see the project README); do not review it yourself.

The canonical `sop` skill owns the full contract and the capability map.
