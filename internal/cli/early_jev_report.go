package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/jev"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// Early JEV checkpoint reporting (P3-012).
//
// The early checkpoints (task triage and pre-execution) persist a diagnostic
// artifact per checkpoint (runpkg.EarlyArtifact, written to early-jev.json and
// early-jev-history.jsonl). This renderer turns those artifacts into a concise,
// deterministic, human-readable block that:
//
//   - LABELS which checkpoint produced each block (TRIAGE vs PRE_EXECUTION),
//     using the shared activity stage vocabulary so the report distinguishes
//     triage/pre-execution evidence from the quality (JEV/QUALITY) evidence;
//   - shows each finding's severity and, when the analysis stated one, the
//     confidence — both display-only;
//   - emits nothing when no early checkpoint ran, so a disabled JEV adds no
//     noise;
//   - never transitions task state, never re-runs JEV, and never produces a
//     gate/policy value. It is a read-only projection of already-persisted
//     evidence, exactly like the JEV011 report renderer.

// earlyArtifactRenderFileName is the run-directory early-checkpoint artifact the
// renderer reads. It matches the run package's early-jev.json convention (see
// run.earlyJEVArtifactFileName) so the report and the persisted artifact agree.
const earlyArtifactRenderFileName = "early-jev.json"

// earlyStageForCheckpoint maps a persisted checkpoint to the shared activity
// stage vocabulary (PRD §10.2.4), so triage, pre-execution, and quality evidence
// are labelled with one vocabulary and can never be conflated. An unknown
// checkpoint maps to "" so the caller can fall back to the raw value rather than
// silently mislabelling it.
func earlyStageForCheckpoint(c runpkg.Checkpoint) string {
	switch c {
	case runpkg.CheckpointTaskTriage:
		return activity.StageTriage
	case runpkg.CheckpointPreExecution:
		return activity.StagePreExecution
	default:
		return ""
	}
}

// renderEarlyArtifacts renders one block per persisted early-checkpoint artifact,
// labelled by checkpoint, so a reader can tell triage evidence from
// pre-execution evidence. It returns "" when there are no artifacts, so a
// disabled JEV adds no noise. Each block is derived entirely from persisted data
// and the output is deterministic for a given artifact.
func renderEarlyArtifacts(arts []runpkg.EarlyArtifact) string {
	if len(arts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, a := range arts {
		renderEarlyArtifact(&b, a)
	}
	return b.String()
}

// renderEarlyArtifact renders one early-checkpoint evidence block. The block
// names its checkpoint (TRIAGE / PRE_EXECUTION), states the status, and lists the
// findings with severity and confidence. A provider failure is rendered
// distinctly and never as an empty PASS.
func renderEarlyArtifact(b *strings.Builder, a runpkg.EarlyArtifact) {
	label := earlyStageForCheckpoint(a.Checkpoint)
	if label == "" {
		label = strings.ToUpper(string(a.Checkpoint))
	}
	fmt.Fprintf(b, "%s:\n", label)

	if a.ProviderFailed {
		// A provider failure is not a finding and never a pass.
		fmt.Fprintf(b, "  %-11s %s\n", "Status:", "UNAVAILABLE (provider failure recorded)")
		if r := strings.TrimSpace(a.PolicyReason); r != "" {
			fmt.Fprintf(b, "  %-11s %s\n", "Reason:", r)
		}
		return
	}

	fmt.Fprintf(b, "  %-11s %s\n", "Status:", earlyEvidenceStatusLabel(a.Evidence.Status))
	if conf, stated := earlyConfidence(a.Evidence); stated {
		fmt.Fprintf(b, "  %-11s %s\n", "Confidence:", conf)
	}
	if len(a.Evidence.Items) == 0 {
		fmt.Fprintf(b, "  %-11s %s\n", "Findings:", "none")
		return
	}
	fmt.Fprintf(b, "  Findings: %d\n", len(a.Evidence.Items))
	for _, item := range a.Evidence.Items {
		writeEarlyEvidenceItem(b, item)
	}
}

// writeEarlyEvidenceItem renders one structured evidence item with its severity
// and (when present) its typed category. The detail is included only when it is a
// single, non-empty line, so a stray newline cannot break the block. It never
// renders raw prompts or model output.
func writeEarlyEvidenceItem(b *strings.Builder, item jev.EvidenceItem) {
	sev := strings.ToUpper(strings.TrimSpace(string(item.Severity)))
	if sev == "" {
		sev = "UNKNOWN"
	}
	line := fmt.Sprintf("  - [%s]", sev)
	if cat := strings.ToLower(strings.TrimSpace(string(item.Category))); cat != "" {
		line += " " + cat
	}
	if d := earlyOneLine(item.Detail); d != "" {
		line += " — " + d
	}
	b.WriteString(line + "\n")
}

// earlyOneLine collapses a field to a single trimmed line, so a stray newline
// cannot break the one-line-per-finding contract. It matches the activity
// stream's oneLine behavior so both renderers treat text identically.
func earlyOneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// earlyEvidenceStatusLabel renders an evidence status verbatim, preserving PRD
// terminology. An empty or unknown status renders as ERROR rather than implying
// success: only the known evidence statuses are shown as themselves.
func earlyEvidenceStatusLabel(s jev.EvidenceStatus) string {
	v := strings.ToUpper(strings.TrimSpace(string(s)))
	switch v {
	case "PASS", "FINDINGS", "INCOMPLETE", "ERROR":
		return v
	default:
		return "ERROR"
	}
}

// loadEarlyArtifacts reads the persisted early-checkpoint artifact for a run
// directory, if any. It returns false when no artifact exists, so a disabled
// checkpoint adds no report noise. It is read-only: it never writes, re-runs
// analysis, or derives policy.
func loadEarlyArtifacts(runDir string) ([]runpkg.EarlyArtifact, bool) {
	path := filepath.Join(runDir, earlyArtifactRenderFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var a runpkg.EarlyArtifact
	if err := json.Unmarshal(data, &a); err != nil {
		// A malformed artifact is not evidence: render nothing rather than a
		// fabricated block.
		return nil, false
	}
	if err := a.Validate(); err != nil {
		return nil, false
	}
	return []runpkg.EarlyArtifact{a}, true
}

// writeEarlyReport renders the early-checkpoint evidence block for a run, if any,
// to w. It writes nothing when no early checkpoint ran, so a disabled JEV leaves
// the CLI output exactly as it was. It is human-readable only and is never read
// back to drive a decision.
func writeEarlyReport(w io.Writer, runDir string) {
	arts, ok := loadEarlyArtifacts(runDir)
	if !ok {
		return
	}
	block := renderEarlyArtifacts(arts)
	if block == "" {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprint(w, block)
}
