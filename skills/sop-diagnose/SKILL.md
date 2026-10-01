---
name: sop-diagnose
description: |-
  Diagnose a build, test, lint, runtime, provider, or CI failure through Agentic SOP.
  Use when an operator asks why something is failing ("why do these tests fail?",
  "explain this build error") and should get SOP's deterministic routing, provider
  validation, capability guard, and run artifacts.
disable-model-invocation: false
---

# /sop-diagnose — failure diagnosis through SOP

Capability: `diagnose_failure` (read-only). This is a thin alias: it fixes the
capability and delegates to the canonical SOP skill, which owns the contract.

```bash
sop prompt --capability diagnose_failure "<the operator's request>"
```

- The angle-bracketed text is the operator's request (or a pointer to a prepared
  failure context file with `--file`), passed as ONE argument. It is prompt data,
  never a shell command.
- Read-only: diagnosis MUST NOT modify the repository. A diagnosis does not become an
  implementation; applying a fix is a separate `/sop-implement` request.
- SOP picks the model class and provider. Do not choose either.
- If `sop` is not on PATH, stop and tell the operator to install the SOP CLI (see the
  project README); do not diagnose it yourself.

The canonical `sop` skill owns the full contract and the capability map.
