// Package projectdetect detects a repository's project types from its manifests,
// structure, and explicit Makefile targets, and proposes validation commands for
// the existing configuration schema. It never reads or writes SOP state and never
// assumes a language: an unrecognized repository yields an explicit unknown
// detection with no proposed commands rather than a Go default.
//
// Detection precedence is deliberate:
//
//  1. Repository manifests (go.mod/go.work, Cargo.toml, package.json) select the
//     languages. Next.js is selected by next.config.* or a "next" dependency.
//  2. The project structure is inspected only to find manifests in a conventional
//     subdirectory (for example web/package.json); hidden, vendor, dependency, and
//     build directories are never searched.
//  3. A Makefile is read for explicit aggregate targets (for example rust-check,
//     web-check, web-e2e). A target contributes a command only when its language
//     was actually detected, so a stale Makefile target cannot select an
//     unsupported toolchain.
//
// Commands for a nested manifest run from that manifest's directory. Root-level
// commands keep their established spelling (no `cd` prefix), and root-level
// Makefile aggregate targets are preserved.
//
// Proposed commands use only the existing validation schema (build/test/lint).
// Callers own configuration precedence: an existing configuration is never
// replaced by this proposal.
package projectdetect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Language is a detected implementation language.
type Language string

const (
	LanguageGo     Language = "go"
	LanguageRust   Language = "rust"
	LanguageNode   Language = "node"
	LanguageNextJS Language = "nextjs"
)

// Kind is the detected project shape.
type Kind string

const (
	KindGo      Kind = "go"
	KindRust    Kind = "rust"
	KindNode    Kind = "node"
	KindNextJS  Kind = "nextjs"
	KindMixed   Kind = "mixed"
	KindUnknown Kind = "unknown"
)

// Validation is the proposed command set. It mirrors the existing configuration
// schema exactly; no new keys are introduced.
type Validation struct {
	Build []string
	Test  []string
	Lint  []string
}

// Detection is the deterministic result of inspecting one repository root.
type Detection struct {
	Kind        Kind
	Languages   []Language
	Manifests   []string
	MakeTargets []string
	Validation  Validation
	// Warnings describe detection facts an operator should see. They never
	// authorize a command by themselves.
	Warnings []string
	// Conflicts describe unresolved, mutually inconsistent evidence (for example a
	// Makefile target for a language with no manifest). A conflicting aggregate
	// command is never proposed.
	Conflicts []string
}

// Empty reports whether no validation command was proposed.
func (d Detection) Empty() bool {
	return len(d.Validation.Build) == 0 && len(d.Validation.Test) == 0 && len(d.Validation.Lint) == 0
}

// LanguageText renders the detected languages for a human or a config comment.
func (d Detection) LanguageText() string {
	if len(d.Languages) == 0 {
		return "unknown"
	}
	parts := make([]string, len(d.Languages))
	for i, l := range d.Languages {
		parts[i] = string(l)
	}
	return strings.Join(parts, ", ")
}

