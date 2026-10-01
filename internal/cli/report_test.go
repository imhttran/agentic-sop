package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeRunArtifact creates runs/<id>/<name> and sets its mtime, so latestRun's
// ordering can be controlled deterministically.
func writeRunArtifact(t *testing.T, runsRoot, id, name string, mod time.Time) {
	t.Helper()
	path := filepath.Join(runsRoot, id, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

// TestLatestRunConsidersPromptRuns proves `sop report` with no argument selects the
// newest run of EITHER kind: a read-only prompt run (runs/prompts/<id>/metadata.json)
// is a candidate alongside task runs (runs/<id>/report.json).
func TestLatestRunConsidersPromptRuns(t *testing.T) {
	base := time.Now().Add(-time.Hour)

	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "prompt newer than task",
			setup: func(t *testing.T, root string) {
				writeRunArtifact(t, root, "T001", "report.json", base)
				writeRunArtifact(t, root, filepath.Join(promptsDirName, "prompt-1"), "metadata.json", base.Add(30*time.Minute))
			},
			want: filepath.Join(promptsDirName, "prompt-1"),
		},
		{
			name: "task newer than prompt",
			setup: func(t *testing.T, root string) {
				writeRunArtifact(t, root, filepath.Join(promptsDirName, "prompt-1"), "metadata.json", base)
				writeRunArtifact(t, root, "T002", "report.json", base.Add(30*time.Minute))
			},
			want: "T002",
		},
		{
			name: "prompt only",
			setup: func(t *testing.T, root string) {
				writeRunArtifact(t, root, filepath.Join(promptsDirName, "prompt-1"), "metadata.json", base)
			},
			want: filepath.Join(promptsDirName, "prompt-1"),
		},
		{
			name: "the prompts directory itself is not a run",
			setup: func(t *testing.T, root string) {
				writeRunArtifact(t, root, "T003", "report.json", base)
				if err := os.MkdirAll(filepath.Join(root, promptsDirName), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: "T003",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)
			got, err := latestRun(root)
			if err != nil {
				t.Fatalf("latestRun: %v", err)
			}
			if got != tc.want {
				t.Fatalf("latestRun = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestReportNoArgRendersPromptRun proves the whole path: after a read-only prompt,
// `sop report` with no argument renders the prompt result document.
func TestReportNoArgRendersPromptRun(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: "ok"}
	if code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "review this"); code != exitOK {
		t.Fatalf("setup prompt failed: %s", stderr)
	}

	code, out, stderr := runCLI(t, dir, "report")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"Kind: prompt", "Capability: review", "Status: completed"} {
		if !strings.Contains(out, want) {
			t.Errorf("no-arg report missing %q:\n%s", want, out)
		}
	}
}
