# Example: diagnose_failure

Use when the operator asks why something is failing: a build, a test, a lint check,
a runtime error, a provider error, or CI.

```bash
sop prompt \
  --capability diagnose_failure \
  "Explain why the latest provider validation test failed."
```

From a prepared context file:

```bash
sop prompt \
  --capability diagnose_failure \
  --file failure-context.md
```

Read-only: SOP diagnoses; it does not change the repository. If the operator then
wants a fix applied, run a separate `--capability implement` request.
