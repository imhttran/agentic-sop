package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// semgrepDeclaredExcludeSummary is Semgrep's real summary shape after the
// ignored set is declared with --exclude: the skips are attributed to configured
// exclusions, which is scope rather than a coverage gap.
const semgrepDeclaredExcludeSummary = `┌──────────────┐
│ Scan Summary │
└──────────────┘
 • Targets scanned: 1
 • Scan skipped:
   ◦ Matching --exclude patterns: 1
Ran 1 rule on 1 file: 0 findings.
`

type semgrepScopeStub struct {
	argv   []string
	stdout []byte
	stderr []byte
}

// semgrepScopeAdapter builds a Semgrep adapter whose scanner call is recorded
// instead of executed. The workspace and its Git state are real.
func semgrepScopeAdapter(t *testing.T, rules string, stub *semgrepScopeStub) *semgrepAdapter {
	t.Helper()
	return &semgrepAdapter{
		id:       "semgrep",
		category: CategorySAST,
		command:  []string{"semgrep"},
		rules:    rules,
		exec: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
			stub.argv = append([]string(nil), argv...)
			return stub.stdout, stub.stderr, 0, nil
		}},
		timeout:   time.Minute,
		maxOutput: 1 << 20,
	}
}

// isolateGitConfig keeps the adapter's own Git calls deterministic. The adapter
// runs git with the ambient environment, so a host or CI global excludes file
// would otherwise change the ignored set under test.
func isolateGitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

// writeSemgrepScopeRules creates a minimal Go ruleset so language coverage is
// satisfied and the scan reaches the scanner call.
func writeSemgrepScopeRules(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "rules")
	writeFile(t, filepath.Join(dir, "r.yaml"), "rules:\n- id: probe\n  languages: [go]\n  message: probe\n  severity: WARNING\n  pattern: println(...)\n")
	return dir
}

func semgrepEmptyReport() []byte {
	return []byte(`{"version":"1.183.0","results":[],"errors":[],"paths":{"scanned":["main.go"],"skipped":[]}}`)
}

func scopeDiagnostics(exec Execution) string { return strings.Join(exec.Diagnostics, " ") }

// TestSemgrepScopeDeclaresGitIgnoredPaths is the positive control: a path Git
// ignores is declared to Semgrep as an exclusion, so its skip is scope rather
// than a coverage gap.
func TestSemgrepScopeDeclaresGitIgnoredPaths(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeFile(t, filepath.Join(root, ".semgrepignore"), ":include .gitignore\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "ignored", "a.go"), "package main\n")
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", ".semgrepignore", "main.go", "rules/r.yaml")
	gitRun(t, root, "commit", "-q", "-m", "init")

	stub := &semgrepScopeStub{stdout: semgrepEmptyReport(), stderr: []byte(semgrepDeclaredExcludeSummary)}
	exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted {
		t.Fatalf("status = %s, want COMPLETED (%v)", exec.Status, exec.Diagnostics)
	}
	if !containsArg(stub.argv, "--exclude=/ignored") {
		t.Errorf("argv has no derived exclusion for the ignored directory: %v", stub.argv)
	}
	if exec.Coverage != CoverageComplete {
		t.Errorf("coverage = %s, want COMPLETE: %v", exec.Coverage, exec.Diagnostics)
	}
	if !strings.Contains(scopeDiagnostics(exec), "git-ignored paths are declared out of scope") {
		t.Errorf("diagnostics = %v, want the derived-scope statement", exec.Diagnostics)
	}
}

// TestSemgrepScopeKeepsUntrackedNonIgnored is the negative control for an
// untracked file: it is repository source, so nothing may exclude it.
func TestSemgrepScopeKeepsUntrackedNonIgnored(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "untracked.go"), "package main\n")
	writeFile(t, filepath.Join(root, "ignored", "a.go"), "package main\n")
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", "main.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	stub := &semgrepScopeStub{stdout: semgrepEmptyReport(), stderr: []byte(semgrepDeclaredExcludeSummary)}
	exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range stub.argv {
		if strings.Contains(arg, "untracked") {
			t.Errorf("in-scope untracked file was declared out of scope: %s", arg)
		}
	}
	if exec.Coverage != CoverageComplete {
		t.Errorf("coverage = %s, want COMPLETE: %v", exec.Coverage, exec.Diagnostics)
	}
}

