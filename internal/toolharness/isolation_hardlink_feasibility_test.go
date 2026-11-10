//go:build linux

package toolharness

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/sensitivepath"
)

// Feasibility probes for a possible subprocess-hardlink mitigation. They do not
// change production isolation. They measure whether preflight discovery or
// path-masking can reliably cover pre-existing ordinary-name hardlinks inside
// the sandbox's visible filesystem.

func identityKey(info fs.FileInfo) (string, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), true
}

// scanProtectedIdentitiesForProbe maps accessible credential inodes to their
// canonical relative paths. This is the bounded discovery half of Option C,
// not a production registry.
func scanProtectedIdentitiesForProbe(root string) (map[string]string, error) {
	protected := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !sensitivepath.IsToolReadDenied(rel) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if key, ok := identityKey(info); ok {
			protected[key] = rel
		}
		return nil
	})
	return protected, err
}

// scanAliasesForProbe reports visible regular files whose identity matches a
// protected credential but whose path is not the protected path.
func scanAliasesForProbe(root string, protected map[string]string) (map[string]string, error) {
	aliases := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		key, ok := identityKey(info)
		if !ok {
			return nil
		}
		original, protectedInode := protected[key]
		if !protectedInode {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != original {
			aliases[rel] = original
		}
		return nil
	})
	return aliases, err
}

// visibleTempBase creates a temporary directory outside /tmp so the sandbox's
// private /tmp does not hide it. The parent of the repository is used because
// it is on the host filesystem and read-only-visible inside the sandbox.
func visibleTempBase(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("cannot determine working directory: %v", err)
	}
	base, err := os.MkdirTemp(filepath.Join(wd, "..", "..", ".."), ".sop-hardlink-probe-")
	if err != nil {
		t.Skipf("cannot create a visible temporary base: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	return base
}

func TestSubprocessHardlinkOutsideWorkspaceVisible(t *testing.T) {
	needSandbox(t)
	base := visibleTempBase(t)
	home := filepath.Join(base, "home")
	other := filepath.Join(base, "other")
	writeRepoFile(t, home, ".ssh/probe", "subprocess-hardlink-canary-91")
	writeRepoFile(t, other, "README", "ordinary\n")
	alias := filepath.Join(other, "alias.txt")
	if err := os.Link(filepath.Join(home, ".ssh", "probe"), alias); err != nil {
		t.Skipf("hardlink is not supported on this filesystem: %v", err)
	}
	t.Setenv("HOME", home)

	root := t.TempDir()
	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(alias))
	if err == nil && strings.Contains(out, "subprocess-hardlink-canary-91") {
		t.Logf("observed outside-workspace alias exposure: the sandbox can read a host path outside the command root")
		return
	}
	t.Fatalf("the outside-workspace alias is no longer readable; update the feasibility evidence (err=%v out=%q)", err, out)
}

func TestSubprocessHardlinkSnapshotRace(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", "subprocess-hardlink-canary-92")
	t.Setenv("HOME", home)

	// A preflight snapshot before the alias exists sees only the protected path.
	protected, err := scanProtectedIdentitiesForProbe(root)
	if err != nil {
		t.Fatal(err)
	}
	before, err := scanAliasesForProbe(root, protected)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("fixture unexpectedly contains aliases before creation: %v", before)
	}

	alias := filepath.Join(home, "late-alias.txt")
	if err := os.Link(filepath.Join(home, ".ssh", "probe"), alias); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	after, err := scanAliasesForProbe(root, protected)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("a post-creation scan should find the late alias, got %v", after)
	}

	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(alias))
	if err == nil && strings.Contains(out, "subprocess-hardlink-canary-92") {
		t.Logf("observed snapshot race: an alias created after the preflight snapshot is readable")
		return
	}
	t.Fatalf("the late alias is no longer readable; update the feasibility evidence (err=%v out=%q)", err, out)
}

