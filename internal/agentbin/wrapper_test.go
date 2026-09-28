package agentbin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBootstrapWrapperNeverSelfBuilds proves scripts/sop-ollama-agent.sh resolves
// the installed known-good binary and, when none is installed, fails with an
// actionable message instead of compiling candidate working-tree source. This pin
// is what keeps a broken working copy repairable by the agent. S10.
func TestBootstrapWrapperNeverSelfBuilds(t *testing.T) {
	script := filepath.Join("..", "..", "scripts", "sop-ollama-agent.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("bootstrap wrapper not found: %v", err)
	}

	emptyHome := t.TempDir()
	cmd := exec.Command("sh", script)
	// A clean environment: no override, and a home directory holding no binary.
	cmd.Env = []string{
		"SOP_OLLAMA_AGENT_HOME=" + emptyHome,
		"PATH=" + os.Getenv("PATH"),
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader("{}")

	if err := cmd.Run(); err == nil {
		t.Fatalf("wrapper succeeded with no installed binary; stdout=%q", stdout.String())
	}
	msg := stderr.String()
	if !strings.Contains(msg, "install-sop-ollama-agent.sh") {
		t.Errorf("stderr = %q, want an actionable install hint", msg)
	}
	if !strings.Contains(msg, "never compiles candidate source") {
		t.Errorf("stderr = %q, want it to state that no build is attempted", msg)
	}

	// The wrapper built nothing: the (empty) install directory stays empty.
	entries, err := os.ReadDir(emptyHome)
	if err != nil {
		t.Fatalf("read install dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("wrapper wrote %d entries into the install dir; it must not build", len(entries))
	}
}
