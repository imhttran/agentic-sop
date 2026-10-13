package web

import (
	"database/sql"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	assets "sop-controller"

	"sop-controller/internal/config"
	"sop-controller/internal/sopclient"

	_ "modernc.org/sqlite"
)

// decisionServer builds a single-project server whose one task ("x1") has an
// applicable approval gate in SOP's listing, driven by the given sop binary body
// ("" for a zero-exit binary). It returns the server, project id, and the
// recorded-argv accessor so a test can assert the exact `sop approve`/
// `sop decline` invocation the handler drove.
func decisionServer(t *testing.T, sopBody string) (*httptest.Server, string, func() []string) {
	return decisionServerOperator(t, sopBody, "")
}

// decisionServerOperator is decisionServer with a configured decision actor
// (SOP_CONTROLLER_OPERATOR), for the attribution tests.
func decisionServerOperator(t *testing.T, sopBody, operator string) (*httptest.Server, string, func() []string) {
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
		`INSERT INTO tasks VALUES ('x1','Task X','objective','criteria','BLOCKED','REVIEW_UNRESOLVED',2,3,'` + now + `','` + now + `')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()
	if err := os.WriteFile(filepath.Join(dir, "approvals.json"), []byte(approvalListing("needs a human")), 0o644); err != nil {
		t.Fatal(err)
	}

	bin, args := recordSop(t, sopBody)
	sop, err := sopclient.New([]string{root}, withApprovals(t, bin), time.Minute)
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
	srv := httptest.NewServer(NewServer(Options{SOP: sop, Views: views, StaticFS: staticFS, Poll: time.Second, CommandTimeout: time.Minute, Operator: operator}))
	t.Cleanup(srv.Close)
	return srv, config.ProjectID(root), args
}

// recordSop writes an executable that records its argv (one arg per line) and,
// when body is empty, exits 0. When body is non-empty it exits non-zero with the
// body on stderr, so a decision rejection can be exercised end to end.
func recordSop(t *testing.T, body string) (bin string, args func() []string) {
	t.Helper()
	work := t.TempDir()
	record := filepath.Join(work, "args")
	bin = filepath.Join(work, "sop")
	script := "#!/bin/sh\n: > " + record + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + record + "; done\n"
	if body == "" {
		script += "exit 0\n"
	} else {
		script += "echo \"" + body + "\" 1>&2\nexit 1\n"
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, func() []string {
		raw, err := os.ReadFile(record)
		if err != nil {
			return nil
		}
		return strings.Fields(string(raw))
	}
}

// postApproval POSTs one approval action with a valid CSRF token and the given
// actor/note form values, returning the response status and body.
func postApproval(t *testing.T, srv *httptest.Server, project, task, verb, by, note string) (int, string) {
	t.Helper()
	client := &http.Client{Jar: mustJar(t)}
	resp, err := client.Get(srv.URL + "/projects/" + project)
	if err != nil {
		t.Fatal(err)
	}
	var token string
	for _, c := range resp.Cookies() {
		if c.Name == "sop_ctrl_csrf" {
			token = c.Value
		}
	}
	resp.Body.Close()
	if token == "" {
		t.Fatal("no CSRF cookie issued")
	}
	form := "csrf=" + token
	if by != "" {
		form += "&by=" + by
	}
	if note != "" {
		form += "&note=" + note
	}
	req, err := http.NewRequest("POST", srv.URL+"/projects/"+project+"/tasks/"+task+"/commands/"+verb, strings.NewReader(form))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

// waitDecision polls the approval status route until the command leaves running.
func waitDecision(t *testing.T, srv *httptest.Server, project, task, verb string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, body := get(t, srv.URL+"/projects/"+project+"/tasks/"+task+"/commands/"+verb)
		if !strings.Contains(body, "· running") {
			return body
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("decision command did not finish")
	return ""
}

// A configured decision actor (SOP_CONTROLLER_OPERATOR) is authoritative: a
// browser-supplied actor field cannot impersonate another operator, and the note is
// still preserved. Without it, the actor field is used (see
// TestApproveDelegatesWithMetadata).
func TestConfiguredOperatorOverridesActorField(t *testing.T) {
	srv, id, args := decisionServerOperator(t, "", "svc-operator")
	code, _ := postApproval(t, srv, id, "x1", "approve", "Impostor", "looks-good")
	if code != http.StatusOK {
		t.Fatalf("approve POST = %d, want 200", code)
	}
	waitDecision(t, srv, id, "x1", "approve")
	got := args()
	want := []string{"approve", "x1", "--by", "svc-operator", "--note", "looks-good"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("sop argv = %v, want the configured operator %v", got, want)
	}
}

// With a configured operator the approval form shows it read-only and offers no editable
// actor field; without one the editable field remains (backwards compatible).
func TestOperatorFieldRendersReadOnlyWhenConfigured(t *testing.T) {
	srv, id, _ := decisionServerOperator(t, "", "svc-operator")
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
	if code != http.StatusOK {
		t.Fatalf("task page = %d, want 200", code)
	}
	if strings.Contains(body, `name="by"`) {
		t.Error("a configured operator must not render an editable actor field")
	}
	if !strings.Contains(body, "svc-operator") {
		t.Error("the decision form must show the configured operator")
	}

	srv2, id2, _ := decisionServer(t, "")
	if _, body2 := get(t, srv2.URL+"/projects/"+id2+"/tasks/x1"); !strings.Contains(body2, `name="by"`) {
		t.Error("without a configured operator the editable actor field must remain")
	}
}

// The approve control invokes exactly `sop approve <task-id>` and forwards the
// operator's actor/note as --by/--note; no --run is ever passed.
func TestApproveDelegatesWithMetadata(t *testing.T) {
	srv, id, args := decisionServer(t, "")
	code, _ := postApproval(t, srv, id, "x1", "approve", "alice", "looks-good")
	if code != http.StatusOK {
		t.Fatalf("approve POST = %d, want 200", code)
	}
	body := waitDecision(t, srv, id, "x1", "approve")
	if !strings.Contains(body, "done") {
		t.Errorf("approve status = %q, want done", body)
	}
	got := args()
	want := []string{"approve", "x1", "--by", "alice", "--note", "looks-good"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("sop argv = %v, want %v", got, want)
	}
	for _, a := range got {
		if a == "--run" {
			t.Fatalf("argv %v must not include --run", got)
		}
	}
}

// The decline control invokes exactly `sop decline <task-id>` with metadata and
// never manufactures a failure: a zero-exit decline is reported as done.
func TestDeclineDelegatesWithMetadata(t *testing.T) {
	srv, id, args := decisionServer(t, "")
	code, _ := postApproval(t, srv, id, "x1", "decline", "bob", "nope")
	if code != http.StatusOK {
		t.Fatalf("decline POST = %d, want 200", code)
	}
	body := waitDecision(t, srv, id, "x1", "decline")
	if !strings.Contains(body, "done") {
		t.Errorf("decline status = %q, want done (not a failure)", body)
	}
	got := args()
	want := []string{"decline", "x1", "--by", "bob", "--note", "nope"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("sop argv = %v, want %v", got, want)
	}
}

// A SOP rejection of a stale/not-applicable decision is surfaced as an actionable
// conflict (a classified error), never a 500 and never a fabricated success.
func TestApproveRejectionIsConflictNotSuccess(t *testing.T) {
	srv, id, _ := decisionServer(t, "gate is stale")
	code, _ := postApproval(t, srv, id, "x1", "approve", "", "")
	if code != http.StatusOK {
		// The POST returns a status fragment (200) that then reports the conflict;
		// a 500 here would be the bug we are guarding against.
		t.Fatalf("approve POST = %d, want 200 (a rejection is reported in the fragment, not a 500)", code)
	}
	body := waitDecision(t, srv, id, "x1", "approve")
	if !strings.Contains(body, "conflict") {
		t.Errorf("rejection body = %q, want the conflict classification", body)
	}
	if !strings.Contains(body, "gate is stale") {
		t.Errorf("rejection body = %q, want SOP's own message", body)
	}
	if strings.Contains(body, "done") {
		t.Errorf("rejection body = %q, must NOT report success", body)
	}
}

// The task page offers the approve/decline controls (both supported) with the
// optional actor/note inputs.
// The task page offers the approve/decline controls (both supported) from one shared
// actor and decision-note field.
func TestApprovalControlsOfferMetadataInputs(t *testing.T) {
	srv, id, _ := decisionServer(t, "")
	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/x1")
	if code != http.StatusOK {
		t.Fatalf("task page = %d", code)
	}
	for _, want := range []string{
		"/tasks/x1/commands/approve", "/tasks/x1/commands/decline",
		`name="by"`, `name="note"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("task page missing %q", want)
		}
	}
	if n := strings.Count(body, `name="note"`); n != 1 {
		t.Errorf("task approval panel should offer one shared decision note, got %d", n)
	}
}