func TestSubprocessHardlinkCredentialReplacement(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	credential := filepath.Join(home, ".ssh", "probe")
	writeRepoFile(t, home, ".ssh/probe", "subprocess-hardlink-canary-old-93")
	// Keep the old inode alive so the replacement must use a different inode.
	if err := os.Link(credential, filepath.Join(home, "old-link.txt")); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	protected, err := scanProtectedIdentitiesForProbe(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(credential); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, home, ".ssh/probe", "subprocess-hardlink-canary-new-93")
	alias := filepath.Join(home, "replacement-alias.txt")
	if err := os.Link(credential, alias); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	t.Setenv("HOME", home)

	aliases, err := scanAliasesForProbe(root, protected)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := aliases["home/replacement-alias.txt"]; found {
		t.Fatalf("the snapshot unexpectedly matched the replacement inode: %v", aliases)
	}
	out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(alias))
	if err == nil && strings.Contains(out, "subprocess-hardlink-canary-new-93") {
		t.Logf("observed replacement gap: a new inode behind the same credential path is readable")
		return
	}
	t.Fatalf("the replacement alias is no longer readable; update the feasibility evidence (err=%v out=%q)", err, out)
}

func TestSubprocessAliasMaskingEnumerationBound(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	home := fakeHomeUnder(t, root)
	writeRepoFile(t, home, ".ssh/probe", "subprocess-hardlink-canary-94")
	aliasOne := filepath.Join(home, "alias-one.txt")
	aliasTwo := filepath.Join(home, "alias-two.txt")
	if err := os.Link(filepath.Join(home, ".ssh", "probe"), aliasOne); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	if err := os.Link(filepath.Join(home, ".ssh", "probe"), aliasTwo); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	t.Setenv("HOME", home)

	deny, err := homeCredentials()
	if err != nil {
		t.Fatal(err)
	}
	// Mask only the first alias, as a discovery-based implementation would.
	deny = append(deny, credentialPath{path: aliasOne, dir: false})

	out, err := sandboxedWithDeny(t, root, deny, "cat "+strconv.Quote(aliasOne))
	if strings.Contains(out, "subprocess-hardlink-canary-94") {
		t.Fatalf("masking the first alias still exposed its content: %q", out)
	}
	out, err = sandboxedWithDeny(t, root, deny, "cat "+strconv.Quote(aliasTwo))
	if err == nil && strings.Contains(out, "subprocess-hardlink-canary-94") {
		t.Logf("observed enumeration bound: masking one known alias leaves an unmasked alias readable")
		return
	}
	t.Fatalf("the second alias is no longer readable; update the feasibility evidence (err=%v out=%q)", err, out)
}

func sandboxedWithDeny(t *testing.T, root string, deny []credentialPath, script string) (string, error) {
	t.Helper()
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skipf("bwrap is not installed: %v", err)
	}
	cache, err := os.MkdirTemp("", "sop-hardlink-cache-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(cache) }()
	env, _ := sandboxEnv(cache)
	args := bubblewrapArgs(root, writableDirs(root), deny)
	cmd := exec.Command(bwrap, append(args, "sh", "-c", script)...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestSubprocessOrdinaryMultilinkPositive(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	writeRepoFile(t, root, "a.txt", "ordinary-multilink-marker")
	if err := os.Link(filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		t.Logf("ordinary multi-linked file Nlink=%d", st.Nlink)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		out, err := sandboxedOutput(t, root, "cat "+strconv.Quote(filepath.Join(root, name)))
		if err != nil || !strings.Contains(out, "ordinary-multilink-marker") {
			t.Fatalf("ordinary multi-linked file %s was not readable: out=%q err=%v", name, out, err)
		}
	}
}

func TestSubprocessHardlinkScanCost(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	for i := 0; i < 100; i++ {
		writeRepoFile(t, home, fmt.Sprintf(".ssh/cred-%03d", i), "credential\n")
	}
	for i := 0; i < 5000; i++ {
		writeRepoFile(t, root, fmt.Sprintf("tree/%02d/file-%04d.txt", i%50, i), "ordinary\n")
	}
	if err := os.Link(filepath.Join(home, ".ssh", "cred-000"), filepath.Join(root, "alias.txt")); err != nil {
		t.Skipf("hardlink is not supported here: %v", err)
	}

	start := time.Now()
	protected, err := scanProtectedIdentitiesForProbe(root)
	if err != nil {
		t.Fatal(err)
	}
	build := time.Since(start)
	start = time.Now()
	aliases, err := scanAliasesForProbe(root, protected)
	scan := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(protected) != 100 || len(aliases) != 1 {
		t.Fatalf("protected=%d aliases=%d, want 100 and 1", len(protected), len(aliases))
	}
	t.Logf("protected=%d aliases=%d build=%s alias-scan=%s", len(protected), len(aliases), build, scan)
	if build+scan > 5*time.Second {
		t.Fatalf("bounded scan took %s, want <= 5s", build+scan)
	}
}
