# ETOE-005 — Disposable dogfood fixture and deterministic baseline

Create a disposable dogfood fixture and deterministic baseline: one success task,
one intentional validation failure, and one human-gated task. Setup and teardown
must be reproducible and isolated from any production checkout.

## Deliverables
- `docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md` — recorded setup/teardown, expected outcomes, and run IDs.
- Reproducible fixture setup and teardown scripts or documented commands.

## Acceptance criteria
- A reproducible setup and teardown exists and creates a disposable, initialized SOP project outside any production checkout.
- The fixture contains one success task, one intentionally failing task, and one human-gated task, with expected outcomes recorded before execution.
- The recorded run IDs and expected outcomes are sufficient to reproduce the baseline.
