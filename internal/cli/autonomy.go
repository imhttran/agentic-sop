package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
)

// decideAutonomy resolves the configured autonomy policy and applies it to a
// failure classification: the single place the lifecycle turns a classification
// into an approval decision. A blank classification (a passing run) yields an
// empty decision. It is deterministic and consults no model.
func decideAutonomy(cfg config.Config, cls failure.Classification) autonomy.Decision {
	if cls.Disposition == "" {
		return autonomy.Decision{}
	}
	return autonomy.Decide(cls, cfg.AutonomyPolicy())
}

// policyForcesHuman reports whether the autonomy policy turned an otherwise
// recoverable classification into a human boundary — for example a conservative
// level that withholds a deterministic fix. It is deliberately distinct from the
// classifier's own human boundary (a genuine boundary, an unknown failure), which
// the run stage already reflects, so the policy only ADDS human boundaries and
// never silently removes one the classifier found.
func policyForcesHuman(cls failure.Classification, d autonomy.Decision) bool {
	return d.RequiresHuman && cls.Disposition != failure.NeedsHuman
}

// emitAutonomyActivity reports an autonomy decision on the activity stream, so an
// operator sees WHY SOP continued automatically or stopped for a human. It is a
// no-op for an empty decision.
func emitAutonomyActivity(ctx context.Context, d autonomy.Decision) {
	if d.Action == "" {
		return
	}
	activity.FromContext(ctx).Emit(activity.StageAutonomy, string(d.Action), autonomyDetail(d))
}

// autonomyDetail renders the concise activity detail: the risk and the level the
// decision was made at.
func autonomyDetail(d autonomy.Decision) string {
	return fmt.Sprintf("risk=%s level=%s", d.Risk, strings.ToUpper(string(d.Level)))
}

// writeAutonomyReport renders the autonomy decision in the run report, so it is
// obvious why SOP stopped or continued. It renders nothing for an empty decision.
func writeAutonomyReport(b *strings.Builder, d autonomy.Decision) {
	if d.Action == "" {
		return
	}
	b.WriteString("\n## Autonomy\n\n")
	fmt.Fprintf(b, "- Autonomy: `%s`\n", strings.ToUpper(string(d.Level)))
	fmt.Fprintf(b, "- Risk: `%s`\n", d.Risk)
	fmt.Fprintf(b, "- Decision: `%s`\n", d.Action)
	if d.RequiresHuman {
		b.WriteString("- Human approval required\n")
	}
	if reason := strings.TrimSpace(d.Reason); reason != "" {
		fmt.Fprintf(b, "- Reason: %s\n", reason)
	}
}

// classificationArtifact is the persisted classification document. It embeds the
// failure classification at the top level (so existing readers keep working) and
// adds the autonomy decision as provenance.
type classificationArtifact struct {
	failure.Classification
	// Autonomy is the risk-based decision applied to the classification. It is
	// omitted when no decision was made (for example an infrastructure error before
	// classification).
	Autonomy *autonomy.Decision `json:"autonomy,omitempty"`
}
