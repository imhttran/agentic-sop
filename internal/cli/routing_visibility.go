package cli

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// Routing visibility (P35-005).
//
// The class, resolved model, source, reasons, and the typed evidence that drove
// the decision are surfaced to an operator through the CLI and the report so it is
// clear WHY a task ran on the model it did. All of the displayed values already
// exist on the in-memory routing decision (taskRouting) and its persisted report
// projection (routingDoc); this file only renders them deterministically.
//
// Gating rule: nothing is emitted unless routing actually applied. Every surface
// is reached only through a non-nil routing decision (nil means no routing), so an
// unrouted run prints no routing block anywhere.
//
// Reason phrases are emitted verbatim from the router/model constants (never
// re-derived from prose) and joined deterministically; the evidence values are
// rendered from the typed signals, again verbatim.

// routingEvidenceLine renders the typed routing evidence as a short, deterministic
// phrase. It reports the values the router actually consumed: whether JEV evidence
// was available, the risk/complexity/scope levels, the cross-cutting and
// requires-context flags, the analysis confidence, and the task counts.
//
// It returns "" when there is no evidence block to render (a nil signals pointer),
// so a manual override without router signals adds no fabricated evidence line.
func routingEvidenceLine(sig *runpkg.RoutingSignals) string {
	if sig == nil {
		return ""
	}
	var parts []string
	if sig.JEVAvailable {
		parts = append(parts, "jev=available")
	} else {
		parts = append(parts, "jev=unavailable")
	}
	parts = append(parts,
		fmt.Sprintf("risk=%s", levelText(sig.Risk)),
		fmt.Sprintf("complexity=%s", levelText(sig.Complexity)),
		fmt.Sprintf("scope=%s", scopeText(sig.Scope)),
		fmt.Sprintf("cross_cutting=%t", sig.CrossCutting),
		fmt.Sprintf("requires_context=%t", sig.RequiresContext),
		fmt.Sprintf("confidence=%.2f", sig.Confidence),
		fmt.Sprintf("criteria=%d", sig.AcceptanceCriteria),
		fmt.Sprintf("dependencies=%d", sig.Dependencies),
		fmt.Sprintf("files=%d", sig.FilesAffected),
	)
	return strings.Join(parts, " ")
}

// routingEvidenceMarkdown renders the typed routing evidence for the Markdown
// report. It returns "" when there is no evidence to render.
func routingEvidenceMarkdown(sig *runpkg.RoutingSignals) string {
	if sig == nil {
		return ""
	}
	return fmt.Sprintf(
		"jev_available=%t risk=%s complexity=%s scope=%s cross_cutting=%t requires_context=%t confidence=%.2f acceptance_criteria=%d dependencies=%d files_affected=%d",
		sig.JEVAvailable, levelText(sig.Risk), levelText(sig.Complexity), scopeText(sig.Scope),
		sig.CrossCutting, sig.RequiresContext, sig.Confidence,
		sig.AcceptanceCriteria, sig.Dependencies, sig.FilesAffected)
}

// levelText renders a typed level verbatim. An empty (unset) level reads "none"
// rather than being fabricated, so a signal the router did not set is never
// displayed as a value it did not have.
func levelText(v string) string {
	if strings.TrimSpace(v) == "" {
		return "none"
	}
	return v
}

// scopeText renders a typed scope verbatim, falling back to "none" for an unset
// scope.
func scopeText(v string) string {
	if strings.TrimSpace(v) == "" {
		return "none"
	}
	return v
}

// routingCheckpointsText renders the checkpoints that informed the decision as a
// deterministic, comma-joined list. It returns "" when none informed it.
func routingCheckpointsText(cps []string) string {
	if len(cps) == 0 {
		return ""
	}
	return strings.Join(cps, ", ")
}

// routingClassLine renders the routing summary shown by the CLI when a task's
// routing decision is made: the class and resolved model with the reasons on the
// headline line, followed by the source and the typed evidence as indented
// continuation lines.
//
// The headline keeps the stable, machine-parseable shape
// `Task routing: <class> (<model>; <reasons>)` so downstream readers and existing
// operators see a consistent line; the source and the typed evidence (the values
// the router actually consumed) are appended as indented lines.
//
// It is deterministic: the same signals always render the same block. It is only
// ever called for an active routing decision, so nothing is shown when routing did
// not apply.
func routingClassLine(r *routingDoc) string {
	if r == nil {
		return ""
	}
	line := fmt.Sprintf("Task routing: %s (%s; %s)", r.Class, r.Model, router.ReasonsText(r.Reasons))
	line += fmt.Sprintf("\n  source: %s", r.Source)
	if cps := routingCheckpointsText(r.Checkpoints); cps != "" {
		line += "\n  informed by: " + cps
	}
	if ev := routingEvidenceLine(r.Signals); ev != "" {
		line += "\n  evidence: " + ev
	}
	return line
}
