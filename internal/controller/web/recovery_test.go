package web

import (
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	assets "github.com/imhttran/agentic-sop/internal/controller/assets"

	"github.com/imhttran/agentic-sop/internal/controller/config"
	"github.com/imhttran/agentic-sop/internal/controller/sopclient"

	_ "modernc.org/sqlite"
)

// fakeRunSopSrc is a minimal fake `sop` for the recovery/activity fixtures. It
// answers the C2-003 changed-executed-task listing
// (`sop reconcile <PLAN.md> --list-changed --json`) from
// .agent-sdlc/changed_listing.json when present (else an observed EMPTY listing),
// and exits 0 for every other verb. It lets a fixture exercise the real listing
// decode path without a real SOP binary.
const fakeRunSopSrc = `package main

import (
	"os"
	"path/filepath"
)

func main() {
	listing := false
	for _, a := range os.Args[1:] {
		if a == "--list-changed" {
			listing = true
		}
	}
	if listing {
		if raw, err := os.ReadFile(filepath.Join(".agent-sdlc", "changed_listing.json")); err == nil {
			os.Stdout.Write(raw)
			return
		}
		os.Stdout.Write([]byte("{\"version\":1,\"source\":\"docs/PLAN.md\",\"plan_id\":\"\",\"plan_changed\":false,\"unchanged\":[],\"updated\":[],\"added\":[],\"removed\":[],\"changed_executed\":[],\"removed_executed\":[],\"auto_reconciled\":[]}"))
	}
}
`

var (
	fakeRunSopBinOnce sync.Once
	fakeRunSopBinPath string
	fakeRunSopBinErr  error
)

// fakeRunSopBin compiles fakeRunSopSrc once per test run and returns the binary
// path. The fake reads its listing fixture from its cwd, which Commander.Exec
// sets to the project root.
func fakeRunSopBin(t *testing.T) string {
	t.Helper()
	fakeRunSopBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "run-sop-src")
		if err != nil {
			fakeRunSopBinErr = err
			return
		}
		src := filepath.Join(dir, "main.go")
		if err := os.WriteFile(src, []byte(fakeRunSopSrc), 0o644); err != nil {
			fakeRunSopBinErr = err
			return
		}
		bin := filepath.Join(dir, "sop")
		cmd := exec.Command("go", "build", "-o", bin, src)
		cmd.Dir = moduleRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			fakeRunSopBinErr = fmt.Errorf("%w: %s", err, out)
			return
		}
		fakeRunSopBinPath = bin
	})
	if fakeRunSopBinErr != nil {
		t.Fatalf("build fake sop: %v", fakeRunSopBinErr)
	}
	return fakeRunSopBinPath
}

