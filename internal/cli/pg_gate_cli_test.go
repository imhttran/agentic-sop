package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// pgCLIProject builds a disposable SOP project that is ALSO a Go module with a
// real (disposable) integration-test fixture. The integration command is a genuine
// `go test -tags=postgres …`, classified via the real integration-test marker; no
// printf/comment workaround is used.
func pgCLIProject(t *testing.T, skip bool) string {
	t.Helper()
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n  integration:\n    - \"go test -tags=postgres ./pgfix/\"\n")
	writePlanJSON(t, dir, maxTasksPlanJSON)
	writeFile(t, dir, "go.mod", "module pgfixture\n\ngo 1.21\n")
	if err := os.MkdirAll(filepath.Join(dir, "pgfix"), 0o755); err != nil {
		t.Fatalf("mkdir pgfix: %v", err)
	}
	body := "package pgfix\n\nimport \"testing\"\n\nfunc TestMigrate(t *testing.T) {\n"
	if skip {
		body += "\tt.Skip(\"no database in this fixture\")\n"
	}
	body += "}\n"
	writeFile(t, dir, "pgfix/migrate_test.go", body)
	return dir
}

const pgMigrationDiff = "diff --git a/db/migrations/001.sql b/db/migrations/001.sql\n" +
	"--- a/db/migrations/001.sql\n+++ b/db/migrations/001.sql\n@@ -0,0 +1 @@\n+select 1;\n"

func pgGateDecision(t *testing.T, dir, id string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", id, "gate.json"))
	if err != nil {
		t.Fatalf("read gate.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse gate.json: %v", err)
	}
	d, _ := m["Decision"].(string)
	return d
}

func pgTaskStatus(t *testing.T, dir, id string) domain.TaskStatus {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return task.Status
}

func TestPgIntegrationAllSkippedFailsGateCLI(t *testing.T) {
	dir := pgCLIProject(t, true)
	a := &fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}
	_, stdout, _ := runInjectedCLI(t, dir, pgMigrationDiff, a, "run", "--max-tasks", "1")

	if d := pgGateDecision(t, dir, "S001"); d == "PASS" {
		t.Fatalf("gate PASSed with an all-skipped PostgreSQL integration check; stdout=%s", stdout)
	}
	if s := pgTaskStatus(t, dir, "S001"); s == domain.LOCAL_DONE {
		t.Fatalf("task reached LOCAL_DONE despite the PostgreSQL gate failure")
	}
}

func TestPgIntegrationExecutedPassesGateCLI(t *testing.T) {
	dir := pgCLIProject(t, false)
	a := &fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}
	_, stdout, stderr := runInjectedCLI(t, dir, pgMigrationDiff, a, "run", "--max-tasks", "1")

	if d := pgGateDecision(t, dir, "S001"); d != "PASS" {
		t.Fatalf("gate = %q, want PASS with an executed PostgreSQL integration test; stderr=%s stdout=%s", d, stderr, stdout)
	}
	if s := pgTaskStatus(t, dir, "S001"); s != domain.LOCAL_DONE {
		t.Fatalf("task status = %s, want LOCAL_DONE", s)
	}
	if strings.Contains(stdout, "required PostgreSQL integration") {
		t.Fatalf("unexpected PostgreSQL gate failure: %s", stdout)
	}
}
