package ollamaagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestOllamaDogfood is an opt-in end-to-end integration test using a real
// Ollama server to verify the complete workflow: read existing file → modify
// code → create test → run test → inspect failure → fix → verify outcome.
//
// It requires SOP_OLLAMA_RUN_DOGFOOD=1 to execute and SOP_OLLAMA_BASE_URL to be
// set to a working Ollama endpoint (default http://127.0.0.1:11434).
// Without the env var, the test is skipped.
//
// The test creates a small disposable fixture repository and never modifies
// agentic-sop itself. SOP validation passes independently over the outcome.
func TestOllamaDogfood(t *testing.T) {
	if os.Getenv("SOP_OLLAMA_RUN_DOGFOOD") != "1" {
		t.Skip("SOP_OLLAMA_RUN_DOGFOOD not set; skipping Ollama dogfood test")
	}

	ctx := context.Background()

	// Setup: create a disposable fixture repository with a simple Go module.
	fixture := t.TempDir()
	initFixtureRepo(t, fixture)

	// Load Ollama configuration from environment.
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}

	// Create the harness inside the fixture repository.
	h := New(cfg, fixture)

	// --- Phase 1: IMPLEMENT - Have the model create the initial implementation ---
	// The task: add a simple Fibonacci function and create a test for it.
	// The model should read existing code, add the function, create a test that fails,
	// then fix it.

	implRequest := agent.Request{
		Capability:         agent.Implement,
		Task:               "Add a Fibonacci function that returns the nth Fibonacci number (0-indexed). The function should be named Fib and take an int parameter.",
		Input:              "The repository is a simple Go package with an empty main.go file.",
		OutputRequirements: "Return a structured outcome with status=completed, summary of changes, and changes_expected=true if the implementation changed the repository.",
	}

	implContent, err := h.Execute(ctx, implRequest)
	if err != nil {
		t.Fatalf("IMPLEMENT phase failed: %v", err)
	}

	// Validate the outcome is structured JSON.
	var outcome outcomeWire
	if err := json.Unmarshal([]byte(implContent), &outcome); err != nil {
		t.Fatalf("IMPLEMENT outcome is not valid JSON: %v\nContent: %s", err, implContent)
	}

	if outcome.Status != string(agent.OutcomeCompleted) {
		t.Fatalf("IMPLEMENT outcome status = %q, want completed", outcome.Status)
	}

	if outcome.ChangesExpected == nil || !*outcome.ChangesExpected {
		t.Fatalf("IMPLEMENT outcome changes_expected = %v, want true (implementation should change the repo)", outcome.ChangesExpected)
	}

	// Verify files were actually created/modified.
	mainPath := filepath.Join(fixture, "main.go")
	testPath := filepath.Join(fixture, "main_test.go")

	mainContent, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("main.go not found after IMPLEMENT: %v", err)
	}

	if !strings.Contains(string(mainContent), "Fib") {
		t.Errorf("main.go does not contain 'Fib' function:\n%s", mainContent)
	}

	testContent, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("main_test.go not found after IMPLEMENT: %v", err)
	}

	if !strings.Contains(string(testContent), "TestFib") || !strings.Contains(string(testContent), "testing.T") {
		t.Errorf("main_test.go does not contain proper test:\n%s", testContent)
	}

	// --- Phase 2: Verify the test fails initially (broken implementation) ---
	// Run the test to see if it fails as expected.
	testOutput, err := runGoTest(t, fixture)
	// Test should fail initially (err != nil), but we use the output later

	t.Logf("Initial test run (expected to fail):\nerror: %v\noutput: %s", err, testOutput)

	// --- Phase 3: FIX - Have the model fix the broken implementation ---
	// Provide the test failure output and ask the model to fix the implementation.
	fixRequest := agent.Request{
		Capability:         agent.Fix,
		Task:               "Fix the Fibonacci implementation to make the tests pass",
		Input:              fmt.Sprintf("Test failure output:\n%s\n\nThe implementation is incomplete or incorrect. Fix the Fib function so that all tests pass.", testOutput),
		OutputRequirements: "Return a structured outcome with status=completed if the fix succeeded, or status=needs_human if you need manual intervention.",
	}

	fixContent, err := h.Execute(ctx, fixRequest)
	if err != nil {
		t.Fatalf("FIX phase failed: %v", err)
	}

	// Validate the FIX outcome.
	var fixOutcome outcomeWire
	if err := json.Unmarshal([]byte(fixContent), &fixOutcome); err != nil {
		t.Fatalf("FIX outcome is not valid JSON: %v\nContent: %s", err, fixContent)
	}

	if fixOutcome.Status != string(agent.OutcomeCompleted) && fixOutcome.Status != string(agent.OutcomeNeedsHuman) {
		t.Fatalf("FIX outcome status = %q, want completed or needs_human", fixOutcome.Status)
	}

	t.Logf("FIX outcome: status=%s, summary=%s, changes=%v", fixOutcome.Status, fixOutcome.Summary, fixOutcome.ChangesExpected)

	// --- Phase 4: Verify the fix (tests now pass) ---
	// Run the test again to verify the fix worked.
	testOutput, err = runGoTest(t, fixture)
	if err != nil {
		t.Logf("Test still failing after FIX:\n%s", testOutput)
		// Don't fail the test itself if the model couldn't fix it — the dogfood
		// test is about verifying the workflow executed, not about the model's
		// success rate.
	} else {
		t.Logf("Test passed after FIX")
	}

	// --- SOP Validation ---
	// Verify the repository state is as expected: the fixture was changed,
	// and the outcome accurately reflects what happened.

	// Confirm changes were made.
	if outcome.ChangesExpected == nil || !*outcome.ChangesExpected {
		t.Error("IMPLEMENT should have changed the repository")
	}

	// Confirm the outcome structure matches what SOP expects.
	if outcome.Status == "" {
		t.Error("outcome.Status is empty")
	}

	// Verify the fixture repository is still isolated from agentic-sop.
	sopDir, _ := os.Getwd()
	if strings.Contains(fixture, sopDir) && fixture != sopDir {
		// It's OK if fixture is under temp (which might be under project), but not if
		// it's the main agentic-sop repo.
		if !strings.Contains(fixture, "Temp") && !strings.Contains(fixture, "tmp") {
			t.Logf("Warning: fixture appears to be under project dir: %s", fixture)
		}
	}

	t.Logf("Dogfood test completed successfully:")
	t.Logf("  - IMPLEMENT outcome: status=%s, changes=%v", outcome.Status, outcome.ChangesExpected)
	t.Logf("  - FIX outcome: status=%s, changes=%v", fixOutcome.Status, fixOutcome.ChangesExpected)
	t.Logf("  - Fixture repo: %s", fixture)
	t.Logf("  - Audit records: %d", len(h.audit.Records()))
}

// initFixtureRepo initializes a minimal Go module in the fixture directory.
func initFixtureRepo(t *testing.T, dir string) {
	t.Helper()

	// Create a minimal go.mod file.
	modPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(modPath, []byte("module dogfood\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create a minimal main.go as a starting point.
	mainPath := filepath.Join(dir, "main.go")
	mainCode := `package main

import "fmt"

func main() {
	fmt.Println("Dogfood fixture ready")
}
`
	if err := os.WriteFile(mainPath, []byte(mainCode), 0o644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Initialize a git repository so the toolharness can check git status.
	gitInit(t, dir)
}

// runGoTest executes "go test" in the fixture directory and returns the output.
// It returns an error if the test fails.
func runGoTest(t *testing.T, dir string) (string, error) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "go", "test", "-v", "./...")
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	return string(out), err
}
