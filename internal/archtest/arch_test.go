package archtest

// phase8CorePackages are the Phase 8 subsystems that must stay provider/model neutral:
// the deterministic harness owns policy and reason, and provider/model behavior belongs
// behind the canonical capability interface.
//
// The guard contract, the policy/governance inventory, the provider registry, and the
// enforcement tests live in guard_test.go.
var phase8CorePackages = []string{
	"internal/context",
	"internal/repoindex",
	"internal/retrieval",
	"internal/retrievalgate",
	"internal/prompt",
	"internal/normalize",
	"internal/verifcache",
	"internal/promptcache",
	"internal/vectoreval",
	"internal/decisionmemory",
	"internal/adaptiveroute",
	"internal/prompttuning",
}
