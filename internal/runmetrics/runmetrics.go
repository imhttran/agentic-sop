// Package runmetrics is a deterministic, model-free aggregator over recorded run
// artifacts. It reads only what a run leaves behind under
// .agent-sdlc/runs/<TASK-ID>/ through the existing reader shapes
// (internal/run, internal/perf, internal/runtrace), emits the RM-002 metric
// dimensions with explicit numerators, denominators, and coverage, and
// distinguishes five quantities that must never be conflated:
//
//   - total runs      — the corpus size (every run directory);
//   - artifacts present — runs carrying at least one source artifact;
//   - eligible runs   — runs that can contribute to the metric (the artifact is
//     present and readable);
//   - valid measurements — runs where a usable value was actually read;
//   - unavailable measurements — total runs minus valid measurements.
//
// An absent artifact yields UNAVAILABLE and reduces coverage; a present artifact
// with a recorded zero is a real measurement that counts toward the denominator.
// Absence is never folded in as a zero-valued measurement.
//
// The package is a pure internal library: it makes no model call, no network
// call, and no provider- or model-specific decision. It is wired to no CLI
// command and no production runtime path, and it adds no go.mod requirement. It
// is diagnostic only: nothing it emits drives a gate, a budget, or a routing
// decision, and token usage is never an input to any of them.
package runmetrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/perf"
)

// UNAVAILABLE is the value emitted for a metric whose artifact or field was not
// recorded. It is distinct from a recorded zero: an absent artifact is unknown,
// never a zero-valued measurement, and it reduces coverage rather than counting
// toward the denominator.
const UNAVAILABLE = "UNAVAILABLE"

// runsDirName is the subdirectory of the project state dir holding runs. It
// mirrors internal/run's unexported constant; the aggregator reads the same
// layout rather than introducing a parallel reader.
const runsDirName = "runs"

// Recorded artifact file names, read through the existing reader shapes.
const (
	reportFileName         = "report.json"
	metricsFileName        = "metrics.json"
	modelSelectionFileName = "model-selection.json"
	classificationFileName = "classification.json"
	approvalFileName       = "approval.json"
	traceFileName          = "trace.json"
	attemptFileName        = "attempt.txt"
	continuationsFileName  = "continuations.txt"
	reviewFileName         = "review.json"
)

// Metric identifies one emitted RM-002 metric dimension. The names match the
// seven dimensions fixed by docs/reports/run-metrics/RM-002-metric-register.md.
type Metric string

const (
	MetricAgentSuccess Metric = "agent_success_failure_rate"
	MetricRouting      Metric = "routing_finding_severity"
	MetricFixLoop      Metric = "fix_loop_convergence"
	MetricRetry        Metric = "retry_efficiency"
	MetricApproval     Metric = "human_approval_frequency"
	MetricLatency      Metric = "execution_latency"
	MetricTokens       Metric = "token_usage"
)

// metricOrder fixes the emission order of the seven dimensions, matching the
// register's order so output is deterministic.
var metricOrder = []Metric{
	MetricAgentSuccess,
	MetricRouting,
	MetricFixLoop,
	MetricRetry,
	MetricApproval,
	MetricLatency,
	MetricTokens,
}

// Status is a metric's classification carried from the RM-002 register.
type Status string

const (
	// StatusAvailable means the metric is measurable without inference.
	StatusAvailable Status = "AVAILABLE"
	// StatusPartial means the signal is present for only some runs.
	StatusPartial Status = "PARTIAL"
	// StatusUnavailable means the metric has no recorded source.
	StatusUnavailable Status = "UNAVAILABLE"
)