// Detect inspects dir and returns a deterministic detection. A missing or
// unreadable directory is reported as unknown with a warning; it is never an
// error, so project bootstrap can still write a safe empty validation policy.
func Detect(dir string) Detection {
	root, err := filepath.Abs(dir)
	if err != nil {
		root = dir
	}

	files := findManifests(root)
	makefile, targets := readMakeTargets(root)

	goManifest, hasGo := selectManifest(files.gos)
	rustManifest, hasRust := selectManifest(files.rusts)
	pkg, packageWarnings := selectPackage(files)
	hasNode := pkg != nil
	hasNext := hasNode && (files.nextDirs[pkg.dir] || pkg.hasDependency("next"))

	d := Detection{MakeTargets: targetList(targets)}
	d.Manifests = append(d.Manifests, files.paths...)
	sort.Strings(d.Manifests)
	d.Warnings = append(d.Warnings, packageWarnings...)

	if hasGo {
		d.Languages = append(d.Languages, LanguageGo)
	}
	if hasRust {
		d.Languages = append(d.Languages, LanguageRust)
	}
	if hasNext {
		d.Languages = append(d.Languages, LanguageNextJS)
	} else if hasNode {
		d.Languages = append(d.Languages, LanguageNode)
	}

	stacks := 0
	if hasGo {
		stacks++
	}
	if hasRust {
		stacks++
	}
	if hasNode {
		stacks++
	}
	switch {
	case stacks == 0:
		d.Kind = KindUnknown
	case stacks > 1:
		d.Kind = KindMixed
	case hasGo:
		d.Kind = KindGo
	case hasRust:
		d.Kind = KindRust
	case hasNext:
		d.Kind = KindNextJS
	default:
		d.Kind = KindNode
	}

	d.detectConflicts(makefile, hasGo, hasRust, hasNode)

	switch {
	case hasGo && hasRust && hasNode:
		d.Validation = mergeValidation(
			mergeValidation(goValidation(goManifest.Dir), rustValidation(rustManifest.Dir, targets, hasRust)),
			webValidation(pkg, targets, hasNode),
		)
	case hasGo && hasRust:
		d.Validation = mergeValidation(goValidation(goManifest.Dir), rustValidation(rustManifest.Dir, targets, hasRust))
	case hasGo && hasNode:
		d.Validation = mergeValidation(goValidation(goManifest.Dir), webValidation(pkg, targets, hasNode))
	case hasRust && hasNode:
		d.Validation = mergeValidation(rustValidation(rustManifest.Dir, targets, hasRust), webValidation(pkg, targets, hasNode))
	case hasGo:
		d.Validation = goValidation(goManifest.Dir)
	case hasRust:
		d.Validation = rustValidation(rustManifest.Dir, targets, hasRust)
	case hasNode:
		d.Validation = webValidation(pkg, targets, hasNode)
	}

	if d.Kind == KindUnknown {
		d.Warnings = append(d.Warnings, "no supported project manifest (go.mod, go.work, Cargo.toml, package.json) was detected; validation is not configured and SOP will fail closed before running an unrecognized command")
	}
	return d
}

// goValidation is the established Go command set. A nested manifest keeps the
// same checks but runs them from its own directory.
func goValidation(dir string) Validation {
	prefix := dirPrefix(dir)
	return Validation{
		Build: []string{prefix + "go build ./..."},
		Test:  []string{prefix + "go test ./..."},
		Lint:  []string{prefix + "go vet ./...", prefix + "gofmt -l ."},
	}
}

// rustValidation proposes Cargo commands and, when the repository declares one,
// its root-level aggregate rust-check target.
func rustValidation(dir string, targets map[string]bool, detected bool) Validation {
	if !detected {
		return Validation{}
	}
	prefix := dirPrefix(dir)
	v := Validation{
		Build: []string{prefix + "cargo build --locked"},
		Test:  []string{prefix + "cargo test --locked"},
		Lint: []string{
			prefix + "cargo fmt --check",
			prefix + "cargo clippy --locked --all-targets -- -D warnings",
		},
	}
	if targets["rust-check"] {
		v.Build = append(v.Build, "make rust-check")
	}
	return dedupe(v)
}

// webValidation proposes the package manager's scripts and, when the repository
// declares them, its root-level aggregate web targets. The selected package is
// the one that owns the detected web app; a nested package runs its scripts from
// its own directory.
func webValidation(pkg *packageInfo, targets map[string]bool, detected bool) Validation {
	if !detected || pkg == nil {
		return Validation{}
	}
	pm := pkg.packageManager()
	prefix := dirPrefix(pkg.dir)
	var v Validation
	if targets["web-check"] {
		v.Build = append(v.Build, "make web-check")
	} else if pkg.hasScript("build") {
		v.Build = append(v.Build, prefix+pm+" run build")
	}
	if targets["web-e2e"] {
		v.Test = append(v.Test, "make web-e2e")
	} else if pkg.hasScript("e2e") {
		v.Test = append(v.Test, prefix+pm+" run e2e")
	} else if pkg.hasScript("test") {
		v.Test = append(v.Test, prefix+pm+" test")
	}
	if pkg.hasScript("lint") {
		v.Lint = append(v.Lint, prefix+pm+" run lint")
	}
	if !targets["web-check"] && pkg.hasScript("typecheck") {
		v.Lint = append(v.Lint, prefix+pm+" run typecheck")
	}
	return dedupe(v)
}

