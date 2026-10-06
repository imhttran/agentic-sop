package ollamaagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

func TestPhasedVerifiedMutation(t *testing.T) {
	for _, capability := range []agent.Capability{agent.Implement, agent.Fix} {
		for _, tc := range []struct {
			name, tool string
			args       map[string]any
			want       bool
		}{
			{"identical write", "write_file", map[string]any{"path": "a.go", "content": "package a\n\nvar x = 1\n"}, false},
			{"changed write", "write_file", map[string]any{"path": "a.go", "content": "package a\n\nvar x = 2\n"}, true},
			{"create", "create_file", map[string]any{"path": "new.txt", "content": ""}, true},
			{"delete", "delete_file", map[string]any{"path": "a.go"}, true},
			{"restore unchanged", "restore_file", map[string]any{"path": "a.go"}, false},
			{"restore dirty", "restore_file", map[string]any{"path": "unrelated.txt"}, true},
			{"gofmt unchanged", "run_command", map[string]any{"command": "gofmt -w a.go"}, false},
			{"gofmt changed", "run_command", map[string]any{"command": "gofmt -w nested/b.go"}, true},
			{"gofmt list", "run_command", map[string]any{"command": "gofmt -l nested/b.go"}, false},
			{"failed create", "create_file", map[string]any{"path": "a.go", "content": "different"}, false},
			{"failed command", "run_command", map[string]any{"command": "gofmt -w missing.go"}, false},
			{"denied write", "write_file", map[string]any{"path": ".agent-sdlc/state.db", "content": "no"}, false},
		} {
			t.Run(string(capability)+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				gitInit(t, dir)
				writeFile(t, dir, "a.go", "package a\n\nvar x = 1\n")
				writeFile(t, dir, "unrelated.txt", "committed")
				commitAll(t, dir, "seed")
				writeFile(t, dir, "unrelated.txt", "pre-existing dirty work")
				writeFile(t, dir, "nested/b.go", "package b\nvar x=1\n")
				call, err := json.Marshal(map[string]any{"tool": tc.tool, "args": tc.args})
				if err != nil {
					t.Fatal(err)
				}
				fake, srv := newFakeOllama(t, string(call), `{"status":"completed","summary":"done","changes_expected":true}`)
				h := New(testConfig(srv.URL), dir)
				req := implementRequest()
				req.Capability = capability
				ev := &mutationEvidence{}
				sink := &recordSink{}
				ctx := activity.WithRecorder(context.Background(), activity.New("TASK", sink))
				content, _, err := h.CompleteWithEvidence(ctx, req, ev)
				if err != nil {
					t.Fatal(err)
				}
				if ev.observed != tc.want || hasEvent(h.TraceRecords(), implementChangeEvent) != tc.want {
					t.Fatalf("observed=%v trace=%+v, want mutation=%v", ev.observed, h.TraceRecords(), tc.want)
				}
				if !strings.Contains(content, fmt.Sprintf(`"changes_expected":%t`, tc.want)) {
					t.Errorf("outcome=%s, want changes_expected=%t", content, tc.want)
				}
				if !tc.want && len(ev.mutationPaths()) != 0 {
					t.Errorf("no-op paths=%v", ev.mutationPaths())
				}
				for _, event := range sink.events {
					if !tc.want && event.Stage == activity.StageChange {
						t.Errorf("no-op emitted change evidence: %+v", event)
					}
				}
				if fake.count() != 2 || len(h.AuditRecords()) != 1 {
					t.Errorf("tool not executed exactly once: audit=%+v", h.AuditRecords())
				}
			})
		}
	}
}

func TestPhasedObservationFailureCannotEarnMutation(t *testing.T) {
	for _, capability := range []agent.Capability{agent.Implement, agent.Fix} {
		t.Run(string(capability), func(t *testing.T) {
			dir := t.TempDir()
			_, srv := newFakeOllama(t, `{"tool":"create_file","args":{"path":"new.txt","content":"created"}}`, `{"status":"completed","summary":"done","changes_expected":true}`)
			audit := toolharness.NewAuditLog(0)
			h := New(testConfig(srv.URL), dir)
			toolCfg := toolharness.DefaultConfig()
			toolCfg.CommandTimeout = -time.Nanosecond
			h.tools = toolharness.New(dir, toolCfg, audit)
			req := implementRequest()
			req.Capability = capability
			ev := &mutationEvidence{}
			_, err := h.ExecuteWithEvidence(context.Background(), req, ev)
			if err != nil {
				t.Fatal(err)
			}
			if len(audit.Records()) != 1 || ev.observed || len(ev.mutationPaths()) != 0 || hasEvent(h.TraceRecords(), implementChangeEvent) {
				t.Fatalf("unverified execution earned evidence: %+v", ev)
			}
			found := false
			for _, rec := range h.TraceRecords() {
				if rec.Detail == "verification_unavailable" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing safe observation diagnostic: %+v", h.TraceRecords())
			}
		})
	}
}

