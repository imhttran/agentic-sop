package sopclient

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeArtifact(t *testing.T, root, task, name, content string) {
	t.Helper()
	dir := filepath.Join(root, ".agent-sdlc", "runs", task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// SOP's task status vocabulary collapses into the coarse display states the UI
// groups by. The controller never invents a status; it only groups SOP's.
func TestTaskStateCategories(t *testing.T) {
	cases := map[string]string{
		StatusDone:         "DONE",
		StatusLocalDone:    "DONE",
		StatusMerged:       "DONE",
		StatusBlocked:      "BLOCKED",
		StatusReady:        "READY",
		StatusPlanned:      "PLANNED",
		StatusImplementing: "RUNNING",
		StatusReview:       "RUNNING",
		StatusCIRunning:    "RUNNING",
		StatusFixRequired:  "FAILED",
	}
	for status, want := range cases {
		if got := TaskState(status); got != want {
			t.Errorf("TaskState(%s) = %s, want %s", status, got, want)
		}
	}
	for _, terminal := range []string{StatusDone, StatusLocalDone, StatusMerged} {
		if !IsTerminal(terminal) {
			t.Errorf("IsTerminal(%s) = false, want true", terminal)
		}
	}
	if IsTerminal(StatusReview) {
		t.Error("IsTerminal(REVIEW) = true, want false")
	}
}

// A nil classification is never a human boundary.
func TestClassificationNilSafety(t *testing.T) {
	var c *Classification
	if c.HumanRequired() {
		t.Fatal("nil classification must not require a human")
	}
}

// Activity is read from SOP's activity.jsonl; a malformed line is skipped rather
// than failing the whole view, and a missing artifact yields no activity.
func TestActivityParsing(t *testing.T) {
	root := newProject(t)
	seed(t, root, `INSERT INTO tasks VALUES ('a1','A','o','a','IMPLEMENTING',NULL,0,3,'`+
		time.Now().UTC().Format(time.RFC3339)+`','`+time.Now().UTC().Format(time.RFC3339)+`')`)
	writeArtifact(t, root, "a1", "activity.jsonl",
		`{"stage":"IMPLEMENT","action":"implementing","timestamp":"2026-01-01T00:00:01Z"}`+"\n"+
			`this line is not json`+"\n"+
			`{"stage":"VALIDATE","action":"go test ./...","timestamp":"2026-01-01T00:00:02Z"}`+"\n")

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d, err := st.Task(context.Background(), "a1", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Activity) != 2 {
		t.Fatalf("activity = %d events, want 2 (malformed line skipped): %+v", len(d.Activity), d.Activity)
	}
	if d.Activity[0].Stage != "IMPLEMENT" || d.Activity[1].Action != "go test ./..." {
		t.Fatalf("activity not parsed correctly: %+v", d.Activity)
	}

	// No activity artifact -> no activity, no error.
	seed(t, root, `INSERT INTO tasks VALUES ('a2','B','o','a','PLANNED',NULL,0,3,'`+
		time.Now().UTC().Format(time.RFC3339)+`','`+time.Now().UTC().Format(time.RFC3339)+`')`)
	d2, err := st.Task(context.Background(), "a2", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if d2.Activity != nil {
		t.Fatalf("activity = %+v, want nil", d2.Activity)
	}
}

// SOP's classification is read from report.json when present, and from the
// dedicated classification.json when a run stopped before writing a report.
// C2-001: a NEEDS_HUMAN classification or a WAITING_FOR_HUMAN stage no longer
// produces an approval gate on its own; only SOP's approval listing does.
func TestClassificationSources(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('r1','R','o','a','IMPLEMENTING',NULL,1,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('r2','S','o','a','IMPLEMENTING',NULL,1,3,'`+now+`','`+now+`')`,
	)
	writeArtifact(t, root, "r1", "report.json", `{
	  "id":"r1","stage":"FAILED","decision":"FAIL","fix_cycles":1,
	  "classification":{"kind":"TEST_FAILURE","disposition":"AUTO_FIX","confidence":"HIGH","reason":"a deterministic test failed"},
	  "jev":{"status":"PASS","findings":[]},
	  "generated_at":"2026-01-01T00:00:00Z"
	}`)
	writeArtifact(t, root, "r2", "classification.json",
		`{"kind":"AMBIGUOUS_CONTRACT","disposition":"NEEDS_HUMAN","confidence":"HIGH","reason":"two contracts remain"}`)
	writeArtifact(t, root, "r2", "state.json", `{"id":"r2","stage":"WAITING_FOR_HUMAN"}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	r1, err := st.Task(context.Background(), "r1", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if r1.Run.Classification == nil || r1.Run.Classification.Disposition != DispositionAutoFix {
		t.Fatalf("r1 classification = %+v, want AUTO_FIX from report.json", r1.Run.Classification)
	}
	if r1.Run.FixCycles != 1 || r1.Run.JEV == nil || r1.Run.JEV.Status != "PASS" {
		t.Fatalf("r1 run info incomplete: %+v", r1.Run)
	}
	if r1.NeedsHuman() || r1.Recovering() != DispositionAutoFix {
		t.Fatalf("r1 needsHuman=%v recovering=%q, want false/AUTO_FIX", r1.NeedsHuman(), r1.Recovering())
	}

	r2, err := st.Task(context.Background(), "r2", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	// The NEEDS_HUMAN run classification is still surfaced as a recovery
	// disposition, but C2-001 makes the approval listing the ONLY source of a
	// gate: without a listing entry the task reports no approval.
	if r2.Recovering() != "" {
		t.Fatalf("r2 recovering=%q, want \"\"", r2.Recovering())
	}
	if r2.NeedsHuman() {
		t.Fatalf("r2 needsHuman=true, want false: a NEEDS_HUMAN classification alone must not produce a gate")
	}
	if r2.Run.Stage != StageWaitingForHuman {
		t.Fatalf("r2 stage = %q, want WAITING_FOR_HUMAN", r2.Run.Stage)
	}
}

// The task activity timeline is bounded to the most recent events, so a heavily
// retried task cannot force an unbounded parse or render.
func TestActivityCappedToMostRecent(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('cap','C','o','a','IMPLEMENTING',NULL,0,3,'`+now+`','`+now+`')`)

	var b strings.Builder
	for i := 0; i < maxActivityEvents+25; i++ {
		b.WriteString(`{"stage":"CHANGE","action":"step","detail":"`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`","timestamp":"2026-01-01T00:00:00Z"}` + "\n")
	}
	writeArtifact(t, root, "cap", "activity.jsonl", b.String())

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d, err := st.Task(context.Background(), "cap", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Activity) != maxActivityEvents {
		t.Fatalf("activity = %d events, want cap of %d", len(d.Activity), maxActivityEvents)
	}
	// The most recent events are kept, in order: the last retained detail is the
	// last written one.
	if last := d.Activity[len(d.Activity)-1].Detail; last != strconv.Itoa(maxActivityEvents+24) {
		t.Fatalf("last retained event = %q, want the most recent", last)
	}
}

// The task list carries the cheap stage/recovery fields, and the active plan
// source is read from SOP's plan.meta.json.
func TestRunMetaAndPlanSource(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('m1','M','o','a','IMPLEMENTING',NULL,1,3,'`+now+`','`+now+`')`)
	writeArtifact(t, root, "m1", "state.json", `{"id":"m1","stage":"VALIDATING"}`)
	writeArtifact(t, root, "m1", "classification.json",
		`{"kind":"INCOMPLETE_IMPLEMENTATION","disposition":"CONTINUE","confidence":"HIGH","reason":"work remains"}`)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md","plan_id":"plan"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tasks, err := st.Tasks(context.Background(), readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Stage != "VALIDATING" || tasks[0].Recovery != DispositionContinue {
		t.Fatalf("run meta = %+v, want stage VALIDATING recovery CONTINUE", tasks)
	}
	if src, ok := st.PlanSource(); !ok || src != "docs/PLAN.md" {
		t.Fatalf("PlanSource = %q,%v, want docs/PLAN.md,true", src, ok)
	}
}

// SOP's recorded model selection (report.json "model_selection") is carried
// verbatim: class, locality, source, fallback, and reason are read as SOP
// persisted them. The controller reports the selection; it never selects a
// model or provider (docs/specs/AGENT-PROVIDER.md). Without the field the
// projection is absent, not a zero-filled class.
func TestModelSelectionReadVerbatim(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('sel','S','o','a','IMPLEMENTING',NULL,1,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('nosel','N','o','a','PLANNED',NULL,0,3,'`+now+`','`+now+`')`,
	)
	writeArtifact(t, root, "sel", "report.json", `{
	  "id":"sel","stage":"PASSED","decision":"PASS","provider":"ollama","model":"deepseek-v4.1-flash:cloud",
	  "model_selection":{"class":"large","provider":"ollama","model":"deepseek-v4.1-flash:cloud","locality":"cloud","source":"env","fallback":false,"reason":"environment default class"},
	  "generated_at":"2026-01-01T00:00:00Z"
	}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d, err := st.Task(context.Background(), "sel", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	ms := d.Run.ModelSelection
	if !ms.Present {
		t.Fatal("ModelSelection.Present = false, want true")
	}
	if ms.Class != "large" || ms.Locality != "cloud" || ms.Source != "env" || ms.Fallback || ms.Reason != "environment default class" {
		t.Fatalf("model selection not verbatim: %+v", ms)
	}

	d2, err := st.Task(context.Background(), "nosel", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if d2.Run.ModelSelection.Present {
		t.Fatalf("ModelSelection.Present = true without a record: %+v", d2.Run.ModelSelection)
	}
}
