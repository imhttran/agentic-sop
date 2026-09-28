package cli

import (
	"strconv"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/perf"
)

// JEV diagnostic-evidence report types (JEV010).
//
// A JEV result is persisted as a run artifact (jev.json) and reported as JEV
// diagnostic evidence, clearly separate from the validation and review sections.
// These types are the on-disk/report shape of that evidence; they are read-only
// data and are never consumed as workflow state.

// jevArtifactName is the run-directory artifact holding the latest JEV result.
// It matches the run package's jev.json convention (see run.WriteJEVArtifact).
const jevArtifactName = "jev.json"

// jevFindingDoc is the persisted/report shape of one JEV finding, using the PRD
// field vocabulary: severity, file, line, category, finding, and evidence.
type jevFindingDoc struct {
	Severity string `json:"severity"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Category string `json:"category,omitempty"`
	Finding  string `json:"finding"`
	Evidence string `json:"evidence,omitempty"`
}

// jevMetricsDoc is the persisted/report shape of one JEV invocation's diagnostic
// metrics (JEV014): invocation count, duration, provider/model, tool calls,
// finding count, and blocking finding count. It is metadata only and never feeds
// a decision; a missing/zero value never implies PASS or FAIL.
type jevMetricsDoc struct {
	Invocations      int    `json:"invocations"`
	DurationMS       int64  `json:"duration_ms"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	ToolCalls        int    `json:"tool_calls"`
	Findings         int    `json:"findings"`
	BlockingFindings int    `json:"blocking_findings"`
}

