package toolharness

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNoTestsMatchedDetection(t *testing.T) {
	cases := []struct {
		argv   []string
		output string
		want   bool
	}{
		{[]string{"go", "test", "./pkg", "-run", "Nothing", "-v"}, "testing: warning: no tests to run\nok\tpkg\t0.001s", true},
		{[]string{"go", "test", "./pkg", "-run", "Nothing"}, "ok\tpkg\t0.001s [no tests to run]", true},
		{[]string{"go", "test", "./pkg", "-run", "Something", "-v"}, "=== RUN   TestSomething\n--- PASS: TestSomething\nok\tpkg", false},
		{[]string{"go", "test", "./..."}, "ok\tpkg\t[no tests to run]", false}, // not targeted
		{[]string{"go", "build", "./..."}, "=== RUN", false},
	}
	for _, c := range cases {
		if got := noTestsMatched(c.argv, c.output); got != c.want {
			t.Errorf("noTestsMatched(%v, %q) = %t, want %t", c.argv, c.output, got, c.want)
		}
	}
}

// TestTargetedGoTestZeroMatchIsSurfaced proves a targeted go test that matched no
// tests annotates its result, so exit 0 cannot be read as a passing selected suite.
func TestTargetedGoTestZeroMatchIsSurfaced(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir+"/go.mod", "module fixture\n\ngo 1.21\n")
	writeFixture(t, dir+"/only_test.go", "package fixture\n\nimport \"testing\"\n\nfunc TestPresent(t *testing.T) {}\n")
	h := New(dir, Config{CommandTimeout: 60 * time.Second, MaxOutputBytes: 1 << 20}, nil)
	out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go test ./... -run TestAbsent -v"})
	if err != nil {
		t.Fatalf("run_command: %v", err)
	}
	if !strings.Contains(out, noTestMatchDiagnostic) {
		t.Fatalf("zero-match targeted go test not surfaced: %q", out)
	}
}
