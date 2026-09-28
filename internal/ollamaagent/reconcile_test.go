package ollamaagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// PREJEV009 — Hardening repository-change reconciliation.
//
// These tests pin the split between the invocation's own mutation evidence (the
// primary execution signal) and the working-tree divergence from the invocation's
// baseline fingerprint (reconciliation evidence), so a pre-dirty working tree is
// never attributed to the invocation, and the model's changes_expected claim can
// never override observed reality.

// gitCommand builds a git command rooted at dir, for the pre-dirty fixtures.
func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

// preDirtyWorktree establishes a working tree that is already dirty before the
// invocation runs: a modified tracked file plus an untracked file. The baseline
// captured afterwards therefore already contains those changes, so nothing the
// invocation does not itself change may be attributed to it.
func preDirtyWorktree(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "tracked.txt", "committed")
	commitAll(t, dir, "seed tracked file")
	// Pre-existing changes, present before the invocation's baseline.
	writeFile(t, dir, "tracked.txt", "modified before the invocation")
	writeFile(t, dir, "untracked.txt", "created before the invocation")
}

// commitAll commits the current working tree, so a test can establish a clean
// HEAD and then dirty the tree deliberately.
func commitAll(t *testing.T, dir, message string) {
	t.Helper()
	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-q", "-m", message},
	} {
		cmd := gitCommand(dir, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
}

// --- Mutation evidence is the primary execution signal ---

// TestObservedRepositoryChangeMutationEvidenceWins proves a successful controlled
// mutation is a positive change signal regardless of what the working tree looks
// like at reconciliation time — including a tree that already had changes before
// the invocation.
func TestObservedRepositoryChangeMutationEvidenceWins(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()

	ev := &mutationEvidence{}
	ev.record(toolharness.ToolWriteFile)

	// A blank baseline (fingerprint unavailable) must not defeat positive evidence.
	observed, known := observedRepositoryChange(ctx, h, "", ev)
	if !known || !observed {
		t.Errorf("observedRepositoryChange = (%v, %v), want (true, true): a successful controlled mutation is positive evidence", observed, known)
	}
}

// TestObservedRepositoryChangeNoEvidenceNoBaselineUnknown proves that with no
// mutation evidence and no baseline there is no observed signal at all, so the
// model's claim must be left untouched rather than invented.
func TestObservedRepositoryChangeNoEvidenceNoBaselineUnknown(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	h := New(testConfig("http://fake"), dir)

	observed, known := observedRepositoryChange(context.Background(), h, "", &mutationEvidence{})
	if known {
		t.Errorf("observedRepositoryChange known = true, want false (no signal available)")
	}
	if observed {
		t.Errorf("observedRepositoryChange observed = true, want false when unknown")
	}
}

// TestMutationEvidenceIsInvocationScoped proves two sequential Complete calls use
// independent mutation-evidence accumulators: invocation A's mutation cannot make
// invocation B claim a change it did not make.
func TestMutationEvidenceIsInvocationScoped(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	// Invocation A mutates; invocation B only reads and then claims no change.
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"a.txt","content":"a"}}`,
		`{"status":"completed","summary":"A changed it","changes_expected":true}`,
		`{"tool":"read_file","args":{"path":"a.txt"}}`,
		`{"status":"completed","summary":"B changed nothing","changes_expected":false}`,
	)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	h := New(testConfig(srv.URL), dir)
	ctx := context.Background()

	first, firstReconciled, err := h.Complete(ctx, implementRequest())
	if err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if !strings.Contains(first, `"changes_expected":true`) {
		t.Errorf("first outcome = %q, want changes_expected=true", first)
	}
	if firstReconciled {
		t.Errorf("first reconciled = true, want false (the model's claim matched the mutation)")
	}

	second, secondReconciled, err := h.Complete(ctx, implementRequest())
	if err != nil {
		t.Fatalf("second Complete: %v", err)
	}
	if !strings.Contains(second, `"changes_expected":false`) {
		t.Errorf("second outcome = %q, want changes_expected=false: invocation A's mutation must not leak into invocation B", second)
	}
	if secondReconciled {
		t.Errorf("second reconciled = true, want false (no disagreement)")
	}
}

// --- Pre-dirty reconciliation via baseline fingerprint ---

// TestPreDirtyNoInvocationChangeIsNotAttributed proves a completed claim of
// changes_expected=true over a working tree that was already dirty before the
// invocation is reconciled to false when the invocation itself changed nothing.
func TestPreDirtyNoInvocationChangeIsNotAttributed(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	preDirtyWorktree(t, dir)

	// The model claims a change but performs none; the working tree was already
	// dirty before the invocation's baseline, so nothing is attributed to it.
	_, srv := newFakeOllama(t, `{"status":"completed","summary":"did it","changes_expected":true}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut strings.Builder
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"changes_expected":false`) {
		t.Errorf("out = %q, want pre-existing dirtiness reconciled away", out.String())
	}
}

// TestPreDirtyWithInvocationChangeIsAttributed proves that when the invocation
// does mutate over a pre-dirty tree, the change is attributed to it.
func TestPreDirtyWithInvocationChangeIsAttributed(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	preDirtyWorktree(t, dir)

	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"new.txt","content":"by the invocation"}}`,
		`{"status":"completed","summary":"did it","changes_expected":true}`,
	)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut strings.Builder
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"changes_expected":true`) {
		t.Errorf("out = %q, want the invocation's own mutation observable", out.String())
	}
}

// TestPreDirtyContentPreservedAcrossInvocation proves a completed invocation over
// a pre-dirty tree leaves the pre-existing user changes exactly as they were: the
// harness reconciles the model's claim but never reverts or rewrites the working
// tree, so pre-existing work is preserved rather than discarded.
func TestPreDirtyContentPreservedAcrossInvocation(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	preDirtyWorktree(t, dir)

	// The model claims a change but performs none; reconciliation must not touch
	// the pre-existing files.
	_, srv := newFakeOllama(t, `{"status":"completed","summary":"did it","changes_expected":true}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut strings.Builder
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("read tracked.txt: %v", err)
	}
	if string(got) != "modified before the invocation" {
		t.Errorf("tracked.txt = %q, want the pre-existing content preserved", got)
	}
	got, err = os.ReadFile(filepath.Join(dir, "untracked.txt"))
	if err != nil {
		t.Fatalf("read untracked.txt: %v", err)
	}
	if string(got) != "created before the invocation" {
		t.Errorf("untracked.txt = %q, want the pre-existing content preserved", got)
	}
}

