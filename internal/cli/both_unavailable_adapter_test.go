package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// This file proves the COMPLETE both-unavailable decision path through the real
// production stack, with disposable fixtures and NO running service consulted:
//
//	SOP lifecycle -> config-driven adapter connector -> REAL unified `sop decision`
//	subprocess -> clef primary (unreachable) -> nimble backup -> [served | unavailable]
//	-> SOP deterministic fallback (autonomy.Decide)
//
// It asserts the recorded decision-evidence.json (outcome class, failure reason,
// provider attribution, baseline/final governance action) and that the governed
// baseline is preserved: an unavailable provider chain can only fall back to SOP's
// own deterministic policy, never approve, lower risk, or bypass the boundary.

// buildUnifiedSOPBinary compiles ./cmd/sop from the current source into a disposable
// directory, so the test drives the REAL unified decision adapter rather than an
// in-process double.
func buildUnifiedSOPBinary(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH; cannot build the unified adapter")
	}
	bin := filepath.Join(t.TempDir(), "sop")
	cmd := exec.Command(goBin, "build", "-o", bin, "./cmd/sop")
	cmd.Dir = repoRootCLI()
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build ./cmd/sop: %v\n%s", err, out)
	}
	return bin
}

// nimbleScoreServer is a disposable standalone /score backend that answers every
// requested enum field with one fixed choice, so the adapter's nimble backup is
// "available" without touching the operator's running service.
func nimbleScoreServer(t *testing.T, choice string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/health") {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req struct {
			Schema map[string]struct {
				Choices []string `json:"choices"`
			} `json:"schema"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		fields := map[string]any{}
		for id, f := range req.Schema {
			scores := map[string]float64{}
			for _, c := range f.Choices {
				if c == choice {
					scores[c] = 0.9
				} else {
					scores[c] = 0.05
				}
			}
			fields[id] = map[string]any{"value": choice, "scores": scores}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":  "bespokelabs/Bespoke-Nimble-9B",
			"output": map[string]any{},
			"fields": fields,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func decisionAdapterConfigYAML(bin string) string {
	return "project:\n  name: x\n" +
		"validation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n" +
		"autonomy:\n  level: high\n" +
		"decision:\n  enabled: true\n  provider: adapter\n  adapter_provider: clef\n" +
		"  adapter_command:\n    - " + fmt.Sprintf("%q", bin) + "\n    - decision\n"
}

func readDecisionEvidenceDoc(t *testing.T, dir, taskID string) decisionEvidenceDoc {
	t.Helper()
	p := filepath.Join(dir, stateDirName, "runs", taskID, decisionEvidenceArtifactName)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("decision-evidence.json not written: %v", err)
	}
	var doc decisionEvidenceDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode decision-evidence.json: %v", err)
	}
	return doc
}

// TestDecisionEvidenceAdapterBothUnavailable covers both terminal states of the
// adapter chain: the backup serving after a primary failure (with attribution), and
// the full both-unavailable path falling back to SOP's deterministic baseline.
func TestDecisionEvidenceAdapterBothUnavailable(t *testing.T) {
	clearDecisionEnv(t)
	bin := buildUnifiedSOPBinary(t)

	// Primary (clef) is always unreachable: the adapter's chat transport targets
	// OLLAMA_BASE_URL, pointed at a closed loopback port.
	const unreachable = "http://127.0.0.1:1"

	t.Run("primary down, backup serves with attribution", func(t *testing.T) {
		srv := nimbleScoreServer(t, "LOW") // benign: must not add attention
		t.Setenv("CLEF_ENABLED", "1")
		t.Setenv("CLEF_TRANSPORT", "chat")
		t.Setenv("CLEF_MODEL", "clef-4bit")
		t.Setenv("OLLAMA_BASE_URL", unreachable)
		t.Setenv("SOP_DECISION_FALLBACK", "nimble")
		t.Setenv("NIMBLE_TRANSPORT", "score")
		t.Setenv("NIMBLE_SCORE_URL", srv.URL+"/score")

		dir := t.TempDir()
		writeFile(t, dir, "TASK.md", runTaskFile)
		writeConfig(t, dir, decisionAdapterConfigYAML(bin))
		a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}

		code, stdout, stderr := runInjectedCLIWithDecision(t, dir, "diff\n", a, decisionProviderFromConfig, "run", "--task", "TASK.md")
		if code != exitError {
			t.Fatalf("code=%d, want a non-pass result; stdout=%s stderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "decision=AUTO_CONTINUE") {
			t.Fatalf("a benign backup result must keep the deterministic baseline (AUTO_CONTINUE):\n%s", stdout)
		}
		doc := readDecisionEvidenceDoc(t, dir, "T001")
		if doc.OutcomeClass != "benign" {
			t.Errorf("outcome_class = %q, want benign", doc.OutcomeClass)
		}
		if doc.ServedProvider != "nimble" {
			t.Errorf("served_provider = %q, want nimble (actual attribution)", doc.ServedProvider)
		}
		if doc.FallbackReason == "" {
			t.Error("fallback_reason must record why the backup served")
		}
		if doc.Choice != string("LOW") {
			t.Errorf("choice = %q, want LOW (SOP-accepted evidence)", doc.Choice)
		}
		if doc.Escalated {
			t.Error("a benign result must not escalate")
		}
		if doc.BaselineAction != doc.FinalAction {
			t.Errorf("governance changed: baseline=%q final=%q", doc.BaselineAction, doc.FinalAction)
		}
	})

	t.Run("both unavailable falls back to the deterministic baseline", func(t *testing.T) {
		t.Setenv("CLEF_ENABLED", "1")
		t.Setenv("CLEF_TRANSPORT", "chat")
		t.Setenv("CLEF_MODEL", "clef-4bit")
		t.Setenv("OLLAMA_BASE_URL", unreachable)
		t.Setenv("SOP_DECISION_FALLBACK", "nimble")
		t.Setenv("NIMBLE_TRANSPORT", "score")
		t.Setenv("NIMBLE_SCORE_URL", unreachable+"/score") // backup unreachable too

		dir := t.TempDir()
		writeFile(t, dir, "TASK.md", runTaskFile)
		writeConfig(t, dir, decisionAdapterConfigYAML(bin))
		a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}

		code, stdout, stderr := runInjectedCLIWithDecision(t, dir, "diff\n", a, decisionProviderFromConfig, "run", "--task", "TASK.md")
		if code != exitError {
			t.Fatalf("code=%d, want a non-pass result; stdout=%s stderr=%s", code, stdout, stderr)
		}
		// Governance preservation: the unavailable chain leaves SOP's own
		// deterministic decision in force — it neither authorizes nor blocks.
		if !strings.Contains(stdout, "decision=AUTO_CONTINUE") {
			t.Fatalf("both-unavailable must fall back to the deterministic SOP baseline (AUTO_CONTINUE):\n%s", stdout)
		}
		doc := readDecisionEvidenceDoc(t, dir, "T001")
		if !doc.Consulted {
			t.Error("consulted must be true: a configured provider was attempted")
		}
		if doc.OutcomeClass != "unavailable" {
			t.Errorf("outcome_class = %q, want unavailable", doc.OutcomeClass)
		}
		if doc.FailureLabel != "provider unavailable" {
			t.Errorf("failure_label = %q, want provider unavailable", doc.FailureLabel)
		}
		// A failure must never record accepted evidence.
		if doc.Choice != "" || doc.Confidence != 0 {
			t.Errorf("a failure recorded choice=%q confidence=%v; must be empty", doc.Choice, doc.Confidence)
		}
		// No provider served, so none may be attributed.
		if doc.ServedProvider != "" {
			t.Errorf("served_provider = %q, want empty (no provider served)", doc.ServedProvider)
		}
		if doc.Escalated {
			t.Error("an unavailable chain must not escalate")
		}
		if doc.BaselineAction == "" || doc.BaselineAction != doc.FinalAction {
			t.Errorf("governance changed: baseline=%q final=%q", doc.BaselineAction, doc.FinalAction)
		}
	})
}
