package planflow

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// DefinitionIdentity is the observable identity of the definition a request was
// raised against. It is derived from the persisted active plan
// (.agent-sdlc/plan.json) — SOP's single source of truth for executed
// definitions — so it never introduces a second definition store.
type DefinitionIdentity struct {
	// TaskID is the task the identity was asked for.
	TaskID string
	// Digest is a deterministic hex digest of the persisted stage for the task.
	// It is empty when Present is false.
	Digest string
	// Present reports whether the persisted active plan defines a stage for
	// TaskID. When it is false the plan is missing, unreadable, or has no stage
	// with that ID, and no definition evidence exists.
	Present bool
}

// TaskDefinitionIdentity reads the definition identity of one task from the
// persisted active plan. The digest covers the stage fields that define the
// work: deliverables, acceptance criteria, acceptance checks, the acceptance
// mapping, dependencies, and requires. It also covers the stage's own identity
// and descriptive fields (ID, title, objective, kind, execution mode, discovery
// allowance, required evidence) so a change to any of them is observable.
//
// List ordering is canonicalized and fields follow a fixed order, so the digest
// is stable across reads and processes. It changes when any covered field
// changes. It reads only; it never writes plan.json, plan.meta.json, or any
// other file. A task the active plan does not define yields an empty digest
// with Present false and no error.
func TaskDefinitionIdentity(dir, taskID string) (DefinitionIdentity, error) {
	id := strings.TrimSpace(taskID)
	if id == "" {
		return DefinitionIdentity{}, errors.New("definition identity: a task id is required")
	}
	plan, ok := loadPersistedPlan(filepath.Join(dir, config.DirName, planFileName))
	if !ok {
		return DefinitionIdentity{TaskID: id}, nil
	}
	for _, stage := range plan.Stages {
		if stage.ID != id {
			continue
		}
		return DefinitionIdentity{TaskID: id, Digest: stageDigest(stage), Present: true}, nil
	}
	return DefinitionIdentity{TaskID: id}, nil
}

// stageDigestByID returns the definition digest of one stage in a compiled plan,
// or an empty string when the plan does not define it. It is the join between
// reconciliation acceptance and a request's recorded digest: both sides use
// stageDigest, so the values are directly comparable.
func stageDigestByID(plan *planner.Plan, taskID string) string {
	if plan == nil {
		return ""
	}
	id := strings.TrimSpace(taskID)
	for _, stage := range plan.Stages {
		if stage.ID == id {
			return stageDigest(stage)
		}
	}
	return ""
}

// TaskDefinitionText renders the persisted stage for a task as text, for consistency
// checks over what the current definition says. It is not a second definition store:
// it reads plan.json and returns the same fields stageDigest covers.
func TaskDefinitionText(dir, taskID string) (string, bool) {
	plan, ok := loadPersistedPlan(filepath.Join(dir, config.DirName, planFileName))
	if !ok {
		return "", false
	}
	id := strings.TrimSpace(taskID)
	for _, stage := range plan.Stages {
		if stage.ID != id {
			continue
		}
		var b strings.Builder
		parts := []string{stage.ID, stage.Title, stage.Objective}
		parts = append(parts, stage.Deliverables...)
		parts = append(parts, stage.AcceptanceCriteria...)
		parts = append(parts, stage.AcceptanceChecks...)
		parts = append(parts, mappingList(stage.AcceptanceMapping)...)
		parts = append(parts, stage.Dependencies...)
		parts = append(parts, stage.Requires...)
		parts = append(parts, stage.RequiredEvidence...)
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				b.WriteString(part)
				b.WriteString("\n")
			}
		}
		return b.String(), true
	}
	return "", false
}

// stageDigest is the deterministic digest over one persisted plan stage. Fields
// are hashed in a fixed order, each terminated by a NUL separator, and every
// list is canonicalized (sorted, de-duplicated) so the digest never depends on
// map iteration or on the order a source listed its items.
func stageDigest(stage planner.Stage) string {
	sum := sha256.New()
	field := func(name, value string) {
		_, _ = io.WriteString(sum, name)
		sum.Write([]byte{':'})
		_, _ = io.WriteString(sum, value)
		sum.Write([]byte{0})
	}
	list := func(name string, values []string) {
		field(name, strings.Join(canonicalList(values), "\x1f"))
	}

	field("id", stage.ID)
	field("title", stage.Title)
	field("objective", stage.Objective)
	list("deliverables", stage.Deliverables)
	list("acceptance_criteria", stage.AcceptanceCriteria)
	list("acceptance_checks", stage.AcceptanceChecks)
	list("acceptance_mapping", mappingList(stage.AcceptanceMapping))
	list("dependencies", stage.Dependencies)
	list("requires", stage.Requires)
	list("required_evidence", stage.RequiredEvidence)
	field("kind", stage.Kind)
	field("execution_mode", string(stage.ExecutionMode))
	field("discovery_iterations", strconv.Itoa(stage.DiscoveryIterations))

	return hex.EncodeToString(sum.Sum(nil))
}

