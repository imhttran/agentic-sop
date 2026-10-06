// Package prompt is the CTX-005 Prompt Compiler: the deterministic owner of the
// structure of the model input SOP sends.
//
// The compiler turns canonical evidence the harness already holds — the CTX-001
// Context Engine's Context, plus the task, the previous attempt, and caller
// observations — into a bounded, predictable model request. It performs no
// retrieval, invokes no model, and reads no repository state: given the same
// Input it produces byte-identical output.
//
// The model must not determine what instructions govern itself, so the compiler
// owns section selection, order, provenance, and bounding. Instructions are kept
// separate from the task, and the evidence carried alongside them is rendered from
// the ordered context, never re-ranked here (ranking belongs to CTX-003/CTX-009).
//
// Limits are expressed in bytes, never tokens: reliable provider-independent token
// accounting does not exist, so a token budget would be authority SOP cannot
// justify.
package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/imhttran/agentic-sop/internal/agent"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
)

// Class is the model-class variant the compiler targets. The variant changes only
// the byte bound, deterministically; the section structure is identical for every
// class, so a compiled prompt is comparable across classes.
type Class string

const (
	Small  Class = "SMALL"
	Medium Class = "MEDIUM"
	Large  Class = "LARGE"
)

// Valid reports whether c is a known class.
func (c Class) Valid() bool { return c == Small || c == Medium || c == Large }

// ParseClass parses a case-insensitive class name, reporting whether it was known.
func ParseClass(s string) (Class, bool) {
	c := Class(strings.ToUpper(strings.TrimSpace(s)))
	if !c.Valid() {
		return Medium, false
	}
	return c, true
}

// Limits bound the compiled prompt in deterministic units (bytes only). A zero
// MaxBytes means unbounded.
type Limits struct {
	MaxBytes int
}

// LimitsFor returns the built-in byte bound for a class. Medium admits a full
// CTX-001 context (64 KiB) together with the compiler's provenance headers, so the
// default class adds no truncation beyond the Context Engine's own bound. Small
// keeps only the highest-priority evidence; Large admits the fullest context. The
// values are structural defaults, not evidence-tuned.
func LimitsFor(c Class) Limits {
	switch c {
	case Small:
		return Limits{MaxBytes: 24 << 10}
	case Large:
		return Limits{MaxBytes: 256 << 10}
	default:
		return Limits{MaxBytes: 80 << 10}
	}
}

// Section is one named, deterministically ordered part of the compiled prompt. Its
// Provenance names where the section's evidence came from, so compiled input is
// auditable rather than opaque.
type Section struct {
	Name       string `json:"name"`
	Provenance string `json:"provenance"`
	Body       string `json:"body"`
	Bytes      int    `json:"bytes"`
}

// Input is the canonical evidence the compiler assembles. Every field is evidence
// the caller already holds. The compiler performs no selection of its own beyond
// ordering and bounding.
type Input struct {
	// Capability is the deterministic capability being requested.
	Capability agent.Capability
	// Class selects the byte bound; an invalid or empty class compiles as Medium.
	Class Class
	// Task is the task instruction text. It is kept separate from the instructions
	// section and is not part of the evidence text rendered by Evidence.
	Task string
	// Context is the canonical CTX-001 context. Its items are rendered, in their
	// deterministic order, into the context section with explicit provenance.
	Context sopctx.Context
	// Attempt is the previous attempt's bounded signature, if the task is a
	// continuation. It is harness evidence, not model output.
	Attempt string
	// Observations are caller-supplied task data (never completion or approval
	// evidence).
	Observations string
	// OutputRequirements is the caller-owned output shape. It governs the
	// instructions section.
	OutputRequirements string
}

// Compiled is the deterministic compiled prompt: the ordered sections, the total
// size, whether a bound truncated it, and a stable digest over the output.
type Compiled struct {
	Class     Class     `json:"class"`
	Sections  []Section `json:"sections"`
	Bytes     int       `json:"bytes"`
	Truncated bool      `json:"truncated"`
	Digest    string    `json:"digest"`
}

// Canonical section names, in their fixed order.
const (
	sectionInstructions = "instructions"
	sectionTask         = "task"
	sectionContext      = "context"
	sectionAttempt      = "previous_attempt"
	sectionObservations = "observations"
)

// Compile assembles, orders, and bounds the compiled prompt. It is pure: identical
// Input yields byte-identical output, so repeated compilation is reproducible.
func Compile(in Input) Compiled {
	class := in.Class
	if !class.Valid() {
		class = Medium
	}

	secs := []Section{
		newSection(sectionInstructions, "caller: output requirements", instructions(in)),
		newSection(sectionTask, "caller: task", strings.TrimSpace(in.Task)),
		newSection(sectionContext, "CTX-001 context engine", renderContext(in.Context)),
		newSection(sectionAttempt, "harness: prior attempt", strings.TrimSpace(in.Attempt)),
		newSection(sectionObservations, "caller: task observations", strings.TrimSpace(in.Observations)),
	}
	secs = nonEmpty(secs)

	bounded, truncated := bound(secs, LimitsFor(class).MaxBytes)

	c := Compiled{Class: class, Sections: bounded, Truncated: truncated}
	for _, s := range bounded {
		c.Bytes += s.Bytes
	}
	c.Digest = digest(class, bounded)
	return c
}

