package handoff

import (
	"context"
	"errors"
	"testing"
)

type fakeCandidate struct {
	healthy bool
	checks  int
}

func (c *fakeCandidate) Name() string { return "Caveman" }

func (c *fakeCandidate) Check(context.Context) error {
	c.checks++
	if c.healthy {
		return nil
	}
	return errors.New("not ready")
}

func (c *fakeCandidate) Compress(context.Context, ContextBundle) (CompressedContext, error) {
	return CompressedContext{Content: "compressed"}, nil
}

func TestSelectCompressor(t *testing.T) {
	healthy := &fakeCandidate{healthy: true}
	unhealthy := &fakeCandidate{healthy: false}

	tests := []struct {
		name        string
		mode        Mode
		candidate   Candidate
		wantNoOp    bool
		wantNil     bool
		wantCompres string
		wantReason  bool
		wantChecks  int
	}{
		{"none uses NoOp without probing", ModeNone, healthy, true, false, "NoOp", true, 0},
		{"auto unavailable falls back", ModeAuto, nil, true, false, "NoOp", true, 0},
		{"auto unhealthy falls back", ModeAuto, unhealthy, true, false, "NoOp", true, 1},
		{"auto healthy uses candidate", ModeAuto, healthy, false, false, "Caveman", false, 1},
		{"caveman unavailable preserves capsule", ModeCaveman, nil, false, true, "", true, 0},
		{"caveman unhealthy preserves capsule", ModeCaveman, unhealthy, false, true, "", true, 1},
		{"caveman healthy uses candidate", ModeCaveman, healthy, false, false, "Caveman", false, 1},
		{"unknown mode falls back", Mode("bogus"), healthy, true, false, "NoOp", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if c, ok := tt.candidate.(*fakeCandidate); ok {
				c.checks = 0
			}
			compressor, diag := SelectCompressor(context.Background(), tt.mode, tt.candidate)

			switch {
			case tt.wantNil && compressor != nil:
				t.Errorf("compressor = %T, want nil", compressor)
			case tt.wantNoOp:
				if _, ok := compressor.(NoOpCompressor); !ok {
					t.Errorf("compressor = %T, want NoOpCompressor", compressor)
				}
			}
			if diag.Compressor != tt.wantCompres {
				t.Errorf("diag.Compressor = %q, want %q", diag.Compressor, tt.wantCompres)
			}
			if (diag.Reason != "") != tt.wantReason {
				t.Errorf("diag.Reason = %q, want set=%v", diag.Reason, tt.wantReason)
			}
			if c, ok := tt.candidate.(*fakeCandidate); ok && c.checks != tt.wantChecks {
				t.Errorf("health checks = %d, want %d", c.checks, tt.wantChecks)
			}
		})
	}
}

// TestHealthCheckCancellation ensures a cancelled health check is treated as
// unavailable rather than corrupting state.
func TestHealthCheckCancellation(t *testing.T) {
	candidate := &cancelCandidate{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, diag := SelectCompressor(ctx, ModeAuto, candidate)
	if diag.Compressor != "NoOp" {
		t.Errorf("auto+cancelled: compressor = %q, want NoOp", diag.Compressor)
	}

	compressor, diag := SelectCompressor(ctx, ModeCaveman, candidate)
	if compressor != nil {
		t.Errorf("caveman+cancelled: compressor = %T, want nil", compressor)
	}
	if diag.Reason == "" {
		t.Error("caveman+cancelled: expected a diagnostic reason")
	}
}

type cancelCandidate struct{}

func (cancelCandidate) Name() string                    { return "Caveman" }
func (cancelCandidate) Check(ctx context.Context) error { return ctx.Err() }
func (cancelCandidate) Compress(context.Context, ContextBundle) (CompressedContext, error) {
	return CompressedContext{}, nil
}

// TestExplicitCavemanUnavailablePreservesCapsule exercises the end-to-end path:
// with mode caveman and no candidate, compression is skipped, the capsule is
// retained, a diagnostic is produced, and workflow correctness is unaffected.
func TestExplicitCavemanUnavailablePreservesCapsule(t *testing.T) {
	compressor, diag := SelectCompressor(context.Background(), ModeCaveman, nil)
	if compressor != nil {
		t.Fatalf("compressor = %T, want nil", compressor)
	}
	if diag.Reason == "" {
		t.Fatal("expected a diagnostic reason for unavailable Caveman")
	}

	store := &fakeStore{}
	manager := newManager(store, compressor, 1) // over budget, but no compressor
	record, err := manager.Complete(context.Background(), doneTask("T1"), Facts{Summary: "kept"}, []Artifact{artifact(100)})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if record.Status != StatusNotRequested {
		t.Errorf("status = %s, want NOT_REQUESTED", record.Status)
	}
	if record.Capsule.Summary != "kept" {
		t.Errorf("capsule not retained: %+v", record.Capsule)
	}
	if _, ok := store.saved["T1"]; !ok {
		t.Error("handoff should still be persisted")
	}
}
