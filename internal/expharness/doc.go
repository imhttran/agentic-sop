// Package expharness is experiment-local tooling for the independent design-review
// experiment (docs/reports/INDEPENDENT-DESIGN-REVIEW-WORKSTREAM-B-HARDENING.md).
//
// It is NOT wired into any SOP command and changes no SOP routing, provider or
// governance behavior. It lets an experiment operator (1) refuse to start when the
// routing configuration or environment is not the approved overlay, (2) observe the
// provider traffic of one run from outside SOP, with hard call and token ceilings,
// (3) assemble a per-run attribution record that says which facts are measured and
// which are client-side only, and (4) render a deterministic review request.
//
// Nothing here attests a provider's model identity: a server-returned model field is
// self-reported by the server and is recorded as such.
package expharness
