# Local SOP run reference

This directory records the reliability review and the bounded SOP-006 workflow.
It is reference material, not project content.

- `reference/SOP-RELIABILITY-REVIEW-REPORT.md` - review evidence, defects fixed,
  validation, candidate provenance, and limitations.
- `reference/sop-006-rust/` - the first controlled-failure Rust run and its
  final reports/counts.
- `reference/sop-006-rust-approval/` - the approval-safe rerun: pending gate,
  explicit approval, approved recovery, final reports/counts, and preserved
  command output.
- `reference/sop-cargo-tool/` - native tool-harness audits for enabled,
  disabled, unsupported, and manifest-escape Cargo commands.
- `reference/candidate/PROVENANCE.md` - candidate checksum and source identity;
  the 27 MB binary itself stays local.

Raw disposable fixtures and the candidate binary remain local. The scoped
`.gitignore` in this directory keeps them out of Git while allowing this
reference subtree to be tracked.
