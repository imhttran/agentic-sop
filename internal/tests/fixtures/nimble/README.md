# Nimble golden fixture

`systemone_response.json` is a **real** response captured from a locally
installed Ollama + Nimble instance via `POST /v1/systemone` during Phase 1.1.

It is the first known-good provider contract fixture. The production response
parser is driven from it so parser behavior can be tested fully offline: any
divergence between this fixture and the parser indicates a parser defect, not a
fixture problem.

Provenance: captured from a real Nimble instance. Do not hand-edit values.
The `noul` field is SystemOne transport terminology and must never appear in
fixtures consumed by the public `decision` package.