// TestSemgrepScopeKeepsTrackedEvenIfIgnored proves a tracked file stays in scope
// even when an ignore rule matches it, in both the per-file and per-directory
// forms.
func TestSemgrepScopeKeepsTrackedEvenIfIgnored(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "generated/\n*.gen.go\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	// Tracked despite both rules, using the per-file and per-directory forms.
	writeFile(t, filepath.Join(root, "generated", "tracked.go"), "package main\n")
	writeFile(t, filepath.Join(root, "tracked.gen.go"), "package main\n")
	// Ignored and untracked, so legitimately out of scope.
	writeFile(t, filepath.Join(root, "generated", "other.go"), "package main\n")
	writeFile(t, filepath.Join(root, "untracked.gen.go"), "package main\n")
	gitRun(t, root, "add", ".gitignore", "main.go")
	gitRun(t, root, "add", "-f", "generated/tracked.go", "tracked.gen.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	scope, err := deriveSemgrepIgnoredScope(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	for _, inScope := range []string{"main.go", "generated/tracked.go", "tracked.gen.go"} {
		if scope.contains(inScope) {
			t.Errorf("in-scope path %q was declared out of scope: %v", inScope, scope.Excludes)
		}
	}
	for _, outOfScope := range []string{"generated/other.go", "untracked.gen.go"} {
		if !scope.contains(outOfScope) {
			t.Errorf("ignored path %q was not declared out of scope: %v", outOfScope, scope.Excludes)
		}
	}
	if contains(scope.Excludes, "generated") || contains(scope.Excludes, "/generated") {
		t.Errorf("the whole generated/ directory was excluded while it holds a tracked file: %v", scope.Excludes)
	}
}

