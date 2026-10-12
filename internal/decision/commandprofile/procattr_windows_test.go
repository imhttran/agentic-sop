//go:build windows

package commandprofile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