// MetricResult is one emitted metric. It carries the five distinct measurement
// quantities (TotalRuns, EligibleRuns, ArtifactsPresent, ValidMeasurements,
// UnavailableMeasurements), an explicit Numerator and Denominator, and the
// coverage percentage of the corpus. The fields are never conflated: a metric
// value is derived from ValidMeasurements, never from the corpus or from the
// set of artifacts present.
//
//   - TotalRuns is the corpus size (every run directory).
//   - ArtifactsPresent is the number of runs carrying at least one of the
//     metric's source artifacts.
//   - EligibleRuns is the number of runs that can contribute to the metric (the
//     source artifact is present and readable). For the current metrics this
//     coincides with ArtifactsPresent; it is kept separate so a metric whose
//     eligibility is narrower than artifact presence is expressible.
//   - ValidMeasurements is the number of runs where a usable value was actually
//     read; it is the denominator base for the metric.
//   - UnavailableMeasurements is TotalRuns minus ValidMeasurements.
//   - Numerator and Denominator are the explicit fraction for the metric:
//     for a rate the count of runs satisfying the condition over the valid
//     measurements; for a magnitude the summed value over the valid
//     measurements. A recorded zero renders as "0" and counts toward both; a
//     UNAVAILABLE metric reports Numerator 0, Denominator 0, and lowers Coverage.
type MetricResult struct {
	Metric                  Metric  `json:"metric"`
	Status                  Status  `json:"status"`
	Value                   string  `json:"value"`
	Unit                    string  `json:"unit,omitempty"`
	TotalRuns               int     `json:"total_runs"`
	EligibleRuns            int     `json:"eligible_runs"`
	ArtifactsPresent        int     `json:"artifacts_present"`
	ValidMeasurements       int     `json:"valid_measurements"`
	UnavailableMeasurements int     `json:"unavailable_measurements"`
	Numerator               int     `json:"numerator"`
	Denominator             int     `json:"denominator"`
	CoveragePct             float64 `json:"coverage_pct"`
	// Sources names the recorded artifacts this metric reads, in the register's
	// order. It is evidence, never a gate/budget/routing field.
	Sources []string `json:"sources,omitempty"`
	// Note explains how the numerator is measured or why a value is UNAVAILABLE.
	Note string `json:"note,omitempty"`
}

// ArtifactState classifies one recorded artifact's presence for the corpus.
type ArtifactState string

const (
	// ArtifactPresent means the artifact was recorded for at least one run.
	ArtifactPresent ArtifactState = "present"
	// ArtifactAbsent means the artifact was never recorded for the corpus.
	ArtifactAbsent ArtifactState = "absent"
	// ArtifactZero records artifacts that are present with an explicit zero
	// measurement. Presence with a zero value is a recorded measurement, never
	// conflated with absence.
	ArtifactZero ArtifactState = "zero"
)

// ArtifactPresence records one artifact's presence across the corpus.
type ArtifactPresence struct {
	Name        string        `json:"name"`
	State       ArtifactState `json:"state"`
	PresentRuns int           `json:"present_runs"`
	ZeroRuns    int           `json:"zero_runs"`
	Denominator int           `json:"denominator"`
}

// Aggregate is the deterministic output of one aggregation. Its JSON encoding is
// stable: metrics are emitted in register order and artifacts in a sorted order,
// so two aggregations over identical inputs produce byte-identical output. It
// carries no wall-clock field.
type Aggregate struct {
	CorpusRuns  int                `json:"corpus_runs"`
	Metrics     []MetricResult     `json:"metrics"`
	Artifacts   []ArtifactPresence `json:"artifacts"`
	Unavailable []string           `json:"unavailable"`
}

