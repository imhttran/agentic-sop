package security

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func cargoTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cargoTestPackage(t *testing.T, root, dir, name string) {
	t.Helper()
	cargoTestFile(t, root, dir+"/Cargo.toml", "[package]\nname = \""+name+"\"\nversion = \"0.1.0\"\n")
}

func cargoTestLock(t *testing.T, root, dir string) {
	t.Helper()
	rel := "Cargo.lock"
	if dir != "." {
		rel = dir + "/Cargo.lock"
	}
	cargoTestFile(t, root, rel, "version = 3\n")
}

func workspaceFor(t *testing.T, root string) CargoWorkspace {
	t.Helper()
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Workspaces) != 1 {
		t.Fatalf("workspaces = %+v, want 1", detection.Workspaces)
	}
	return detection.Workspaces[0]
}

func TestCargoWorkspaceQuotedMembersAreEquivalent(t *testing.T) {
	for _, quote := range []string{`"`, `'`} {
		t.Run("quote"+quote, func(t *testing.T) {
			root := t.TempDir()
			cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = ["+quote+"crates/app"+quote+"]\n")
			cargoTestPackage(t, root, "crates/app", "app")
			cargoTestLock(t, root, ".")
			workspace := workspaceFor(t, root)
			if !sameStrings(workspace.Members, []string{"crates/app"}) {
				t.Fatalf("members = %v, want [crates/app]", workspace.Members)
			}
			if !sameStrings(workspace.Resolved, []string{"crates/app"}) {
				t.Fatalf("resolved = %v, want [crates/app]", workspace.Resolved)
			}
		})
	}
}

func TestCargoVirtualWorkspaceResolvesRootLockfile(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/app\", \"crates/lib\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestPackage(t, root, "crates/lib", "lib")
	cargoTestLock(t, root, ".")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	lockfiles := detection.PresentLockfiles()
	if len(lockfiles) != 1 || lockfiles[0].Path != "Cargo.lock" {
		t.Fatalf("lockfiles = %+v, want only the workspace root Cargo.lock", lockfiles)
	}
	if len(lockfiles[0].Manifests) != 2 {
		t.Fatalf("lockfile manifests = %v, want both members", lockfiles[0].Manifests)
	}
}

func TestCargoOrdinaryWorkspaceIncludesRootPackage(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[package]\nname = \"root\"\nversion = \"0.1.0\"\n\n[workspace]\nmembers = [\"crates/app\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestLock(t, root, ".")
	workspace := workspaceFor(t, root)
	if !sameStrings(workspace.Resolved, []string{".", "crates/app"}) {
		t.Fatalf("resolved = %v, want root package and member", workspace.Resolved)
	}
}

func TestCargoImplicitWorkspaceMembers(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[package]\nname = \"root\"\nversion = \"0.1.0\"\n\n[workspace]\n\n[dependencies]\napp = { path = \"crates/app\" }\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestLock(t, root, ".")
	workspace := workspaceFor(t, root)
	if !sameStrings(workspace.Resolved, []string{".", "crates/app"}) {
		t.Fatalf("resolved = %v, want root and path-dependency member", workspace.Resolved)
	}
}

func TestCargoExplicitWorkspaceReferenceWithMembership(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/app\"]\n")
	cargoTestFile(t, root, "crates/app/Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\nworkspace = \"../..\"\n")
	cargoTestLock(t, root, ".")
	workspace := workspaceFor(t, root)
	if !sameStrings(workspace.Resolved, []string{"crates/app"}) {
		t.Fatalf("resolved = %v, want the listed workspace member", workspace.Resolved)
	}
	if len(workspace.Issues) != 0 {
		t.Fatalf("issues = %v, want no invalid-layout issue", workspace.Issues)
	}
}

func TestCargoPackageWorkspaceAloneIsNotMembership(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = []\n")
	cargoTestFile(t, root, "crates/app/Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\nworkspace = \"../..\"\n")
	cargoTestLock(t, root, ".")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Workspaces) != 1 || len(detection.Workspaces[0].Resolved) != 0 {
		t.Fatalf("workspaces = %+v, want an empty workspace", detection.Workspaces)
	}
	if !strings.Contains(strings.Join(detection.Issues, ";"), "does not match workspace membership") {
		t.Fatalf("issues = %v, want the unlisted package.workspace issue", detection.Issues)
	}
}