// writeChangedListing writes SOP's authoritative changed-task listing document
// for a fixture, carrying the given changed_executed ids.
func writeChangedListing(t *testing.T, root string, ids ...string) {
	t.Helper()
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = `"` + id + `"`
	}
	doc := `{"version":1,"source":"docs/PLAN.md","plan_id":"p","plan_changed":true,` +
		`"unchanged":[],"updated":[],"added":[],"removed":[],` +
		`"changed_executed":[` + strings.Join(quoted, ",") + `],` +
		`"removed_executed":[],"auto_reconciled":[]}`
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "changed_listing.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRunServer builds a single-project server whose one task ("x1") has the given
// SOP status and run artifacts. It returns the server, the project id, and the
// project root. It is the fixture for the recovery/activity views. A map key of
// "approvals.json" is written to the .agent-sdlc directory (SOP's authoritative
// approval listing); every other key is a run artifact under runs/<id>/.
//
// The server's sop binary is the fakeRunSopBin, which answers the C2-003
// changed-task listing from .agent-sdlc/changed_listing.json (written by
// writeChangedListing) and exits 0 for every other verb.
func newRunServer(t *testing.T, status string, artifacts map[string]string) (*httptest.Server, string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".agent-sdlc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("project:\n  name: \"demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	stmts := []string{
		testSchema,
		`INSERT INTO tasks VALUES ('x1','Task X','objective','criteria','` + status + `','REVIEW_UNRESOLVED',2,3,'` + now + `','` + now + `')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()
	for name, content := range artifacts {
		if name == "approvals.json" {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		writeRun(t, root, "x1", name, content)
	}

	sop, err := sopclient.New([]string{root}, withApprovals(t, fakeRunSopBin(t)), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sop.Close() })
	views, err := NewViews(assets.FS)
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	staticFS, err := fs.Sub(assets.FS, "static")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(Options{SOP: sop, Views: views, StaticFS: staticFS, Poll: time.Second, CommandTimeout: time.Minute}))
	t.Cleanup(srv.Close)
	return srv, config.ProjectID(root), root
}

// approvalListing returns SOP's approval listing document with one applicable
// gate for x1, for fixtures that expect a human boundary.
func approvalListing(reason string) string {
	return `{"approvals":[{"task_id":"x1","kind":"NEEDS_HUMAN","target":"x1","reason":"` + reason + `","evidence":"","stage":"WAITING_FOR_HUMAN","disposition":"NEEDS_HUMAN","status":"PENDING","requested_at":"2026-01-01T00:00:00Z","task_status":"BLOCKED"}]}`
}

// Structured activity from SOP's activity.jsonl renders as a timeline, covering
// the discovery/change/validation/review/JEV/quality stages SOP emits.
func TestTaskActivityTimelineRendersStructured(t *testing.T) {
	srv, id, _ := newRunServer(t, "IMPLEMENTING", map[string]string{
		"activity.jsonl": `{"stage":"DISCOVER","action":"reading","detail":"internal/web/handlers.go","timestamp":"2026-01-01T00:00:01Z"}` + "\n" +
			`{"stage":"CHANGE","action":"adding tests","timestamp":"2026-01-01T00:00:02Z"}` + "\n" +
			`{"stage":"VALIDATE","action":"go test ./...","timestamp":"2026-01-01T00:00:03Z"}` + "\n" +
			`{"stage":"REVIEW","action":"reviewing changes","timestamp":"2026-01-01T00:00:04Z"}` + "\n" +
			`{"stage":"JEV","action":"analyzing changes","timestamp":"2026-01-01T00:00:05Z"}` + "\n" +
			`{"stage":"QUALITY","action":"PASS","timestamp":"2026-01-01T00:00:06Z"}` + "\n",
	})
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1/activity")
	if code != 200 {
		t.Fatalf("activity fragment: %d", code)
	}
	for _, want := range []string{
		"timeline", "DISCOVER", "internal/web/handlers.go", "CHANGE", "VALIDATE", "go test ./...",
		"REVIEW", "JEV", "QUALITY", "PASS",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("activity timeline missing %q", want)
		}
	}
}

// With no activity artifact the view says so rather than rendering an error.
func TestTaskActivityEmptyState(t *testing.T) {
	srv, id, _ := newRunServer(t, "PLANNED", nil)
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1/activity")
	if code != 200 {
		t.Fatalf("activity fragment: %d", code)
	}
	if !strings.Contains(body, "No activity recorded") {
		t.Errorf("empty activity state not shown: %q", body)
	}
}

// Each failure disposition gets its own treatment. A non-human disposition shows
// SOP's recovery (not a human callout). C2-001: a NEEDS_HUMAN disposition is not
// a gate by itself; a human callout is shown only when SOP's authoritative
// approval listing reports an applicable gate for the task.
func TestRecoveryDispositionsRendered(t *testing.T) {
	cases := []struct {
		name        string
		disposition string
		kind        string
		wantHuman   bool
	}{
		{"auto fix", "AUTO_FIX", "TEST_FAILURE", false},
		{"continue", "CONTINUE", "INCOMPLETE_IMPLEMENTATION", false},
		{"retry", "RETRY", "TRANSIENT_PROVIDER", false},
		{"replan", "REPLAN", "REPLAN_REQUIRED", false},
		{"needs human", "NEEDS_HUMAN", "AMBIGUOUS_CONTRACT", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cls := `{"kind":"` + tc.kind + `","disposition":"` + tc.disposition +
				`","confidence":"HIGH","reason":"reason for ` + tc.disposition + `"}`
			artifacts := map[string]string{
				"state.json":          `{"id":"x1","stage":"WAITING_FOR_HUMAN"}`,
				"classification.json": cls,
			}
			if tc.wantHuman {
				// SOP must report the gate in its authoritative listing for the human
				// callout to appear (C2-001).
				artifacts["approvals.json"] = approvalListing("reason for " + tc.disposition)
			}
			srv, id, _ := newRunServer(t, "IMPLEMENTING", artifacts)
			code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
			if code != 200 {
				t.Fatalf("task page: %d", code)
			}
			if !strings.Contains(body, tc.disposition) && tc.disposition != "NEEDS_HUMAN" {
				t.Errorf("disposition %s not shown", tc.disposition)
			}
			if !strings.Contains(body, tc.kind) {
				t.Errorf("failure kind %s not shown", tc.kind)
			}
			if !strings.Contains(body, "reason for "+tc.disposition) {
				t.Errorf("classification reason not shown for %s", tc.disposition)
			}
			if tc.wantHuman {
				if !strings.Contains(body, "callout-human") || !strings.Contains(body, "NEEDS HUMAN") {
					t.Errorf("%s: expected a human callout", tc.disposition)
				}
				if strings.Contains(body, "callout-auto") {
					t.Errorf("%s: must not show the auto-recovery callout", tc.disposition)
				}
			} else {
				if !strings.Contains(body, "callout-auto") {
					t.Errorf("%s: expected the auto-recovery callout", tc.disposition)
				}
				if strings.Contains(body, "callout-human") {
					t.Errorf("%s: must not show a human callout", tc.disposition)
				}
			}
		})
	}
}

