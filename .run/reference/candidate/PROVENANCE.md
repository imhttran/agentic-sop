# Candidate binary provenance

The candidate binary is local and intentionally not committed.

- Local path: `.run/candidate/sop`
- SHA-256: `c161b0aab7e18bf75b90ce940dc878d64148dabf15214f676d0b9d35c6602d03`
- Module: `github.com/imhttran/agentic-sop`
- VCS revision: `e99cde03aff969b458e6d23579d6b3bb3a6fbe56`
- VCS modified: `true`
- Build: `go build -trimpath -o .run/candidate/sop ./cmd/sop`

It is excluded because it is a 27 MB generated build output and this repository
already excludes root build binaries. Rebuild it from the recorded source
revision and verify the checksum before relying on this evidence.
