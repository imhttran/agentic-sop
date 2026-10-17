package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// externalAdapter runs an operator-provided executable that speaks the versioned
// JSON request/response protocol. Descriptor fields are configured; the response
// is validated before SOP uses it. The process never receives lifecycle,
// approval, or policy authority.
type externalAdapter struct {
	id           string
	category     Category
	command      []string
	version      string
	configDigest string
	exec         Executor
	timeout      time.Duration
	maxOutput    int
}

func (a *externalAdapter) Descriptor() Descriptor {
	return Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       a.id,
		AdapterVersion:  a.version,
		Categories:      []Category{a.category},
		ScopeModes:      []string{"workspace", "changed-files"},
		Locality:        "local",
		Network:         "declared by the adapter configuration; SOP makes no request",
	}
}

func (a *externalAdapter) currentProvenance(req Request) (Provenance, error) {
	return Provenance{
		ScannerVersion:      a.version,
		ConfigurationDigest: a.configDigest,
		ContentIdentity:     req.Content,
	}, nil
}

func (a *externalAdapter) Scan(ctx context.Context, req Request) (Execution, error) {
	exec := newExecution(a.id, a.category, a.version, a.configDigest, "", req)
	exec.Provenance.ScannerVersion = a.version
	if strings.TrimSpace(req.WorkspaceRoot) == "" {
		exec.Status = StatusFailed
		exec.Diagnostics = []string{"workspace root is required"}
		return finish(exec, nil)
	}
	req.ProtocolVersion = ProtocolVersion
	payload, err := json.Marshal(req)
	if err != nil {
		exec.Status = StatusFailed
		exec.Diagnostics = []string{"encode request: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	runCtx, cancel := scanContext(ctx, a.timeout)
	defer cancel()

	stdout, stderr, exitCode, runErr := a.exec.Run(runCtx, req.WorkspaceRoot, a.command, payload, a.maxOutput)
	exec.ExitCode = exitCode
	if runErr != nil {
		switch {
		case errors.Is(runErr, ErrUnavailable):
			exec.Status = StatusUnavailable
			exec.Diagnostics = []string{"adapter executable is unavailable"}
		case errors.Is(runErr, context.DeadlineExceeded):
			exec.Status = StatusTimedOut
			exec.Diagnostics = []string{"adapter exceeded the scan deadline"}
		case errors.Is(runErr, context.Canceled):
			exec.Status = StatusCancelled
			exec.Diagnostics = []string{"adapter was cancelled"}
		default:
			exec.Status = StatusFailed
			exec.Diagnostics = []string{"adapter execution failed: " + SanitizeText(runErr.Error(), MaxDiagnosticBytes)}
		}
		return finish(exec, nil)
	}
	if len(stderr) > 0 {
		// Stderr is diagnostics only. It is bounded and sanitized, never parsed.
		exec.Diagnostics = append(exec.Diagnostics, SanitizeText(string(stderr), MaxDiagnosticBytes))
	}
	if len(stdout) == 0 {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, "adapter returned empty output")
		return finish(exec, nil)
	}
	if a.maxOutput > 0 && len(stdout) >= a.maxOutput {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("adapter output reached the %d-byte limit", a.maxOutput))
		return finish(exec, nil)
	}
	var response Execution
	if err := json.Unmarshal(stdout, &response); err != nil {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, "adapter returned malformed JSON: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
		return finish(exec, nil)
	}
	if exitCode != 0 {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("adapter exited with code %d", exitCode))
		return finish(exec, nil)
	}
	response.ProtocolVersion = ProtocolVersion
	if response.AdapterID != a.id {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("adapter response id %q does not match configured id %q", response.AdapterID, a.id))
		return finish(exec, nil)
	}
	if response.Category != a.category {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("adapter response category %q does not match configured category %q", response.Category, a.category))
		return finish(exec, nil)
	}
	response.Provenance = mergeProvenance(response.Provenance, req, a.configDigest)
	if response.Provenance.ScannerVersion == "" {
		response.Provenance.ScannerVersion = a.version
	}
	response.StartedAt = nonzeroTime(response.StartedAt, exec.StartedAt)
	response.EndedAt = nonzeroTime(response.EndedAt, time.Now().UTC())
	if response.AdapterVersion == "" {
		response.AdapterVersion = a.version
	}
	sanitizeExecution(&response)
	if err := response.Validate(); err != nil {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, "adapter response invalid: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
		return finish(exec, nil)
	}
	return finish(response, nil)
}

func mergeProvenance(prov Provenance, req Request, configDigest string) Provenance {
	if prov.ContentIdentity.Commit == "" && prov.ContentIdentity.Tree == "" && prov.ContentIdentity.Digest == "" {
		prov.ContentIdentity = req.Content
	}
	if prov.ConfigurationDigest == "" {
		prov.ConfigurationDigest = configDigest
	}
	return prov
}

func nonzeroTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func sanitizeExecution(exec *Execution) {
	exec.Diagnostics = sanitizeList(exec.Diagnostics)
	for i := range exec.Findings {
		f := &exec.Findings[i]
		f.AdapterID = exec.AdapterID
		f.Provider = firstNonEmpty(f.Provider, exec.AdapterID)
		if f.Category == "" {
			f.Category = exec.Category
		}
		f.Message = SanitizeText(f.Message, MaxMessageBytes)
		if f.Path != "" {
			if safe, reason := NormalizePath(f.Path); safe != "" {
				f.Path = safe
			} else {
				exec.Diagnostics = append(exec.Diagnostics, "finding path rejected: "+reason)
				f.Path = ""
			}
		}
		if f.Metadata != nil {
			sanitized := make(map[string]string, len(f.Metadata))
			for key, value := range f.Metadata {
				sanitized[SanitizeText(key, 128)] = SanitizeText(value, MaxMessageBytes)
			}
			f.Metadata = sanitized
		}
	}
	exec.Skipped = sanitizeSkipped(exec.Skipped)
}

func sanitizeList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = SanitizeText(value, MaxDiagnosticBytes); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func sanitizeSkipped(values []SkippedFile) []SkippedFile {
	out := make([]SkippedFile, 0, len(values))
	for _, value := range values {
		path, reason := NormalizePath(value.Path)
		if path == "" {
			path = "."
		}
		out = append(out, SkippedFile{Path: path, Reason: SanitizeText(reason+" "+value.Reason, MaxDiagnosticBytes)})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