// TestSemgrepScopeAnchorsRootEntries proves a root-level entry is anchored, so it
// cannot match an in-scope directory of the same name at depth.
func TestSemgrepScopeAnchorsRootEntries(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "sop\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "cmd", "sop", "x.go"), "package main\n")
	writeFile(t, filepath.Join(root, "sop"), "built artifact\n")
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", "main.go")
	gitRun(t, root, "add", "-f", "cmd/sop/x.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	stub := &semgrepScopeStub{stdout: semgrepEmptyReport(), stderr: []byte(semgrepDeclaredExcludeSummary)}
	exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(stub.argv, "--exclude=/sop") {
		t.Fatalf("root entry was not anchored: %v", stub.argv)
	}
	for _, arg := range stub.argv {
		if arg == "--exclude=/cmd/sop" || arg == "--exclude=sop" {
			t.Errorf("unanchored or over-broad exclusion: %s", arg)
		}
	}
	scope, err := deriveSemgrepIgnoredScope(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if scope.contains("cmd/sop/x.go") {
		t.Errorf("in-scope cmd/sop/x.go was declared out of scope: %v", scope.Excludes)
	}
	if exec.Coverage != CoverageComplete {
		t.Errorf("coverage = %s, want COMPLETE: %v", exec.Coverage, exec.Diagnostics)
	}
}

// TestSemgrepScopeRejectsPatternSyntax proves a path carrying ignore-pattern
// syntax is never handed to Semgrep, which would widen or reinterpret the match.
// The path stays in scope, so its skips keep counting as gaps.
func TestSemgrepScopeRejectsPatternSyntax(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "*.log\n*.txt\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	for _, name := range []string{"a*b.txt", "!keep.txt", "d[1].txt", "plain.log"} {
		writeFile(t, filepath.Join(root, name), "ignored\n")
	}
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", "main.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	scope, err := deriveSemgrepIgnoredScope(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(scope.Excludes, "/plain.log") {
		t.Errorf("plain ignored path was not declared out of scope: %v", scope.Excludes)
	}
	for _, entry := range scope.Excludes {
		if strings.ContainsAny(entry, "*?[]{}!") {
			t.Errorf("pattern syntax reached the exclusion list: %s", entry)
		}
	}
	if scope.Dropped != 3 {
		t.Errorf("dropped = %d, want the three pattern-like paths: %v", scope.Dropped, scope.Excludes)
	}
	for _, path := range []string{"a*b.txt", "!keep.txt", "d[1].txt"} {
		if scope.contains(path) {
			t.Errorf("pattern-like path %q was declared out of scope: %v", path, scope.Excludes)
		}
	}

	stub := &semgrepScopeStub{stdout: semgrepEmptyReport(), stderr: []byte(semgrepCoverageSummaryNoSkip)}
	exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(scopeDiagnostics(exec), "remain in scope") {
		t.Errorf("diagnostics = %v, want the dropped-path statement", exec.Diagnostics)
	}
}

// TestPlanSemgrepIgnoredScopeNeverShadowsInScopeContent exercises the admission
// guard with adversarial input instead of trusting Git's accounting.
func TestPlanSemgrepIgnoredScopeNeverShadowsInScopeContent(t *testing.T) {
	scope, err := planSemgrepIgnoredScope(
		[]string{"build/", "ignored/a.go", "tracked.go", "../escape"},
		[]string{"build/keep.go", "ignored/b.go", "tracked.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if contains(scope.Excludes, "/build") || scope.contains("build/keep.go") {
		t.Errorf("an ignored directory containing in-scope content was excluded: %v", scope.Excludes)
	}
	if contains(scope.Excludes, "/tracked.go") || scope.contains("tracked.go") {
		t.Errorf("an in-scope path was excluded: %v", scope.Excludes)
	}
	if contains(scope.Excludes, "/../escape") {
		t.Errorf("a traversal path reached the exclusion list: %v", scope.Excludes)
	}
	if !contains(scope.Excludes, "ignored/a.go") {
		t.Errorf("a genuinely ignored path was dropped: %v", scope.Excludes)
	}
	if scope.Dropped != 3 {
		t.Errorf("dropped = %d, want 3: %v", scope.Dropped, scope.Excludes)
	}
	// Directory membership covers descendants of an ignored directory.
	if !scope.contains("ignored/a.go") || scope.contains("ignored/b.go") {
		t.Errorf("membership index is wrong for %v", scope.Excludes)
	}
}

// TestSemgrepScopeBoundedSetIsNotDeclared proves an oversized ignored set derives
// nothing rather than a partial exclusion list.
func TestSemgrepScopeBoundedSetIsNotDeclared(t *testing.T) {
	ignored := make([]string, semgrepIgnoredExcludeLimit+1)
	for i := range ignored {
		ignored[i] = fmt.Sprintf("artifact-%d/", i)
	}
	scope, err := planSemgrepIgnoredScope(ignored, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Bounded || len(scope.Excludes) != 0 {
		t.Fatalf("bounded=%v excludes=%d, want a bounded set with no exclusions", scope.Bounded, len(scope.Excludes))
	}
}

// TestSemgrepScopeFailsClosedWithoutUsableGit proves the two fail-closed paths:
// a non-repository has no ignore database so coverage is still PARTIAL, and a
// checkout whose Git state cannot be read fails the scan instead of claiming a
// scope.
func TestSemgrepScopeFailsClosedWithoutUsableGit(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		isolateGitConfig(t)
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "main.go"), "package main\n")
		writeFile(t, filepath.Join(root, "build", "a.go"), "package main\n")
		rules := writeSemgrepScopeRules(t, root)

		stub := &semgrepScopeStub{
			stdout: semgrepEmptyReport(),
			stderr: []byte("Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Files matching .semgrepignore patterns: 1\nRan 1 rule on 1 file: 0 findings.\n"),
		}
		exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
		if err != nil {
			t.Fatal(err)
		}
		if exec.Coverage == CoverageComplete {
			t.Fatalf("coverage = COMPLETE without a usable Git scope: %v", exec.Diagnostics)
		}
		if exec.Coverage != CoveragePartial {
			t.Errorf("coverage = %s, want PARTIAL: %v", exec.Coverage, exec.Diagnostics)
		}
		for _, arg := range stub.argv {
			if strings.HasPrefix(arg, "--exclude=") {
				t.Errorf("an exclusion was derived without Git: %s", arg)
			}
		}
	})

	t.Run("unreadable checkout", func(t *testing.T) {
		isolateGitConfig(t)
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "main.go"), "package main\n")
		// Git metadata that cannot be read: the directory is a checkout, but its
		// scope is unknowable, so the scan must not claim coverage.
		writeFile(t, filepath.Join(root, ".git"), "not a git directory\n")
		rules := writeSemgrepScopeRules(t, root)

		stub := &semgrepScopeStub{stdout: semgrepEmptyReport(), stderr: []byte(semgrepCoverageSummaryNoSkip)}
		exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
		if err != nil {
			t.Fatal(err)
		}
		if exec.Status != StatusFailed || exec.Coverage != CoverageUnknown {
			t.Fatalf("status=%s coverage=%s, want a failed, unknown-coverage scan: %v", exec.Status, exec.Coverage, exec.Diagnostics)
		}
		if !strings.Contains(scopeDiagnostics(exec), "out-of-scope set is unavailable") {
			t.Errorf("diagnostics = %v, want the unavailable-scope statement", exec.Diagnostics)
		}
		if len(stub.argv) != 0 {
			t.Errorf("semgrep ran without a usable scope: %v", stub.argv)
		}
	})
}

