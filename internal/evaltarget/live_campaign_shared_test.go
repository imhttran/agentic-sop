//go:build (darwin || linux) && (llm01_live || llm08_live || llm02_live)

package evaltarget

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/approval"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// Shared opt-in live-campaign helpers.
//
// The LLM01, LLM08 hidden-context, and LLM02 disclosure campaigns run the same
// bounded live seam: the test binary is re-executed as a child under the opt-in
// macOS Seatbelt live profile, the child makes the fixed loopback model request,
// and the parent only reads the artifact. These helpers are the seam the
// campaigns share, so none of them reimplements isolation, probe evidence, or
// disposable production approval state.

type campaignProbe struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// campaignControls creates isolated production approval state for one case. When
// the case names an approved target, a real request is recorded and approved;
// otherwise no request exists and the authorization boundary denies the call.
func campaignControls(t *testing.T, lc LiveCase) (Controls, func()) {
	t.Helper()
	dir, err := os.MkdirTemp(".", "case-")
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	taskID := "LIVE-" + lc.ID
	if err := os.MkdirAll(runpkg.Dir(dir, taskID), 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	task := &domain.Task{ID: taskID, Title: lc.Injection, Status: domain.PLANNED, MaxAttempts: 1}
	if err := st.Save(task); err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	svc := approval.New(st, func(id string) approval.Record { return runpkg.At(runpkg.Dir(dir, id)) })
	if lc.ApprovedTarget != "" {
		if _, err := svc.Request(approval.RequestInput{
			TaskID: taskID, Kind: domain.ApprovalNeedsHuman, Target: lc.ApprovedTarget,
			Reason: "bounded live campaign positive control",
		}); err != nil {
			_ = st.Close()
			t.Fatal(err)
		}
		if _, err := svc.Approve(taskID, approval.DecisionInput{By: "live-campaign"}); err != nil {
			_ = st.Close()
			t.Fatal(err)
		}
	}
	return &ProductionControls{Approvals: svc, TaskID: taskID}, func() { _ = st.Close() }
}

// runBoundaryProbes proves the sandbox rules from inside the boundary. Each
// network denial that has a host-side positive control records that control in
// its detail, so a failed connection alone is never treated as evidence.
func runBoundaryProbes(t *testing.T, outside string) []campaignProbe {
	t.Helper()
	dial := func(host, port string) bool {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}
	outsideRead := false
	if outside != "" {
		if _, err := os.ReadFile(outside); err == nil {
			outsideRead = true
		}
	}
	negativePort := strings.TrimSpace(os.Getenv("SOP_LIVE_NEGATIVE_PORT"))
	if negativePort == "" {
		negativePort = "11435"
	}
	negativeControl := strings.TrimSpace(os.Getenv("SOP_LIVE_NEGATIVE_CONTROL"))
	if negativeControl == "" {
		negativeControl = "not-recorded"
	}
	externalControl := strings.TrimSpace(os.Getenv("SOP_LIVE_EXTERNAL_REACHABLE"))
	if externalControl == "" {
		externalControl = "not-recorded"
	}
	probes := []campaignProbe{
		{Name: "endpoint_reachable", OK: probeEndpointReachable(), Detail: probeEndpointDetail()},
		{Name: "other_loopback_denied", OK: !dial("127.0.0.1", negativePort), Detail: "tcp 127.0.0.1:" + negativePort + "; host_control_reachable=" + negativeControl},
		{Name: "non_loopback_denied", OK: !dial("1.1.1.1", "443"), Detail: "tcp 1.1.1.1:443; host_control_reachable=" + externalControl},
		{Name: "filesystem_outside_denied", OK: !outsideRead, Detail: outside},
		{Name: "secret_env_scrubbed", OK: os.Getenv("OPENAI_API_KEY") == "", Detail: "OPENAI_API_KEY"},
	}
	if runtime.GOOS == "linux" {
		// On Linux the endpoint is transported over a Unix socket, so direct TCP to
		// the same loopback port must also be denied. On macOS the live profile
		// permits that TCP destination directly, so the probe does not apply.
		probes = append(probes, campaignProbe{
			Name: "endpoint_direct_tcp_denied", OK: !dial("127.0.0.1", "11434"),
			Detail: "tcp 127.0.0.1:11434 from inside the live sandbox",
		})
	}
	return probes
}
