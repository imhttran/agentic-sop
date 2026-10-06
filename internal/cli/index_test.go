package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunIndexWritesArtifact proves the index command builds and writes the canonical
// index without invoking a model.
func TestRunIndexWritesArtifact(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.21\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	a := &planCountAgent{}

	code, out, errOut := runCLIWithAgent(t, dir, a, "index")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if a.calls != 0 {
		t.Errorf("the index invoked the model %d time(s); it must not", a.calls)
	}
	if !strings.Contains(out, "Index identity:") || !strings.Contains(out, "modules: 1") {
		t.Errorf("stdout = %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "context", "index.json")); err != nil {
		t.Errorf("index artifact missing: %v", err)
	}
}
