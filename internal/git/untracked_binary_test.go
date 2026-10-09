package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiffAllSummarizesUntrackedBinary proves a generated binary (a NUL-containing
// file, e.g. a compiled program the agent produced with a bare `go build`) is
// summarized in the diff rather than embedded as raw bytes — so it cannot inflate
// the diff, or the model context that consumes it, with megabytes of content.
func TestDiffAllSummarizesUntrackedBinary(t *testing.T) {
	dir := initRepo(t)
	blob := append([]byte("ELF\x00\x00binary-payload"), make([]byte, 4096)...)
	if err := os.WriteFile(filepath.Join(dir, "app.bin"), blob, 0o755); err != nil {
		t.Fatal(err)
	}

	diff, err := New(dir).DiffAll(context.Background(), ".agent-sdlc", "docs/reports")
	if err != nil {
		t.Fatalf("DiffAll failed: %v", err)
	}
	if !strings.Contains(diff, "app.bin") {
		t.Errorf("diff should mention the binary path:\n%s", diff)
	}
	if !strings.Contains(diff, "Binary file app.bin added") {
		t.Errorf("diff should summarize the binary, not embed it:\n%s", diff)
	}
	if strings.Contains(diff, "binary-payload") {
		t.Errorf("diff embedded binary content:\n%s", diff)
	}
}
