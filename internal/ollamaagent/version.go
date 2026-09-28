package ollamaagent

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/agentbin"
)

// Version identifiers for a sop-ollama-agent binary. The install workflow sets
// them with -ldflags "-X main.version=... -X main.sourceRevision=..."; a plain
// `go build` of the working tree keeps the defaults, so the diagnostics can
// always tell a known-good installed binary from a working-tree build.
const (
	// DevelVersion marks a binary built directly from a working tree rather than
	// installed by the pinned known-good workflow.
	DevelVersion = "devel"
	// UnknownRevision marks a binary whose source revision was not recorded at
	// build time.
	UnknownRevision = "unknown"
)

// VersionReport renders the version/source diagnostics for a binary. It is
// secret-free by construction: it reports only the version, the source revision,
// the resolved binary path, and the effective provider/model — never prompts,
// file contents, or credentials.
func VersionReport(version, sourceRevision string) string {
	if strings.TrimSpace(version) == "" {
		version = DevelVersion
	}
	if strings.TrimSpace(sourceRevision) == "" {
		sourceRevision = UnknownRevision
	}

	cfg, cfgErr := ConfigFromEnv()

	var b strings.Builder
	fmt.Fprintf(&b, "sop-ollama-agent %s\n", version)
	fmt.Fprintf(&b, "Source revision: %s\n", sourceRevision)

	if exe, err := agentbin.BinaryPath(); err != nil {
		b.WriteString("Binary path: unknown\n")
	} else {
		fmt.Fprintf(&b, "Binary path: %s\n", exe)
	}

	// Keep the diagnostics consistent with the startup runtime visibility.
	b.WriteString("Harness: tool\n")
	b.WriteString("Provider: ollama\n")
	if cfgErr != nil {
		// A malformed configuration is reported, not hidden: the value is
		// operator-set and contains no secret beyond the model name.
		fmt.Fprintf(&b, "Model: (invalid configuration: %v)\n", cfgErr)
		b.WriteString("Provider source: environment\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Model: %s\n", cfg.Model)
	fmt.Fprintf(&b, "Provider source: %s\n", configSource(agent.EnvOllamaModel))
	return b.String()
}
