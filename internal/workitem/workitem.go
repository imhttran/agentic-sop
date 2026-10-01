// Package workitem defines SOP's unified execution input: the small adapter
// boundary that lets the same governed execution machinery accept planned project
// tasks and ad-hoc operator prompts.
//
// A WorkItem describes WHAT SOP is being asked to process. It deliberately carries
// no policy: it holds no lifecycle state, no routing rule, no provider fallback,
// no approval authority, and no model-selection logic. Those remain in their
// existing layers (internal/router, internal/model, internal/provider, the
// lifecycle in internal/cli). The item is a projection of the request, not an
// engine.
//
// The abstraction is intentionally small, so Phase 5 recovery can later operate on
// either input form (a task or a prompt, including one submitted through the SOP
// skill) without a second workflow engine:
//
//	Input            ─┬─ task (planned project work)
//	                  ├─ prompt (operator-submitted, ad-hoc)
//	                  └─ skill prompt (an agent invoking `sop prompt`)
//	                       ↓
//	                   WorkItem
//	                       ↓
//	             shared execution services
package workitem

import (
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// Kind is the closed enumeration of what a WorkItem describes. An unknown kind
// fails closed rather than defaulting.
type Kind string

const (
	// KindTask is planned project work managed by the SOP task lifecycle.
	KindTask Kind = "task"
	// KindPrompt is ad-hoc work submitted directly by an operator.
	KindPrompt Kind = "prompt"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool { return k == KindTask || k == KindPrompt }

// WorkItem is the common execution input. It is plain data: an identity, a kind,
// the capability the caller is asking for, a short title, and the content to
// process.
type WorkItem struct {
	// ID identifies the item for run/report organization. It never influences
	// routing or policy.
	ID string
	// Kind is the item's kind (task or prompt).
	Kind Kind
	// Capability is the exact capability being requested. It is explicit, never
	// inferred from prose: a request for REVIEW is never silently upgraded to
	// IMPLEMENT.
	Capability agent.Capability
	// Title is a short human-readable summary for display and metadata.
	Title string
	// Content is the text to process: the rendered task for a task, the operator's
	// prompt for a prompt. Prompt content is preserved verbatim apart from
	// surrounding whitespace.
	Content string
}

// Validate rejects a malformed item. Callers MUST treat an invalid item as a
// failure rather than guessing a default.
func (w WorkItem) Validate() error {
	if strings.TrimSpace(w.ID) == "" {
		return errors.New("workitem: id is required")
	}
	if !w.Kind.Valid() {
		return fmt.Errorf("workitem: unknown kind %q", w.Kind)
	}
	if strings.TrimSpace(string(w.Capability)) == "" {
		return errors.New("workitem: capability is required")
	}
	if strings.TrimSpace(w.Content) == "" {
		return errors.New("workitem: content is empty")
	}
	return nil
}

// FromTask adapts a task specification into a WorkItem.
//
// It is a projection, not a replacement: the task-specific lifecycle keeps using
// the full taskfile.Spec (its acceptance criteria, dependencies, and execution
// mode), which stay available to it. The WorkItem only names the input — its id,
// title, and rendered content — and records that a task is implemented.
func FromTask(spec *taskfile.Spec) WorkItem {
	if spec == nil {
		return WorkItem{}
	}
	return WorkItem{
		ID:         strings.TrimSpace(spec.ID),
		Kind:       KindTask,
		Capability: agent.Implement,
		Title:      strings.TrimSpace(spec.Title),
		Content:    spec.Render(),
	}
}

// FromPrompt builds a prompt WorkItem from direct text.
//
// It rejects an empty prompt and an empty id, and preserves the operator's prompt
// content verbatim apart from surrounding whitespace. It never inspects the prompt
// text: no keyword is read, and no capability, class, or policy is inferred from
// the prose.
func FromPrompt(id string, capability agent.Capability, prompt string) (WorkItem, error) {
	content := strings.TrimSpace(prompt)
	if content == "" {
		return WorkItem{}, errors.New("workitem: prompt is empty")
	}
	w := WorkItem{
		ID:         strings.TrimSpace(id),
		Kind:       KindPrompt,
		Capability: capability,
		Title:      promptTitle(content),
		Content:    content,
	}
	if err := w.Validate(); err != nil {
		return WorkItem{}, err
	}
	return w, nil
}

// promptTitle derives a short single-line title for display and metadata. It is
// descriptive only: it never participates in routing or any decision.
func promptTitle(prompt string) string {
	line := prompt
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.Join(strings.Fields(line), " ")
	const maxTitle = 120
	if runes := []rune(line); len(runes) > maxTitle {
		line = strings.TrimSpace(string(runes[:maxTitle])) + "…"
	}
	return line
}
