# Run: ETOE-005 — Disposable dogfood fixture and deterministic baseline

- Provider: `ollama`
- Review engine: `self`
- Stage: `WAITING_FOR_HUMAN`
- Fix cycles: 3/3
- Gate: `NEEDS_HUMAN`

## Model selection

- Class: `large`
- Provider: `ollama`
- Model: `deepseek-v4.1-flash:cloud`
- Locality: `cloud`
- Source: `cli`
- Reason: explicit CLI model class

## Model routing

- Class: `large`
- Provider: `ollama`
- Model: `deepseek-v4.1-flash:cloud`
- Locality: `cloud`
- Source: `manual_override`
- Reasons: manual model-class override
- Evidence: jev_available=false risk=none complexity=none scope=none cross_cutting=false requires_context=false confidence=0.00 acceptance_criteria=0 dependencies=0 files_affected=0

## Task

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

## Validation

- PASS BUILD `go build ./...`
- PASS UNIT_TEST `go test ./...`
- PASS LINT `go vet ./...`
- PASS LINT `test -z "$(gofmt -l .)"`

## Review

- HIGH docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md:285 — Acceptance criterion 3 not actually delivered: no concrete run IDs or observed outcomes recorded
- HIGH scripts/etoe-005-fixture-run.sh:106 — Outcome parsing is fragile and can silently misclassify the intentionally failing task
- MEDIUM scripts/etoe-005-fixture-run.sh:91 — Run-ID capture assumes exactly one new run directory yet silently continues with UNAVAILABLE
- MEDIUM scripts/etoe-005-fixture-run.sh:132 — Scripts depend on `sop run --task` but never verify that surface exists
- MEDIUM scripts/etoe-005-fixture-setup.sh:246 — FIX-GATE task definition does not clearly guarantee it reaches the human approval gate
- LOW scripts/etoe-005-fixture-setup.sh:96 — Setup's in-checkout guard depends on the script being run from its repository location
- LOW docs/reports/end-to-end-reliability/ETOE-005-fixture-baseline.md:120 — Report references config-schema documentation as authoritative without verifying it in this change

## Gate

- fix-loop limit reached (3/3)

## Classification

- Kind: `AUTO_FIX_EXHAUSTED`
- Disposition: `NEEDS_HUMAN`
- Confidence: `HIGH`
- Reason: automatic fixes were exhausted (3/3) without resolving the failure

## Autonomy

- Autonomy: `BALANCED`
- Risk: `MEDIUM`
- Decision: `HUMAN_APPROVAL_REQUIRED`
- Human approval required
- Reason: automatic fixes were exhausted (3/3) without resolving the failure
