package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/memindex"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"gopkg.in/yaml.v3"
)

// advisoryOverrideNames lists the advisory-retrieval environment variables in a stable
// order, so an enablement report names the same overrides in the same order every time.
var advisoryOverrideNames = []string{
	config.EnvAdvisoryRetrievalEnabled,
	config.EnvAdvisoryRetrievalIndex,
	config.EnvAdvisoryRetrievalOutcomes,
	config.EnvAdvisoryRetrievalCorpora,
	config.EnvAdvisoryRetrievalTimeout,
	config.EnvAdvisoryRetrievalResults,
	config.EnvAdvisoryRetrievalTokens,
	config.EnvAdvisoryRetrievalCompatFloor,
}

// runMemoryOperations changes only the workspace advisory block. Model weights,
// installer defaults, and approval policy are not changed.
func runMemoryOperations(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, memoryUsage)
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	if args[0] == "status" {
		return memoryStatus(dir, *cfg, stdout, stderr)
	}
	enabled := args[0] == "enable"
	block, err := configureMemory(dir, enabled)
	if err != nil {
		fmt.Fprintln(stderr, err)
		writeAdvisoryOverrides(stderr)
		return exitError
	}
	fmt.Fprintf(stdout, "Workspace advisory memory enabled=%t. Installer defaults and approval gates unchanged.\n", enabled)
	writeAdvisoryOverrides(stdout)
	writeEffectiveMemory(stdout, block)
	return exitOK
}

// writeEffectiveMemory reports the configuration that retrieval and capture will actually
// use. Environment overrides are already applied by Resolved, so the workspace flag and the
// effective flag can differ. A disable must stay usable when the surrounding configuration
// is unhealthy, so an unresolvable block is reported without failing the command.
func writeEffectiveMemory(w io.Writer, block config.AdvisoryRetrievalConfig) {
	resolved, err := block.Resolved(nil)
	if err != nil {
		fmt.Fprintf(w, "Effective advisory memory configuration is invalid: %v\n", err)
		return
	}
	fmt.Fprintf(w, "Effective advisory memory enabled=%t (index=%s; outcomes=%s).\n", resolved.Enabled, resolved.IndexPath, resolved.OutcomesDir)
}

// writeAdvisoryOverrides names the environment overrides that apply in this shell, so a
// workspace flag that an override neutralizes is never mistaken for the effective setting.
func writeAdvisoryOverrides(w io.Writer) {
	var applied []string
	for _, name := range advisoryOverrideNames {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			applied = append(applied, name+"="+v)
		}
	}
	if len(applied) == 0 {
		fmt.Fprintln(w, "Advisory memory environment overrides: none.")
		return
	}
	fmt.Fprintf(w, "Advisory memory environment overrides applied: %s\n", strings.Join(applied, ", "))
}

// validateEffectiveMemory rejects an enabled configuration that the retrieval boundary would
// refuse, so `sop memory enable` cannot report success for a feature whose every retrieval
// would fail. It reuses the read path's own query construction and validation.
func validateEffectiveMemory(resolved config.AdvisoryRetrievalConfig) error {
	if !resolved.Enabled {
		return nil
	}
	for _, corpus := range advisoryCorpora(resolved) {
		q := advisoryQuery(resolved, nil, "", "", AdvisoryPhasePlanning, corpus)
		if _, err := retrievalboundary.Validate(q); err != nil {
			return fmt.Errorf("memory: effective advisory retrieval configuration is invalid: %w; fix allowed_corpora, timeout_ms, max_results, or token_budget and retry", err)
		}
	}
	return nil
}