// canonicalList returns the sorted, de-duplicated copy of values. Blank items
// are dropped so a cosmetic difference never changes the digest.
func canonicalList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}

	// A small insertion sort keeps the digest independent of the caller's order
	// without depending on sort helpers for a short list.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// mappingList renders each acceptance mapping as its stable string form.
func mappingList(mappings []planner.AcceptanceMapping) []string {
	if len(mappings) == 0 {
		return nil
	}
	out := make([]string, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, m.String())
	}
	return out
}

// ReconciliationProvenance reports the persisted reconciliation evidence for one
// task. It is derived from the active plan's provenance (.agent-sdlc/plan.meta.json)
// that Reconcile already writes; it introduces no new store or field.
type ReconciliationProvenance struct {
	// TaskID is the task the provenance was asked for.
	TaskID string
	// Reconciled reports whether a human explicitly accepted this task's changed
	// definition during reconciliation (Metadata.ReconciledTasks).
	Reconciled bool
	// Present reports whether reconciliation evidence exists for TaskID. It is
	// true only when Reconciled is true.
	Present bool
	// RecordedAt is the plan provenance's GeneratedAt. It is a plan-wide write
	// time, NOT proof of when this task's change was accepted: it is rewritten by
	// any later plan write, including a no-op one. Callers must not use it as
	// acceptance evidence; AcceptedAt is that evidence.
	RecordedAt time.Time
	// Accepted reports whether a task-scoped accepted-change record exists.
	Accepted bool
	// AcceptedAt is when this task's definition change was explicitly accepted. It
	// is zero unless Accepted is true.
	AcceptedAt time.Time
	// DefinitionDigestBefore and DefinitionDigestAfter bind that acceptance to the
	// concrete change it covers. Both are empty unless Accepted is true.
	DefinitionDigestBefore string
	DefinitionDigestAfter  string
	// AcceptanceSourceSHA256 is the accepted plan revision.
	AcceptanceSourceSHA256 string
	// AutoRecords carries the automatic-reconciliation records for TaskID, when an
	// autonomy policy reconciled it instead of a human.
	AutoRecords []AutoReconciled
}

// TaskReconciliationProvenance reads the persisted reconciliation evidence for
// one task. It answers whether the task was explicitly accepted as reconciled and
// when that reconciliation was recorded, and reports the evidence absent (not an
// error) when the task is not recorded or the metadata is missing or unreadable.
// It reads only and never mutates plan.json, plan.meta.json, or the task store.
func TaskReconciliationProvenance(dir, taskID string) (ReconciliationProvenance, error) {
	id := strings.TrimSpace(taskID)
	if id == "" {
		return ReconciliationProvenance{}, errors.New("reconciliation provenance: a task id is required")
	}
	meta := readMetadata(filepath.Join(dir, config.DirName, metaFileName))
	out := ReconciliationProvenance{TaskID: id}
	for _, rec := range meta.ReconciledTasks {
		if strings.TrimSpace(rec) == id {
			out.Reconciled = true
			out.Present = true
			out.RecordedAt = meta.GeneratedAt
			break
		}
	}
	// A task-scoped acceptance record is the only supported proof of when and what
	// was accepted. The newest record for this task wins; an entry missing its time
	// or digests stays unusable rather than being completed by inference.
	for _, rec := range meta.AcceptedChanges {
		if strings.TrimSpace(rec.TaskID) != id {
			continue
		}
		if out.Accepted && !rec.AcceptedAt.After(out.AcceptedAt) {
			continue
		}
		out.Accepted = rec.AcceptedAt != (time.Time{})
		out.AcceptedAt = rec.AcceptedAt
		out.DefinitionDigestBefore = rec.DefinitionDigestBefore
		out.DefinitionDigestAfter = rec.DefinitionDigestAfter
		out.AcceptanceSourceSHA256 = rec.SourceSHA256
	}
	for _, rec := range meta.AutoReconciled {
		if rec.TaskID == id {
			out.AutoRecords = append(out.AutoRecords, rec)
		}
	}
	return out, nil
}
