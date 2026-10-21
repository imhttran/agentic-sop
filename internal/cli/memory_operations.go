package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"gopkg.in/yaml.v3"
)

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
	if err := configureMemory(dir, enabled); err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	fmt.Fprintf(stdout, "Workspace advisory memory enabled=%t. Installer defaults and approval gates unchanged.\n", enabled)
	if os.Getenv(config.EnvAdvisoryRetrievalEnabled) != "" {
		fmt.Fprintf(stdout, "Environment override %s=%s takes precedence.\n", config.EnvAdvisoryRetrievalEnabled, os.Getenv(config.EnvAdvisoryRetrievalEnabled))
	}
	return exitOK
}

func configureMemory(dir string, enabled bool) error {
	path := config.Path(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("memory: configuration must be a mapping")
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
		return fmt.Errorf("memory: advisory_retrieval must be a mapping")
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
		return err
	}
	if enabled {
		base, err := filepath.Abs(filepath.Join(dir, config.DirName, "memory"))
		if err != nil {
			return err
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
				return err
			}
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(out)
	if err != nil {
		return err
	}
	resolved, err := cfg.AdvisoryRetrieval.Resolved(func(string) string { return "" })
	if err != nil {
		return err
	}
	if enabled {
		if err := os.MkdirAll(resolved.OutcomesDir, 0700); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(resolved.IndexPath), 0700); err != nil {
			return err
		}
	}
	return writeFileAtomic(path, string(out))
}

type memoryCaptureTelemetry struct {
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	DurationMS int64     `json:"duration_ms"`
	RecordedAt time.Time `json:"recorded_at"`
}

func writeMemoryCapture(rn *runpkg.Run, cfg config.Config, started time.Time, captureErr error, stdout io.Writer) {
	if rn == nil {
		return
	}
	resolved, err := cfg.AdvisoryRetrieval.Resolved(nil)
	rec := memoryCaptureTelemetry{Status: "succeeded", DurationMS: time.Since(started).Milliseconds(), RecordedAt: time.Now().UTC()}
	if err == nil && !resolved.Enabled {
		rec.Status = "skipped"
		rec.Reason = "ADVISORY_DISABLED"
	}
	if captureErr != nil {
		rec.Status = "failed"
		rec.Reason = ragReasonFromError(captureErr)
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err == nil {
		err = rn.Write("memory-capture.json", string(b)+"\n")
		if err == nil && rec.Reason != "ADVISORY_DISABLED" {
			err = rn.Write(fmt.Sprintf("memory-capture-%d.json", rec.RecordedAt.UnixNano()), string(b)+"\n")
		}
	}
	if err != nil {
		fmt.Fprintln(stdout, "decision memory: capture telemetry unavailable")
	}
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
			fmt.Fprintf(stdout, "  capture=%s reason=%s duration=%dms\n", c.Status, c.Reason, c.DurationMS)
		}
	}
	fmt.Fprintln(stdout, "Prompt inclusion is observed; usefulness and causal benefit are not inferred.")
	return exitOK
}
