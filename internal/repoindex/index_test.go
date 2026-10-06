package repoindex

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture builds a small repository with two modules, a nested package, a test file, and
// documents spanning current / historical / unknown authority.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "main.go", `package main

import "fmt"

type Config struct{ Name string }

type Reader interface{ Read() error }

func (c Config) Label() string { return c.Name }

func main() { fmt.Println("hi") }
`)
	write(t, root, "sub/sub.go", "package sub\n\nfunc Do() int { return 1 }\n")
	write(t, root, "sub/sub_test.go", "package sub\n\nfunc TestDo(t *testing.T) {}\n")
	write(t, root, "nested/go.mod", "module example.com/nested\n\ngo 1.21\n")
	write(t, root, "nested/x.go", "package nested\n\nfunc X() {}\n")
	write(t, root, "docs/plans/ACTIVE.md", "# Active plan\n")
	write(t, root, "docs/plans/PLAN-Future.md", "# Future plan\n")
	write(t, root, "docs/history/plans/OLD.md", "# Old plan\n")
	write(t, root, "docs/history/plans/MARKED.md", "# Marked\n\n> **Document class:** plan · **Lifecycle:** superseded · **Authority:** historical.\n")
	write(t, root, "docs/specs/SPEC.md", "# Spec\n")
	write(t, root, "docs/README.md", "# Docs\n")
	write(t, root, "config.yaml", "project:\n  name: x\n")
	write(t, root, ".agent-sdlc/plan.meta.json", `{"source":"docs/plans/ACTIVE.md","plan_id":"active","source_kind":"plan"}`)
	return root
}

func build(t *testing.T, root string) Index {
	t.Helper()
	idx, err := Build(Options{Root: root, Head: "abc123", Dirty: false})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return idx
}

func docByPath(idx Index, path string) *Document {
	for i := range idx.Documents {
		if idx.Documents[i].Path == path {
			return &idx.Documents[i]
		}
	}
	return nil
}

func symByID(idx Index, id string) *Symbol {
	for i := range idx.Symbols {
		if idx.Symbols[i].ID == id {
			return &idx.Symbols[i]
		}
	}
	return nil
}

func TestBuildIndexesGoStructure(t *testing.T) {
	idx := build(t, fixture(t))
	if idx.Counts.Modules != 2 {
		t.Errorf("modules = %d, want 2", idx.Counts.Modules)
	}
	if idx.Counts.Packages != 3 {
		t.Errorf("packages = %d, want 3 (main, sub, nested)", idx.Counts.Packages)
	}
	for _, want := range []string{
		"struct:example.com/m.Config", "interface:example.com/m.Reader",
		"function:example.com/m.main", "method:example.com/m.Config.Label",
		"function:example.com/m/sub.Do", "function:example.com/m/sub.TestDo", "function:example.com/nested.X",
	} {
		if symByID(idx, want) == nil {
			t.Errorf("missing symbol %s", want)
		}
	}
	// Test classification.
	if s := symByID(idx, "function:example.com/m/sub.TestDo"); s == nil || !s.Test {
		t.Error("TestDo must be classified as a test symbol")
	}
	// Imports.
	found := false
	for _, imp := range idx.Imports {
		if imp.Path == "fmt" && imp.Package == "example.com/m" {
			found = true
		}
	}
	if !found {
		t.Error("missing import fmt for example.com/m")
	}
}

func TestBuildDocumentAuthority(t *testing.T) {
	idx := build(t, fixture(t))
	cases := []struct {
		path      string
		class     DocumentClass
		authority Authority
		lifecycle Lifecycle
		source    string
	}{
		{"docs/plans/ACTIVE.md", ClassPlan, AuthorityCurrent, LifecycleActive, "sop"},
		{"docs/plans/PLAN-Future.md", ClassPlan, AuthorityUnknown, LifecycleUnknown, "unknown"},
		{"docs/history/plans/OLD.md", ClassPlan, AuthorityHistorical, LifecycleUnknown, "path"},
		{"docs/history/plans/MARKED.md", ClassPlan, AuthorityHistorical, LifecycleSuperseded, "header"},
		{"docs/specs/SPEC.md", ClassSpec, AuthorityUnknown, LifecycleUnknown, "unknown"},
		{"docs/README.md", ClassDocumentation, AuthorityUnknown, LifecycleUnknown, "unknown"},
		{"config.yaml", ClassConfig, AuthorityUnknown, LifecycleUnknown, "unknown"},
	}
	for _, tc := range cases {
		d := docByPath(idx, tc.path)
		if d == nil {
			t.Errorf("missing document %s", tc.path)
			continue
		}
		if d.Class != tc.class || d.Authority != tc.authority || d.Lifecycle != tc.lifecycle || d.AuthoritySource != tc.source {
			t.Errorf("%s = %+v, want class=%s authority=%s lifecycle=%s source=%s", tc.path, *d, tc.class, tc.authority, tc.lifecycle, tc.source)
		}
	}
	if idx.Counts.Current != 1 {
		t.Errorf("current = %d, want 1", idx.Counts.Current)
	}
	if idx.Counts.Historical != 2 {
		t.Errorf("historical = %d, want 2", idx.Counts.Historical)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	root := fixture(t)
	a := build(t, root)
	b := build(t, root)
	da, err := a.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	db, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(da, db) {
		t.Error("indexing the same repository twice must produce byte-identical output")
	}
	if a.Identity.Digest == "" || a.Identity != b.Identity {
		t.Errorf("identity = %+v, want a stable non-empty digest", a.Identity)
	}
}

func TestBuildOrderingIsSorted(t *testing.T) {
	idx := build(t, fixture(t))
	if !sortedBy(len(idx.Files), func(i, j int) bool { return idx.Files[i].Path < idx.Files[j].Path }) {
		t.Error("files must be sorted by path")
	}
	if !sortedBy(len(idx.Symbols), func(i, j int) bool { return idx.Symbols[i].ID < idx.Symbols[j].ID }) {
		t.Error("symbols must be sorted by id")
	}
	if !sortedBy(len(idx.Documents), func(i, j int) bool { return idx.Documents[i].Path < idx.Documents[j].Path }) {
		t.Error("documents must be sorted by path")
	}
}

func sortedBy(n int, less func(i, j int) bool) bool {
	for i := 1; i < n; i++ {
		if less(i, i-1) {
			return false
		}
	}
	return true
}

func TestBuildFailsWithoutRoot(t *testing.T) {
	if _, err := Build(Options{}); err == nil {
		t.Fatal("an empty root must be rejected")
	}
}

// TestBuildControllerFixture proves SOP lifecycle authority beats filesystem convention on a
// repository following the controller layout: the ACTIVE plan lives at
// docs/PLAN-SOP-Controller.md (not under docs/plans/) yet must classify current/active via SOP
// metadata, and every plan under docs/history/plans/ must be historical. The fixture is
// self-contained in a temporary directory, so the test neither reads nor mutates any live
// checkout and does not depend on another repository's current lifecycle state.
func TestBuildControllerFixture(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/controller\n\ngo 1.21\n")
	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	write(t, root, "docs/PLAN-SOP-Controller.md", "# Controller plan\n")
	write(t, root, "docs/plans/PLAN-Other.md", "# Other plan\n")
	write(t, root, "docs/history/plans/PLAN-Retired.md", "# Retired plan\n")
	write(t, root, ".agent-sdlc/plan.meta.json", `{"source":"docs/PLAN-SOP-Controller.md","plan_id":"controller","source_kind":"plan"}`)

	idx := build(t, root)

	d := docByPath(idx, "docs/PLAN-SOP-Controller.md")
	if d == nil {
		t.Fatal("the controller plan was not indexed")
	}
	if d.Authority != AuthorityCurrent || d.Lifecycle != LifecycleActive || d.AuthoritySource != "sop" {
		t.Errorf("docs/PLAN-SOP-Controller.md = %+v, want current/active via sop", *d)
	}
	for _, doc := range idx.Documents {
		if strings.HasPrefix(doc.Path, "docs/history/plans/") {
			if doc.Authority != AuthorityHistorical {
				t.Errorf("%s = %s, want historical", doc.Path, doc.Authority)
			}
		}
	}
}
