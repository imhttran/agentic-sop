// Package egmmanifest provides offline schema, canonicalization, integrity, leakage,
// access-separation, and export checks for the Independent Design Review (EGM) sealed
// case manifest.
//
// It is experiment-local tooling. Nothing in SOP imports it, and it is not wired into
// any production lifecycle, approval, or provider path. It operates on synthetic or
// operator-supplied files only and never contacts a model or a network service.
//
// The manifest it validates is the one described in
// docs/reports/independent-design-review-corpus/CASE-CONSTRUCTION-SPECIFICATION.md §6.1.
// The canonicalization, schema, and export shapes here are implementation proposals, not
// operator-approved governance. See
// docs/reports/independent-design-review-corpus/DECISION-005-MANIFEST-TECHNICAL-VERIFICATION.md.
package egmmanifest
