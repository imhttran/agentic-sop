package security

import (
	"runtime"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// The per-platform pin applies only to the platform it names: the running platform
// gets the pinned executor, and a platform with no entry keeps the plain executor.
func TestBuildAdapterUsesRunningPlatformPin(t *testing.T) {
	good := strings.Repeat("b", 64)
	base := config.SecurityAdapterConfig{ID: "gitleaks", Category: "secrets", Protocol: config.SecurityProtocolBuiltin, Command: []string{"gitleaks"}}

	mine := base
	mine.BinarySHA256ByPlatform = map[string]string{runtime.GOOS + "/" + runtime.GOARCH: good}
	a, err := BuildAdapter(mine, BuildOptions{Executor: &countingExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.(*gitleaksAdapter).exec.(pinnedExecutor); !ok {
		t.Fatal("a pin for the running platform must wrap the executor")
	}

	other := base
	other.BinarySHA256ByPlatform = map[string]string{"windows/amd64": good}
	if a, err = BuildAdapter(other, BuildOptions{Executor: &countingExecutor{}}); err != nil {
		t.Fatal(err)
	}
	if _, wrapped := a.(*gitleaksAdapter).exec.(pinnedExecutor); wrapped {
		t.Fatal("a pin for another platform must not wrap the executor")
	}
}
