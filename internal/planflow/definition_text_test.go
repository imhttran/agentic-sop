package planflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// TaskDefinitionText must cover every list stageDigest covers, or the attested
// consistency guards can miss a requirement that still lives in those lists.
func TestTaskDefinitionTextCoversDigestedLists(t *testing.T) {
	dir := t.TempDir()
	plan := planner.Plan{Project: "demo", Stages: []planner.Stage{{
		ID:               "T-1",
		Title:            "title",
		Dependencies:     []string{"dep-marker"},
		Requires:         []string{"requires-marker"},
		RequiredEvidence: []string{"evidence-marker"},
	}}}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.DirName, planFileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
	text, ok := TaskDefinitionText(dir, "T-1")
	if !ok {
		t.Fatal("persisted plan was not loaded")
	}
	for _, want := range []string{"dep-marker", "requires-marker", "evidence-marker"} {
		if !strings.Contains(text, want) {
			t.Errorf("definition text missing %q: %q", want, text)
		}
	}
}
