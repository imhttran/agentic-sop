# Run: ETOE-004 — Decision adapter integration audit

- Provider: `ollama`
- Review engine: `self`
- Stage: `FAILED`
- Fix cycles: 0/3
- Gate: `FAIL`

## Model selection

- Class: `medium`
- Provider: `ollama`
- Model: `glm-5.3-flash:cloud`
- Locality: `cloud`
- Source: `cli`
- Reason: explicit CLI model class

## Model routing

- Class: `medium`
- Provider: `ollama`
- Model: `glm-5.3-flash:cloud`
- Locality: `cloud`
- Source: `manual_override`
- Reasons: manual model-class override
- Evidence: jev_available=false risk=none complexity=none scope=none cross_cutting=false requires_context=false confidence=0.00 acceptance_criteria=0 dependencies=0 files_affected=0

## Task

# ETOE-004 — Decision adapter integration audit

Decision adapter integration audit: actual SOP wiring versus a standalone adapter,
LOW/MEDIUM/HIGH routing, normalized results, fallbacks, and failure handling. May
run in parallel with ETOE-002 and ETOE-003.

## Deliverables
- `docs/reports/end-to-end-reliability/ETOE-004-adapter-integration-matrix.md` — explicit distinction between integrated and standalone-only capabilities.

## Acceptance criteria
- The report explicitly distinguishes capabilities that are integrated into SOP from those that are standalone-only.
- Decision-adapter integration is marked UNAVAILABLE until real SOP-to-adapter wiring is configured and verified; standalone adapter tests are not evidence of integration.
- The report names the exact integration seam (`decision.provider: command` plus `decision.command`) and whether it is configured in the audited project.

## Validation

Validation checks configured but NOT RUN: the lifecycle stopped before validation.

- NOT RUN BUILD `go build ./...`
- NOT RUN UNIT_TEST `go test ./...`
- NOT RUN LINT `go vet ./...`
- NOT RUN LINT `test -z "$(gofmt -l .)"`

## Review

No findings.

## Gate

- malformed tool request: "tool" must be a non-empty string

## Classification

- Kind: `UNKNOWN`
- Disposition: `NEEDS_HUMAN`
- Confidence: `HIGH`
- Reason: the agent reported a failure without an authoritative signal: malformed tool request: "tool" must be a non-empty string

## Autonomy

- Autonomy: `BALANCED`
- Risk: `MEDIUM`
- Decision: `HUMAN_APPROVAL_REQUIRED`
- Human approval required
- Reason: the agent reported a failure without an authoritative signal: malformed tool request: "tool" must be a non-empty string
