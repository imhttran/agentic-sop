package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

const codeRetrievalMarker = "Repository pointers ranked by BM25"

// widgetRepo writes a tiny Go module whose symbol matches runTaskFile's title ("Add widget").
// The symbol is a single token on purpose: the BM25 tokenizer does not split camelCase,
// so "AddWidget" would not match the query term "widget".
func widgetRepo(t *testing.T, configYAML string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeFile(t, dir, "go.mod", "module example.com/w\n\ngo 1.22\n")
	writeFile(t, dir, "widget.go", "package w\n\n// Widget adds a widget.\nfunc Widget() {}\n")
	writeConfig(t, dir, configYAML)
	return dir
}

const codeRetrievalBaseConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n"

// TestRunCodeRetrievalFlag proves context.code_retrieval is off by default (no pointers,
// no telemetry) and, when on, adds the ranked pointers (with the symbol's file) to the
// implement context and records code-retrieval.json.
func TestRunCodeRetrievalFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		on   bool
	}{
		{"default off", codeRetrievalBaseConfig, false},
		{"opted in", codeRetrievalBaseConfig + "context:\n  code_retrieval: true\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := widgetRepo(t, tc.yaml)
			a := &memoryAgent{plan: validPlanJSON}
			if code, _, se := runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md"); code != exitOK {
				t.Fatalf("run: %s", se)
			}
			if len(a.implInputs) == 0 {
				t.Fatal("no implement invocation recorded")
			}
			in := a.implInputs[0]
			if got := strings.Contains(in, codeRetrievalMarker) && strings.Contains(in, "w.Widget in widget.go"); got != tc.on {
				t.Errorf("BM25 pointers in implement context = %v, want %v:\n%s", got, tc.on, in)
			}
			m, _ := filepath.Glob(filepath.Join(dir, ".agent-sdlc", "runs", "*", "code-retrieval.json"))
			if (len(m) > 0) != tc.on {
				t.Errorf("code-retrieval.json written = %v, want %v", len(m) > 0, tc.on)
			}
		})
	}
}

// TestCodeRetrievalItemsBoundedStableAndFailOpen proves the item lists at most
// codeRetrievalTopK hits, is identical across calls, and an error yields no item.
func TestCodeRetrievalItemsBoundedStableAndFailOpen(t *testing.T) {
	dir := widgetRepo(t, codeRetrievalBaseConfig)
	for _, pkg := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		writeFileT(t, dir, pkg+"/widget.go", "package "+pkg+"\n\nfunc Widget() {}\n")
	}
	cfg := config.Config{ContextEfficiency: config.ContextEfficiency{CodeRetrieval: true}}
	spec := &taskfile.Spec{ID: "T001", Title: "Add widget"}
	d := deps{getwd: func() (string, error) { return dir, nil }}

	first := codeRetrievalItems(cfg, spec, "", nil, d)
	if len(first) != 1 {
		t.Fatalf("want one retrieval item, got %d", len(first))
	}
	// Nine widget.go files and nine Widget symbols match, so the bound must cut to exactly k.
	if n := strings.Count(first[0].Text, "\n"); n != codeRetrievalTopK {
		t.Errorf("want exactly %d hits, got %d:\n%s", codeRetrievalTopK, n, first[0].Text)
	}
	if again := codeRetrievalItems(cfg, spec, "", nil, d); again[0].Text != first[0].Text {
		t.Errorf("retrieval must be deterministic:\n%s\n---\n%s", first[0].Text, again[0].Text)
	}

	broken := deps{getwd: func() (string, error) { return "", errors.New("no cwd") }}
	if got := codeRetrievalItems(cfg, spec, "", nil, broken); got != nil {
		t.Errorf("an error must yield no item, got %v", got)
	}
}