func TestCargoOutsideWorkspaceMemberWithPackageReference(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "ws/Cargo.toml", "[workspace]\nmembers = [\"../pkg\"]\n")
	cargoTestFile(t, root, "pkg/Cargo.toml", "[package]\nname = \"pkg\"\nversion = \"0.1.0\"\nworkspace = \"../ws\"\n")
	cargoTestLock(t, root, "ws")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Workspaces) != 1 {
		t.Fatalf("workspaces = %+v, want one workspace", detection.Workspaces)
	}
	workspace := detection.Workspaces[0]
	if !sameStrings(workspace.Resolved, []string{"pkg"}) {
		t.Fatalf("resolved = %v, want the outside member", workspace.Resolved)
	}
	if len(workspace.Issues) != 0 {
		t.Fatalf("issues = %v, want no invalid-layout issue", workspace.Issues)
	}
}

func TestCargoStringDependenciesAreValid(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/app\"]\n")
	cargoTestFile(t, root, "crates/app/Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n\n[dependencies]\nserde = \"1.0\"\n")
	cargoTestLock(t, root, ".")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Issues) != 0 {
		t.Fatalf("issues = %v, want string dependencies to parse", detection.Issues)
	}
	if len(detection.Workspaces) != 1 || !sameStrings(detection.Workspaces[0].Resolved, []string{"crates/app"}) {
		t.Fatalf("workspaces = %+v, want crates/app", detection.Workspaces)
	}
}

func TestCargoWorkspaceDependencyPathIsImplicitMember(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"app\"]\n\n[workspace.dependencies]\nhelper = { path = \"helper\" }\n")
	cargoTestFile(t, root, "app/Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n\n[dependencies]\nhelper = { workspace = true }\n")
	cargoTestPackage(t, root, "helper", "helper")
	cargoTestLock(t, root, ".")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Workspaces) != 1 || !sameStrings(detection.Workspaces[0].Resolved, []string{"app", "helper"}) {
		t.Fatalf("workspaces = %+v, want app and helper", detection.Workspaces)
	}
	if len(detection.Obligations) != 1 || detection.Obligations[0].Input != "Cargo.lock" {
		t.Fatalf("obligations = %+v, want only the workspace root lockfile", detection.Obligations)
	}
}

func TestCargoUnusedWorkspaceDependencyPathIsNotMember(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"app\"]\n\n[workspace.dependencies]\nunused = { path = \"unused\" }\n")
	cargoTestPackage(t, root, "app", "app")
	cargoTestPackage(t, root, "unused", "unused")
	cargoTestLock(t, root, ".")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Workspaces) != 1 || !sameStrings(detection.Workspaces[0].Resolved, []string{"app"}) {
		t.Fatalf("workspaces = %+v, want only app", detection.Workspaces)
	}
	missing := detection.MissingObligations(LanguageRust)
	if len(missing) != 1 || missing[0].Input != "unused/Cargo.lock" {
		t.Fatalf("missing obligations = %+v, want the independent unused package", missing)
	}
}

func TestCargoExcludedPathDependencyDoesNotPromoteDescendants(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"app\"]\nexclude = [\"helper\"]\n")
	cargoTestFile(t, root, "app/Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n\n[dependencies]\nhelper = { path = \"../helper\" }\n")
	cargoTestFile(t, root, "helper/Cargo.toml", "[package]\nname = \"helper\"\nversion = \"0.1.0\"\n\n[dependencies]\ninner = { path = \"../inner\" }\n")
	cargoTestPackage(t, root, "inner", "inner")
	cargoTestLock(t, root, ".")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Workspaces) != 1 || !sameStrings(detection.Workspaces[0].Resolved, []string{"app"}) {
		t.Fatalf("workspaces = %+v, want only app", detection.Workspaces)
	}
	missing := map[string]bool{}
	for _, obligation := range detection.MissingObligations(LanguageRust) {
		missing[obligation.Input] = true
	}
	if !missing["helper/Cargo.lock"] || !missing["inner/Cargo.lock"] {
		t.Fatalf("missing obligations = %+v, want independent helper and inner obligations", missing)
	}
}

func TestCargoRootPackageIsNotExcludedByDot(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[package]\nname = \"root\"\nversion = \"0.1.0\"\n\n[workspace]\nmembers = []\nexclude = [\".\"]\n")
	cargoTestLock(t, root, ".")
	workspace := workspaceFor(t, root)
	if !sameStrings(workspace.Resolved, []string{"."}) {
		t.Fatalf("resolved = %v, want the root package", workspace.Resolved)
	}
}

