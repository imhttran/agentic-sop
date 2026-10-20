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
// Proposed commands use only the existing validation schema (build/test/lint).
// Callers own configuration precedence: an existing configuration is never
// replaced by this proposal.
package projectdetect

import (
	"encoding/json"
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

	hasGo := files.has("go.mod") || files.has("go.work")
	hasRust := files.has("Cargo.toml")
	pkg := files.packageJSON()
	hasNode := pkg != nil
	hasNext := files.hasNextConfig() || (pkg != nil && pkg.hasDependency("next"))

	d := Detection{MakeTargets: targetList(targets)}
	d.Manifests = append(d.Manifests, files.paths...)
	sort.Strings(d.Manifests)

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
			mergeValidation(goValidation(), rustValidation(targets, hasRust)),
			webValidation(pkg, targets, hasNode),
		)
	case hasGo && hasRust:
		d.Validation = mergeValidation(goValidation(), rustValidation(targets, hasRust))
	case hasGo && hasNode:
		d.Validation = mergeValidation(goValidation(), webValidation(pkg, targets, hasNode))
	case hasRust && hasNode:
		d.Validation = mergeValidation(rustValidation(targets, hasRust), webValidation(pkg, targets, hasNode))
	case hasGo:
		d.Validation = goValidation()
	case hasRust:
		d.Validation = rustValidation(targets, hasRust)
	case hasNode:
		d.Validation = webValidation(pkg, targets, hasNode)
	}

	if d.Kind == KindUnknown {
		d.Warnings = append(d.Warnings, "no supported project manifest (go.mod, go.work, Cargo.toml, package.json) was detected; validation is not configured and SOP will fail closed before running an unrecognized command")
	}
	return d
}

// goValidation is the established Go command set. It is used only when a Go
// manifest was detected.
func goValidation() Validation {
	return Validation{
		Build: []string{"go build ./..."},
		Test:  []string{"go test ./..."},
		Lint:  []string{"go vet ./...", "gofmt -l ."},
	}
}

// rustValidation proposes Cargo commands and, when the repository declares one,
// its aggregate rust-check target.
func rustValidation(targets map[string]bool, detected bool) Validation {
	if !detected {
		return Validation{}
	}
	v := Validation{
		Build: []string{"cargo build --locked"},
		Test:  []string{"cargo test --locked"},
		Lint: []string{
			"cargo fmt --check",
			"cargo clippy --locked --all-targets -- -D warnings",
		},
	}
	if targets["rust-check"] {
		v.Build = append(v.Build, "make rust-check")
	}
	return dedupe(v)
}

// webValidation proposes the package manager's scripts and, when the repository
// declares them, its aggregate web targets. The package manager is selected from
// the lockfiles next to the detected package.json.
func webValidation(pkg *packageInfo, targets map[string]bool, detected bool) Validation {
	if !detected || pkg == nil {
		return Validation{}
	}
	pm := pkg.packageManager()
	var v Validation
	if targets["web-check"] {
		v.Build = append(v.Build, "make web-check")
	} else if pkg.hasScript("build") {
		v.Build = append(v.Build, pm+" run build")
	}
	if targets["web-e2e"] {
		v.Test = append(v.Test, "make web-e2e")
	} else if pkg.hasScript("e2e") {
		v.Test = append(v.Test, pm+" run e2e")
	} else if pkg.hasScript("test") {
		v.Test = append(v.Test, pm+" test")
	}
	if pkg.hasScript("lint") {
		v.Lint = append(v.Lint, pm+" run lint")
	}
	if !targets["web-check"] && pkg.hasScript("typecheck") {
		v.Lint = append(v.Lint, pm+" run typecheck")
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

// manifestFiles records the manifests found during the bounded walk.
type manifestFiles struct {
	paths       []string
	set         map[string]bool
	packages    []string
	nextConfigs []string
}

func (m manifestFiles) has(name string) bool { return m.set[name] }

func (m manifestFiles) hasNextConfig() bool { return len(m.nextConfigs) > 0 }

func (m manifestFiles) packageJSON() *packageInfo {
	if len(m.packages) == 0 {
		return nil
	}
	info, err := readPackageJSON(m.packages[0])
	if err != nil {
		return nil
	}
	return info
}

// maxManifestDepth bounds the structure walk. A manifest deeper than this is
// not treated as repository-level evidence.
const maxManifestDepth = 2

var skippedDirs = map[string]bool{
	".git": true, ".agent-sdlc": true, "node_modules": true, "vendor": true,
	"target": true, "dist": true, "build": true, ".next": true, "coverage": true,
}

func findManifests(root string) manifestFiles {
	res := manifestFiles{set: map[string]bool{}}
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
		switch entry.Name() {
		case "go.mod", "go.work", "Cargo.toml":
			res.set[entry.Name()] = true
			res.paths = append(res.paths, rel)
		case "package.json":
			res.packages = append(res.packages, path)
			res.paths = append(res.paths, rel)
		default:
			if strings.HasPrefix(entry.Name(), "next.config.") {
				res.nextConfigs = append(res.nextConfigs, path)
				res.paths = append(res.paths, rel)
			}
		}
		return nil
	})
	sort.Slice(res.packages, func(i, j int) bool {
		return strings.Count(res.packages[i], string(filepath.Separator)) <
			strings.Count(res.packages[j], string(filepath.Separator))
	})
	return res
}

type packageInfo struct {
	path    string
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

// readMakeTargets returns the Makefile path (empty when absent) and its declared
// targets. Only a target line is recognized; recipe lines are ignored.
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