// TestPreDirtyStagedChangeNotAttributed covers a staged pre-existing change (one
// already in the index before the invocation).
func TestPreDirtyStagedChangeNotAttributed(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, "staged.txt", "committed")
	commitAll(t, dir, "seed staged file")
	// A staged, pre-existing change: present in the index before the invocation.
	writeFile(t, dir, "staged.txt", "staged before the invocation")
	if out, err := gitCommand(dir, "add", "staged.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v (%s)", err, out)
	}

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()
	baseline, err := h.tools.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("WorkingTreeFingerprint: %v", err)
	}
	changed, err := h.tools.ChangedSince(ctx, baseline)
	if err != nil {
		t.Fatalf("ChangedSince: %v", err)
	}
	if changed {
		t.Error("ChangedSince = true for an unchanged tree, want false: a staged pre-existing change must be captured in the baseline")
	}
}

// TestChangedSinceReportsBaselineDivergence proves ChangedSince is false for a
// tree unchanged since the baseline and true once the invocation changes it.
func TestChangedSinceReportsBaselineDivergence(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, "file.txt", "committed")
	commitAll(t, dir, "seed")

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()
	baseline, err := h.tools.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("WorkingTreeFingerprint: %v", err)
	}

	changed, err := h.tools.ChangedSince(ctx, baseline)
	if err != nil {
		t.Fatalf("ChangedSince: %v", err)
	}
	if changed {
		t.Error("ChangedSince = true for an unchanged tree, want false")
	}

	writeFile(t, dir, "file.txt", "modified after the baseline")
	changed, err = h.tools.ChangedSince(ctx, baseline)
	if err != nil {
		t.Fatalf("ChangedSince: %v", err)
	}
	if !changed {
		t.Error("ChangedSince = false for a changed tree, want true")
	}
}

// TestChangedSinceIgnoresRestoredFile proves the working-tree fingerprint is
// content-sensitive: a tracked file changed and then restored to its committed
// content is not a change, so a restored file is never attributed to the
// invocation.
func TestChangedSinceIgnoresRestoredFile(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, "file.txt", "committed")
	commitAll(t, dir, "seed")

	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()
	baseline, err := h.tools.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("WorkingTreeFingerprint: %v", err)
	}

	// A real change diverges from the baseline.
	writeFile(t, dir, "file.txt", "changed after the baseline")
	changed, err := h.tools.ChangedSince(ctx, baseline)
	if err != nil {
		t.Fatalf("ChangedSince after change: %v", err)
	}
	if !changed {
		t.Error("ChangedSince = false for a changed tree, want true")
	}

	// Restoring the original content returns the tree to its baseline: no change.
	writeFile(t, dir, "file.txt", "committed")
	changed, err = h.tools.ChangedSince(ctx, baseline)
	if err != nil {
		t.Fatalf("ChangedSince after restore: %v", err)
	}
	if changed {
		t.Error("ChangedSince = true for a file restored to baseline, want false: a restored file is not a change")
	}
}

// --- Observed reality overrides model claims ---
// TestReconcileOutcomeFingerprintFailureKeepsModelClaim proves that when the
// working tree cannot be inspected and there is no mutation evidence, the model's
// claim is left as reported rather than invented.
func TestReconcileOutcomeFingerprintFailureKeepsModelClaim(t *testing.T) {
	dir := t.TempDir()
	// Not a git repository: fingerprinting fails, so there is no observed signal.
	h := New(testConfig("http://fake"), dir)

	input := `{"status":"completed","summary":"done","changes_expected":true}`
	result, mismatch := h.reconcileOutcome(context.Background(), "", &mutationEvidence{}, input)
	if mismatch {
		t.Error("mismatch = true with no observed signal, want false (no invented reality)")
	}
	var wire outcomeWire
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if wire.ChangesExpected == nil || *wire.ChangesExpected != true {
		t.Errorf("changes_expected = %v, want the model's true preserved when nothing could be observed", wire.ChangesExpected)
	}
}