// jevRunDoc is the persisted JEV run artifact. It associates the result with the
// task and run that produced it and records the status, the fail-closed flag and
// reason, a summary, any findings, and the invocation's diagnostic metrics. It is
// diagnostic evidence only: nothing reads it back to drive a decision, and it is
// never workflow state.
type jevRunDoc struct {
	TaskID     string          `json:"task_id,omitempty"`
	RunID      string          `json:"run_id,omitempty"`
	Status     string          `json:"status"`
	FailClosed bool            `json:"fail_closed"`
	Reason     string          `json:"reason,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	Findings   []jevFindingDoc `json:"findings"`
	// Metrics are the invocation's diagnostic metrics (JEV014). They are
	// omitted when none were captured, so an existing artifact stays compatible.
	Metrics    *jevMetricsDoc `json:"metrics,omitempty"`
	RecordedAt time.Time      `json:"recorded_at"`
}

// jevReportDoc is the run report's JEV section: a distinct, attributed summary of
// the JEV diagnostic evidence and a reference to the persisted artifact. It is
// separate from the validation and review sections, which remain authoritative.
type jevReportDoc struct {
	TaskID       string          `json:"task_id,omitempty"`
	RunID        string          `json:"run_id,omitempty"`
	Status       string          `json:"status"`
	FailClosed   bool            `json:"fail_closed"`
	Reason       string          `json:"reason,omitempty"`
	Findings     []jevFindingDoc `json:"findings"`
	ArtifactPath string          `json:"artifact_path"`
	// Metrics are the JEV invocation's diagnostic metrics (JEV014), kept apart
	// from the validation and review sections so JEV cost/time is distinguishable
	// from IMPLEMENT/FIX. They are metadata only and never assert PASS or FAIL.
	Metrics *jevMetricsDoc `json:"metrics,omitempty"`
}

// buildJEVDoc turns JEV run evidence into the persisted artifact document. It is a
// pure projection of the evidence: it fabricates nothing, so a PASS or
// finding-free result yields a well-formed document with an empty findings list,
// and a fail-closed result records the fail-closed flag and reason rather than an
// invented finding. A nil evidence yields no document (JEV did not run).
func buildJEVDoc(ev *jevRunEvidence, now time.Time) (*jevRunDoc, bool) {
	if ev == nil {
		return nil, false
	}
	doc := &jevRunDoc{
		TaskID:     ev.TaskID,
		RunID:      ev.RunID,
		Status:     string(ev.Result.Status),
		Findings:   jevFindingDocs(ev.Result.Findings),
		Metrics:    jevMetricsDocFrom(ev.Metrics),
		RecordedAt: now.UTC(),
	}
	if ev.Evidence != nil {
		doc.FailClosed = ev.Evidence.FailClosed
		doc.Reason = ev.Evidence.Reason
		doc.Summary = ev.Evidence.Summary
	}
	if doc.Status == "" {
		// An analyzer error carries a zero Result; record the status explicitly
		// rather than an empty string. A fail-closed result records its reason
		// above, so the artifact never reads as a clean pass.
		doc.Status = "ERROR"
	}
	if doc.FailClosed && strings.TrimSpace(doc.Summary) == "" {
		// A fail-closed result always carries an explanation: keep the artifact
		// self-describing even when no summary was recorded.
		if strings.TrimSpace(doc.Reason) != "" {
			doc.Summary = doc.Reason
		}
	}
	return doc, true
}

// jevMetricsDocFrom projects a JEV metrics record into the artifact/report shape.
// It returns nil for a nil record, so a run without metrics omits the field and an
// existing artifact/report stays compatible.
func jevMetricsDocFrom(m *perf.JEV) *jevMetricsDoc {
	if m == nil {
		return nil
	}
	return &jevMetricsDoc{
		Invocations:      m.Invocations,
		DurationMS:       m.TotalMS,
		Provider:         m.Provider,
		Model:            m.Model,
		ToolCalls:        m.ToolCalls,
		Findings:         m.Findings,
		BlockingFindings: m.BlockingFindings,
	}
}

// jevReportSection builds the report's JEV section from the persisted document,
// pointing at the artifact path. It returns nil when JEV did not run, so a report
// without JEV is unchanged.
func jevReportSection(doc *jevRunDoc, artifactPath string) *jevReportDoc {
	if doc == nil {
		return nil
	}
	return &jevReportDoc{
		TaskID:       doc.TaskID,
		RunID:        doc.RunID,
		Status:       doc.Status,
		FailClosed:   doc.FailClosed,
		Reason:       doc.Reason,
		Findings:     doc.Findings,
		ArtifactPath: artifactPath,
		Metrics:      doc.Metrics,
	}
}

// jevFindingDocs projects JEV findings into the artifact/report shape. It preserves
// severity, file, line, category, finding, and evidence. An empty result yields a
// non-nil empty slice so a finding-free result serializes as [] rather than null.
func jevFindingDocs(findings []jev.Finding) []jevFindingDoc {
	out := make([]jevFindingDoc, 0, len(findings))
	for _, f := range findings {
		out = append(out, jevFindingDoc{
			Severity: string(f.Severity),
			File:     f.Path,
			Line:     f.Line,
			Category: f.Category,
			Finding:  f.Message,
			Evidence: f.Evidence,
		})
	}
	return out
}

// persistedJEVDoc writes the JEV artifact into the run directory and returns the
// document written (or nil when JEV did not run), so the report can reference the
// same result. Persistence is best-effort, like the other run artifacts: a write
// failure never changes the run's outcome.
func persistedJEVDoc(rn jevArtifactWriter, ev *jevRunEvidence) *jevRunDoc {
	doc, ok := buildJEVDoc(ev, time.Now())
	if !ok {
		return nil
	}
	_ = rn.WriteJEVArtifact(doc)
	return doc
}

// jevArtifactRef returns the repository-relative path of the persisted JEV
// artifact, so the report points a reader at the same file that was written. It
// returns "" when no run directory is available, so a report degrades cleanly. It
// names the run directory's jev.json (see run.WriteJEVArtifact) and never invents
// a path that was not written.
func jevArtifactRef(dir string, rn interface{ Dir() string }) string {
	if rn == nil {
		return ""
	}
	runDir := rn.Dir()
	if runDir == "" {
		return ""
	}
	path := runDir + "/" + jevArtifactName
	if rel := relDir(dir, path); rel != "" {
		return rel
	}
	return path
}

// jevArtifactWriter is the run handle's JEV-persistence surface. It is an
// interface so the persistence call is explicit and testable and so the JEV
// artifact path can never be confused with workflow-state persistence.
type jevArtifactWriter interface {
	WriteJEVArtifact(v any) error
	Dir() string
}

// renderJEVSection renders the human-readable JEV section for the run report,
// distinct from the validation and review sections. It names the evidence as JEV
// diagnostic evidence, renders its status, metrics, and any findings, and points
// at the persisted artifact. It renders nothing when JEV did not run.
func renderJEVSection(doc *jevRunDoc, artifactPath string) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## JEV\n\n")
	b.WriteString("JEV diagnostic evidence (read-only; not workflow state).\n\n")
	b.WriteString("- Status: `" + doc.Status + "`\n")
	if doc.FailClosed {
		reason := strings.TrimSpace(doc.Reason)
		if reason == "" {
			reason = "JEV analysis was not a clean pass"
		}
		b.WriteString("- Fail closed: " + reason + "\n")
	}
	if s := strings.TrimSpace(doc.Summary); s != "" {
		b.WriteString("- Summary: " + s + "\n")
	}
	writeJEVMetrics(&b, doc.Metrics)
	if len(doc.Findings) == 0 {
		b.WriteString("- Findings: none\n")
	} else {
		b.WriteString("- Findings:\n")
		for _, f := range doc.Findings {
			location := f.File
			if f.Line > 0 {
				location = f.File + ":" + strconv.Itoa(f.Line)
			}
			if location != "" {
				b.WriteString("  - " + f.Severity + " " + location + " — " + f.Finding + "\n")
			} else {
				b.WriteString("  - " + f.Severity + " — " + f.Finding + "\n")
			}
		}
	}
	if p := strings.TrimSpace(artifactPath); p != "" {
		b.WriteString("- Artifact: `" + p + "`\n")
	}
	return b.String()
}

// writeJEVMetrics renders the JEV invocation metrics as labeled diagnostics,
// kept apart from the validation and review accounting so JEV cost/time is
// distinguishable from IMPLEMENT/FIX. Nothing is rendered when metrics are
// absent, and nothing here asserts PASS or FAIL.
func writeJEVMetrics(b *strings.Builder, m *jevMetricsDoc) {
	if m == nil {
		return
	}
	b.WriteString("- Metrics (diagnostic):\n")
	b.WriteString("  - Invocations: " + strconv.Itoa(m.Invocations) + "\n")
	b.WriteString("  - Duration: " + humanMS(m.DurationMS) + "\n")
	if line := strings.TrimSpace(m.Provider + " " + m.Model); line != "" {
		b.WriteString("  - Provider/model: " + line + "\n")
	}
	b.WriteString("  - Tool calls: " + strconv.Itoa(m.ToolCalls) + "\n")
	b.WriteString("  - Findings: " + strconv.Itoa(m.Findings) + "\n")
	b.WriteString("  - Blocking findings: " + strconv.Itoa(m.BlockingFindings) + "\n")
}

// humanMS renders milliseconds in the shared performance vocabulary, reusing the
// perf package's single formatting so a JEV metric reads like every other timing.
func humanMS(ms int64) string {
	var b strings.Builder
	perf.WriteJEVDuration(&b, ms)
	return b.String()
}
