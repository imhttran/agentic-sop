// Package context defines the canonical, deterministic representation of the
// evidence supplied to an agent for one execution — the Context Engine.
//
// The deterministic harness owns selection, ordering, provenance, limits, and
// truncation. The model consumes the result; it never selects or retrieves its own
// context. Construction is pure and model-free: Build invokes no agent and no
// provider.
//
// Limits are expressed in deterministic units — bytes, characters, lines, items,
// and files — never in tokens: reliable provider-independent token accounting does
// not exist, so a token budget would be authority SOP cannot justify.
package context

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Source is the deterministic category a context item comes from. The sources are the
// canonical organization of the context supplied to an agent.
type Source string

const (
	// SourceTask is the task specification, its acceptance criteria, and the plan it
	// executes.
	SourceTask Source = "task"
	// SourceRepository is repository evidence already identified deterministically
	// (repository root, branch, known/changed files, package metadata, explicit task
	// references). It is not semantic retrieval.
	SourceRepository Source = "repository"
	// SourceExecution is the execution identity: the selected model class, provider,
	// model, execution budget, and attempt number.
	SourceExecution Source = "execution"
	// SourceRecovery is the failure, continuation, escalation, and replan evidence
	// handed to another attempt.
	SourceRecovery Source = "recovery"
	// SourceMemory is durable decision memory: engineering decisions applicable to the
	// current repository, supplied as evidence. It is guidance, not authority: it carries
	// the lowest priority and never overrides current repository, lifecycle, or
	// verification evidence.
	SourceMemory Source = "memory"
)

// Valid reports whether s is a known source.
func (s Source) Valid() bool {
	switch s {
	case SourceTask, SourceRepository, SourceExecution, SourceRecovery, SourceMemory:
		return true
	}
	return false
}

// Priority is the deterministic inclusion priority of a context item; a lower value
// sorts first (higher priority). Explicit and current evidence outranks generic
// context.
const (
	PriorityExplicit   = 0 // explicit task references
	PriorityLifecycle  = 1 // current task/lifecycle evidence
	PriorityRepository = 2 // repository evidence already identified deterministically
	PriorityExecution  = 3 // execution identity and budget
	PriorityRecovery   = 4 // failure/continuation/escalation/replan evidence
	PriorityGeneric    = 5 // generic bounded context
)