// configureMemory writes the workspace advisory block and returns it. When enabling, it
// validates the EFFECTIVE configuration (workspace plus environment overrides) and prepares
// the effective index parent and outcome directories before the file changes. A successful
// enable therefore means the next capture and retrieval can use the configured storage.
func configureMemory(dir string, enabled bool) (config.AdvisoryRetrievalConfig, error) {
	path := config.Path(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		return config.AdvisoryRetrievalConfig{}, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return config.AdvisoryRetrievalConfig{}, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return config.AdvisoryRetrievalConfig{}, fmt.Errorf("memory: configuration must be a mapping")
	}
	root := doc.Content[0]
	var block *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "advisory_retrieval" {
			block = root.Content[i+1]
			break
		}
	}
	if block == nil {
		block = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "advisory_retrieval"}, block)
	}
	if block.Kind != yaml.MappingNode {
		return config.AdvisoryRetrievalConfig{}, fmt.Errorf("memory: advisory_retrieval must be a mapping")
	}
	set := func(key string, value any, overwrite bool) error {
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return err
		}
		for i := 0; i < len(block.Content); i += 2 {
			if block.Content[i].Value == key {
				if overwrite {
					block.Content[i+1] = &node
				}
				return nil
			}
		}
		block.Content = append(block.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &node)
		return nil
	}
	if err := set("enabled", enabled, true); err != nil {
		return config.AdvisoryRetrievalConfig{}, err
	}
	if enabled {
		base, err := filepath.Abs(filepath.Join(dir, config.DirName, "memory"))
		if err != nil {
			return config.AdvisoryRetrievalConfig{}, err
		}
		for _, item := range []struct {
			key   string
			value any
		}{
			{"index_path", filepath.Join(base, "index.db")}, {"outcomes_dir", filepath.Join(base, "outcomes")},
			{"allowed_corpora", []string{"failure", "outcome"}}, {"timeout_ms", 2000},
			{"capture_timeout_ms", 5000}, {"max_results", 5}, {"token_budget", 2000},
		} {
			if err := set(item.key, item.value, false); err != nil {
				return config.AdvisoryRetrievalConfig{}, err
			}
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return config.AdvisoryRetrievalConfig{}, err
	}
	cfg, err := config.Parse(out)
	if err != nil {
		return config.AdvisoryRetrievalConfig{}, err
	}
	if !enabled {
		// Disable never depends on healthy storage: the workspace flag is written even when
		// the preserved paths are unusable, so memory can always be turned off.
		if err := writeFileAtomic(path, string(out)); err != nil {
			return config.AdvisoryRetrievalConfig{}, err
		}
		return cfg.AdvisoryRetrieval, nil
	}
	resolved, err := cfg.AdvisoryRetrieval.Resolved(nil)
	if err != nil {
		return config.AdvisoryRetrievalConfig{}, fmt.Errorf("memory: effective configuration is invalid: %w; fix the workspace file or the SOP_ADVISORY_RETRIEVAL_* override and retry", err)
	}
	if err := validateEffectiveMemory(resolved); err != nil {
		return config.AdvisoryRetrievalConfig{}, err
	}
	if resolved.Enabled {
		if err := os.MkdirAll(resolved.OutcomesDir, 0700); err != nil {
			return config.AdvisoryRetrievalConfig{}, fmt.Errorf("memory: prepare outcomes directory %s: %w", resolved.OutcomesDir, err)
		}
		if err := os.MkdirAll(filepath.Dir(resolved.IndexPath), 0700); err != nil {
			return config.AdvisoryRetrievalConfig{}, fmt.Errorf("memory: prepare index directory %s: %w", filepath.Dir(resolved.IndexPath), err)
		}
	}
	if err := writeFileAtomic(path, string(out)); err != nil {
		return config.AdvisoryRetrievalConfig{}, err
	}
	return cfg.AdvisoryRetrieval, nil
}

// Capture statuses stay distinct so a partial success can never be read as a success.
const (
	memoryCaptureSucceeded = "succeeded"
	memoryCaptureDegraded  = "degraded"
	memoryCaptureFailed    = "failed"
	memoryCaptureDisabled  = "disabled"

	// memoryCaptureReasonRejected is the bounded reason for a degraded capture: at least
	// one outcome document was rejected while every valid record was still indexed.
	memoryCaptureReasonRejected = "ADVISORY_RECORDS_REJECTED"

	// Rejection diagnostics are bounded so telemetry stays small and stable: at most this
	// many rejected files are retained, and each file or reason is truncated.
	memoryCaptureRejectedLimit = 5
	memoryCaptureDiagnosticMax = 200
)

