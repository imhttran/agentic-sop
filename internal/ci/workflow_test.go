package ci

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sdlc/internal/testrunner"
)

func TestRenderTriggersAndCheckout(t *testing.T) {
	out, err := Render(testrunner.Commands{UnitTest: "go test ./..."})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	for _, want := range []string{"on:\n  push:\n  pull_request:\n", "actions/checkout@v4", "runs-on: ubuntu-latest"} {
		if !strings.Contains(out, want) {
			t.Errorf("workflow missing %q:\n%s", want, out)
		}
	}
}

func TestRenderStepsInOrder(t *testing.T) {
	out, err := Render(testrunner.Commands{
		Build:    "go build ./...",
		UnitTest: "go test ./...",
		Lint:     "go vet ./...",
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	iBuild := strings.Index(out, "name: build")
	iUnit := strings.Index(out, "name: unit_test")
	iLint := strings.Index(out, "name: lint")
	if iBuild < 0 || iUnit < 0 || iLint < 0 || !(iBuild < iUnit && iUnit < iLint) {
		t.Errorf("steps out of order or missing:\n%s", out)
	}
	if !strings.Contains(out, "run: go build ./...") || !strings.Contains(out, "run: go vet ./...") {
		t.Errorf("commands missing:\n%s", out)
	}
}

func TestRenderOmitsUnconfigured(t *testing.T) {
	out, err := Render(testrunner.Commands{UnitTest: "npm test"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if strings.Contains(out, "name: build") || strings.Contains(out, "name: lint") {
		t.Errorf("unconfigured commands should be omitted:\n%s", out)
	}
	if strings.Count(out, "      - name:") != 1 {
		t.Errorf("expected exactly one step:\n%s", out)
	}
}

func TestRenderDeterministic(t *testing.T) {
	cmd := testrunner.Commands{Build: "b", UnitTest: "u", DockerBuild: "d"}
	first, err := Render(cmd)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	second, _ := Render(cmd)
	if first != second {
		t.Error("Render is not deterministic")
	}
}

func TestRenderEmptyConfigurationIsError(t *testing.T) {
	if _, err := Render(testrunner.Commands{}); err == nil {
		t.Error("expected error for an empty configuration")
	}
}

func TestRenderRejectsNewlineCommand(t *testing.T) {
	if _, err := Render(testrunner.Commands{UnitTest: "go test ./...\ngo build ./..."}); err == nil {
		t.Error("expected error for a command containing a newline")
	}
}
