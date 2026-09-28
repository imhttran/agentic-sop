// Package harness provides deterministic test infrastructure for the full
// agent harness regression suite (PREJEV012). Everything here is fake and
// in-process: no live Ollama, no network, no real clock, and no real ids. Tests
// in this suite build fake providers, fake command adapters, temp repositories,
// and structured outcomes on top of these helpers so lifecycle coverage is
// reproducible across repeated runs and under the race detector.
//
// The helpers are deliberately small and focused. They live in the internal e2e
// tree so they can exercise the real agent/planner/agentbin packages while still
// being importable by every regression test in the suite.
package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// FakeProvider is a deterministic agent.Agent used across the suite. It records
// every request it receives (so tests can assert on invocation isolation),
// returns canned responses in order, and never touches the network or the
// filesystem.
type FakeProvider struct {
	mu        sync.Mutex
	requests  []agent.Request
	responses []agent.Response
	errs      []error
	caps      agent.Capabilities
	calls     int64
	// GenerateFunc, when set, takes precedence over the canned responses. It
	// receives the call index (0-based) and the request.
	GenerateFunc func(callIndex int, req agent.Request) (agent.Response, error)
}

// NewFakeProvider returns a FakeProvider serving every capability with the given
// canned responses returned in order. Missing responses fall back to a stable
// completed outcome so tests only need to specify the responses they assert on.
func NewFakeProvider(responses ...agent.Response) *FakeProvider {
	return &FakeProvider{
		responses: responses,
		caps:      agent.AllCapabilities(),
	}
}

// NewFakeProviderWithErr returns a FakeProvider whose canned calls return err.
func NewFakeProviderWithErr(err error) *FakeProvider {
	return &FakeProvider{errs: []error{err}, caps: agent.AllCapabilities()}
}

// WithCapabilities restricts the provider's declared capabilities.
func (f *FakeProvider) WithCapabilities(caps agent.Capabilities) *FakeProvider {
	f.caps = caps
	return f
}

// Capabilities reports the provider's declared capabilities.
func (f *FakeProvider) Capabilities() agent.Capabilities { return f.caps }

// Generate records the request and returns the next canned response. It is safe
// for concurrent use so the race detector can exercise shared state.
func (f *FakeProvider) Generate(_ context.Context, req agent.Request) (agent.Response, error) {
	index := int(atomic.AddInt64(&f.calls, 1)) - 1

	f.mu.Lock()
	f.requests = append(f.requests, req)
	generateFunc := f.GenerateFunc
	var (
		resp agent.Response
		err  error
		ok   bool
	)
	if generateFunc == nil {
		switch {
		case index < len(f.responses):
			resp, ok = f.responses[index], true
		case len(f.errs) > 0:
			err = f.errs[len(f.errs)-1]
		}
	}
	f.mu.Unlock()

	if generateFunc != nil {
		return generateFunc(index, req)
	}
	if ok {
		return resp, nil
	}
	if err != nil {
		return agent.Response{}, err
	}
	return agent.Response{
		Content: "ok",
		Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "ok", ChangesExpected: false},
	}, nil
}

// Requests returns a copy of the requests the provider has received.
func (f *FakeProvider) Requests() []agent.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]agent.Request, len(f.requests))
	copy(out, f.requests)
	return out
}

// Calls reports how many times Generate was invoked.
func (f *FakeProvider) Calls() int { return int(atomic.LoadInt64(&f.calls)) }

// Reset clears the recorded requests and the call counter while keeping the
// canned responses, so a subsequent invocation starts from a clean slate.
func (f *FakeProvider) Reset() {
	f.mu.Lock()
	f.requests = nil
	f.mu.Unlock()
	atomic.StoreInt64(&f.calls, 0)
}

// Clock is a deterministic clock. Tests read it explicitly instead of sleeping,
// which keeps the suite fast and repeatable.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

// NewClock returns a Clock anchored at a fixed instant.
func NewClock(start time.Time) *Clock { return &Clock{now: start} }

// Now returns the current instant.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// IDs is a deterministic id generator producing stable, sequential values.
type IDs struct {
	mu  sync.Mutex
	n   int
	got string
}

// NewIDs returns an id generator whose values are prefixed with prefix.
func NewIDs(prefix string) *IDs { return &IDs{got: prefix} }

// Next returns the next id, e.g. "test-1", "test-2".
func (g *IDs) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return fmt.Sprintf("%s-%d", g.got, g.n)
}

