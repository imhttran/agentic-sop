package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// symlinkedRepo returns a project directory containing a .git entry and a symlink that
// names the same directory, as /var and /private/var do on macOS.
func symlinkedRepo(t *testing.T) (real, link string) {
	t.Helper()
	base := t.TempDir()
	real = filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return real, link
}

// TestRepoFingerprintResolvesSymlinks proves one repository has one identity however a
// process reached it.
func TestRepoFingerprintResolvesSymlinks(t *testing.T) {
	real, link := symlinkedRepo(t)
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if got := repoFingerprint(real); got != want {
		t.Fatalf("repoFingerprint(real) = %q, want %q", got, want)
	}
	if got := repoFingerprint(link); got != want {
		t.Fatalf("repoFingerprint(link) = %q, want %q", got, want)
	}
}

// TestCLIAnswerAcceptedAcrossRepositorySpellings proves an answer is not refused as a
// repository change when the question was bound under one spelling of the repository
// path and the answering process sees another. It covers a question recorded before
// identities were resolved (bound to the symlink spelling) and the reverse.
func TestCLIAnswerAcceptedAcrossRepositorySpellings(t *testing.T) {
	for _, tc := range []struct{ name, bindVia, answerVia string }{
		{"bound via symlink, answered via real path", "link", "real"},
		{"bound via real path, answered via symlink", "real", "link"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			real, link := symlinkedRepo(t)
			path := map[string]string{"real": real, "link": link}
			initProject(t, real)
			seedTask(t, real, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
			// Bind with the raw, unresolved spelling, as a question recorded before this
			// change would have been.
			q := createQuestion(t, real, &question.Question{
				Identity: "T001", Kind: "k", Body: "q", Revision: 1, RepoFingerprint: path[tc.bindVia],
			})
			if _, out, _ := runCLI(t, path[tc.answerVia], "answer", q.ID, "--body", "sort"); !strings.Contains(out, "accepted") {
				t.Fatalf("out=%q, want accepted", out)
			}
		})
	}
}

// TestCLIAnswerDifferentRepositoryStale proves a genuinely different repository is still
// refused.
func TestCLIAnswerDifferentRepositoryStale(t *testing.T) {
	real, _ := symlinkedRepo(t)
	other, _ := symlinkedRepo(t)
	initProject(t, real)
	seedTask(t, real, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	q := createQuestion(t, real, &question.Question{
		Identity: "T001", Kind: "k", Body: "q", Revision: 1, RepoFingerprint: other,
	})
	if _, out, _ := runCLI(t, real, "answer", q.ID, "--body", "sort"); !strings.Contains(out, "stale") || !strings.Contains(out, "repository changed") {
		t.Fatalf("out=%q, want a stale repository refusal", out)
	}
	if _, out, _ := runCLI(t, real, "question", q.ID); !strings.Contains(out, "PENDING") {
		t.Fatalf("stale answer advanced the question: %q", out)
	}
}

// TestPlannerStatusRepositorySpelling proves the reconcile view does not read another
// spelling of the same repository as a change, and still reads a different repository
// as stale.
func TestPlannerStatusRepositorySpelling(t *testing.T) {
	real, link := symlinkedRepo(t)
	other, _ := symlinkedRepo(t)
	for _, tc := range []struct {
		bound     string
		wantStale bool
	}{{link, false}, {other, true}} {
		root := real
		if tc.wantStale {
			root, _ = symlinkedRepo(t)
		}
		initProject(t, root)
		seedTask(t, root, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
		enablePlanner(t, root, "")
		createQuestion(t, root, &question.Question{
			Identity: "T001", Kind: "k", Body: "q", RunID: "T001", Revision: 1, RepoFingerprint: tc.bound,
		})
		_, out, _ := runCLI(t, root, "planner", "status")
		if got := strings.Contains(out, "SKIP_STALE"); got != tc.wantStale {
			t.Fatalf("bound %q: status = %q, want stale=%t", tc.bound, out, tc.wantStale)
		}
	}
}