func TestCargoDotMemberIsTheRootPackage(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[package]\nname = \"root\"\nversion = \"0.1.0\"\n\n[workspace]\nmembers = [\".\"]\n")
	cargoTestLock(t, root, ".")
	workspace := workspaceFor(t, root)
	if !sameStrings(workspace.Resolved, []string{"."}) {
		t.Fatalf("resolved = %v, want the root package", workspace.Resolved)
	}
	if len(workspace.Issues) != 0 {
		t.Fatalf("issues = %v, want a valid root member", workspace.Issues)
	}
}

func TestCargoExcludeParentRemovesGlobMembers(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/*\"]\nexclude = [\"crates\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestLock(t, root, ".")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(detection.Workspaces) != 1 || len(detection.Workspaces[0].Resolved) != 0 {
		t.Fatalf("workspaces = %+v, want the glob member excluded by its parent", detection.Workspaces)
	}
	missing := detection.MissingObligations(LanguageRust)
	if len(missing) != 1 || missing[0].Input != "crates/app/Cargo.lock" {
		t.Fatalf("missing obligations = %+v, want the independent excluded package", missing)
	}
}

func TestCargoExplicitMemberWinsOverExclude(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/app\", \"crates/*\"]\nexclude = [\"crates\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestPackage(t, root, "crates/other", "other")
	cargoTestLock(t, root, ".")
	workspace := workspaceFor(t, root)
	if !sameStrings(workspace.Resolved, []string{"crates/app"}) {
		t.Fatalf("resolved = %v, want the explicit member only", workspace.Resolved)
	}
}

func TestCargoExcludeLiteralGlobDoesNotMatch(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/*\"]\nexclude = [\"crates/*\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestLock(t, root, ".")
	workspace := workspaceFor(t, root)
	if !sameStrings(workspace.Resolved, []string{"crates/app"}) {
		t.Fatalf("resolved = %v, want the glob member; exclude is not a glob", workspace.Resolved)
	}
}

func TestCargoExcludedPackageKeepsSeparateLockfile(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/*\"]\nexclude = [\"crates/independent\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestPackage(t, root, "crates/independent", "independent")
	cargoTestLock(t, root, ".")
	cargoTestLock(t, root, "crates/independent")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace := detection.Workspaces[0]
	if !sameStrings(workspace.Resolved, []string{"crates/app"}) {
		t.Fatalf("resolved = %v, want only the declared member", workspace.Resolved)
	}
	if len(detection.PresentLockfiles()) != 2 {
		t.Fatalf("lockfiles = %+v, want workspace and excluded package obligations", detection.PresentLockfiles())
	}
}

func TestCargoUndeclaredNestedPackageIsIndependent(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/app\"]\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestPackage(t, root, "crates/other", "other")
	cargoTestLock(t, root, ".")
	cargoTestLock(t, root, "crates/other")
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if !sameStrings(detection.Workspaces[0].Resolved, []string{"crates/app"}) {
		t.Fatalf("resolved = %v, want only the declared member", detection.Workspaces[0].Resolved)
	}
	if len(detection.PresentLockfiles()) != 2 {
		t.Fatalf("lockfiles = %+v, want an independent obligation for the undeclared package", detection.PresentLockfiles())
	}
}

func TestCargoInvalidAndUnsafeLayoutsReportIssues(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "invalid toml", body: "[workspace\nmembers = [\"crates/app\"]\n"},
		{name: "unsafe member", body: "[workspace]\nmembers = [\"../outside\"]\n"},
		{name: "unsafe exclude", body: "[workspace]\nmembers = [\"crates/*\"]\nexclude = [\"../outside\"]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cargoTestFile(t, root, "Cargo.toml", tc.body)
			cargoTestPackage(t, root, "crates/app", "app")
			cargoTestLock(t, root, ".")
			detection, err := DetectRepository(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(detection.Issues) == 0 {
				t.Fatalf("detection issues = %v, want an explicit unsupported/ambiguous layout issue", detection.Issues)
			}
		})
	}
}

func TestCargoDiscoveryDoesNotModifyFiles(t *testing.T) {
	root := t.TempDir()
	cargoTestFile(t, root, "Cargo.toml", "[workspace]\nmembers = ['crates/app']\n")
	cargoTestPackage(t, root, "crates/app", "app")
	cargoTestLock(t, root, ".")
	before := directorySnapshot(t, root)
	if _, err := DetectRepository(root); err != nil {
		t.Fatal(err)
	}
	after := directorySnapshot(t, root)
	if !sameStrings(before, after) {
		t.Fatalf("discovery modified the repository:\nbefore=%v\nafter=%v", before, after)
	}
}

func directorySnapshot(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel)+":"+DigestBytes(data))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	return strings.Join(ac, "\x00") == strings.Join(bc, "\x00")
}