type memoryCaptureRejection struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

type memoryCaptureTelemetry struct {
	Status          string                   `json:"status"`
	Reason          string                   `json:"reason,omitempty"`
	DurationMS      int64                    `json:"duration_ms"`
	RecordedAt      time.Time                `json:"recorded_at"`
	Indexed         int                      `json:"indexed"`
	Excluded        int                      `json:"excluded,omitempty"`
	Rejected        int                      `json:"rejected,omitempty"`
	RejectedReasons []memoryCaptureRejection `json:"rejected_reasons,omitempty"`
}

// writeMemoryCapture retains one observable capture result. Capture is non-blocking: the
// status never changes the task verdict, and a rejected document is a degraded capture
// rather than a failed one because the valid records stay indexed.
func writeMemoryCapture(rn *runpkg.Run, cfg config.Config, started time.Time, report memindex.RebuildReport, captureErr error, stdout io.Writer) {
	if rn == nil {
		return
	}
	rec := memoryCaptureTelemetry{
		Status:          memoryCaptureSucceeded,
		DurationMS:      time.Since(started).Milliseconds(),
		RecordedAt:      time.Now().UTC(),
		Indexed:         report.Indexed,
		Excluded:        report.Excluded,
		Rejected:        len(report.Rejected),
		RejectedReasons: boundedRejections(report.Rejected),
	}
	resolved, err := cfg.AdvisoryRetrieval.Resolved(nil)
	switch {
	case err != nil:
		rec.Status, rec.Reason = memoryCaptureFailed, ragReasonAdvisoryFailed
	case !resolved.Enabled:
		rec.Status, rec.Reason = memoryCaptureDisabled, ragReasonAdvisoryDisabled
	case captureErr != nil:
		rec.Status, rec.Reason = memoryCaptureFailed, ragReasonFromError(captureErr)
	case len(report.Rejected) > 0:
		rec.Status, rec.Reason = memoryCaptureDegraded, memoryCaptureReasonRejected
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err == nil {
		err = rn.Write("memory-capture.json", string(b)+"\n")
		if err == nil && rec.Status != memoryCaptureDisabled {
			err = rn.Write(fmt.Sprintf("memory-capture-%d.json", rec.RecordedAt.UnixNano()), string(b)+"\n")
		}
	}
	if err != nil {
		fmt.Fprintln(stdout, "decision memory: capture telemetry unavailable")
	}
	if rec.Status == memoryCaptureDegraded {
		fmt.Fprintf(stdout, "decision memory: capture degraded; %d record(s) rejected, valid records indexed (task verdict unchanged; see `sop memory status`)\n", rec.Rejected)
	}
}

// boundedRejections returns a deterministic, bounded sample of rejected files and their
// diagnostic reasons, ordered by file name. Control characters are replaced so a retained
// diagnostic cannot rewrite the operator's terminal.
func boundedRejections(rejected map[string]string) []memoryCaptureRejection {
	if len(rejected) == 0 {
		return nil
	}
	names := make([]string, 0, len(rejected))
	for name := range rejected {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > memoryCaptureRejectedLimit {
		names = names[:memoryCaptureRejectedLimit]
	}
	out := make([]memoryCaptureRejection, 0, len(names))
	for _, name := range names {
		out = append(out, memoryCaptureRejection{
			File:   boundedDiagnostic(name),
			Reason: boundedDiagnostic(rejected[name]),
		})
	}
	return out
}

func boundedDiagnostic(s string) string {
	runes := []rune(s)
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			out = append(out, ' ')
		case r < 0x20 || r == 0x7f:
			out = append(out, '?')
		default:
			out = append(out, r)
		}
	}
	if len(out) > memoryCaptureDiagnosticMax {
		out = append(out[:memoryCaptureDiagnosticMax], '.', '.', '.')
	}
	return string(out)
}

