package security

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

type countingExecutor struct{ runs int }

func (c *countingExecutor) Run(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
	c.runs++
	return nil, nil, 0, nil
}

// recordingExecutor records the path it was asked to run and the digest of the
// bytes at that path at execution time. swapTo, when set, rewrites the original
// file after the pin check, to simulate a writer racing the scan.
type recordingExecutor struct {
	argv0   string
	sum     string
	swapTo  string
	swapTxt string
}

func (r *recordingExecutor) Run(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
	if r.swapTo != "" {
		if err := os.WriteFile(r.swapTo, []byte(r.swapTxt), 0o755); err != nil {
			panic(err)
		}
	}
	r.argv0 = argv[0]
	r.sum, _ = fileSHA256(argv[0])
	return nil, nil, 0, nil
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	writeFile(t, path, content)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// LLM04 (PROPOSAL-SCANNER-BINARY-PINS option B): a configured binary pin is
// checked before every execution and a failed check never starts the scanner.
// Without a pin BuildAdapter keeps today's executor.
func TestPinnedExecutor(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "scanner")
	writeExecutable(t, bin, "#!/bin/sh\necho ok\n")
	binHash, _ := fileSHA256(bin)

	run := func(argv0 string) (int, error) {
		inner := &countingExecutor{}
		_, _, _, err := (pinnedExecutor{inner: inner, binarySHA256: binHash}).Run(context.Background(), dir, []string{argv0, "--version"}, nil, 0)
		return inner.runs, err
	}
	if n, err := run(bin); err != nil || n != 1 {
		t.Fatalf("matching pin: runs=%d err=%v, want one run", n, err)
	}
	if n, err := run(filepath.Join(dir, "missing")); !errors.Is(err, ErrUnavailable) || n != 0 {
		t.Fatalf("missing executable: runs=%d err=%v, want ErrUnavailable and no run", n, err)
	}

	// A relative command resolves against the scan dir (the child's Dir), not this
	// process's cwd. The executed file is a private copy of the pinned bytes, not the
	// original path, and the original is rewritten after the check to model a racing
	// writer: the copy must still hold the pinned digest.
	rec := &recordingExecutor{swapTo: bin, swapTxt: "#!/bin/sh\necho attacker\n"}
	if _, _, _, err := (pinnedExecutor{inner: rec, binarySHA256: binHash}).Run(context.Background(), dir, []string{"./scanner"}, nil, 0); err != nil {
		t.Fatalf("relative command: err=%v", err)
	}
	if rec.argv0 == bin {
		t.Fatalf("executed the original path %q; want a private copy", rec.argv0)
	}
	if rec.sum != binHash {
		t.Fatalf("executed copy digest %s, want the pinned %s", rec.sum, binHash)
	}
	if _, err := os.Stat(rec.argv0); !os.IsNotExist(err) {
		t.Fatalf("staged copy %s was not removed after the run", rec.argv0)
	}

	// A same-size one-byte replacement fails closed.
	writeExecutable(t, bin, "#!/bin/sh\necho OK\n")
	if n, err := run(bin); err == nil || !strings.Contains(err.Error(), "binary pin mismatch") || n != 0 {
		t.Fatalf("replaced binary: runs=%d err=%v, want mismatch and no run", n, err)
	}

	// BuildAdapter wraps only when a binary pin is set, and the pin moves the resume identity.
	raw := config.SecurityAdapterConfig{ID: "gitleaks", Category: "secrets", Command: []string{bin}}
	plain := adapterConfigDigest(raw)
	a, err := BuildAdapter(raw, BuildOptions{Executor: &countingExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, wrapped := a.(*gitleaksAdapter).exec.(pinnedExecutor); wrapped {
		t.Fatal("an unpinned adapter must keep the plain executor")
	}
	raw.BinarySHA256 = binHash
	if adapterConfigDigest(raw) == plain {
		t.Fatal("a binary pin must change the adapter config digest")
	}
	if a, err = BuildAdapter(raw, BuildOptions{Executor: &countingExecutor{}}); err != nil {
		t.Fatal(err)
	}
	if _, wrapped := a.(*gitleaksAdapter).exec.(pinnedExecutor); !wrapped {
		t.Fatal("a pinned adapter must use the pinned executor")
	}
}

// OWASP LLM03 (repository Plan 04; PROPOSAL-SCANNER-BINARY-PINS option B): a configured rules pin is
// verified against a private copy, and the scanner reads that copy. A writer
// that changes the original after the digest is computed cannot change what the
// scanner loads, and a changed tree still fails closed.
func TestRulesPinStagesVerifiedTree(t *testing.T) {
	root := t.TempDir()
	rules := filepath.Join(root, "rules")
	pinned := "rules:\n  - id: a\n    languages: [go]\n"
	writeFile(t, filepath.Join(rules, "r.yaml"), pinned)
	writeFile(t, filepath.Join(rules, "nested", "s.yaml"), "rules: []\n")
	// "a.txt" sorts before "a/z.yaml", but WalkDir visits the directory first.
	// The digest assertion below proves stageRules uses the same sorted framing.
	writeFile(t, filepath.Join(rules, "a.txt"), "x\n")
	writeFile(t, filepath.Join(rules, "a", "z.yaml"), "rules: []\n")
	pin, err := rulesIdentity(rules)
	if err != nil {
		t.Fatal(err)
	}

	staged, digest, cleanup, err := stageRules(rules, pin)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if digest != pin {
		t.Fatalf("staged digest = %s, want the pin %s", digest, pin)
	}
	if staged == rules {
		t.Fatal("staged path is the writable original")
	}
	if got, _ := os.ReadFile(filepath.Join(staged, "r.yaml")); string(got) != pinned {
		t.Fatalf("staged rule = %q, want the pinned bytes", got)
	}
	// The original is rewritten after staging; the copy keeps the pinned bytes.
	writeFile(t, filepath.Join(rules, "r.yaml"), "rules: [attacker]\n")
	if got, _ := os.ReadFile(filepath.Join(staged, "r.yaml")); string(got) != pinned {
		t.Fatalf("staged rule changed after the original was rewritten: %q", got)
	}
	cleanup()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged rules %s were not removed", staged)
	}

	// The changed tree no longer matches the pin.
	if _, _, _, err := stageRules(rules, pin); err == nil || !strings.Contains(err.Error(), "rules pin mismatch") {
		t.Fatalf("changed tree: err=%v, want a rules pin mismatch", err)
	}
	// A symlink entry is refused even though rulesIdentity would skip it.
	writeFile(t, filepath.Join(root, "outside.yaml"), "rules: [x]\n")
	if err := os.Symlink(filepath.Join(root, "outside.yaml"), filepath.Join(rules, "extra.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := stageRules(rules, pin); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink entry: err=%v, want a refusal", err)
	}
	// A single-file rules path is staged as a regular file, not as its directory.
	single := filepath.Join(root, "single.toml")
	writeFile(t, single, "[rules]\n")
	singlePin, err := rulesIdentity(single)
	if err != nil {
		t.Fatal(err)
	}
	stagedFile, singleDigest, cleanupSingle, err := stageRules(single, singlePin)
	if err != nil {
		t.Fatalf("single-file stage: %v", err)
	}
	if singleDigest != singlePin {
		t.Fatalf("single-file digest = %s, want %s", singleDigest, singlePin)
	}
	if info, err := os.Stat(stagedFile); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("single-file rules staged at %q, want a regular file", stagedFile)
	}
	cleanupSingle()

	// A single-file rules path that is itself a symlink is refused before hashing.
	linked := filepath.Join(root, "linked.yaml")
	if err := os.Symlink(filepath.Join(root, "outside.yaml"), linked); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := stageRules(linked, pin); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlinked rules file: err=%v, want a refusal", err)
	}
}

