package jev

// JEV finding payload contract for FIX (JEV009).
//
// This file defines the exact data shape that carries JEV findings into the
// existing FIX capability, so FIX can act on actionable severity/file/line/
// finding/evidence values. The contract is additive and keeps the JEV/FIX
// boundary intact:
//
//   - JEV remains read-only: the payload is plain data. Producing it performs no
//     repository mutation, no state transition, and no persistence write — the
//     type exposes no mutation method and no runtime handle.
//   - FIX remains responsible for mutation: the payload is context for FIX. It
//     carries no authority to change state on its own; only FIX acts on it.
//   - Findings remain associated with the task/run that produced them: every
//     payload carries the originating TaskID and RunID, so findings from
//     distinct tasks/runs remain distinguishable and are never reassigned.
//   - No new mutation mechanism is introduced: the payload flows into the
//     existing FIX context and the existing fix-cycle budget is unchanged.
//
// PRD terminology is preserved field-for-field: severity, file, line, category,
// finding (the actionable message), and evidence. Category classifies a finding;
// it is never overloaded to carry the actionable message, which stays in Finding.

// FindingPayload is the JEV finding shape delivered into FIX context. It mirrors
// the fields FIX needs to act on a finding, plus the task/run association that
// keeps findings attributable to the run that produced them.
//
// It is data only. It is never a mutation request, never a lifecycle command,
// and never carries a handle to mutate SOP state; FIX decides whether and how to
// act, and FIX is the only component that mutates.
type FindingPayload struct {
	// TaskID and RunID associate the finding with the task/run that produced it.
	// They are propagated unchanged from JEV into FIX context, so findings from
	// distinct tasks/runs never merge or get reassigned.
	TaskID string
	RunID  string

	// Severity is the finding's severity (for example HIGH, CRITICAL, LOW).
	Severity string
	// File is the repository-relative path the finding is about (may be empty).
	File string
	// Line is the 1-based line within File (0 when not line-specific).
	Line int
	// Category classifies the finding (for example quality, security). It is
	// metadata and never carries the actionable problem; that is Finding.
	Category string
	// Finding is the actionable problem the finding describes — the message FIX
	// must address (Finding.Message). It is never the category: the two are
	// distinct fields, so one can never be silently substituted for the other.
	Finding string
	// Evidence is the supporting detail that justifies the finding.
	Evidence string
}

// PayloadsForTask converts a JEV result into the FIX-bound payload contract,
// stamping every finding with the originating task and run identifiers.
//
// It is the single projection from JEV findings to FIX context. It is a pure
// read-only conversion: it performs no mutation and returns no handle capable of
// mutating anything. Only findings are converted; a PASS or a finding-free
// result yields no payloads, and the caller decides (via the existing quality
// gate and FIX cycle) whether anything acts on them.
//
// Findings retain their severity, file, line, category, finding, and evidence
// values, and they keep the supplied task/run association so they are never lost
// or reassigned across task/run boundaries. Category is preserved as metadata;
// Finding carries the actionable message, never the category.
func (r Result) PayloadsForTask(taskID, runID string) []FindingPayload {
	if len(r.Findings) == 0 {
		return nil
	}
	out := make([]FindingPayload, 0, len(r.Findings))
	for _, f := range r.Findings {
		out = append(out, FindingPayload{
			TaskID:   taskID,
			RunID:    runID,
			Severity: string(f.Severity),
			File:     f.Path,
			Line:     f.Line,
			Category: f.Category,
			Finding:  f.Message,
			Evidence: f.Evidence,
		})
	}
	return out
}