// Render returns the full canonical prompt text: every non-empty section under its
// deterministic heading, in order. It is the human/CLI view of a compiled prompt;
// the model request carries the evidence sections via Evidence and the task and
// output shape as separate request fields.
func (c Compiled) Render() string {
	var b strings.Builder
	for _, s := range c.Sections {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "# %s\n\n%s", title(s.Name), s.Body)
	}
	return b.String()
}

// Evidence returns the evidence text carried alongside the task: the context, the
// previous attempt, and caller observations, each under its deterministic heading.
// Instructions and the task are excluded because the model request carries them as
// distinct fields, keeping task and instruction separation explicit.
func (c Compiled) Evidence() string {
	var b strings.Builder
	for _, s := range c.Sections {
		switch s.Name {
		case sectionContext, sectionAttempt, sectionObservations:
		default:
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "# %s\n\n%s", title(s.Name), s.Body)
	}
	return b.String()
}

// Section returns the named section, if present.
func (c Compiled) Section(name string) (Section, bool) {
	for _, s := range c.Sections {
		if s.Name == name {
			return s, true
		}
	}
	return Section{}, false
}

// instructions renders the caller-owned output shape, plus the capability for a
// standalone view. It contains no model-supplied text and grants no authority.
func instructions(in Input) string {
	cap := strings.TrimSpace(string(in.Capability))
	reqs := strings.TrimSpace(in.OutputRequirements)
	if cap == "" && reqs == "" {
		return ""
	}
	var b strings.Builder
	if cap != "" {
		fmt.Fprintf(&b, "Capability: %s", cap)
	}
	if reqs != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(reqs)
	}
	return b.String()
}

// renderContext renders the ordered context items with explicit provenance. It
// preserves each item's text verbatim and never reorders or re-ranks them: the
// Context Engine already fixed the order.
func renderContext(c sopctx.Context) string {
	var parts []string
	for _, it := range c.Items() {
		if strings.TrimSpace(it.Text) == "" {
			continue
		}
		var b strings.Builder
		header := "### " + string(it.Source)
		if it.Identity != "" {
			header += ": " + it.Identity
		}
		b.WriteString(header)
		if it.Reason != "" {
			b.WriteString("\nReason: ")
			b.WriteString(it.Reason)
		}
		b.WriteString("\n\n")
		b.WriteString(it.Text)
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "\n\n")
}

// newSection builds a section, measuring its body.
func newSection(name, provenance, body string) Section {
	return Section{Name: name, Provenance: provenance, Body: body, Bytes: len(body)}
}

// nonEmpty drops sections with no body, so absent evidence leaves no heading.
func nonEmpty(secs []Section) []Section {
	out := secs[:0:0]
	for _, s := range secs {
		if strings.TrimSpace(s.Body) == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// bound keeps sections in order until the byte ceiling is reached. The section that
// would exceed the ceiling is included only up to a rune-safe prefix of the
// remaining budget, and every later section is dropped. It is deterministic and
// reports whether it truncated. A non-positive ceiling means unbounded.
func bound(secs []Section, max int) ([]Section, bool) {
	if max <= 0 {
		return secs, false
	}
	out := make([]Section, 0, len(secs))
	used := 0
	for _, s := range secs {
		sep := 0
		if len(out) > 0 {
			sep = 2 // "\n\n" between rendered sections
		}
		if used+sep+s.Bytes <= max {
			out = append(out, s)
			used += sep + s.Bytes
			continue
		}
		if remaining := max - used - sep; remaining > 0 {
			if body := truncateRunes(s.Body, remaining); body != "" {
				ns := s
				ns.Body = body
				ns.Bytes = len(body)
				out = append(out, ns)
			}
		}
		return out, true
	}
	return out, false
}

// truncateRunes returns the longest prefix of s that is at most n bytes and ends on
// a rune boundary, so a truncated section never splits a UTF-8 sequence.
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

// digest is a stable content hash over the compiled output, so a compiled prompt
// has a deterministic identity independent of map or parse order.
func digest(class Class, secs []Section) string {
	h := sha256.New()
	fmt.Fprintf(h, "prompt-compiler/1\nclass=%s\n", class)
	for _, s := range secs {
		fmt.Fprintf(h, "section=%s\nprovenance=%s\n", s.Name, s.Provenance)
		h.Write([]byte(s.Body))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// title maps a canonical section name to its deterministic heading.
func title(name string) string {
	switch name {
	case sectionInstructions:
		return "Instructions"
	case sectionTask:
		return "Task"
	case sectionContext:
		return "Context"
	case sectionAttempt:
		return "Previous attempt"
	case sectionObservations:
		return "Caller observations"
	default:
		return name
	}
}