// memoryStatus reads artifacts without opening or rebuilding the index. Inclusion
// means the prompt contained a source; it never means the model used it successfully.
func memoryStatus(dir string, cfg config.Config, stdout, stderr io.Writer) int {
	resolved, err := cfg.AdvisoryRetrieval.Resolved(nil)
	if err != nil {
		fmt.Fprintf(stderr, "memory configuration: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "Advisory memory: enabled=%t (workspace=%t; environment takes precedence)\n", resolved.Enabled, cfg.AdvisoryRetrieval.Enabled)
	writeAdvisoryOverrides(stdout)
	fmt.Fprintf(stdout, "Retrieval budget=%dms; capture budget=%dms; max records=%d; token budget=%d\n", resolved.TimeoutMS, resolved.CaptureTimeoutMS, resolved.MaxResults, resolved.TokenBudget)
	fmt.Fprintf(stdout, "Index: %s\nOutcomes: %s\n", resolved.IndexPath, resolved.OutcomesDir)
	entries, err := os.ReadDir(filepath.Join(dir, config.DirName, "runs"))
	if os.IsNotExist(err) {
		fmt.Fprintln(stdout, "No run observations yet.")
		return exitOK
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	type observation struct {
		task     string
		modified time.Time
		rag      *ragTelemetry
		capture  *memoryCaptureTelemetry
		problem  string
	}
	var observations []observation
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		obs := observation{task: entry.Name()}
		for _, name := range []string{"rag.json", "memory-capture.json"} {
			path := filepath.Join(dir, config.DirName, "runs", entry.Name(), name)
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				obs.problem = "unreadable memory telemetry"
				continue
			}
			if info.ModTime().After(obs.modified) {
				obs.modified = info.ModTime()
			}
			b, err := os.ReadFile(path)
			if err == nil {
				if name == "rag.json" {
					err = json.Unmarshal(b, &obs.rag)
				} else {
					err = json.Unmarshal(b, &obs.capture)
				}
			}
			if err != nil {
				obs.problem = "unreadable memory telemetry"
			}
		}
		if obs.rag != nil || obs.capture != nil || obs.problem != "" {
			observations = append(observations, obs)
		}
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].modified.After(observations[j].modified) })
	if len(observations) > 5 {
		observations = observations[:5]
	}
	if len(observations) == 0 {
		fmt.Fprintln(stdout, "No run observations yet.")
	}
	for _, obs := range observations {
		fmt.Fprintf(stdout, "%s:\n", obs.task)
		if obs.problem != "" {
			fmt.Fprintf(stdout, "  %s\n", obs.problem)
		}
		if r := obs.rag; r != nil && r.AdvisoryStatus != "" {
			fmt.Fprintf(stdout, "  retrieval=%s phase=%s reason=%s duration=%dms retrieved=%d included=%d\n", r.AdvisoryStatus, r.AdvisoryPhase, r.AdvisoryReason, r.AdvisoryDurationMS, r.AdvisoryCount, len(r.AdvisoryIncluded))
			for _, ref := range r.AdvisoryRecords {
				fmt.Fprintf(stdout, "  source: %s\n", ref)
			}
		}
		if c := obs.capture; c != nil {
			fmt.Fprintf(stdout, "  capture=%s reason=%s duration=%dms indexed=%d excluded=%d rejected=%d\n", c.Status, c.Reason, c.DurationMS, c.Indexed, c.Excluded, c.Rejected)
			for i, rej := range c.RejectedReasons {
				if i >= memoryCaptureRejectedLimit {
					break
				}
				fmt.Fprintf(stdout, "  rejected: %s: %s\n", rej.File, rej.Reason)
			}
		}
	}
	fmt.Fprintln(stdout, "Prompt inclusion is observed; usefulness and causal benefit are not inferred.")
	return exitOK
}
