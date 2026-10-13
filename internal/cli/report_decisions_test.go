package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// writeRunFixture writes one run directory with the given artifacts. Any name not
// supplied is left absent, so missing/partial histories are expressible.
func writeRunFixture(t *testing.T, dir, id string, files map[string]string) {
	t.Helper()
	runDir := filepath.Join(dir, stateDirName, "runs", id)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const evidJSON = `{"schema_version":1,"cases":[
{"case_id":"c1","source":"clef","comparable":true,"outcome":"shadow_agreement","primary":"a","shadow":"a","reference":"a","confidence":0.9,"latency_ms":10},
{"case_id":"c1","source":"nimble","comparable":true,"outcome":"shadow_disagreement","primary":"a","shadow":"b","reference":"a","latency_ms":30},
{"case_id":"c2","source":"clef","comparable":true,"outcome":"shadow_timeout","latency_ms":50}
]}`

// TestReportDecisionsText proves `sop report --decisions` renders the deterministic
// decision-telemetry text: the tier distribution, the adapter dimensions, and
// explicit coverage/denominators, without execution or a run id.
func TestReportDecisionsText(t *testing.T) {
	dir := t.TempDir()
	writeRunFixture(t, dir, "RUN-A", map[string]string{
		"model-selection.json": `{"class":"large","provider":"ollama","model":"m","source":"router"}`,
		"metrics.json":         `{"id":"RUN-A","total_ms":10,"counts":{},"jev":{"invocations":2}}`,
		"shadow-evidence.json": evidJSON,
	})

	code, out, errOut := runCLI(t, dir, "report", "--decisions")
	if code != exitOK {
		t.Fatalf("report --decisions code=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{
		"Decision telemetry (corpus 1 runs)",
		"model_tier_distribution",
		"tier large: 1",
		"jev_invocation",
		"adapter_invocation",
		"adapter_agreement",
		"adapter_latency",
		"coverage",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report --decisions missing %q:\n%s", want, out)
		}
	}
}

// TestReportDecisionsJSON proves `--format json` emits the deterministic aggregate
// with the corpus, the seven metrics, and the decision dimensions.
func TestReportDecisionsJSON(t *testing.T) {
	dir := t.TempDir()
	writeRunFixture(t, dir, "RUN-A", map[string]string{
		"model-selection.json": `{"class":"small","provider":"ollama","model":"m","source":"cli","fallback":true}`,
		"shadow-evidence.json": evidJSON,
	})

	code, out, errOut := runCLI(t, dir, "report", "--decisions", "--format", "json")
	if code != exitOK {
		t.Fatalf("report --decisions --format json code=%d stderr=%s", code, errOut)
	}
	var agg struct {
		CorpusRuns int `json:"corpus_runs"`
		Metrics    []struct {
			Metric string `json:"metric"`
		} `json:"metrics"`
		Decisions []struct {
			Metric string `json:"metric"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(out), &agg); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if agg.CorpusRuns != 1 {
		t.Errorf("corpus_runs = %d, want 1", agg.CorpusRuns)
	}
	if len(agg.Metrics) != 7 {
		t.Errorf("metrics = %d, want the seven RM metrics", len(agg.Metrics))
	}
	if len(agg.Decisions) == 0 {
		t.Error("decisions is empty, want the decision dimensions")
	}

	// The equals form is accepted too and yields identical output.
	code2, out2, _ := runCLI(t, dir, "report", "--decisions", "--format=json")
	if code2 != exitOK || out2 != out {
		t.Errorf("--format=json and --format json differ (code %d vs %d)", code2, code)
	}
}

// TestReportDecisionsDeterministic proves two invocations produce byte-identical
// output (text and JSON).
func TestReportDecisionsDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeRunFixture(t, dir, "RUN-A", map[string]string{
		"model-selection.json": `{"class":"medium","provider":"ollama","model":"m","source":"router"}`,
		"shadow-evidence.json": evidJSON,
	})
	for _, args := range [][]string{
		{"report", "--decisions"},
		{"report", "--decisions", "--format", "json"},
	} {
		_, a, _ := runCLI(t, dir, args...)
		_, b, _ := runCLI(t, dir, args...)
		if a != b {
			t.Errorf("non-deterministic output for %v:\n%s\n%s", args, a, b)
		}
	}
}

// TestReportDecisionsEmptyProject proves an empty project renders successfully with
// an explicit zero-corpus aggregate rather than failing.
func TestReportDecisionsEmptyProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, stateDirName, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, dir, "report", "--decisions")
	if code != exitOK {
		t.Fatalf("empty project code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "corpus 0 runs") {
		t.Errorf("empty project output missing zero corpus:\n%s", out)
	}
}

// TestReportDecisionsMixedHistory proves a partial history (one run with evidence,
// one without) reports the recorded run and leaves the rest UNAVAILABLE, never
// zero-filled.
func TestReportDecisionsMixedHistory(t *testing.T) {
	dir := t.TempDir()
	writeRunFixture(t, dir, "RUN-A", map[string]string{
		"model-selection.json": `{"class":"large","provider":"ollama","model":"m","source":"router"}`,
		"shadow-evidence.json": evidJSON,
	})
	writeRunFixture(t, dir, "RUN-B", map[string]string{"state.json": `{}`})

	code, out, errOut := runCLI(t, dir, "report", "--decisions", "--format", "json")
	if code != exitOK {
		t.Fatalf("mixed history code=%d stderr=%s", code, errOut)
	}
	var agg struct {
		CorpusRuns int `json:"corpus_runs"`
		Decisions  []struct {
			Metric string `json:"metric"`
			Value  string `json:"value"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(out), &agg); err != nil {
		t.Fatal(err)
	}
	if agg.CorpusRuns != 2 {
		t.Errorf("corpus_runs = %d, want 2", agg.CorpusRuns)
	}
	// Runs without evidence keep the adapter dimensions UNAVAILABLE.
	for _, d := range agg.Decisions {
		if d.Metric == "decision_confidence" && d.Value != "UNAVAILABLE" {
			t.Errorf("%s value = %q, want UNAVAILABLE", d.Metric, d.Value)
		}
	}
}

// TestReportDecisionsUnsupportedSchema proves an unknown evidence schema version is
// present but UNAVAILABLE, and the command still succeeds deterministically.
func TestReportDecisionsUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	writeRunFixture(t, dir, "RUN-A", map[string]string{
		"shadow-evidence.json": `{"schema_version":999,"cases":[]}`,
	})
	code, out, errOut := runCLI(t, dir, "report", "--decisions", "--format", "json")
	if code != exitOK {
		t.Fatalf("unsupported schema code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, `"adapter_agreement"`) || !strings.Contains(out, `"UNAVAILABLE"`) {
		t.Errorf("unsupported schema should render adapter_agreement UNAVAILABLE:\n%s", out)
	}
}

// TestReportDecisionsUsage proves the command rejects a run id and an unknown format.
func TestReportDecisionsUsage(t *testing.T) {
	dir := t.TempDir()
	if code, _, _ := runCLI(t, dir, "report", "--decisions", "T001"); code != exitUsage {
		t.Errorf("report --decisions with a run id: code=%d, want usage", code)
	}
	if code, _, _ := runCLI(t, dir, "report", "--decisions", "--format", "xml"); code != exitUsage {
		t.Errorf("report --decisions --format xml: code=%d, want usage", code)
	}
}

// TestReportDecisionsHasNoExecutionSideEffects proves the command is read-only: it
// creates, deletes, and modifies nothing under the runs directory.
func TestReportDecisionsHasNoExecutionSideEffects(t *testing.T) {
	dir := t.TempDir()
	writeRunFixture(t, dir, "RUN-A", map[string]string{
		"model-selection.json": `{"class":"large","provider":"ollama","model":"m","source":"router"}`,
		"shadow-evidence.json": evidJSON,
	})
	runsRoot := filepath.Join(dir, stateDirName, "runs")

	before := snapshotTree(t, runsRoot)
	if code, _, errOut := runCLI(t, dir, "report", "--decisions"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	after := snapshotTree(t, runsRoot)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("report --decisions changed the runs tree\nbefore=%v\nafter=%v", before, after)
	}
}

// snapshotTree lists every file under root with its size, so a create/delete/write
// is detectable.
func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			out = append(out, "d:"+rel)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, "f:"+rel+":"+strconv.FormatInt(info.Size(), 10))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}