// Item is one piece of context with deterministic provenance: what it is, where it
// came from, why it was included, its priority, and its size.
type Item struct {
	Source   Source `json:"source"`
	Identity string `json:"identity,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Priority int    `json:"priority"`
	Text     string `json:"text,omitempty"`
}

// Bytes is the item size in bytes.
func (i Item) Bytes() int { return len(i.Text) }

// Lines is the item size in lines (0 for an empty item, otherwise at least 1).
func (i Item) Lines() int {
	if i.Text == "" {
		return 0
	}
	return strings.Count(i.Text, "\n") + 1
}

// Limits bound the context in deterministic units. A zero field means "unbounded"
// for that unit; DefaultLimits are finite.
type Limits struct {
	MaxItems int
	MaxFiles int
	MaxBytes int
}

// DefaultLimits are the built-in finite context limits.
func DefaultLimits() Limits {
	return Limits{MaxItems: 64, MaxFiles: 32, MaxBytes: 64 << 10}
}

// Context is the canonical, deterministically ordered and bounded context supplied
// to an agent. The zero Context is empty and valid.
type Context struct {
	items     []Item
	limits    Limits
	truncated bool
}

// Build orders and bounds items deterministically. The result depends only on the
// set of items and the limits, so identical inputs produce a semantically identical
// Context regardless of the caller's iteration order. Ordering is total — priority,
// then source, identity, reason, and finally content — so no map, filesystem, or
// serialization order can leak into the result.
//
// Truncation is deterministic and observable: items are kept in order until the item
// ceiling or the file ceiling is reached, the item that would exceed the byte ceiling
// is included only up to a rune-safe prefix of the remaining budget, and every later
// item is dropped.
func Build(items []Item, lim Limits) Context {
	ordered := append([]Item(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool { return less(ordered[i], ordered[j]) })

	c := Context{limits: lim}
	seen := make(map[string]bool)
	bytes := 0
	for _, it := range ordered {
		if lim.MaxItems > 0 && len(c.items) >= lim.MaxItems {
			c.truncated = true
			break
		}
		isNewFile := isFile(it) && !seen[it.Identity]
		if isNewFile && lim.MaxFiles > 0 && len(seen) >= lim.MaxFiles {
			c.truncated = true
			break
		}
		if lim.MaxBytes > 0 && bytes+len(it.Text) > lim.MaxBytes {
			if remaining := lim.MaxBytes - bytes; remaining > 0 {
				it.Text = truncateRunes(it.Text, remaining)
				c.items = append(c.items, it)
			}
			c.truncated = true
			break
		}
		c.items = append(c.items, it)
		bytes += len(it.Text)
		if isNewFile {
			seen[it.Identity] = true
		}
	}
	return c
}

// isFile reports whether an item represents a repository file: a non-empty identity
// on repository evidence. Only file identities count toward the file ceiling.
func isFile(it Item) bool {
	return it.Source == SourceRepository && it.Identity != ""
}

// less is the total order over items: priority, then source, identity, reason, and
// finally content, so the outcome never depends on the caller's iteration order.
func less(a, b Item) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Identity != b.Identity {
		return a.Identity < b.Identity
	}
	if a.Reason != b.Reason {
		return a.Reason < b.Reason
	}
	return a.Text < b.Text
}

// truncateRunes returns the longest prefix of s that is at most n bytes and ends on
// a rune boundary, so a truncated item never splits a UTF-8 sequence.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// Items returns a copy of the ordered, bounded items, so a caller cannot mutate the
// Context's retained evidence.
func (c Context) Items() []Item { return append([]Item(nil), c.items...) }

// Bytes is the total retained content size in bytes.
func (c Context) Bytes() int {
	n := 0
	for _, it := range c.items {
		n += it.Bytes()
	}
	return n
}

// Lines is the total retained content size in lines.
func (c Context) Lines() int {
	n := 0
	for _, it := range c.items {
		n += it.Lines()
	}
	return n
}

// Files is the number of distinct repository file identities represented.
func (c Context) Files() int {
	seen := make(map[string]bool)
	for _, it := range c.items {
		if isFile(it) {
			seen[it.Identity] = true
		}
	}
	return len(seen)
}

// Truncated reports whether a limit dropped or shortened any item.
func (c Context) Truncated() bool { return c.truncated }

// Empty reports whether the context retains no items.
func (c Context) Empty() bool { return len(c.items) == 0 }

// SourceCount is the per-source item and byte count, for deterministic observability.
type SourceCount struct {
	Source Source `json:"source"`
	Items  int    `json:"items"`
	Bytes  int    `json:"bytes"`
}

// sourceOrder is the canonical, stable order of sources in a summary.
var sourceOrder = []Source{SourceTask, SourceRepository, SourceExecution, SourceRecovery, SourceMemory}

// Sources returns the per-source counts in the canonical source order. Only sources
// that contributed an item are returned, so the summary is deterministic.
func (c Context) Sources() []SourceCount {
	counts := make(map[Source]*SourceCount, len(sourceOrder))
	for _, it := range c.items {
		sc := counts[it.Source]
		if sc == nil {
			sc = &SourceCount{Source: it.Source}
			counts[it.Source] = sc
		}
		sc.Items++
		sc.Bytes += it.Bytes()
	}
	var out []SourceCount
	for _, s := range sourceOrder {
		if sc := counts[s]; sc != nil {
			out = append(out, *sc)
		}
	}
	return out
}

// Render returns the deterministic text of the retained context: each non-empty
// item's content, in order, separated by a blank line. Rendering depends only on the
// ordered items.
func (c Context) Render() string {
	parts := make([]string, 0, len(c.items))
	for _, it := range c.items {
		if it.Text == "" {
			continue
		}
		parts = append(parts, it.Text)
	}
	return strings.Join(parts, "\n\n")
}

// Inputs is the already-known evidence the Context Engine organizes. Every field is
// evidence the harness already holds; the Context Engine performs no retrieval.
type Inputs struct {
	// TaskID and Task are the task's identity and its rendered specification.
	TaskID string
	Task   string
	// Plan is the plan the task executes.
	Plan string
	// AcceptanceCriteria are the task's caller-owned criteria.
	AcceptanceCriteria []string
	// ChangedFiles are repository-relative paths of files already identified as this
	// task's change. Their content is not read: repository context is the known
	// evidence, never semantic retrieval.
	ChangedFiles []string
	// Execution and Recovery are items the harness built from execution identity and
	// from failure/continuation/escalation/replan evidence.
	Execution []Item
	Recovery  []Item
	// Memory is the durable decision memory already identified as applicable to this
	// repository. It is included verbatim and carries the lowest priority; it never
	// overrides current repository, lifecycle, or verification evidence.
	Memory []Item
}

// FromInputs assembles the canonical items from already-known evidence and returns
// the ordered, bounded Context. Selection is deterministic: task and plan evidence
// carries lifecycle priority, changed-file paths carry repository priority, and the
// caller's execution, recovery, and memory items are included verbatim.
func FromInputs(in Inputs, lim Limits) Context {
	var items []Item

	if text := strings.TrimSpace(in.Task); text != "" {
		items = append(items, Item{
			Source:   SourceTask,
			Identity: in.TaskID,
			Reason:   "the task under execution",
			Priority: PriorityLifecycle,
			Text:     text,
		})
	}
	if text := strings.TrimSpace(in.Plan); text != "" {
		items = append(items, Item{
			Source:   SourceTask,
			Identity: in.TaskID,
			Reason:   "the plan the task executes",
			Priority: PriorityLifecycle,
			Text:     text,
		})
	}
	if len(in.AcceptanceCriteria) > 0 {
		items = append(items, Item{
			Source:   SourceTask,
			Identity: in.TaskID,
			Reason:   "the task's acceptance criteria",
			Priority: PriorityLifecycle,
			Text:     strings.Join(in.AcceptanceCriteria, "\n"),
		})
	}

	// Repository evidence is the set of already-known changed paths, sorted so the
	// representation never depends on discovery order.
	files := append([]string(nil), in.ChangedFiles...)
	sort.Strings(files)
	for _, path := range files {
		if strings.TrimSpace(path) == "" {
			continue
		}
		items = append(items, Item{
			Source:   SourceRepository,
			Identity: path,
			Reason:   "changed by this task",
			Priority: PriorityRepository,
			Text:     path,
		})
	}

	items = append(items, in.Execution...)
	items = append(items, in.Recovery...)
	items = append(items, in.Memory...)
	return Build(items, lim)
}
