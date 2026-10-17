package validate

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/commandpolicy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// TestGeneratedValidationCommandsAreHarnessExecutable guards the generated
// configuration: every validation command it ships must be executable by the agent
// command harness, which rejects shell metacharacters and any program SOP does not
// classify SAFE (read-only gofmt is allow-listed explicitly). A generated command the
// harness refuses cannot be demonstrated by the agent, so the already-satisfied
// contract becomes impossible to meet - the defect this test exists to prevent.
func TestGeneratedValidationCommandsAreHarnessExecutable(t *testing.T) {
	c, err := config.Parse([]byte(config.Template("demo")))
	if err != nil {
		t.Fatalf("generated template does not parse: %v", err)
	}
	checks := Checks(c.Validation)
	if len(checks) == 0 {
		t.Fatal("generated template defines no validation checks")
	}
	for _, check := range checks {
		argv, err := toolharness.SplitCommand(check.Command)
		if err != nil {
			t.Errorf("generated %s command %q is not harness-executable: %v", check.Category, check.Command, err)
			continue
		}
		if argv[0] == "gofmt" {
			continue // the harness allow-lists read-only gofmt explicitly
		}
		if class, cerr := commandpolicy.Classify(argv); cerr != nil || class != commandpolicy.Safe {
			t.Errorf("generated %s command %q is not SAFE under the command policy: class=%s err=%v", check.Category, check.Command, class, cerr)
		}
	}
}
