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
