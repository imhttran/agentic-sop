package sopclient

// CTRL010 bounded progress SOP reports for multi-invocation work, e.g.
// "11/15 cases analyzed, covered: 11, missing: 4, next: ...". Values are
// verbatim; no percentage, interpolation, or extrapolation. Partial progress is
// never task completion.

// CheckpointProgress is the most recent checkpoint SOP reported for a task.
// Present=false means none was reported; show an absence, not a number.
type CheckpointProgress struct {
	Present bool
	// Label is SOP's checkpoint identifier (e.g. "JEV012").
	Label string
	// Source is where the checkpoint was read from (artifact or activity).
	Source string
	// Has* flags distinguish a reported zero from an absent counter.
	Analyzed    int
	HasAnalyzed bool
	Total       int
	HasTotal    bool
	Covered     int
	HasCovered  bool
	Missing     int
	HasMissing  bool
	// Next is SOP's next-step hint.
	Next string
}

// Complete reports whether the checkpoint reached its total. It never
// overrides SOP's task status.
func (c CheckpointProgress) Complete() bool {
	if !c.Present || !c.HasAnalyzed || !c.HasTotal {
		return false
	}
	return c.Analyzed >= c.Total
}

// InProgress reports a partial checkpoint (analyzed < total).
func (c CheckpointProgress) InProgress() bool {
	if !c.Present || !c.HasAnalyzed || !c.HasTotal {
		return false
	}
	return c.Analyzed < c.Total
}

// sanitized marks c present and bounds its free-form fields with the shared
// sanitizeDetail rule, so every read path applies one bound.
func (c CheckpointProgress) sanitized() CheckpointProgress {
	c.Present = true
	c.Label, c.Source, c.Next = sanitizeDetail(c.Label), sanitizeDetail(c.Source), sanitizeDetail(c.Next)
	return c
}