// configRecordingExecutor records the --config path it was asked to read and the
// bytes at that path at execution time. swapTo, when set, rewrites the original
// after the pin check, to model a writer racing the scan.
type configRecordingExecutor struct {
	config       string
	configIsFile bool
	body         []byte
	swapTo       string
	swapTxt      string
	stdout       []byte
}

func (r *configRecordingExecutor) Run(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
	if r.swapTo != "" {
		if err := os.WriteFile(r.swapTo, []byte(r.swapTxt), 0o644); err != nil {
			panic(err)
		}
	}
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--config" {
			r.config = argv[i+1]
		}
	}
	if r.config == "" {
		return r.stdout, nil, 0, nil
	}
	if info, err := os.Stat(r.config); err == nil {
		r.configIsFile = info.Mode().IsRegular()
		if info.IsDir() {
			r.body, _ = os.ReadFile(filepath.Join(r.config, filepath.Base(r.swapTo)))
		} else {
			r.body, _ = os.ReadFile(r.config)
		}
	}
	return r.stdout, nil, 0, nil
}

func TestSemgrepRulesPinStagesTheTreeItRuns(t *testing.T) {
	root := t.TempDir()
	rules := filepath.Join(root, "rules")
	pinned := "rules:\n  - id: a\n    languages: [go]\n"
	writeFile(t, filepath.Join(rules, "r.yaml"), pinned)
	pin, err := rulesIdentity(rules)
	if err != nil {
		t.Fatal(err)
	}

	rec := &configRecordingExecutor{
		swapTo:  filepath.Join(rules, "r.yaml"),
		swapTxt: "rules: [attacker]\n",
		stdout:  []byte("{\"version\":\"1.183.0\",\"results\":[],\"errors\":[],\"paths\":{\"scanned\":[],\"skipped\":[]}}"),
	}
	a := &semgrepAdapter{
		id:        "semgrep",
		category:  CategorySAST,
		command:   []string{"semgrep"},
		rules:     "rules",
		rulesPin:  pin,
		exec:      rec,
		timeout:   time.Minute,
		maxOutput: 1 << 20,
	}
	exec, err := a.Scan(context.Background(), Request{WorkspaceRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if exec.Provenance.RulesIdentity != pin {
		t.Fatalf("provenance rules identity = %s, want the staged digest %s", exec.Provenance.RulesIdentity, pin)
	}
	if rec.config == "" {
		t.Fatal("no --config argument was recorded")
	}
	if rec.config == rules {
		t.Fatal("semgrep read the writable rules path")
	}
	if string(rec.body) != pinned {
		t.Fatalf("semgrep read %q, want the pinned rules", rec.body)
	}
}

func TestGitleaksRulesPinStagesTheConfigItRuns(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "gitleaks.toml")
	pinned := "[rules]\n"
	writeFile(t, configPath, pinned)
	pin, err := rulesIdentity(configPath)
	if err != nil {
		t.Fatal(err)
	}

	rec := &configRecordingExecutor{
		swapTo:  configPath,
		swapTxt: "[rules]\nattacker = true\n",
	}
	a := &gitleaksAdapter{
		id:        "gitleaks",
		category:  CategorySecrets,
		command:   []string{"gitleaks"},
		rules:     "gitleaks.toml",
		rulesPin:  pin,
		exec:      rec,
		timeout:   time.Minute,
		maxOutput: 1 << 20,
	}
	exec, _ := a.Scan(context.Background(), Request{WorkspaceRoot: root})
	if exec.Provenance.RulesIdentity != pin {
		t.Fatalf("provenance rules identity = %s, want the staged digest %s", exec.Provenance.RulesIdentity, pin)
	}
	if rec.config == "" {
		t.Fatal("no --config argument was recorded")
	}
	if rec.config == configPath {
		t.Fatal("gitleaks read the writable config path")
	}
	if !rec.configIsFile {
		t.Fatalf("gitleaks --config %q is not the staged regular file", rec.config)
	}
	if string(rec.body) != pinned {
		t.Fatalf("gitleaks read %q, want the pinned config", rec.body)
	}
}

// The rules pin is checked inside the adapter against the exact path it hands
// the scanner (relative paths resolve against the workspace root, not the
// process cwd or the executor dir), and a symlink the digest would skip fails.
func TestRulesPin(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "rules", "r.yaml"), "rules: []\n")
	pin, _ := rulesIdentity(filepath.Join(root, "rules"))

	scan := func(rulesPin string) (Execution, int) {
		inner := &countingExecutor{}
		a := &semgrepAdapter{id: "semgrep", category: CategorySAST, rules: "rules", rulesPin: rulesPin, exec: inner}
		exec, _ := a.Scan(context.Background(), Request{WorkspaceRoot: root})
		return exec, inner.runs
	}
	pinFailed := func(e Execution) bool {
		return e.Status == StatusFailed && len(e.Diagnostics) > 0 && strings.Contains(e.Diagnostics[0], "rules pin")
	}

	if e, _ := scan(pin); pinFailed(e) {
		t.Fatalf("matching relative rules pin was refused: %+v", e.Diagnostics)
	}
	if e, n := scan(strings.Repeat("0", 64)); !pinFailed(e) || n != 0 {
		t.Fatalf("wrong pin: status=%s diag=%v runs=%d, want pin failure and no run", e.Status, e.Diagnostics, n)
	}
	writeFile(t, filepath.Join(root, "outside.yaml"), "rules: [x]\n")
	if err := os.Symlink(filepath.Join(root, "outside.yaml"), filepath.Join(root, "rules", "extra.yaml")); err != nil {
		t.Fatal(err)
	}
	if e, n := scan(pin); !pinFailed(e) || !strings.Contains(e.Diagnostics[0], "not a regular file") || n != 0 {
		t.Fatalf("symlinked rule: status=%s diag=%v runs=%d, want refusal and no run", e.Status, e.Diagnostics, n)
	}
}
