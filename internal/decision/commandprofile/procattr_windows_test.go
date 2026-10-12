//go:build windows

package commandprofile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
)

// TestAdapterConnectorKillsProcessGroupWindows proves that on Windows a timed-out
// adapter's descendant does not survive: the adapter is assigned to a
// kill-on-close Job Object, so terminating it reclaims the whole tree.
func TestAdapterConnectorKillsProcessGroupWindows(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "descendant-marker")
	t.Setenv("ADAPTER_MARKER", marker)
	c := testConnector(t, "grandchild", 500*time.Millisecond)
	if _, err := c.Decide(context.Background(), req()); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout classification", err)
	}
	time.Sleep(1500 * time.Millisecond) // give a surviving descendant time to write
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a descendant process survived the Windows job-object kill")
	}
}

// TestAdapterJobHandleNoLeak proves the job handle is released on the NORMAL
// completion path (no leak per successful invocation).
func TestAdapterJobHandleNoLeak(t *testing.T) {
	t.Setenv("ADAPTER_CHOICE", "high")
	c := testConnector(t, "success", 10*time.Second)
	if _, err := c.Decide(context.Background(), decision.Request{UseCase: "complexity"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	jobMu.Lock()
	n := len(jobFor)
	jobMu.Unlock()
	if n != 0 {
		t.Fatalf("job handles leaked after normal completion: %d", n)
	}
}
