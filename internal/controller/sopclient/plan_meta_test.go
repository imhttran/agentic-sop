package sopclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// writeRawPlanMeta records SOP's plan metadata verbatim, so a test can damage it.
// writePlanMeta (performance_test.go) writes a valid record.
func writeRawPlanMeta(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A plan.meta.json that cannot be parsed is damage, not "no active plan": the file
// is named and the plan gate says it is unknown, not absent.
func TestDamagedPlanMetaIsReported(t *testing.T) {
	root := newProject(t)
	writeRawPlanMeta(t, root, `{"source": `)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	src, ok, err := st.PlanSource()
	if err == nil {
		t.Fatal("PlanSource error = nil, want the damaged file reported")
	}
	if ok || src != "" {
		t.Fatalf("PlanSource = %q,%v, want no source", src, ok)
	}
	if !strings.Contains(err.Error(), "plan.meta.json") {
		t.Fatalf("err = %q, want it to name plan.meta.json", err)
	}

	gate := st.Plan()
	if gate.Recorded {
		t.Fatalf("Plan = %+v, want Recorded=false for damaged metadata", gate)
	}
	if gate.Error == "" {
		t.Fatal("Plan.Error = \"\", want the damaged file reported")
	}
	if !strings.Contains(gate.Error, "plan.meta.json") {
		t.Fatalf("Plan.Error = %q, want it to name plan.meta.json", gate.Error)
	}
}

// Absence stays absence: no metadata file, no error.
func TestAbsentPlanMetaIsNotAnError(t *testing.T) {
	root := newProject(t)
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	src, ok, err := st.PlanSource()
	if err != nil || ok || src != "" {
		t.Fatalf("PlanSource = %q,%v,%v, want a plain absence", src, ok, err)
	}
	gate := st.Plan()
	if gate.Recorded || gate.Error != "" {
		t.Fatalf("Plan = %+v, want an absence with no error", gate)
	}
}

// Metadata without a source is still an absence (there is nothing to reconcile),
// but the plan itself is recorded.
func TestPlanMetaWithoutSourceIsAnAbsence(t *testing.T) {
	root := newProject(t)
	writeRawPlanMeta(t, root, `{"plan_id":"p1","final_gate":"PASS"}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if src, ok, err := st.PlanSource(); err != nil || ok || src != "" {
		t.Fatalf("PlanSource = %q,%v,%v, want no source and no error", src, ok, err)
	}
	gate := st.Plan()
	if !gate.Recorded || gate.Error != "" || gate.PlanID != "p1" {
		t.Fatalf("Plan = %+v, want a recorded plan with no error", gate)
	}
}

// A readable plan still resolves its source.
func TestPlanMetaReadsSource(t *testing.T) {
	root := newProject(t)
	writeRawPlanMeta(t, root, `{"source":"docs/PLAN.md"}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	src, ok, err := st.PlanSource()
	if err != nil || !ok || src != "docs/PLAN.md" {
		t.Fatalf("PlanSource = %q,%v,%v, want docs/PLAN.md", src, ok, err)
	}
}

// ChangedTasks must not answer ErrNoActivePlan for damaged metadata: that is the
// sentinel callers render as a legitimate absence.
func TestChangedTasksReportsDamagedPlanMeta(t *testing.T) {
	root := newProject(t)
	writeRawPlanMeta(t, root, `{oops`)
	c, err := New([]string{root}, "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.ChangedTasks(context.Background(), config.ProjectID(root))
	if err == nil {
		t.Fatal("err = nil, want the damaged metadata reported")
	}
	if errors.Is(err, ErrNoActivePlan) {
		t.Fatalf("err = %v, want a damaged-file error, not ErrNoActivePlan", err)
	}
	if !strings.Contains(err.Error(), "plan.meta.json") {
		t.Fatalf("err = %q, want it to name plan.meta.json", err)
	}
}

// A project with no metadata still answers the absence sentinel.
func TestChangedTasksMissingPlanMetaIsAnAbsence(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.ChangedTasks(context.Background(), config.ProjectID(root))
	if !errors.Is(err, ErrNoActivePlan) {
		t.Fatalf("err = %v, want errors.Is(err, ErrNoActivePlan)", err)
	}
}

// Accepting changed tasks has the same split: damaged metadata is an error, no
// metadata is the documented absence.
func TestAcceptChangedTasksPlanMetaSplit(t *testing.T) {
	t.Run("damaged", func(t *testing.T) {
		root := newProject(t)
		writeRawPlanMeta(t, root, `{oops`)
		c, err := New([]string{root}, "", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		err = c.AcceptChangedTasks(context.Background(), config.ProjectID(root), []string{"T1"})
		if err == nil || errors.Is(err, ErrNoActivePlan) {
			t.Fatalf("err = %v, want a damaged-file error, not ErrNoActivePlan", err)
		}
		if !strings.Contains(err.Error(), "plan.meta.json") {
			t.Fatalf("err = %q, want it to name plan.meta.json", err)
		}
	})

	t.Run("absent", func(t *testing.T) {
		root := newProject(t)
		c, err := New([]string{root}, "", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		err = c.AcceptChangedTasks(context.Background(), config.ProjectID(root), []string{"T1"})
		if !errors.Is(err, ErrNoActivePlan) {
			t.Fatalf("err = %v, want errors.Is(err, ErrNoActivePlan)", err)
		}
	})
}
