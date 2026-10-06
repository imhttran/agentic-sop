package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// capturedEvidenceMarker prefixes the SOP-written evidence section. The section is
// always appended last, so re-injecting truncates from the marker and rewrites it —
// idempotent across the bounded IMPLEMENT/FIX loop.
const capturedEvidenceMarker = "<!-- SOP captured command evidence -->"

// injectCapturedEvidence writes the exact command evidence SOP captured for a task
// (command, cwd, exit, and the harness's captured output) into each required
// deliverable, inside a marked section. It makes a task that requires raw output
// structurally satisfiable: the raw output is present even though the deliverable
// itself is authored by the model, so a summary or placeholder cannot stand in for
// it. It is a no-op when SOP captured no evidence, or the deliverable does not exist.
func injectCapturedEvidence(dir string, reports []string, runDir string) error {
	data, err := os.ReadFile(filepath.Join(runDir, "command-evidence.jsonl"))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return nil
	}
	section := buildCapturedEvidenceSection(string(data))
	for _, rel := range reports {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		existing, err := os.ReadFile(path)
		if err != nil {
			continue // deliverable not created yet
		}
		body := strings.TrimRight(string(existing), "\n")
		if i := strings.Index(body, capturedEvidenceMarker); i >= 0 {
			body = strings.TrimRight(body[:i], "\n")
		}
		if err := os.WriteFile(path, []byte(body+"\n\n"+section+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// buildCapturedEvidenceSection renders the JSONL evidence records as a Markdown
// section. It never invents output: a record that cannot be parsed is skipped.
func buildCapturedEvidenceSection(jsonl string) string {
	var b strings.Builder
	b.WriteString(capturedEvidenceMarker + "\n")
	b.WriteString("## Captured command evidence (SOP-recorded)\n\n")
	b.WriteString("Exact commands, working directory, exit code and captured output recorded by SOP for this task. This section is written by SOP, not the implementation agent; it is the raw evidence the task requires.\n")
	for _, line := range strings.Split(strings.TrimSpace(jsonl), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd"`
			Exit    int    `json:"exit"`
			Output  string `json:"output"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		fmt.Fprintf(&b, "\n### `%s`\n\n- cwd: `%s`\n- exit code: %d\n\n```\n%s\n```\n", rec.Command, rec.Cwd, rec.Exit, rec.Output)
	}
	return b.String()
}
