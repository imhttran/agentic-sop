package sopclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// decodeListing parses SOP's document verbatim and reports not-ok on malformed
// or blank output rather than fabricating an empty success.
func TestDecodeListing(t *testing.T) {
	t.Run("well formed", func(t *testing.T) {
		doc, ok := decodeListing([]byte(listingDocJSON("t2", "t3")))
		if !ok {
			t.Fatal("decodeListing ok = false, want true")
		}
		if !doc.PlanChanged || doc.Source != "docs/PLAN.md" || doc.PlanID != "p" || doc.Version != 1 {
			t.Errorf("decoded envelope wrong: %+v", doc)
		}
		if !reflect.DeepEqual(doc.ChangedExecuted, []string{"t2", "t3"}) {
			t.Errorf("changed_executed = %v, want [t2 t3]", doc.ChangedExecuted)
		}
	})

	t.Run("empty changed_executed is a reported listing", func(t *testing.T) {
		doc, ok := decodeListing([]byte(listingDocJSON()))
		if !ok {
			t.Fatal("decodeListing ok = false, want true for an observed empty listing")
		}
		if len(doc.ChangedExecuted) != 0 {
			t.Errorf("changed_executed = %v, want empty", doc.ChangedExecuted)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		if _, ok := decodeListing([]byte(`{not json`)); ok {
			t.Error("decodeListing ok = true for malformed JSON, want false")
		}
	})

	t.Run("blank stdout", func(t *testing.T) {
		if _, ok := decodeListing([]byte("   \n")); ok {
			t.Error("decodeListing ok = true for blank stdout, want false (not an empty success)")
		}
	})
}

// ChangedTasks invokes exactly `sop reconcile <PLAN.md> --list-changed --json`
// with the plan path from SOP's recorded provenance, as discrete argv elements
// (no shell string), and reports the listing.
func TestChangedTasksInvokesListingFromPlanProvenance(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('t2','Task Two','o','a','BLOCKED',NULL,1,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('t3','Task Three','o','a','PLANNED',NULL,0,3,'`+now+`','`+now+`')`,
	)
	writeArtifact(t, root, "t2", "state.json", `{"id":"t2","stage":"WAITING_FOR_HUMAN"}`)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	bin, args := fakeListingSop(t, listingDocJSON("t2", "t3"))
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	changed, err := c.ChangedTasks(context.Background(), config.ProjectID(root))
	if err != nil {
		t.Fatalf("ChangedTasks: %v", err)
	}
	if got := args(); !equal(got, []string{"reconcile", "docs/PLAN.md", "--list-changed", "--json"}) {
		t.Fatalf("sop argv = %v, want [reconcile docs/PLAN.md --list-changed --json]", got)
	}
	if !changed.Reported || changed.Source != listingSource {
		t.Fatalf("changed = %+v, want a reported listing sourced from %q", changed, listingSource)
	}
	if len(changed.Tasks) != 2 || changed.Tasks[0].TaskID != "t2" || changed.Tasks[1].TaskID != "t3" {
		t.Fatalf("changed ids = %+v, want [t2 t3] in SOP order", changed.Tasks)
	}
	// Title/Stage are enriched from SOP's task store, not the listing.
	if changed.Tasks[0].Title != "Task Two" || changed.Tasks[0].Stage != "WAITING_FOR_HUMAN" {
		t.Errorf("t2 enrichment = %q/%q, want Task Two/WAITING_FOR_HUMAN", changed.Tasks[0].Title, changed.Tasks[0].Stage)
	}
	if changed.Tasks[1].Title != "Task Three" {
		t.Errorf("t3 title = %q, want Task Three", changed.Tasks[1].Title)
	}
	// All reported tasks are pending (SOP listing does not mark accepted ones here).
	if len(changed.Pending()) != 2 || !changed.Has("t2") || changed.Has("nope") {
		t.Errorf("Pending/Has wrong: pending=%d has(t2)=%v has(nope)=%v", len(changed.Pending()), changed.Has("t2"), changed.Has("nope"))
	}
}

// A listed id unknown to the task store still appears with an empty title rather
// than being dropped.
func TestChangedTasksKeepsUnknownListedID(t *testing.T) {
	root := newProject(t)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, _ := fakeListingSop(t, listingDocJSON("ghost"))
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	changed, err := c.ChangedTasks(context.Background(), config.ProjectID(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Tasks) != 1 || changed.Tasks[0].TaskID != "ghost" || changed.Tasks[0].Title != "" {
		t.Fatalf("changed = %+v, want the ghost id kept with an empty title", changed.Tasks)
	}
}

// A failed listing is surfaced as Reported=false plus an error, never an empty
// success.
func TestChangedTasksSurfacesFailedListing(t *testing.T) {
	root := newProject(t)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := New([]string{root}, fakeFailingSop(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	changed, err := c.ChangedTasks(context.Background(), config.ProjectID(root))
	if err == nil {
		t.Fatal("ChangedTasks err = nil, want the listing failure surfaced")
	}
	if changed.Reported || len(changed.Tasks) != 0 {
		t.Fatalf("changed = %+v, want Reported=false with no tasks", changed)
	}
}

// Unparsable listing output is surfaced as an error, not an empty success.
func TestChangedTasksSurfacesUnparsableListing(t *testing.T) {
	root := newProject(t)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, _ := fakeListingSop(t, "this is not JSON")
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.ChangedTasks(context.Background(), config.ProjectID(root)); err == nil {
		t.Fatal("ChangedTasks err = nil, want an unparsable-listing error")
	}
}

// With no recorded plan source, the read fails explicitly and never guesses a
// PLAN.md path: the SOP binary is never invoked.
func TestChangedTasksRefusesWithoutPlanSource(t *testing.T) {
	root := newProject(t)
	bin, args := fakeListingSop(t, listingDocJSON("t2"))
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.ChangedTasks(context.Background(), config.ProjectID(root)); !errors.Is(err, ErrNoActivePlan) {
		t.Fatalf("ChangedTasks err = %v, want ErrNoActivePlan", err)
	}
	if got := args(); len(got) != 0 {
		t.Fatalf("sop argv = %v, want no invocation when no plan is recorded", got)
	}
}

// An unknown project yields ErrProjectNotFound, the boundary sentinel.
func TestChangedTasksUnknownProject(t *testing.T) {
	root := newProject(t)
	bin, _ := fakeListingSop(t, listingDocJSON())
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ChangedTasks(context.Background(), "nope"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
}

// A stale reconcile.json artifact is never read: a project that has only the
// retired artifact reports Reported=false rather than the artifact's tasks.
func TestChangedTasksNeverReadsStaleArtifact(t *testing.T) {
	root := newProject(t)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "reconcile.json"),
		[]byte(`{"changed":[{"task_id":"stale","title":"Stale"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// SOP reports an observed EMPTY listing; the stale artifact's task must not
	// appear.
	bin, _ := fakeListingSop(t, listingDocJSON())
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	changed, err := c.ChangedTasks(context.Background(), config.ProjectID(root))
	if err != nil {
		t.Fatal(err)
	}
	if !changed.Reported || len(changed.Tasks) != 0 {
		t.Fatalf("changed = %+v, want a reported empty listing with no stale-artifact tasks", changed)
	}
}

// Store.Summary folds the pending changed tasks from the SOP listing into
// NeedsAttention, and adds nothing when SOP reported no set.
func TestSummaryCountsPendingChangedTasks(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('t2','Task Two','o','a','BLOCKED',NULL,1,3,'`+now+`','`+now+`')`,
	)
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	reported := ChangedTasks{Reported: true, Source: listingSource, Tasks: []ChangedExecutedTask{
		{TaskID: "t2"}, {TaskID: "t3", Approved: true},
	}}
	sum, err := st.Summary(ctx, reported, readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if sum.NeedsAttention != 1 {
		t.Fatalf("NeedsAttention = %d, want 1 (one pending changed task)", sum.NeedsAttention)
	}

	unreported, err := st.Summary(ctx, ChangedTasks{}, readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if unreported.NeedsAttention != 0 {
		t.Fatalf("NeedsAttention = %d, want 0 when SOP reports no set", unreported.NeedsAttention)
	}
}

// The listing is a pure read: SOP-persisted task state, graph, plan provenance,
// acceptance, and run artifacts are byte-for-byte unchanged across the read.
func TestChangedTasksIsPureRead(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('t2','Task Two','o','a','BLOCKED',NULL,1,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('t3','Task Three','o','a','PLANNED',NULL,0,3,'`+now+`','`+now+`')`,
		`INSERT INTO task_dependencies VALUES ('t3','t2')`,
	)
	writeArtifact(t, root, "t2", "state.json", `{"id":"t2","stage":"WAITING_FOR_HUMAN"}`)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md","plan_id":"p","final_gate":"UNKNOWN"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	bin, _ := fakeListingSop(t, listingDocJSON("t2"))
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	// Snapshot SOP-persisted reads before and after the listing.
	beforeProj, err := c.Project(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	beforeSrc, _ := c.PlanSource(pid)

	if _, err := c.ChangedTasks(ctx, pid); err != nil {
		t.Fatal(err)
	}

	afterProj, err := c.Project(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	afterSrc, _ := c.PlanSource(pid)

	if !reflect.DeepEqual(beforeProj.Tasks, afterProj.Tasks) {
		t.Fatalf("listing mutated task state/graph:\nbefore=%+v\nafter =%+v", beforeProj.Tasks, afterProj.Tasks)
	}
	if beforeSrc != afterSrc {
		t.Fatalf("listing mutated the active plan provenance: %q -> %q", beforeSrc, afterSrc)
	}

	// The .agent-sdlc tree (task state, graph, plan meta, acceptance/provenance)
	// is byte-for-byte unchanged.
	dir := filepath.Join(root, ".agent-sdlc")
	before := snapshotTree(t, dir)
	if _, err := c.ChangedTasks(ctx, pid); err != nil {
		t.Fatal(err)
	}
	after := snapshotTree(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("listing mutated SOP persistence:\nbefore=%v\nafter =%v", before, after)
	}
}

// The listing source is named truthfully so a reader can trace the changed set
// to SOP's verb rather than the retired artifact.
func TestListingSourceNamesSOPVerb(t *testing.T) {
	if !strings.Contains(listingSource, "reconcile") || !strings.Contains(listingSource, "--list-changed") {
		t.Fatalf("listingSource = %q, want it to name the SOP listing verb", listingSource)
	}
}
