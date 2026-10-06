package runtrace

import (
	"context"
	"sync"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
)

// maxIterations bounds the recorded trajectory, so a runaway run cannot grow the
// in-memory collector without limit. It is generous relative to a bounded SOP
// invocation.
const maxIterations = 2000

// Collector accumulates the run activity stream into trace iterations. It is an
// activity.Sink: attached to the activity recorder, it observes the same events
// the CLI renders, without consuming them, changing them, or feeding any
// decision. It is safe for concurrent producers.
type Collector struct {
	mu         sync.Mutex
	now        func() time.Time
	started    time.Time
	seq        int
	iterations []Iteration
}

// NewCollector returns an empty collector. A nil clock uses time.Now.
func NewCollector(now func() time.Time) *Collector {
	if now == nil {
		now = time.Now
	}
	return &Collector{now: now, started: now()}
}

// Emit records one activity event as one trace iteration. A CHANGE-phase event is
// a repository mutation performed by THIS invocation, so its detail (the path the
// producer reported) is recorded as the iteration changed file. Narration, reads,
// and denials are recorded as non-mutating iterations.
func (c *Collector) Emit(e activity.Event) {
	if c == nil {
		return
	}
	iter := Iteration{
		Phase:              e.Stage,
		Timestamp:          e.Timestamp,
		Action:             OneLine(e.Action),
		Observation:        OneLine(e.Detail),
		RepositoryMutation: e.Stage == activity.StageChange,
	}
	if iter.Timestamp.IsZero() {
		iter.Timestamp = c.now()
	}
	if iter.RepositoryMutation && iter.Observation != "" {
		iter.ChangedFiles = []string{iter.Observation}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	iter.Sequence = c.seq
	if len(c.iterations) < maxIterations {
		c.iterations = append(c.iterations, iter)
	}
}

// Iterations returns a copy of the recorded iterations, in sequence order.
func (c *Collector) Iterations() []Iteration {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Iteration, len(c.iterations))
	copy(out, c.iterations)
	return out
}

// Mutations returns how many recorded iterations were repository mutations.
func (c *Collector) Mutations() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, it := range c.iterations {
		if it.RepositoryMutation {
			n++
		}
	}
	return n
}

// collectorKey keys the collector on a context, so the lifecycle can build the
// trace from the same events the recorder observed.
type collectorKey struct{}

// WithCollector returns ctx carrying c.
func WithCollector(ctx context.Context, c *Collector) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, collectorKey{}, c)
}

// CollectorFromContext returns the collector carried by ctx, or nil.
func CollectorFromContext(ctx context.Context) *Collector {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(collectorKey{}).(*Collector); ok {
		return c
	}
	return nil
}
