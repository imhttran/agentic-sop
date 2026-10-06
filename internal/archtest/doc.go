// Package archtest holds deterministic architecture and provider-neutrality tests for
// the Phase 8 subsystems (CTX-001 through CTX-012).
//
// It enforces, in Go rather than in prose, that no Phase 8 core package depends on a
// provider/model implementation, and that the core context -> retrieval -> prompt ->
// agent -> normalize pipeline is provider/model neutral. These tests require no live
// provider and no network access.
package archtest
