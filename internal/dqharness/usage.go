package dqharness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// UsageState is the durable accounting for an evaluation run. It lives beside the
// checkpoint and is updated around every provider request, so an interrupted or in-flight
// request is never lost from the totals.
type UsageState struct {
	Binding           Binding `json:"binding"`
	Decisions         int     `json:"decisions"`
	Requests          int     `json:"requests"`
	Retries           int     `json:"retries"`
	Tokens            int     `json:"tokens"`
	ActiveElapsedMS   int64   `json:"active_elapsed_ms"`
	InFlight          bool    `json:"in_flight"`
	UncertainRequests int     `json:"uncertain_requests"`
	UpdatedAt         string  `json:"updated_at"`
}

// LoadUsage reads persisted accounting and validates its binding. A missing file is an
// empty, compatible state. An in-flight request is counted as consumed and recorded as
// uncertain: it must not disappear because the process died.
func LoadUsage(path string, want Binding) (UsageState, error) {
	st := UsageState{Binding: want}
	if path == "" {
		return st, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return UsageState{}, fmt.Errorf("dqharness: decode usage: %w", err)
	}
	if !st.Binding.Same(want) {
		return UsageState{}, fmt.Errorf("dqharness: incompatible usage binding")
	}
	if st.InFlight {
		// The request was already counted before it started. Its token usage is unknown;
		// record the uncertainty instead of silently dropping the request.
		st.InFlight = false
		st.UncertainRequests++
	}
	return st, nil
}

// SaveUsage writes accounting atomically.
func SaveUsage(path string, st UsageState) error {
	if path == "" {
		return nil
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
