package ollamaagent

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// toolbox is the ollamaagent's view of the controlled tools. It is a thin
// adapter over the shared toolharness: all path resolution, repository-boundary
// enforcement, state-database protection, command policy, destructive-Git
// denial, and auditing live in internal/toolharness, so this package and any
// other caller share one policy implementation rather than drifting copies.
type toolbox struct {
	h     *toolharness.Harness
	audit *toolharness.AuditLog
}

// newToolbox returns a toolbox rooted at root. Every tool call is audited into
// an in-memory trail, and, when the operator sets SOP_TOOL_AUDIT_LOG, also into
// that file. The sink never touches .agent-sdlc/state.db.
func newToolbox(root string, cfg Config) *toolbox {
	audit := toolharness.NewAuditLog(defaultMaxAuditRecords)
	var sink toolharness.Auditor = audit
	if path := strings.TrimSpace(lookupEnv(envToolAuditLog)); path != "" {
		sink = toolharness.NewMultiAuditor(audit, toolharness.NewFileAuditor(path))
	}
	return &toolbox{
		h: toolharness.New(root, toolharness.Config{
			CommandTimeout: cfg.CommandTimeout,
			MaxOutputBytes: cfg.MaxOutputBytes,
		}, sink),
		audit: audit,
	}
}

// run dispatches one tool call through the shared harness.
func (t *toolbox) run(ctx context.Context, name string, args map[string]any) (string, error) {
	return t.h.Run(ctx, name, args)
}

// records returns the in-memory audit trail collected so far.
func (t *toolbox) records() []toolharness.AuditRecord { return t.audit.Records() }
