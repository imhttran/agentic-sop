package sopclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// fakeSop writes an executable "sop" that records its argv, one arg per line, and
// returns it. It lets a test assert exactly which SOP verb and arguments the
// controller drives, without invoking a real SOP.
func fakeSop(t *testing.T) (bin string, args func() []string) {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	bin = filepath.Join(dir, "sop")
	script := "#!/bin/sh\n: > " + record + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + record + "; done\nexit 0\n"
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

// refusingSop writes an executable "sop" that prints message to stderr and exits
// with code, so a refused mutating verb can be exercised without a real SOP.
func refusingSop(t *testing.T, message string, code int) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sop")
	script := "#!/bin/sh\necho \"" + message + "\" 1>&2\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// fakeListingSop writes an executable "sop" that records its argv and emits the
// given stdout for every invocation, so a test can drive
// `sop reconcile <PLAN.md> --list-changed --json` and assert the decoded listing
// without a real SOP binary.
func fakeListingSop(t *testing.T, stdout string) (bin string, args func() []string) {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	payload := filepath.Join(dir, "stdout")
	bin = filepath.Join(dir, "sop")
	if err := os.WriteFile(payload, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n: > " + record + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + record + "; done\ncat " + payload + "\nexit 0\n"
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

// fakeFailingSop writes an executable "sop" that always exits non-zero, so a
// failed listing is surfaced as such rather than an empty success.
func fakeFailingSop(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sop")
	script := "#!/bin/sh\necho 'sop: reconcile --list-changed unsupported' >&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// Recovery operations must drive SOP's own commands, never mutate state directly.
func TestRecoveryCommandsDriveSop(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','BLOCKED','X',3,3,'`+now+`','`+now+`')`)

	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	cases := []struct {
		name string
		run  func() error
		want []string
	}{
		{"retry", func() error { return c.Retry(context.Background(), pid, "t1") }, []string{"retry", "t1"}},
		{"retry force", func() error { return c.RetryForce(context.Background(), pid, "t1") }, []string{"retry", "t1", "--force"}},
		{"retry all", func() error { return c.RetryAll(context.Background(), pid) }, []string{"retry", "--all"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := args(); !equal(got, tc.want) {
				t.Fatalf("sop argv = %v, want %v", got, tc.want)
			}
		})
	}

	out, err := c.Reconcile(context.Background(), pid, "docs/PLAN.md")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	_ = out
	if got := args(); !equal(got, []string{"reconcile", "docs/PLAN.md"}) {
		t.Fatalf("reconcile argv = %v", got)
	}
}

// Retry must delegate the decision to SOP's CLI and never touch the retry
// budget itself: after Retry/RetryForce run against fakeSop (which performs no
// real mutation), the controller's own read path must still report exactly the
// attempt/max_attempts SOP's fixture recorded, proving the controller path
// writes nothing to state.db on its own.
func TestRetryDelegatesAndPreservesBudget(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','BLOCKED','X',2,3,'`+now+`','`+now+`')`)

	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	before, err := c.Project(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	var beforeTask TaskSummary
	for _, task := range before.Tasks {
		if task.ID == "t1" {
			beforeTask = task
		}
	}
	if beforeTask.Attempt != 2 || beforeTask.MaxAttempts != 3 {
		t.Fatalf("seed mismatch: %+v", beforeTask)
	}

	if err := c.Retry(context.Background(), pid, "t1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := args(); !equal(got, []string{"retry", "t1"}) {
		t.Fatalf("retry argv = %v", got)
	}

	after, err := c.Project(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	var afterTask TaskSummary
	for _, task := range after.Tasks {
		if task.ID == "t1" {
			afterTask = task
		}
	}
	if afterTask.Attempt != beforeTask.Attempt || afterTask.MaxAttempts != beforeTask.MaxAttempts {
		t.Fatalf("controller mutated retry budget: before=%+v after=%+v", beforeTask, afterTask)
	}
	if !afterTask.Retryable() {
		t.Fatalf("task should remain retryable after delegated retry: %+v", afterTask)
	}
}

// SOP_ACTIVITY is set for controller-driven commands so SOP persists the
// activity artifact the dashboard reads; an operator-set value is preserved.
func TestActivityEnvDefault(t *testing.T) {
	got := activityEnv([]string{"PATH=/bin"})
	if !contains(got, "SOP_ACTIVITY=on") {
		t.Fatalf("activityEnv = %v, want SOP_ACTIVITY=on added", got)
	}
	kept := activityEnv([]string{"SOP_ACTIVITY=off"})
	if contains(kept, "SOP_ACTIVITY=on") {
		t.Fatalf("activityEnv overrode an operator value: %v", kept)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// A command that ran and exited non-zero must report SOP's exit status and its
// own diagnostic: "exit status 1" alone tells an operator nothing about why a
// mutating verb was refused.
func TestExecKeepsRefusalExitStatusAndDiagnostic(t *testing.T) {
	cmd := NewCommander(refusingSop(t, "sop: task t1 is not BLOCKED", 3), time.Minute)

	_, err := cmd.Exec(context.Background(), t.TempDir(), "retry", "t1")
	if err == nil {
		t.Fatal("err = nil, want a refusal")
	}
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v (%T), want *CommandError", err, err)
	}
	if cmdErr.Verb != "retry" || cmdErr.ExitCode != 3 {
		t.Fatalf("CommandError = %+v, want verb retry and exit code 3", cmdErr)
	}
	if !strings.Contains(cmdErr.Output, "not BLOCKED") {
		t.Fatalf("Output = %q, want SOP's diagnostic kept", cmdErr.Output)
	}
	for _, want := range []string{"sop retry", "exit status 3", "not BLOCKED"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to contain %q", err, want)
		}
	}
	// A refusal must stay identifiable as one: the rejection types in decision.go
	// and boundary.go are gated on errors.As(*exec.ExitError).
	if !sopRefused(err) {
		t.Fatalf("sopRefused(%v) = false, want true", err)
	}
}

// The mutating verbs that return only an error must still surface SOP's reason,
// which is the user-visible half of losing the command output.
func TestMutatingVerbSurfacesSopDiagnostic(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, refusingSop(t, "sop: provider credentials are not configured", 1), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = c.Run(context.Background(), config.ProjectID(root))
	if err == nil {
		t.Fatal("err = nil, want SOP's refusal")
	}
	if !strings.Contains(err.Error(), "provider credentials are not configured") {
		t.Fatalf("err = %q, want SOP's own diagnostic included", err)
	}
}

// A timeout or cancellation never reached SOP, so it stays a plain wrapped context
// error rather than a CommandError. The messages must not change.
func TestTimeoutAndCancelAreNotCommandErrors(t *testing.T) {
	timedOut := NewCommander(slowSop(t, "5"), 100*time.Millisecond)
	_, err := timedOut.Exec(context.Background(), t.TempDir(), "run")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want errors.Is(err, context.DeadlineExceeded)", err)
	}
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		t.Fatalf("timeout typed as *CommandError: %+v", cmdErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := NewCommander(refusingSop(t, "unused", 1), time.Minute)
	_, err = cancelled.Exec(ctx, t.TempDir(), "run")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
	}
	if errors.As(err, &cmdErr) {
		t.Fatalf("cancellation typed as *CommandError: %+v", cmdErr)
	}
}

// A runaway command must not carry its whole output into an error the controller
// keeps; the tail is what matters, and the cut must not split a rune.
func TestCommandErrorBoundsDiagnostic(t *testing.T) {
	// Offset 16 lands on the continuation byte of "é", forcing a mid-rune cut.
	out := "a" + strings.Repeat("é", 3000)
	cmdErr := &CommandError{Verb: "run", ExitCode: 1, Output: tailExcerpt(out, 5), Err: errors.New("exit status 1")}

	if len(cmdErr.Output) > 5+len("…") {
		t.Fatalf("Output kept %d bytes, want at most 5 plus the marker", len(cmdErr.Output))
	}
	if !utf8.ValidString(cmdErr.Output) {
		t.Fatalf("Output = %q, want a rune-boundary cut", cmdErr.Output)
	}

	long := &CommandError{Verb: "run", ExitCode: 1, Output: tailExcerpt(out, maxCommandDiagnostic), Err: errors.New("exit status 1")}
	if len(long.Error()) > maxCommandDiagnostic+100 {
		t.Fatalf("Error() kept %d bytes, want it bounded by maxCommandDiagnostic", len(long.Error()))
	}
}

// Exec wraps a timeout so callers can classify it with errors.Is, and keeps the
// human-readable timeout message.
func TestExecTimeoutWrapsDeadlineExceeded(t *testing.T) {
	cmd := NewCommander(slowSop(t, "5"), 100*time.Millisecond)

	_, err := cmd.Exec(context.Background(), t.TempDir(), "run")
	if err == nil {
		t.Fatal("err = nil, want a timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want errors.Is(err, context.DeadlineExceeded)", err)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want the timeout message kept", err)
	}
}

// A failing progress sink is advisory only: it must not turn a successful SOP
// command into a reported failure.
func TestExecIgnoresProgressSinkFailure(t *testing.T) {
	bin, _ := fakeListingSop(t, "ok")
	cmd := NewCommander(bin, time.Minute)

	out, err := cmd.Exec(WithProgress(context.Background(), failingWriter{}), t.TempDir(), "run")
	if err != nil {
		t.Fatalf("err = %v, want nil (a disconnected progress sink is not a command failure)", err)
	}
	if out != "ok" {
		t.Fatalf("out = %q, want the buffered command output", out)
	}
}

// failingWriter models a progress client that has gone away.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("client gone") }

// SanitizeDiagnostic keeps multi-line diagnostics but redacts a line that looks
// like it carries a credential.
func TestSanitizeDiagnostic(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"keeps a plain diagnostic", "sop validate: exit status 1\nreason: build failed", "sop validate: exit status 1\nreason: build failed"},
		{"keeps a bare token budget line", "reason: token budget exhausted", "reason: token budget exhausted"},
		{"redacts an api key assignment", "api_key=sk-live-123", "[redacted]"},
		{"redacts an authorization header", "Authorization: Bearer abcdefghijkl", "[redacted]"},
		{"redacts only the secret line", "reason: ok\npassword: hunter2", "reason: ok\n[redacted]"},
		{"redacts a pem header line", "-----BEGIN RSA PRIVATE KEY-----", "[redacted]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeDiagnostic(tc.in); got != tc.want {
				t.Errorf("SanitizeDiagnostic(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// CommandError renders SOP's diagnostic with credential lines redacted.
func TestCommandErrorRedactsDiagnostic(t *testing.T) {
	ce := &CommandError{Verb: "run", ExitCode: 1, Output: "api_key=sk-live-123\nreason: retry budget exhausted", Err: errors.New("exit status 1")}
	msg := ce.Error()
	if strings.Contains(msg, "sk-live-123") {
		t.Fatalf("CommandError leaked a credential: %q", msg)
	}
	if !strings.Contains(msg, "reason: retry budget exhausted") {
		t.Errorf("CommandError dropped the diagnostic: %q", msg)
	}
}