// C2-001: a terminal BLOCKED task is NOT a human boundary by itself. Only an
// applicable gate in SOP's authoritative approval listing produces the callout;
// BLOCKED alone must not.
func TestBlockedTaskHumanBoundaryRequiresListing(t *testing.T) {
	srv, id, _ := newRunServer(t, "BLOCKED", nil)
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
	if code != 200 {
		t.Fatalf("task page: %d", code)
	}
	if strings.Contains(body, "callout-human") {
		t.Error("a BLOCKED task with no SOP-reported listing entry must not show a human callout")
	}

	srv2, id2, _ := newRunServer(t, "BLOCKED", map[string]string{
		"approvals.json": approvalListing("needs a human"),
	})
	code, body = get(t, srv2.URL+"/projects/"+id2+"/tasks/x1")
	if code != 200 {
		t.Fatalf("task page: %d", code)
	}
	if !strings.Contains(body, "callout-human") {
		t.Error("an applicable listing entry should show the human boundary callout")
	}
}

// The task list shows the run stage and the recovery disposition per task.
func TestProjectListShowsStageAndRecovery(t *testing.T) {
	srv, id, _ := newRunServer(t, "IMPLEMENTING", map[string]string{
		"state.json":          `{"id":"x1","stage":"VALIDATING"}`,
		"classification.json": `{"kind":"INCOMPLETE_IMPLEMENTATION","disposition":"CONTINUE","confidence":"HIGH","reason":"work remains"}`,
	})
	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("project: %d", code)
	}
	for _, want := range []string{"Stage", "Recovery", "VALIDATING", "CONTINUE", `data-label="Stage"`, `data-label="Recovery"`} {
		if !strings.Contains(body, want) {
			t.Errorf("project list missing %q", want)
		}
	}
}

// The recovery panel exposes only SOP-backed recovery operations.
func TestRecoveryActionsPresent(t *testing.T) {
	srv, id, _ := newRunServer(t, "BLOCKED", nil)
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
	if code != 200 {
		t.Fatalf("task page: %d", code)
	}
	for _, want := range []string{
		"/tasks/x1/commands/retry", "/tasks/x1/commands/retry-force", "/tasks/x1/commands/report",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("recovery action missing %q", want)
		}
	}
}

// The project commands include the retry-all and reconcile recovery operations
// driven through SOP.
func TestProjectRecoveryCommands(t *testing.T) {
	srv, id, root := newRunServer(t, "BLOCKED", nil)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("project: %d", code)
	}
	for _, want := range []string{"/commands/retry-all", "/commands/reconcile"} {
		if !strings.Contains(body, want) {
			t.Errorf("project recovery command missing %q", want)
		}
	}
}

// The recovery fragment route renders the same panel used inline.
func TestRecoveryFragmentRoute(t *testing.T) {
	srv, id, _ := newRunServer(t, "IMPLEMENTING", map[string]string{
		"classification.json": `{"kind":"TEST_FAILURE","disposition":"AUTO_FIX","confidence":"HIGH","reason":"fixable"}`,
	})
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1/recovery")
	if code != 200 {
		t.Fatalf("recovery fragment: %d", code)
	}
	if !strings.Contains(body, "AUTO_FIX") {
		t.Errorf("recovery fragment missing disposition: %q", body)
	}
}

// The dashboard stays usable on phones: the task table collapses into labelled
// cards and activity renders as a vertical timeline.
func TestSmallScreenRendering(t *testing.T) {
	srv, id, _ := newRunServer(t, "IMPLEMENTING", map[string]string{
		"state.json":     `{"id":"x1","stage":"VALIDATING"}`,
		"activity.jsonl": `{"stage":"DISCOVER","action":"reading","timestamp":"2026-01-01T00:00:01Z"}` + "\n",
	})
	code, css := get(t, srv.URL+"/static/app.css")
	if code != 200 {
		t.Fatalf("css: %d", code)
	}
	for _, want := range []string{"@media (max-width: 640px)", "table.tasks td::before", ".timeline"} {
		if !strings.Contains(css, want) {
			t.Errorf("stylesheet missing mobile rule %q", want)
		}
	}
	_, list := get(t, srv.URL+"/projects/"+id+"/tasks")
	for _, want := range []string{`data-label="State"`, `data-label="Stage"`, `data-label="Recovery"`, `data-label="Attempts"`} {
		if !strings.Contains(list, want) {
			t.Errorf("task cards missing %q for narrow screens", want)
		}
	}
}

