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
	// RecordedAt is when that reconciliation was recorded: the plan provenance's
	// GeneratedAt, which Reconcile rewrites on each reconciliation. It is zero when
	// Present is false.
	RecordedAt time.Time
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
	for _, rec := range meta.AutoReconciled {
		if rec.TaskID == id {
			out.AutoRecords = append(out.AutoRecords, rec)
		}
	}
	return out, nil
}