// RunIDs returns the sorted run directory identities visible under projectDir's
// state directory. It reads the directory listing only: it never creates or
// modifies a run directory.
func RunIDs(projectDir string) ([]string, error) {
	base := filepath.Join(projectDir, config.DirName, runsDirName)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("runmetrics: read runs dir: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// runEvidence is the reader-level view of one recorded run: the presence/absence
// signals the existing readers expose, plus whether each artifact yielded a
// usable value, never a parsed value standing in for presence.
type runEvidence struct {
	id  string
	dir string

	// Artifact presence (the file exists).
	hasReport     bool
	hasMetrics    bool
	hasModelSel   bool
	hasClass      bool
	hasApproval   bool
	hasTrace      bool
	hasAttempt    bool
	hasContinuatn bool
	hasReview     bool

	// Usable measurements (the artifact parsed to a value the metric can read).
	reportReadable   bool // report.json parsed (even with an empty decision)
	reportDecided    bool // report.json carried a nonempty decision/verdict
	traceReadable    bool // trace.json parsed
	traceTerminal    bool // trace.json carried a nonempty termination stage/disposition
	metricsParsed    bool // metrics.json parsed as the perf.Task shape
	modelSelReadable bool // model-selection.json parsed
	classReadable    bool // classification.json parsed
	continuations    int  // continuations.txt parsed integer
	continuationsOK  bool // continuations.txt carried a well-formed nonnegative integer

	// metricsData is the parsed metrics.json shape (internal/perf.Task).
	metricsData perf.Task
	counts      perf.Counts

	// success is true when a terminal success outcome is recorded in report.json
	// or trace.json. It is only meaningful when reportDecided or traceTerminal.
	success bool
	// routed is true when model-selection.json records a routing selection.
	routed bool
	// hasSeverity is true when review.json records a finding-severity object.
	hasSeverity bool
	// approvalRequested is true when approval.json records a request or a
	// report.json records the NEEDS_HUMAN terminal decision.
	approvalRequested bool
}

// readEvidence reads one run directory through file-presence signals, matching
// the existing reader shapes (internal/run ReadAttempt, internal/perf's
// metrics.json shape, internal/runtrace's trace.json shape). It performs no model
// call and no network call and never mutates the run directory.
func readEvidence(projectDir, id string) runEvidence {
	dir := filepath.Join(projectDir, config.DirName, runsDirName, id)
	ev := runEvidence{id: id, dir: dir}

	if present(dir, reportFileName) {
		ev.hasReport = true
		var report struct {
			Decision string `json:"decision"`
			Verdict  string `json:"verdict"`
		}
		if readJSON(filepath.Join(dir, reportFileName), &report) {
			ev.reportReadable = true
			decision := firstNonEmpty(report.Decision, report.Verdict)
			ev.reportDecided = decision != ""
			ev.success = isSuccess(decision)
			ev.approvalRequested = isNeedsHuman(decision)
		}
	}
	if present(dir, metricsFileName) {
		ev.hasMetrics = true
		// The metrics.json shape is internal/perf.Task; parse it directly rather
		// than through a parallel reader.
		if readJSON(filepath.Join(dir, metricsFileName), &ev.metricsData) {
			ev.metricsParsed = true
			ev.counts = ev.metricsData.Counts
		}
	}
	if present(dir, modelSelectionFileName) {
		ev.hasModelSel = true
		var sel struct {
			Class    string `json:"class"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Source   string `json:"source"`
		}
		if readJSON(filepath.Join(dir, modelSelectionFileName), &sel) {
			ev.modelSelReadable = true
			ev.routed = firstNonEmpty(sel.Class, sel.Provider, sel.Model, sel.Source) != ""
		}
	}
	ev.hasClass = present(dir, classificationFileName)
	if ev.hasClass {
		var cls struct {
			Class string `json:"class"`
		}
		if readJSON(filepath.Join(dir, classificationFileName), &cls) {
			ev.classReadable = true
		}
	}
	if present(dir, approvalFileName) {
		ev.hasApproval = true
		ev.approvalRequested = true
	}
	if present(dir, reviewFileName) {
		ev.hasReview = true
		var review struct {
			Findings json.RawMessage `json:"findings"`
			Severity string          `json:"severity"`
		}
		if readJSON(filepath.Join(dir, reviewFileName), &review) {
			ev.hasSeverity = len(review.Findings) > 0 || strings.TrimSpace(review.Severity) != ""
		}
	}

	if present(dir, traceFileName) {
		ev.hasTrace = true
		var trace struct {
			Termination struct {
				Stage       string `json:"stage"`
				Disposition string `json:"disposition"`
			} `json:"termination"`
		}
		if readJSON(filepath.Join(dir, traceFileName), &trace) {
			ev.traceReadable = true
			ev.traceTerminal = strings.TrimSpace(trace.Termination.Disposition) != "" ||
				strings.TrimSpace(trace.Termination.Stage) != ""
			if isSuccess(trace.Termination.Disposition) || isSuccess(trace.Termination.Stage) {
				ev.success = true
			}
		}
	}

	// Attempt presence is the reader's presence signal: attempt.txt exists AND
	// carries a nonempty signature (internal/run.ReadAttempt returns ("", false)
	// otherwise).
	ev.hasAttempt = nonEmptyFile(filepath.Join(dir, attemptFileName))
	// continuations.txt is read the same way the run reader does: missing or
	// malformed is absent, and a present well-formed value (including "0") is
	// recorded. The parsed count is kept so "0" is a recorded zero, not a signal.
	if n, ok := readInt(filepath.Join(dir, continuationsFileName)); ok {
		ev.hasContinuatn = true
		ev.continuationsOK = true
		ev.continuations = n
	}

	return ev
}

// successTokens are the recorded terminal outcomes that count as success. The
// set is fixed and matched case-insensitively; no model or provider name is
// special-cased.
var successTokens = map[string]bool{
	"passed":    true,
	"pass":      true,
	"success":   true,
	"succeeded": true,
	"ok":        true,
	"approved":  true,
	"results":   true,
}

func isSuccess(s string) bool { return successTokens[strings.ToLower(strings.TrimSpace(s))] }

func isNeedsHuman(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "needs_human", "needs-human", "needs human":
		return true
	default:
		return false
	}
}

func present(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !info.IsDir()
}

func nonEmptyFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) != ""
}

// readInt reads a file holding a single nonnegative integer. A missing,
// malformed, negative, or non-numeric value is not a usable measurement.
func readInt(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func readJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