// --- Observed reality overrides model claims ---

// TestReconcileOutcomeOverrideBothDirections proves reconciliation corrects the
// model's claim in both directions.
func TestReconcileOutcomeOverrideBothDirections(t *testing.T) {
	t.Run("model true observed false", func(t *testing.T) {
		dir := t.TempDir()
		gitInit(t, dir)
		h := New(testConfig("http://fake"), dir)
		ctx := context.Background()
		baseline, err := h.tools.WorkingTreeFingerprint(ctx)
		if err != nil {
			t.Fatalf("WorkingTreeFingerprint: %v", err)
		}
		input := `{"status":"completed","summary":"did it","changes_expected":true}`
		result, mismatch := h.reconcileOutcome(ctx, baseline, &mutationEvidence{}, input)
		if !mismatch {
			t.Error("mismatch = false, want true (model true / observed false)")
		}
		var wire outcomeWire
		if err := json.Unmarshal([]byte(result), &wire); err != nil {
			t.Fatalf("result not valid JSON: %v", err)
		}
		if wire.ChangesExpected == nil || *wire.ChangesExpected != false {
			t.Errorf("changes_expected = %v, want false", wire.ChangesExpected)
		}
		if !strings.Contains(wire.Summary, "reconciled") {
			t.Errorf("summary = %q, want a disagreement note", wire.Summary)
		}
	})

	t.Run("model false observed true", func(t *testing.T) {
		dir := t.TempDir()
		gitInit(t, dir)
		h := New(testConfig("http://fake"), dir)
		ctx := context.Background()
		baseline, err := h.tools.WorkingTreeFingerprint(ctx)
		if err != nil {
			t.Fatalf("WorkingTreeFingerprint: %v", err)
		}
		writeFile(t, dir, "made.txt", "by the invocation")
		input := `{"status":"completed","summary":"no change","changes_expected":false}`
		result, mismatch := h.reconcileOutcome(ctx, baseline, &mutationEvidence{}, input)
		if !mismatch {
			t.Error("mismatch = false, want true (model false / observed true)")
		}
		var wire outcomeWire
		if err := json.Unmarshal([]byte(result), &wire); err != nil {
			t.Fatalf("result not valid JSON: %v", err)
		}
		if wire.ChangesExpected == nil || *wire.ChangesExpected != true {
			t.Errorf("changes_expected = %v, want true", wire.ChangesExpected)
		}
	})
}

// TestOutcomeWireShapeIsSOPCompatible proves the emitted wire shape and the SOP
// outcome vocabulary are unchanged: only status/summary/reason/changes_expected,
// and non-completed outcomes carry no changes_expected.
func TestOutcomeWireShapeIsSOPCompatible(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()
	baseline, err := h.tools.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("WorkingTreeFingerprint: %v", err)
	}

	// A needs_human outcome is returned unchanged and carries no changes_expected.
	needsHuman := `{"status":"needs_human","reason":"needs a decision"}`
	out, _ := h.reconcileOutcome(ctx, baseline, &mutationEvidence{}, needsHuman)
	out = h.ensureStructuredOutcome(ctx, baseline, &mutationEvidence{}, out)

	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("out not valid JSON: %v", err)
	}
	if _, ok := raw["changes_expected"]; ok {
		t.Errorf("needs_human outcome carries changes_expected: %q", out)
	}
	if raw["status"] != string(agent.OutcomeNeedsHuman) {
		t.Errorf("status = %v, want %q", raw["status"], agent.OutcomeNeedsHuman)
	}
	for key := range raw {
		switch key {
		case "status", "summary", "reason", "changes_expected":
		default:
			t.Errorf("outcome carries unexpected field %q (wire shape drifted)", key)
		}
	}
}

// TestEnsureStructuredOutcomeProseUsesObservedReality proves a prose response is
// wrapped as a completed outcome whose changes_expected comes from the
// invocation's mutation evidence, not from any model claim.
func TestEnsureStructuredOutcomeProseUsesObservedReality(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	h := New(testConfig("http://fake"), dir)
	ctx := context.Background()
	baseline, err := h.tools.WorkingTreeFingerprint(ctx)
	if err != nil {
		t.Fatalf("WorkingTreeFingerprint: %v", err)
	}

	ev := &mutationEvidence{}
	ev.record(toolharness.ToolWriteFile)
	out := h.ensureStructuredOutcome(ctx, baseline, ev, "I changed the file")

	var wire outcomeWire
	if err := json.Unmarshal([]byte(out), &wire); err != nil {
		t.Fatalf("out not valid JSON: %v", err)
	}
	if wire.Status != string(agent.OutcomeCompleted) {
		t.Errorf("status = %q, want completed", wire.Status)
	}
	if wire.ChangesExpected == nil || *wire.ChangesExpected != true {
		t.Errorf("changes_expected = %v, want true from the mutation evidence", wire.ChangesExpected)
	}
}
