package sopclient

import (
	"sync"
	"sync/atomic"
	"time"
)

// Refresh-merge rule for checkpoints (CTRL010). The last reported checkpoint per
// task is kept in memory so a transient empty re-read does not regress progress.
// Entries expire after checkpointTTL (so a retracted checkpoint is not replayed
// forever) and the cache is capped at maxCheckpointCache, evicting the oldest.
// Fresh evidence always wins.
const (
	checkpointTTL      = 10 * time.Minute
	maxCheckpointCache = 256
)

type checkpointEntry struct {
	progress CheckpointProgress
	seen     time.Time
}

type checkpointCache struct {
	mu    sync.Mutex
	known map[string]checkpointEntry
	now   func() time.Time // injectable clock for tests
}

func (c *checkpointCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// merge returns the fresh record when present (and remembers it); otherwise the
// remembered one if seen within checkpointTTL; otherwise the absent fresh record.
func (c *checkpointCache) merge(taskID string, fresh CheckpointProgress) CheckpointProgress {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.known == nil {
		c.known = map[string]checkpointEntry{}
	}
	now := c.clock()
	if fresh.Present {
		c.known[taskID] = checkpointEntry{progress: fresh, seen: now}
		c.evictLocked()
		return fresh
	}
	if last, ok := c.known[taskID]; ok {
		if now.Sub(last.seen) <= checkpointTTL {
			return last.progress
		}
		delete(c.known, taskID)
	}
	return fresh
}

// evictLocked drops the least-recently-seen entries. Caller holds c.mu.
func (c *checkpointCache) evictLocked() {
	for len(c.known) > maxCheckpointCache {
		var oldestID string
		var oldest time.Time
		for id, e := range c.known {
			if oldestID == "" || e.seen.Before(oldest) {
				oldestID, oldest = id, e.seen
			}
		}
		delete(c.known, oldestID)
	}
}

// checkpointStore lazily creates the cache with CompareAndSwap so concurrent
// readers of one Store converge on a single instance.
type checkpointStore struct {
	ptr atomic.Pointer[checkpointCache]
}

func (c *checkpointStore) get() *checkpointCache {
	if p := c.ptr.Load(); p != nil {
		return p
	}
	fresh := &checkpointCache{}
	if c.ptr.CompareAndSwap(nil, fresh) {
		return fresh
	}
	return c.ptr.Load()
}