// mergeValidation joins two proposals in stable order and removes exact
// duplicates, so a mixed repository runs each command at most once.
func mergeValidation(a, b Validation) Validation {
	return dedupe(Validation{
		Build: append(append([]string{}, a.Build...), b.Build...),
		Test:  append(append([]string{}, a.Test...), b.Test...),
		Lint:  append(append([]string{}, a.Lint...), b.Lint...),
	})
}

// dedupe removes exact duplicate commands while preserving order.
func dedupe(v Validation) Validation {
	return Validation{Build: dedupeList(v.Build), Test: dedupeList(v.Test), Lint: dedupeList(v.Lint)}
}

func dedupeList(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// dirPrefix renders the shell prefix that runs a command from a nested manifest
// directory. Root-level commands are unchanged (empty prefix).
func dirPrefix(dir string) string {
	dir = filepath.ToSlash(dir)
	if dir == "" || dir == "." || dir == "./" {
		return ""
	}
	return "cd " + shellQuote(dir) + " && "
}

// shellQuote quotes a relative directory for a POSIX shell. It never evaluates
// the path: the value is repository evidence, not caller input.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (d *Detection) detectConflicts(makefile string, hasGo, hasRust, hasNode bool) {
	if !hasRust && containsTarget(d.MakeTargets, "rust-check") {
		d.Conflicts = append(d.Conflicts, "Makefile declares rust-check but no Cargo.toml was detected; the rust-check target is not proposed")
	}
	if !hasNode && (containsTarget(d.MakeTargets, "web-check") || containsTarget(d.MakeTargets, "web-e2e")) {
		d.Conflicts = append(d.Conflicts, "Makefile declares web-check/web-e2e but no package.json was detected; the web targets are not proposed")
	}
	if !hasGo && containsTarget(d.MakeTargets, "go-check") {
		d.Conflicts = append(d.Conflicts, "Makefile declares go-check but no go.mod was detected; the go-check target is not proposed")
	}
}

// manifest is one discovered manifest and the directory that owns it.
type manifest struct {
	// Path is the repository-relative, slash-separated manifest path.
	Path string
	// Dir is the repository-relative, slash-separated directory; "." is root.
	Dir string
	// Abs is the absolute manifest path.
	Abs string
}

// manifestFiles records the manifests found during the bounded walk.
type manifestFiles struct {
	paths    []string
	set      map[string]bool
	gos      []manifest
	rusts    []manifest
	packages []manifest
	nextDirs map[string]bool
}

func (m manifestFiles) has(name string) bool { return m.set[name] }

// selectManifest picks the manifest that owns a language. A root manifest always
// wins; otherwise the shallowest nested manifest is used, with a deterministic
// lexicographic tie-break.
func selectManifest(manifests []manifest) (manifest, bool) {
	if len(manifests) == 0 {
		return manifest{}, false
	}
	for _, m := range manifests {
		if m.Dir == "." {
			return m, true
		}
	}
	return manifests[0], true
}

// selectPackage picks the package.json that owns the web application.
//
// A Next.js package wins over an unrelated root package (for example a root
// tooling package.json next to web/package.json). Without Next.js, a root
// package wins; otherwise the shallowest nested package is used. The choice is
// deterministic and never falls through to a package that does not belong to the
// detected app.
func selectPackage(files manifestFiles) (*packageInfo, []string) {
	if len(files.packages) == 0 {
		return nil, nil
	}
	var warnings []string
	var packages []*packageInfo
	for _, m := range files.packages {
		info, err := readPackageJSON(m.Abs)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("package manifest %s could not be read and was ignored: %v", m.Path, err))
			continue
		}
		info.dir = m.Dir
		packages = append(packages, info)
	}
	if len(packages) == 0 {
		return nil, warnings
	}

	var web []*packageInfo
	for _, pkg := range packages {
		if files.nextDirs[pkg.dir] || pkg.hasDependency("next") {
			web = append(web, pkg)
		}
	}
	if len(web) > 0 {
		chosen := web[0]
		if len(web) > 1 {
			warnings = append(warnings, fmt.Sprintf("multiple Next.js package manifests were detected; using %s", chosen.path))
		}
		if chosen.dir != "" && chosen.dir != "." {
			for _, pkg := range packages {
				if pkg.dir == "." {
					warnings = append(warnings, fmt.Sprintf("using nested web package %s instead of an unrelated root package.json", chosen.path))
					break
				}
			}
		}
		return chosen, warnings
	}

	for _, pkg := range packages {
		if pkg.dir == "." {
			return pkg, warnings
		}
	}
	return packages[0], warnings
}

