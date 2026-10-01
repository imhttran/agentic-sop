---
name: sop-prompt
description: |-
  Send an ad-hoc request through Agentic SOP with no capability chosen. Use when an
  operator wants SOP to handle a general request ("explain this", "what do you think
  about X") and should get SOP's deterministic routing, provider validation,
  capability guard, and run artifacts instead of a bare model answer.
disable-model-invocation: false
---

# /sop-prompt — an ad-hoc request through SOP

This is the generic entry point. It fixes **no** capability: the `sop` CLI's own
conservative default applies (a read-only, text-oriented capability), so a bare
prompt is never treated as an implementation request.

```bash
sop prompt "<the operator's request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Do not add `--capability` here. If the request clearly needs a specific capability,
  use the matching alias instead: `/sop-plan`, `/sop-review`, `/sop-diagnose`,
  `/sop-test`, or `/sop-implement`.
- SOP picks the model class and provider. Do not choose either here.
- If `sop` is not on PATH, stop and tell the operator to install the SOP CLI (see the
  project README); do not answer the request yourself.

The canonical `sop` skill owns the full contract: the capability map, the read-only
versus governed-mutation distinction, and the output format.
