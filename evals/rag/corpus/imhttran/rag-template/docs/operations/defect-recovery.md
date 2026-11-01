# SOP Defect Recovery Policy

**Type:** operator-run policy (normative). Applies when a RAG task hits a
suspected bug in `agentic-sop`. Nothing here authorizes bypassing approvals or
editing SOP state.

When a RAG task encounters a suspected bug in agentic-sop:

1. Capture the failed command, error, and installed SOP version.
2. Check the agentic-sop repository for a newer revision.
3. If the checkout is clean and on the expected branch, pull the latest changes. Otherwise STOP and request operator guidance.
4. Rebuild, validate, and reinstall SOP using the repository's supported installation procedure.
5. Verify the installed executable corresponds to the expected source revision.
6. Retry the original failed operation once, provided the operation is safe to repeat and does not bypass approval requirements.
7. If resolved, record the source revision and successful retry.
8. If still failing, preserve the evidence and create an SOP defect report.

Rules:

- Never manually edit SOP state or approval artifacts.
- Never bypass human approvals.
- Never force-pull, reset, or overwrite local changes.
- Never retry destructive or non-idempotent operations without explicit authorization.
- Do not modify the RAG implementation to work around an SOP defect.
- Do not automatically change production SOP versions without operator authorization.
