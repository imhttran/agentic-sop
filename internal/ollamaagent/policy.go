package ollamaagent

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// CapabilityPolicy bounds and constrains one capability's agent loop. It is the
// single authoritative statement of how long a capability may run and which
// tools it may use, so capability checks are not scattered through the harness.
type CapabilityPolicy struct {
	// MaxIterations is the maximum number of model turns for this capability. It
	// is a ceiling, not a target: the model is expected to finish well before it.
	MaxIterations int
	// ReadOnly is true when the capability must not modify the repository. It is
	// descriptive (the prompt tells the model) as well as enforced (AllowedTools
	// omits the mutation tools).
	ReadOnly bool
	// AllowedTools is the set of tool names the capability may call. A nil set
	// allows every tool.
	AllowedTools map[string]bool
}

// Allows reports whether the capability may call tool. An unknown tool is
// allowed through so the tool harness can reject it as unsupported, which is a
// more specific message than a capability denial.
func (p CapabilityPolicy) Allows(tool string) bool {
	if p.AllowedTools == nil || !allTools[tool] {
		return true
	}
	return p.AllowedTools[tool]
}

// ToolList returns the allowed tool names in the canonical order, for the prompt
// and for denial messages.
func (p CapabilityPolicy) ToolList() []string {
	if p.AllowedTools == nil {
		return toolharness.Tools()
	}
	out := make([]string, 0, len(p.AllowedTools))
	for _, t := range toolharness.Tools() {
		if p.AllowedTools[t] {
			out = append(out, t)
		}
	}
	return out
}

// Capability iteration budgets. They are deliberately modest: a capability that
// needs more turns than this has stopped making progress, and the harness reports
// a diagnostic instead of running on. They are maxima, not targets.
const (
	maxIterationsPlan        = 8
	maxIterationsDesignTests = 12
	maxIterationsImplement   = 24
	maxIterationsFix         = 24
	maxIterationsDiagnose    = 12
	maxIterationsReview      = 12
)

// allTools is every tool the shared harness implements, so a capability policy
// can tell a known-but-disallowed tool from an unknown one.
var allTools = toolset(toolharness.Tools()...)

var (
	// readTools can inspect the repository but never modify it.
	readTools = toolset(
		toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles,
		toolharness.ToolGitStatus, toolharness.ToolGitDiff,
	)
	// inspectTools add non-mutating command execution (to reproduce or observe
	// behavior) but still cannot write files.
	inspectTools = toolset(
		toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles,
		toolharness.ToolRunCommand, toolharness.ToolGitStatus, toolharness.ToolGitDiff,
	)
	// mutationTools is the full controlled surface.
	mutationTools = allTools
)

// toolset builds a tool-name set.
func toolset(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// PolicyFor returns the policy for a capability. An unknown capability gets the
// conservative read-only default, so a future capability cannot accidentally gain
// write access before it is considered here.
func PolicyFor(c agent.Capability) CapabilityPolicy {
	switch c {
	case agent.Plan:
		return CapabilityPolicy{MaxIterations: maxIterationsPlan, ReadOnly: true, AllowedTools: readTools}
	case agent.Review:
		return CapabilityPolicy{MaxIterations: maxIterationsReview, ReadOnly: true, AllowedTools: readTools}
	case agent.DesignTests:
		return CapabilityPolicy{MaxIterations: maxIterationsDesignTests, ReadOnly: true, AllowedTools: readTools}
	case agent.DiagnoseFailure:
		return CapabilityPolicy{MaxIterations: maxIterationsDiagnose, ReadOnly: true, AllowedTools: inspectTools}
	case agent.Implement:
		return CapabilityPolicy{MaxIterations: maxIterationsImplement, AllowedTools: mutationTools}
	case agent.Fix:
		return CapabilityPolicy{MaxIterations: maxIterationsFix, AllowedTools: mutationTools}
	default:
		return CapabilityPolicy{MaxIterations: maxIterationsReview, ReadOnly: true, AllowedTools: readTools}
	}
}

// describeTools renders a tool set for a message.
func describeTools(p CapabilityPolicy) string {
	return strings.Join(p.ToolList(), ", ")
}
