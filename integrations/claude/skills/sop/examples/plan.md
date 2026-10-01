# Example: plan

Use when the operator asks for a plan, a design, or an approach — not for code
changes.

```bash
sop prompt \
  --capability plan \
  "Plan a cache layer for provider model discovery."
```

Read-only: no repository mutation. The plan is printed to stdout and saved under
`.agent-sdlc/runs/prompts/<run-id>/result.md`.
