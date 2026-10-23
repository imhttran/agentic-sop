# S1 — Inspect repository and choose the smallest Promptfoo integration

Inspect repository instructions, branch state, and existing SOP workflows. Locate scanner adapters, command execution, version pinning, reporting, and artifact storage. Locate approval enforcement, verification-evidence checks, RAG, advisory memory, and tool-execution boundaries. Check the pinned Promptfoo release license, provider interfaces, output schema, and data-handling settings against official sources. Select the smallest integration that supports the six scenarios and record the selected command, configuration, target contract, and evidence format in repository documentation. Create an isolated feature branch or worktree and preserve unrelated changes. Defer Garak, PyRIT, Guardrails AI, scheduling, and Tran-specific adapters.

## Expected outcomes (descriptive)
Report these in your summary. They do not require a new file unless an acceptance criterion or a required deliverable file names one.
- Written integration decision, selected Promptfoo pin with verified license/provider/output/data-handling findings, and recorded check-command set
- Recorded target contract and evidence format in repository documentation
- Isolated feature branch or worktree with branch state noted

## Acceptance criteria
- The selected integration adds one optional Promptfoo adapter and no second workflow runner
- No new production endpoint is added solely for evaluations
- The four Phase 1 inspection targets are each recorded with a concrete file path or command
- Repository instructions, branch state, and existing SOP workflow are inspected and cited
- Unrelated working-tree changes are preserved
