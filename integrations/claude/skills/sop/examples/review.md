# Example: review

Use when the operator asks for a review of code, architecture, or design.

```bash
sop prompt \
  --capability review \
  "Review the provider runtime architecture for layering violations."
```

Read-only: no repository mutation. A review runs on any capability-declaring
provider, including a text-only MLX/oMLX runtime.

```bash
sop prompt \
  --capability review \
  --json \
  "Review internal/provider for architectural problems"
```

`--json` prints the result document for a calling agent to consume.
