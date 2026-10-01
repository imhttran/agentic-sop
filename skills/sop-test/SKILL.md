---
name: sop-test
description: |-
  Design tests through Agentic SOP. Use when an operator asks for a test plan, test
  cases, acceptance coverage, or edge-case analysis ("design tests for this", "what
  should we test?") and should get SOP's deterministic routing, provider validation,
  capability guard, and run artifacts.
disable-model-invocation: false
---

# /sop-test — test design through SOP

Capability: `design_tests` (read-only). `sop-test` is the user-facing alias for the
canonical `design_tests` capability; the alias name exists only for convenience. This
is a thin alias: it fixes the capability and delegates to the canonical SOP skill,
which owns the contract.

```bash
sop prompt --capability design_tests "<the operator's request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Read-only: designing tests MUST NOT modify the repository. It produces a test plan,
  not the tests themselves; writing them is a separate `/sop-implement` request.
- SOP picks the model class and provider. Do not choose either.
- If `sop` is not on PATH, stop and tell the operator to install the SOP CLI (see the
  project README); do not design the tests yourself.

The canonical `sop` skill owns the full contract and the capability map.
