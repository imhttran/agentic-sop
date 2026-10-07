package planner

import (
	"fmt"
	"strings"
)

// PlanContext is the authoritative, read-only context a task-level plan is
// generated within. It carries the compiled plan's capability inventory, which is
// authoritative for the task, plus the capability names the task's own compiled
// stage already requires.
//
// It exists to close a real defect (SOP-PLANNER-CAP-001). The task-level planner
// re-derived requirements from task prose alone, so a repository discovery target
// named in the instructions ("inspect the provider registration/factory", "inspect
// existing adapters") could be promoted to an external prerequisite capability —
// downgrading a known capability to UNKNOWN or inventing a parallel one — and
// trigger recurring, model-driven capability repair. Supplying the compiled
// inventory tells the planner which capabilities already exist and which
// prerequisites the task already has, so repository discovery is planned as work
// rather than invented as a prerequisite.
//
// A PlanContext is derived deterministically from the persisted machine plan; it is
// never model-generated and carries no policy of its own. It never grants a
// capability, a permission, or an approval.
type PlanContext struct {
	// Capabilities is the authoritative compiled capability inventory.
	Capabilities []Capability
	// Requires names the capabilities the task's own compiled stage requires.
	Requires []string
}

// IsZero reports whether the context carries no authoritative capability
// information, so a caller can omit the prompt block entirely.
func (c PlanContext) IsZero() bool {
	return len(c.Capabilities) == 0 && len(c.Requires) == 0
}

// index returns the authoritative capabilities keyed by normalized name. The
// normalization is the one validateCapabilities and CapabilityGaps use, so the
// context and the deterministic ownership gate can never disagree about which
// requirement names which declared capability.
func (c PlanContext) index() map[string]Capability {
	if len(c.Capabilities) == 0 {
		return nil
	}
	out := make(map[string]Capability, len(c.Capabilities))
	for _, cap := range c.Capabilities {
		out[normalizeCapabilityName(cap.Name)] = cap
	}
	return out
}

// promptBlock renders the authoritative context as a deterministic prompt
// preamble. It states the two invariants — the compiled inventory is authoritative,
// and repository discovery is work — in general terms, so the planner can
// distinguish a known capability and a task-local discovery target from a genuinely
// missing external prerequisite without any name-specific rule.
func (c PlanContext) promptBlock() string {
	if c.IsZero() {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Authoritative plan context (read-only)\n\n")
	b.WriteString("This task is one stage of an already-compiled plan. Treat the plan's\n")
	b.WriteString("capability inventory below as authoritative; do not re-derive it from the\n")
	b.WriteString("task text.\n\n")
	if len(c.Capabilities) > 0 {
		b.WriteString("Capabilities the compiled plan declares (name — status):\n")
		for _, cap := range c.Capabilities {
			fmt.Fprintf(&b, "- %s — %s\n", strings.TrimSpace(cap.Name), normalizeCapabilityStatus(cap.Status))
		}
		b.WriteString("\n")
	}
	if len(c.Requires) > 0 {
		b.WriteString("Capabilities this task's compiled stage already requires:\n")
		for _, r := range c.Requires {
			if n := strings.TrimSpace(r); n != "" {
				fmt.Fprintf(&b, "- %s\n", n)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(`A capability the compiled plan already declares MUST NOT be re-declared,
re-litigated, or downgraded: refer to it by the same name with its authoritative
status. A repository inspection, search, call-path trace, type/interface/shape,
configuration/default/policy, adapter, registration or factory, CLI-selection,
test, package, or report target named in the task is work this task performs:
describe it in a stage's objective or acceptance criteria, and do NOT declare it as
a capability or add it to "requires". A "requires" entry is only a capability that
is externally supplied (a runtime, permission, tool, service, or artifact) and that
the compiled capability inventory above does not already provide.`)
	return b.String()
}

// applyPlanContext reconciles a generated task-level plan against the authoritative
// compiled capability inventory, deterministically and before any validation or
// repair classification. The compiled inventory is authoritative:
//
//   - A generated capability that names an authoritative capability by normalized
//     name adopts the authoritative entry verbatim (status, evidence, owner, gap,
//     resolution). A cosmetic re-derivation can therefore never downgrade a known
//     EXISTS capability to UNKNOWN, or invent a parallel entry, and trigger
//     recurring model-driven repair.
//   - A stage "requires" entry that resolves to an authoritative capability is
//     guaranteed to be declared, and its spelling is canonicalized to the
//     authoritative name, so the deterministic ownership gate sees one requirement,
//     not two spellings of it.
//
// It changes nothing else. A capability the generated plan proposes that the
// authoritative inventory does not contain is left exactly as generated, so the
// existing deterministic validation and ownership gate still govern a genuinely
// missing external prerequisite rather than silently authorizing or dropping it. It
// never removes a stage, reorders a dependency, or alters an id, so task identity,
// the dependency graph, and lifecycle meaning are untouched.
func applyPlanContext(plan *Plan, pctx PlanContext) {
	auth := pctx.index()
	if len(auth) == 0 {
		return
	}

	// Authoritative capabilities win by normalized name: replace the generated entry
	// with the authoritative one so a known capability can never be downgraded or
	// paralleled by a cosmetic re-derivation.
	declared := make(map[string]bool, len(plan.Capabilities))
	for i, cap := range plan.Capabilities {
		key := normalizeCapabilityName(cap.Name)
		if a, ok := auth[key]; ok {
			plan.Capabilities[i] = a
			declared[normalizeCapabilityName(a.Name)] = true
			continue
		}
		declared[key] = true
	}

	// A requirement that resolves to an authoritative capability is declared (when
	// the generated plan required it without declaring it) and canonicalized to the
	// authoritative spelling, so it validates against one authoritative name.
	for si := range plan.Stages {
		requires := plan.Stages[si].Requires
		for ri, r := range requires {
			key := normalizeCapabilityName(r)
			a, ok := auth[key]
			if !ok {
				continue
			}
			if !declared[key] {
				plan.Capabilities = append(plan.Capabilities, a)
				declared[key] = true
			}
			requires[ri] = a.Name
		}
	}
}
