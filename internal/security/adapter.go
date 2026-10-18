package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// BuildOptions are the execution settings shared by built-in adapters.
type BuildOptions struct {
	Executor       Executor
	Timeout        time.Duration
	MaxOutputBytes int
	// DatabasePath is the global local advisory database path. A cargo-audit
	// adapter may override it with its own database key.
	DatabasePath string
	// StalePolicy is the global stale-database policy. A cargo-audit adapter may
	// override it with its own stale key.
	StalePolicy string
}

// BuildAdapters turns the validated configuration into adapters. Selection is
// configuration only; the runner never branches on an adapter ID.
func BuildAdapters(cfgs []config.SecurityAdapterConfig, opts BuildOptions) ([]Adapter, error) {
	if opts.Executor == nil {
		opts.Executor = execRunner{}
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = 1 << 20
	}
	out := make([]Adapter, 0, len(cfgs))
	for _, raw := range cfgs {
		adapter, err := BuildAdapter(raw, opts)
		if err != nil {
			return nil, err
		}
		if err := adapter.Descriptor().Validate(); err != nil {
			return nil, fmt.Errorf("security adapter %q: %w", raw.ID, err)
		}
		out = append(out, adapter)
	}
	return out, nil
}

// BuildAdapter builds one configured adapter.
func BuildAdapter(raw config.SecurityAdapterConfig, opts BuildOptions) (Adapter, error) {
	if opts.Executor == nil {
		opts.Executor = execRunner{}
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = 1 << 20
	}
	id := strings.TrimSpace(raw.ID)
	category := Category(strings.ToLower(strings.TrimSpace(raw.Category)))
	command := append([]string(nil), raw.Command...)
	configDigest := adapterConfigDigest(raw)
	switch strings.ToLower(strings.TrimSpace(raw.Protocol)) {
	case "", config.SecurityProtocolBuiltin:
		switch id {
		case "semgrep":
			return &semgrepAdapter{id: id, category: category, command: command, rules: raw.Rules, version: raw.Version, configDigest: configDigest, exec: opts.Executor, timeout: opts.Timeout, maxOutput: opts.MaxOutputBytes}, nil
		case "gitleaks":
			return &gitleaksAdapter{id: id, category: category, command: command, rules: raw.Rules, version: raw.Version, configDigest: configDigest, exec: opts.Executor, timeout: opts.Timeout, maxOutput: opts.MaxOutputBytes}, nil
		case "cargo-audit":
			return &cargoAuditAdapter{
				id:           id,
				category:     category,
				command:      command,
				databasePath: firstNonEmpty(raw.Database, opts.DatabasePath),
				lockfile:     raw.Lockfile,
				stalePolicy:  firstNonEmpty(raw.Stale, opts.StalePolicy),
				version:      raw.Version,
				configDigest: configDigest,
				exec:         opts.Executor,
				timeout:      opts.Timeout,
				maxOutput:    opts.MaxOutputBytes,
			}, nil
		default:
			return nil, fmt.Errorf("security: unknown built-in adapter %q", id)
		}
	case config.SecurityProtocolExternal:
		return &externalAdapter{id: id, category: category, command: command, version: raw.Version, configDigest: configDigest, exec: opts.Executor, timeout: opts.Timeout, maxOutput: opts.MaxOutputBytes}, nil
	default:
		return nil, fmt.Errorf("security: adapter %q has unknown protocol %q", id, raw.Protocol)
	}
}

// adapterConfigDigest hashes the non-secret adapter configuration. Credentials
// never belong in this configuration and are never included.
func adapterConfigDigest(raw config.SecurityAdapterConfig) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.ID)+"\x00")
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.Category)+"\x00")
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.Protocol)+"\x00")
	_, _ = io.WriteString(hash, strings.Join(raw.Command, "\x00")+"\x00")
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.Rules)+"\x00")
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.Database)+"\x00")
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.Lockfile)+"\x00")
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.Stale)+"\x00")
	_, _ = io.WriteString(hash, strings.TrimSpace(raw.Version))
	return hex.EncodeToString(hash.Sum(nil))
}

// rulesIdentity returns a deterministic digest of a rules file or directory. It
// is recorded as provenance; an absent path yields an empty identity and is
// reported by the adapter rather than silently downloading rules.
func rulesIdentity(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write(data)
		return hex.EncodeToString(hash.Sum(nil)), nil
	}
	var files []string
	err = filepath.WalkDir(path, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(path, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(hash, rel+"\x00")
		_, _ = hash.Write(data)
		_, _ = io.WriteString(hash, "\x00")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// newExecution starts a contract-shaped execution record for an adapter.
func newExecution(id string, category Category, version, configDigest, policyIdentity string, req Request) Execution {
	start := time.Now().UTC()
	return Execution{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       id,
		AdapterVersion:  version,
		Category:        category,
		Status:          StatusCompleted,
		StartedAt:       start,
		Coverage:        CoverageUnknown,
		Provenance: Provenance{
			ScannerVersion:      version,
			ConfigurationDigest: configDigest,
			ContentIdentity:     req.Content,
			PolicyIdentity:      policyIdentity,
		},
	}
}

// finish stamps the end time and validates the result. A validation failure is
// returned as a FAILED execution, never a completed one.
func finish(exec Execution, err error) (Execution, error) {
	exec.EndedAt = time.Now().UTC()
	if err != nil {
		return exec, err
	}
	if verr := exec.Validate(); verr != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = append(exec.Diagnostics, "contract validation failed: "+SanitizeText(verr.Error(), MaxDiagnosticBytes))
		return exec, nil
	}
	return exec, nil
}

// nonzeroTime returns value when set, otherwise fallback.
func nonzeroTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

// ensureWorkspaceRoot validates the workspace boundary before any scanner runs.
func ensureWorkspaceRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("security: workspace root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("security: workspace root %q is not a directory", abs)
	}
	return abs, nil
}

// scanContext derives the adapter's bounded context. The adapter never invents
// its own retry or continuation loop; one call is one bounded execution.
func scanContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