// Adapter is a fake command adapter. It mirrors the stdin/stdout contract the
// real CommandAgent expects (a JSON request on stdin, raw output on stdout)
// without starting a process, so command adapter compatibility can be exercised
// deterministically.
type Adapter struct {
	// Name identifies the adapter variant under test.
	Name string
	// Output is the raw stdout the adapter produces. When empty, a completed
	// outcome is synthesized.
	Output string
	// ExitCode, when non-zero, makes the adapter behave like a failing process.
	ExitCode int
	// Stderr is diagnostic output a failing adapter emits.
	Stderr string
	// Malformed makes the adapter emit non-JSON output that does not satisfy the
	// contract.
	Malformed bool

	mu       sync.Mutex
	received []agent.Request
}

// Run applies the adapter to a request, recording it and returning the raw
// output or a process-like error.
func (a *Adapter) Run(req agent.Request) (agent.Response, error) {
	a.mu.Lock()
	a.received = append(a.received, req)
	a.mu.Unlock()

	if a.ExitCode != 0 {
		return agent.Response{}, fmt.Errorf("adapter %s exited %d: %s", a.Name, a.ExitCode, a.Stderr)
	}
	if a.Malformed {
		return agent.Response{}, fmt.Errorf("adapter %s returned malformed output", a.Name)
	}
	content := a.Output
	if content == "" {
		content = "{\"status\":\"completed\",\"summary\":\"adapter ok\",\"changes_expected\":false}"
	}
	return agent.Response{Content: content, Outcome: agent.ParseOutcome(content)}, nil
}

// Received returns the requests the adapter has seen.
func (a *Adapter) Received() []agent.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]agent.Request, len(a.received))
	copy(out, a.received)
	return out
}

// Repo is a temporary repository fixture. It can be created clean or pre-dirty
// so lifecycle behavior on an already-modified working tree can be asserted.
type Repo struct {
	Dir string
}

// NewRepo creates a clean temporary git repository containing a single tracked
// file. The directory is removed when the test ends.
func NewRepo(t interface{ TempDir() string }) (*Repo, error) {
	dir := t.TempDir()
	repo := &Repo{Dir: dir}
	if err := repo.git("init", "-q"); err != nil {
		return nil, err
	}
	if err := repo.git("config", "user.email", "test@example.com"); err != nil {
		return nil, err
	}
	if err := repo.git("config", "user.name", "Test"); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("baseline\n"), 0o644); err != nil {
		return nil, err
	}
	if err := repo.git("add", "tracked.txt"); err != nil {
		return nil, err
	}
	if err := repo.git("commit", "-q", "-m", "baseline"); err != nil {
		return nil, err
	}
	return repo, nil
}

// Dirty marks the repository pre-dirty by leaving an uncommitted modification to
// a tracked file. It returns the repository for chaining.
func (r *Repo) Dirty() (*Repo, error) {
	if err := os.WriteFile(filepath.Join(r.Dir, "tracked.txt"), []byte("modified\n"), 0o644); err != nil {
		return nil, err
	}
	return r, nil
}

// Untracked adds an uncommitted file the harness must see as pre-dirty.
func (r *Repo) Untracked(name string) (*Repo, error) {
	if err := os.WriteFile(filepath.Join(r.Dir, name), []byte("new\n"), 0o644); err != nil {
		return nil, err
	}
	return r, nil
}

// IsDirty reports whether the working tree has uncommitted changes. It is the
// deterministic detection the pre-dirty lifecycle tests assert against.
func (r *Repo) IsDirty() (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.Dir
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

func (r *Repo) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return nil
}

// Completed builds a completed structured outcome with the given change intent.
func Completed(summary string, changesExpected bool) *agent.Outcome {
	return &agent.Outcome{Status: agent.OutcomeCompleted, Summary: summary, ChangesExpected: changesExpected}
}

// NeedsHuman builds a needs_human structured outcome.
func NeedsHuman(reason string) *agent.Outcome {
	return &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}
}

// Failed builds a failed structured outcome.
func Failed(reason string) *agent.Outcome {
	return &agent.Outcome{Status: agent.OutcomeFailed, Reason: reason}
}

// Response wraps an outcome with a text body.
func Response(body string, outcome *agent.Outcome) agent.Response {
	return agent.Response{Content: body, Outcome: outcome}
}