// JEV diagnostics from a run report surface on the task page.
func TestJEVStatusShown(t *testing.T) {
	srv, id, _ := newRunServer(t, "LOCAL_DONE", map[string]string{
		"report.json": `{"id":"x1","stage":"PASSED","decision":"PASS","jev":{"status":"PASS","findings":[]},"classification":null}`,
	})
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
	if code != 200 {
		t.Fatalf("task page: %d", code)
	}
	if !strings.Contains(body, "JEV PASS") {
		t.Errorf("JEV status not shown: %q", body)
	}
}

// Every page must render to completion. A template that references a field the
// view model no longer has aborts mid-render with a partial body, so a page is
// only complete when it ends with </html> and contains no error text.
func TestPagesRenderToCompletion(t *testing.T) {
	srv, id, _ := newRunServer(t, "IMPLEMENTING", map[string]string{
		"state.json":          `{"id":"x1","stage":"IMPLEMENTING"}`,
		"activity.jsonl":      `{"stage":"IMPLEMENT","action":"implementing","timestamp":"2026-01-01T00:00:01Z"}` + "\n",
		"classification.json": `{"kind":"TEST_FAILURE","disposition":"AUTO_FIX","confidence":"HIGH","reason":"fixable"}`,
	})
	pages := []string{"/projects", "/projects/" + id, "/projects/" + id + "/tasks/x1"}
	fragments := []string{
		"/projects/" + id + "/tasks",
		"/projects/" + id + "/activity",
		"/projects/" + id + "/tasks/x1/activity",
		"/projects/" + id + "/tasks/x1/recovery",
		"/projects/" + id + "/tasks/x1/review",
		"/projects/" + id + "/tasks/x1/ci",
		"/projects/" + id + "/tasks/x1/handoff",
	}
	for _, path := range pages {
		code, body := get(t, srv.URL+path)
		if code != 200 {
			t.Errorf("%s: status %d", path, code)
			continue
		}
		if strings.Contains(body, "template error") {
			t.Errorf("%s: template execution error", path)
		}
		if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
			t.Errorf("%s: page did not render to completion (no closing </html>)", path)
		}
	}
	for _, path := range fragments {
		code, body := get(t, srv.URL+path)
		if code != 200 {
			t.Errorf("%s: status %d", path, code)
			continue
		}
		if strings.Contains(body, "template error") || strings.TrimSpace(body) == "" {
			t.Errorf("%s: fragment failed to render", path)
		}
	}
}

// postWithCSRF issues a CSRF token via the server, then POSTs the path with that
// token and cookie, the way the browser drives a state-changing route.
func postWithCSRF(t *testing.T, srv *httptest.Server, path string) int {
	t.Helper()
	resp, err := http.Get(srv.URL + "/projects")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var token string
	for _, c := range resp.Cookies() {
		if c.Name == "sop_ctrl_csrf" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("server issued no CSRF cookie")
	}
	req, err := http.NewRequest("POST", srv.URL+path, strings.NewReader("csrf="+token))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "sop_ctrl_csrf", Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// Reconcile with no recorded plan is a precondition failure (409), not a 500.
func TestReconcileWithoutPlanIsConflict(t *testing.T) {
	srv, id, _ := newRunServer(t, "PLANNED", nil)
	if code := postWithCSRF(t, srv, "/projects/"+id+"/commands/reconcile"); code != http.StatusConflict {
		t.Fatalf("reconcile without a plan = %d, want 409", code)
	}
}

// Reconcile with a recorded plan and no pending changed task starts through SOP
// and returns the status fragment (200). The changed-task listing is the fake
// sop's observed EMPTY listing, so nothing is pending.
func TestReconcileWithPlanStarts(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeChangedListing(t, root)
	if code := postWithCSRF(t, srv, "/projects/"+id+"/commands/reconcile"); code != http.StatusOK {
		t.Fatalf("reconcile with a plan = %d, want 200", code)
	}
}

// A command-status poll for an unknown project is a 404, not a phantom idle
// status; a known project still returns its (idle) status fragment.
func TestCommandStatusUnknownProjectIs404(t *testing.T) {
	srv, id, _ := newRunServer(t, "PLANNED", nil)
	for _, path := range []string{
		"/projects/nope/commands/run",
		"/projects/nope/tasks/x1/commands/retry",
	} {
		if code, _ := get(t, srv.URL+path); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
	for _, path := range []string{
		"/projects/" + id + "/commands/run",
		"/projects/" + id + "/tasks/x1/commands/retry",
	} {
		if code, _ := get(t, srv.URL+path); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, code)
		}
	}
}