// TestSemgrepCoverageCountsInScopeSkipsAsGaps is the anti-regression for this
// change: the derived exclusions must not excuse a skip of in-scope content.
func TestSemgrepCoverageCountsInScopeSkipsAsGaps(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "untracked.go"), "package main\n")
	writeFile(t, filepath.Join(root, "ignored", "a.go"), "package main\n")
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", "main.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	stub := &semgrepScopeStub{
		stdout: semgrepEmptyReport(),
		stderr: []byte("Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Matching --exclude patterns: 1\n   ◦ Files matching .semgrepignore patterns: 1\nRan 1 rule on 1 file: 0 findings.\n"),
	}
	exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Coverage != CoveragePartial {
		t.Fatalf("coverage = %s, want PARTIAL while an in-scope target was skipped: %v", exec.Coverage, exec.Diagnostics)
	}
	if !strings.Contains(scopeDiagnostics(exec), ".semgrepignore patterns") {
		t.Errorf("diagnostics = %v, want the in-scope skip reason", exec.Diagnostics)
	}
}

// TestSemgrepCoverageLargeInScopeFileIsStillAGap proves a size skip of in-scope
// content remains a gap; the derived exclusions cover ignored paths only.
func TestSemgrepCoverageLargeInScopeFileIsStillAGap(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "generated_big.go"), "package main\n")
	writeFile(t, filepath.Join(root, "ignored", "a.go"), "package main\n")
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", "main.go", "generated_big.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	stub := &semgrepScopeStub{
		stdout: semgrepEmptyReport(),
		stderr: []byte("Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Matching --exclude patterns: 1\n   ◦ Files larger than files 1.0 MB: 1\nRan 1 rule on 1 file: 0 findings.\n"),
	}
	exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Coverage != CoveragePartial {
		t.Fatalf("coverage = %s, want PARTIAL for a large in-scope file: %v", exec.Coverage, exec.Diagnostics)
	}
	if !strings.Contains(scopeDiagnostics(exec), "Files larger than") {
		t.Errorf("diagnostics = %v, want the size-skip reason", exec.Diagnostics)
	}
}

// TestSemgrepScopeUnusedInChangedFilesMode proves an explicitly requested
// changed file is never declared out of scope.
func TestSemgrepScopeUnusedInChangedFilesMode(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "ignored", "a.go"), "package main\n")
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", "main.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	stub := &semgrepScopeStub{stdout: semgrepEmptyReport(), stderr: []byte(semgrepDeclaredExcludeSummary)}
	req := requestFor(t, root)
	req.Scope = Scope{Mode: "changed-files", Files: []string{"main.go"}}
	if _, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, arg := range stub.argv {
		if strings.HasPrefix(arg, "--exclude=") {
			t.Errorf("changed-files scope derived an exclusion: %s", arg)
		}
	}
}

// TestSemgrepFindingsInIgnoredPathsAreDropped proves a finding attributed to
// out-of-scope content never reaches the normalized report.
func TestSemgrepFindingsInIgnoredPathsAreDropped(t *testing.T) {
	isolateGitConfig(t)
	root := gitWorkspace(t)
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "ignored", "a.go"), "package main\n")
	rules := writeSemgrepScopeRules(t, root)
	gitRun(t, root, "add", ".gitignore", "main.go")
	gitRun(t, root, "commit", "-q", "-m", "init")

	stub := &semgrepScopeStub{
		stdout: []byte(`{"version":"1.183.0","results":[{"check_id":"probe","path":"ignored/a.go","start":{"line":1,"col":1},"end":{"line":1,"col":2},"extra":{"message":"probe","severity":"WARNING","fingerprint":"abc"}}],"errors":[],"paths":{"scanned":["main.go"],"skipped":[]}}`),
		stderr: []byte(semgrepDeclaredExcludeSummary),
	}
	exec, err := semgrepScopeAdapter(t, rules, stub).Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.Findings) != 0 {
		t.Fatalf("findings = %d, want none from an out-of-scope path: %+v", len(exec.Findings), exec.Findings)
	}
	if !strings.Contains(scopeDiagnostics(exec), "git-ignored out-of-scope paths were omitted") {
		t.Errorf("diagnostics = %v, want the out-of-scope finding statement", exec.Diagnostics)
	}
}
