package ollamaagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

func TestEnsureStructuredOutcomeWrapsProseAsCompleted(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// Prose input should be wrapped as a completed outcome
	result := h.ensureStructuredOutcome(ctx, "Implementation complete")

	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if wire.Status != "completed" {
		t.Errorf("status = %q, want completed", wire.Status)
	}
	if wire.Summary != "Implementation complete" {
		t.Errorf("summary = %q, want the prose input", wire.Summary)
	}
	if wire.ChangesExpected == nil || *wire.ChangesExpected != false {
		t.Errorf("changes_expected = %v, want false (no actual changes)", wire.ChangesExpected)
	}
}

func TestEnsureStructuredOutcomePreservesStructuredOutcome(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// Already-structured JSON should be preserved
	input := `{"status":"completed","summary":"done","changes_expected":false}`
	result := h.ensureStructuredOutcome(ctx, input)

	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if wire.Status != "completed" {
		t.Errorf("status = %q, want completed", wire.Status)
	}
	if wire.Summary != "done" {
		t.Errorf("summary = %q, want done", wire.Summary)
	}
}

func TestEnsureStructuredOutcomeHandlesNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// needs_human outcome should be preserved unchanged
	input := `{"status":"needs_human","reason":"manual intervention required"}`
	result := h.ensureStructuredOutcome(ctx, input)

	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if wire.Status != "needs_human" {
		t.Errorf("status = %q, want needs_human", wire.Status)
	}
}

func TestEnsureStructuredOutcomeHandlesEmptyInput(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// Empty input should be wrapped as completed with empty summary
	result := h.ensureStructuredOutcome(ctx, "")

	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if wire.Status != "completed" {
		t.Errorf("status = %q, want completed", wire.Status)
	}
}

func TestEnsureStructuredOutcomeReflectsActualChanges(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Create a file so we can change it
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("original"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// Simulate an actual repository change by modifying the file
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	result := h.ensureStructuredOutcome(ctx, "Changed the file")

	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	// Since the file was modified, changes_expected should be true
	if wire.ChangesExpected == nil || *wire.ChangesExpected != true {
		t.Errorf("changes_expected = %v, want true (file was modified)", wire.ChangesExpected)
	}
}

func TestReconcileOutcomeHandlesProseAndDoesNotChange(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// Prose input: reconcileOutcome should return it unchanged
	prose := "This is just prose from the model"
	result := h.reconcileOutcome(ctx, prose)
	if result != prose {
		t.Errorf("reconcileOutcome changed prose input: got %q, want %q", result, prose)
	}
}

func TestReconcileOutcomeGroundsChangesExpected(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Create a file that we'll later modify
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("original"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// Modify the file to create a repository change
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	// Model claims no changes (false) but we actually changed the file
	input := `{"status":"completed","summary":"done","changes_expected":false}`
	result := h.reconcileOutcome(ctx, input)

	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	// changes_expected should be corrected to true (observed reality)
	if wire.ChangesExpected == nil || *wire.ChangesExpected != true {
		t.Errorf("changes_expected = %v, want true (corrected from model's false)", wire.ChangesExpected)
	}
	// The mismatch should be recorded
	if !h.mismatch {
		t.Error("mismatch flag not set, but model's claim disagreed with observation")
	}
	// The summary should include a note about the reconciliation
	if !strings.Contains(wire.Summary, "reconciled") {
		t.Errorf("summary = %q, want it to mention reconciliation", wire.Summary)
	}
}

func TestRetryNoChangeFailureConvertsFailedToNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	// Model reports failed but didn't change the repository
	input := `{"status":"failed","reason":"could not implement"}`
	result := h.retryNoChangeFailure(ctx, input)

	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	// A failure that changed nothing should become needs_human for retry
	if wire.Status != "needs_human" {
		t.Errorf("status = %q, want needs_human", wire.Status)
	}
	if !strings.Contains(wire.Reason, "no repository change") {
		t.Errorf("reason = %q, want mention of no change", wire.Reason)
	}
}

func TestShowRuntimeVisibilityWithDefault(t *testing.T) {
	t.Setenv(agent.EnvOllamaModel, "")
	dir := t.TempDir()

	cfg := testConfig("http://fake")
	h := New(cfg, dir)

	var buf strings.Builder
	h.ShowRuntimeVisibility(&buf)
	output := buf.String()

	// Verify all required fields are present
	if !strings.Contains(output, "Harness: tool") {
		t.Errorf("output missing 'Harness: tool'")
	}
	if !strings.Contains(output, "Provider: ollama") {
		t.Errorf("output missing 'Provider: ollama'")
	}
	if !strings.Contains(output, "Model: "+DefaultModel) {
		t.Errorf("output missing 'Model: %s'", DefaultModel)
	}
	if !strings.Contains(output, "Provider source: configuration") {
		t.Errorf("output missing 'Provider source: configuration'")
	}
}

func TestShowRuntimeVisibilityWithEnvironment(t *testing.T) {
	t.Setenv(agent.EnvOllamaModel, "custom-model:8b")
	dir := t.TempDir()

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	h := New(cfg, dir)

	var buf strings.Builder
	h.ShowRuntimeVisibility(&buf)
	output := buf.String()

	// Verify environment-based model is shown
	if !strings.Contains(output, "Model: custom-model:8b") {
		t.Errorf("output missing 'Model: custom-model:8b'")
	}
	// Verify source is marked as environment
	if !strings.Contains(output, "Provider source: environment") {
		t.Errorf("output missing 'Provider source: environment'")
	}
}