func TestPhasedPartialCommandFailureDoesNotEarnProgress(t *testing.T) {
	for _, capability := range []agent.Capability{agent.Implement, agent.Fix} {
		t.Run(string(capability), func(t *testing.T) {
			dir := t.TempDir()
			gitInit(t, dir)
			writeFile(t, dir, "valid.go", "package a\nvar x=1\n")
			writeFile(t, dir, "invalid.go", "invalid Go")
			commitAll(t, dir, "seed")
			fake, srv := newFakeOllama(t, `{"tool":"run_command","args":{"command":"gofmt -w valid.go invalid.go"}}`, `{"status":"completed","summary":"partial","changes_expected":false}`)
			h := New(testConfig(srv.URL), dir)
			req := implementRequest()
			req.Capability = capability
			ev := &mutationEvidence{}
			content, _, err := h.CompleteWithEvidence(context.Background(), req, ev)
			if err != nil {
				t.Fatal(err)
			}
			if ev.observed || len(ev.mutationPaths()) != 0 || hasEvent(h.TraceRecords(), implementChangeEvent) {
				t.Fatalf("failed operation earned progress: %+v", ev)
			}
			if !strings.Contains(content, `"changes_expected":true`) {
				t.Errorf("actual partial repository state not reconciled: %s", content)
			}
			if !strings.Contains(messageText(fake.request(1)), "repository state changed during failed operation") {
				t.Error("partial-change diagnostic not supplied")
			}
			if records := h.AuditRecords(); len(records) != 1 || records[0].Outcome != toolharness.OutcomeError {
				t.Errorf("audit=%+v", records)
			}
		})
	}
}

func TestMutationCommandClassification(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    bool
	}{
		{"gofmt -l a.go", false},
		{"gofmt a.go", false},
		{"gofmt -w=false a.go", false},
		{"gofmt -w a.go", true},
		{"gofmt -w=true a.go", true},
		{"gofmt -l -w a.go", true},
		{"/usr/bin/gofmt -w a.go", true},
		{"go test ./...", false},
	} {
		if got := commandMutates(tc.command); got != tc.want {
			t.Errorf("commandMutates(%q)=%v, want %v", tc.command, got, tc.want)
		}
	}
}

func TestPhasedNoOpWritesCannotEvadeNoProgress(t *testing.T) {
	for _, capability := range []agent.Capability{agent.Implement, agent.Fix} {
		t.Run(string(capability), func(t *testing.T) {
			dir := t.TempDir()
			var responses []string
			for i := 0; i < maxNoProgressIterations; i++ {
				name := fmt.Sprintf("noop-%d.txt", i)
				writeFile(t, dir, name, "unchanged")
				responses = append(responses, fmt.Sprintf(`{"tool":"write_file","args":{"path":%q,"content":"unchanged"}}`, name))
			}
			fake, srv := newFakeOllama(t, responses...)
			cfg := testConfig(srv.URL)
			cfg.MaxToolCalls = 20
			h := New(cfg, dir)
			req := implementRequest()
			req.Capability = capability
			ev := &mutationEvidence{}
			_, err := h.ExecuteWithEvidence(context.Background(), req, ev)
			var stalled *noProgressError
			if !errors.As(err, &stalled) || !strings.Contains(err.Error(), string(capability)+"_NO_PROGRESS") {
				t.Fatalf("err=%v, want capability NO_PROGRESS", err)
			}
			if fake.count() != maxNoProgressIterations || ev.observed || len(ev.mutationPaths()) != 0 || hasEvent(h.TraceRecords(), implementChangeEvent) {
				t.Fatalf("no-ops earned progress: turns=%d evidence=%+v trace=%+v", fake.count(), ev, h.TraceRecords())
			}
		})
	}
}