// maxManifestDepth bounds the structure walk. A manifest deeper than this is
// not treated as repository-level evidence.
const maxManifestDepth = 2

// skippedDirs are never searched: dependency and build output, plus auxiliary
// folders (tests, fixtures, examples, tooling, docs) whose manifests describe a
// side module, not the project, and must not make a repository "mixed".
var skippedDirs = map[string]bool{
	".git": true, ".agent-sdlc": true, "node_modules": true, "vendor": true,
	"target": true, "dist": true, "build": true, ".next": true, "coverage": true,
	"test": true, "tests": true, "testdata": true, "fixtures": true,
	"example": true, "examples": true, "tools": true, "scripts": true,
	"docs": true, "third_party": true,
}

func findManifests(root string) manifestFiles {
	res := manifestFiles{set: map[string]bool{}, nextDirs: map[string]bool{}}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if skippedDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			if strings.Count(rel, "/") >= maxManifestDepth {
				return filepath.SkipDir
			}
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		switch entry.Name() {
		case "go.mod", "go.work", "Cargo.toml":
			res.set[entry.Name()] = true
			res.paths = append(res.paths, rel)
			m := manifest{Path: rel, Dir: dir, Abs: path}
			if entry.Name() == "Cargo.toml" {
				res.rusts = append(res.rusts, m)
			} else {
				res.gos = append(res.gos, m)
			}
		case "package.json":
			res.paths = append(res.paths, rel)
			res.packages = append(res.packages, manifest{Path: rel, Dir: dir, Abs: path})
		default:
			if strings.HasPrefix(entry.Name(), "next.config.") {
				res.paths = append(res.paths, rel)
				res.nextDirs[dir] = true
			}
		}
		return nil
	})
	sortManifests(res.gos)
	sortManifests(res.rusts)
	sortManifests(res.packages)
	return res
}

func sortManifests(manifests []manifest) {
	sort.SliceStable(manifests, func(i, j int) bool {
		di, dj := manifestDepth(manifests[i]), manifestDepth(manifests[j])
		if di != dj {
			return di < dj
		}
		return manifests[i].Path < manifests[j].Path
	})
}

func manifestDepth(m manifest) int {
	if m.Dir == "." {
		return 0
	}
	return strings.Count(m.Dir, "/") + 1
}

type packageInfo struct {
	path    string
	dir     string
	Scripts map[string]string `json:"scripts"`
	Deps    map[string]string `json:"dependencies"`
	DevDeps map[string]string `json:"devDependencies"`
}

func readPackageJSON(path string) (*packageInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info packageInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	info.path = path
	return &info, nil
}

func (p *packageInfo) hasDependency(name string) bool {
	if _, ok := p.Deps[name]; ok {
		return true
	}
	_, ok := p.DevDeps[name]
	return ok
}

func (p *packageInfo) hasScript(name string) bool {
	_, ok := p.Scripts[name]
	return ok
}

// packageManager selects a package manager from the lockfile next to the
// detected package.json, with a deterministic priority when several are present.
func (p *packageInfo) packageManager() string {
	dir := filepath.Dir(p.path)
	switch {
	case exists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case exists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	case exists(filepath.Join(dir, "bun.lockb")), exists(filepath.Join(dir, "bun.lock")):
		return "bun"
	default:
		return "npm"
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var makeTargetPattern = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*)\s*:`)

// readMakeTargets returns the root Makefile path (empty when absent) and its
// declared targets. Only a target line is recognized; recipe lines are ignored.
func readMakeTargets(root string) (string, map[string]bool) {
	for _, name := range []string{"GNUmakefile", "Makefile", "makefile"} {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		targets := map[string]bool{}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "\t") {
				continue
			}
			if m := makeTargetPattern.FindStringSubmatch(line); m != nil {
				targets[m[1]] = true
			}
		}
		return path, targets
	}
	return "", nil
}

func targetList(targets map[string]bool) []string {
	out := make([]string, 0, len(targets))
	for name := range targets {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func containsTarget(targets []string, name string) bool {
	for _, t := range targets {
		if t == name {
			return true
		}
	}
	return false
}
