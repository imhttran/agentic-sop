package ollamaagent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

const alreadySatisfiedInstruction = `If the requested implementation already exists, use the supplied already-satisfied verification contract: inspect its concrete evidence, run every required validation, and explicitly return ALREADY_SATISFIED. Do not manufacture changes. Without verified proof, implement the requested change. The existing discovery, stale, and iteration limits remain in force.`

// satisfactionProof is invocation-local verification, separate from mutation
// and discovery progress. Validation never resets the stale streak.
type satisfactionProof struct {
	baseline string
	reads    map[string]bool
	commands map[string]bool
}

func (h *Harness) satisfactionProof(ctx context.Context, req agent.Request) *satisfactionProof {
	p := &satisfactionProof{reads: map[string]bool{}, commands: map[string]bool{}}
	if len(req.AcceptanceCriteria) != 0 && len(req.ValidationCommands) != 0 {
		p.baseline, _ = h.tools.RepositoryFingerprint(ctx)
	}
	return p
}

func commandIdentity(command string) string {
	argv, err := toolharness.SplitCommand(strings.TrimSpace(command))
	if err != nil || len(argv) == 0 {
		return ""
	}
	data, _ := json.Marshal(argv)
	return string(data)
}

func (p *satisfactionProof) observe(root, name string, args map[string]any, result string, err error, commandSucceeded bool) {
	if name == toolharness.ToolReadFile {
		identity, ok := discoveryIdentity(root, name, args, result, err)
		if ok {
			p.reads[identity.path] = true
		}
	}
	if name == toolharness.ToolRunCommand {
		command, _ := args["command"].(string)
		p.commands[commandIdentity(command)] = err == nil && commandSucceeded
	}
}

// verify accepts only an explicit claim with complete caller-owned criteria,
// inspected files and every caller-owned validation command actually passing.
// Unknown or changed repository state fails closed. The model selects evidence
// links; it cannot manufacture tool results or substitute easier validations.
func (h *Harness) verifySatisfied(ctx context.Context, req agent.Request, st *executionState, p *satisfactionProof, final map[string]any) bool {
	if st.mutationObserved || p.baseline == "" || final["status"] != string(agent.OutcomeCompleted) {
		return false
	}
	data, _ := json.Marshal(final["evidence"])
	var evidence agent.CompletionEvidence
	if json.Unmarshal(data, &evidence) != nil || len(evidence.Acceptance) != len(req.AcceptanceCriteria) {
		return false
	}
	seen := map[string]bool{}
	for _, item := range evidence.Acceptance {
		if seen[item.Criterion] || len(item.Paths) == 0 {
			return false
		}
		seen[item.Criterion] = true
		for _, path := range item.Paths {
			if !filepath.IsAbs(path) {
				path = filepath.Join(h.tools.Root(), path)
			}
			canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
			if err != nil || !p.reads[canonical] {
				return false
			}
		}
	}
	for _, criterion := range req.AcceptanceCriteria {
		if strings.TrimSpace(criterion) == "" || !seen[criterion] {
			return false
		}
	}
	for _, command := range req.ValidationCommands {
		identity := commandIdentity(command)
		if identity == "" || !p.commands[identity] {
			return false
		}
	}
	after, err := h.tools.RepositoryFingerprint(ctx)
	if err != nil || after != p.baseline {
		return false
	}
	// Discard model-provided validation claims and mutation counts. Preserve only
	// executed commands and verified inspection links in the completion artifact.
	evidence.ValidationCommands = append([]string(nil), req.ValidationCommands...)
	final["evidence"] = evidence
	final["repository_mutations"] = 0
	final["changes_expected"] = false
	return true
}
