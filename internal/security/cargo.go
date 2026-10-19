package security

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// cargoManifestRecord is one parsed Cargo.toml observed by detection. Detection
// reads only repository content; it never invokes Cargo or resolves dependencies.
type cargoManifestRecord struct {
	Path     string
	Dir      string
	Manifest cargoManifest
	ParseErr error
}

type cargoManifest struct {
	Package           *cargoPackage          `toml:"package"`
	Workspace         *cargoWorkspaceTable   `toml:"workspace"`
	Dependencies      map[string]any         `toml:"dependencies"`
	DevDependencies   map[string]any         `toml:"dev-dependencies"`
	BuildDependencies map[string]any         `toml:"build-dependencies"`
	Target            map[string]cargoTarget `toml:"target"`
}

type cargoPackage struct {
	Name      string `toml:"name"`
	Workspace string `toml:"workspace"`
}

type cargoWorkspaceTable struct {
	Members        []string       `toml:"members"`
	Exclude        []string       `toml:"exclude"`
	DefaultMembers []string       `toml:"default-members"`
	Dependencies   map[string]any `toml:"dependencies"`
}

type cargoTarget struct {
	Dependencies      map[string]any `toml:"dependencies"`
	DevDependencies   map[string]any `toml:"dev-dependencies"`
	BuildDependencies map[string]any `toml:"build-dependencies"`
}

// cargoDependencyPath returns a dependency's explicit path. A dependency may be
// a version string or a table; only the table form can carry a path.
func cargoDependencyPath(value any) string {
	dep, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	path, _ := dep["path"].(string)
	return strings.TrimSpace(path)
}

// cargoDependencyInheritsWorkspace reports whether a dependency table uses
// `workspace = true`. Such a dependency takes its path, if any, from the
// workspace's [workspace.dependencies] table.
func cargoDependencyInheritsWorkspace(value any) bool {
	dep, ok := value.(map[string]any)
	if !ok {
		return false
	}
	inherited, _ := dep["workspace"].(bool)
	return inherited
}

func parseCargoManifest(data []byte) (cargoManifest, error) {
	var manifest cargoManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return cargoManifest{}, err
	}
	return manifest, nil
}

// resolveCargoWorkspaces builds workspace membership and Rust lockfile
// obligations from parsed Cargo.toml records. It is deterministic, read-only, and
// confined to repository-relative paths.
func resolveCargoWorkspaces(root string, records []cargoManifestRecord, dirs []string) ([]CargoWorkspace, []Lockfile, []string) {
	byPath := map[string]cargoManifestRecord{}
	byDir := map[string]cargoManifestRecord{}
	var issues []string
	for _, record := range records {
		if record.ParseErr != nil {
			issues = append(issues, fmt.Sprintf("%s is not valid TOML: %s", record.Path, SanitizeText(record.ParseErr.Error(), MaxDiagnosticBytes)))
			continue
		}
		byPath[record.Path] = record
		byDir[record.Dir] = record
	}

	var workspaces []CargoWorkspace
	owners := map[string]string{} // manifest path -> workspace manifest path

	for _, record := range records {
		if record.ParseErr != nil || record.Manifest.Workspace == nil {
			continue
		}
		workspace, members, workspaceIssues := cargoWorkspaceMembership(record, records, byDir, dirs)
		workspaces = append(workspaces, workspace)
		issues = append(issues, workspaceIssues...)
		for _, member := range members {
			memberPath := joinRel(member, "Cargo.toml")
			if other, ok := byPath[memberPath]; ok && other.Manifest.Workspace != nil {
				issues = append(issues, fmt.Sprintf("nested workspace at %s is a member of %s; nested workspaces are unsupported", other.Path, record.Path))
			}
			if existing, ok := owners[memberPath]; ok && existing != record.Path {
				issues = append(issues, fmt.Sprintf("%s is claimed by multiple workspaces (%s and %s); workspace membership is ambiguous", memberPath, existing, record.Path))
				continue
			}
			owners[memberPath] = record.Path
		}
	}

	// An explicit package.workspace reference must resolve to a real workspace
	// root. A dangling reference is ambiguous and cannot prove coverage.
	workspaceRoots := map[string]bool{}
	for _, workspace := range workspaces {
		workspaceRoots[workspace.Manifest] = true
	}
	for _, record := range records {
		if record.ParseErr != nil || record.Manifest.Package == nil || strings.TrimSpace(record.Manifest.Package.Workspace) == "" {
			continue
		}
		target, ok := normalizeCargoPath(record.Dir, record.Manifest.Package.Workspace)
		if !ok {
			issues = append(issues, fmt.Sprintf("%s package.workspace reference %q is absolute or escapes the repository; unsupported layout", record.Path, record.Manifest.Package.Workspace))
			continue
		}
		if !workspaceRoots[joinRel(target, "Cargo.toml")] {
			issues = append(issues, fmt.Sprintf("%s package.workspace reference %q does not resolve to a workspace root", record.Path, record.Manifest.Package.Workspace))
		}
	}

	obligations := map[string][]string{} // owner manifest/dir -> member manifest paths
	for manifestPath, owner := range owners {
		obligations[owner] = append(obligations[owner], manifestPath)
	}
	for _, record := range records {
		if record.ParseErr != nil || record.Manifest.Workspace != nil {
			continue
		}
		if _, owned := owners[record.Path]; owned {
			continue
		}
		if record.Manifest.Package == nil {
			continue
		}
		obligations[record.Path] = append(obligations[record.Path], record.Path)
	}

	var lockfiles []Lockfile
	for owner, members := range obligations {
		ownerRecord, ok := byPath[owner]
		if !ok {
			continue
		}
		lockPath := joinRel(ownerRecord.Dir, "Cargo.lock")
		present := false
		if info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(lockPath))); err == nil && info.Mode().IsRegular() {
			present = true
		}
		sortedMembers := append([]string(nil), members...)
		sort.Strings(sortedMembers)
		lockfiles = append(lockfiles, Lockfile{
			Path:      lockPath,
			Language:  LanguageRust,
			Present:   present,
			Manifests: sortedMembers,
			Owner:     owner,
			Kind:      cargoObligationKind(ownerRecord),
		})
	}
	sort.Slice(lockfiles, func(i, j int) bool { return lockfiles[i].Path < lockfiles[j].Path })
	return mergeWorkspaces(workspaces), lockfiles, sortedUniqueStrings(issues)
}

// cargoWorkspaceMembership resolves one workspace root's members, exclusions,
// root package, and in-root path dependencies. Cargo membership comes from
// members/exclude, the root package, and path dependencies; package.workspace
// only identifies the workspace a package believes it belongs to and never
// creates membership by itself.
func cargoWorkspaceMembership(workspaceRecord cargoManifestRecord, records []cargoManifestRecord, byDir map[string]cargoManifestRecord, dirs []string) (CargoWorkspace, []string, []string) {
	workspace := CargoWorkspace{Manifest: workspaceRecord.Path}
	var issues []string
	declared := append([]string(nil), workspaceRecord.Manifest.Workspace.Members...)
	workspace.Members = sortedUniqueStrings(declared)
	workspace.Excluded = sortedUniqueStrings(workspaceRecord.Manifest.Workspace.Exclude)

	members := map[string]bool{}
	explicitMembers := map[string]bool{}
	excludeMatcher := cargoExcludeMatcher(workspaceRecord.Dir, workspaceRecord.Manifest.Workspace.Exclude, &issues)
	// A non-virtual workspace root is an implicit member.
	if workspaceRecord.Manifest.Package != nil {
		members[workspaceRecord.Dir] = true
	}
	for _, pattern := range declared {
		// Resolve the pattern against the workspace root first so a member outside
		// the root (for example "../pkg") is matched by its repository-relative
		// path, exactly as Cargo resolves it.
		normalized, ok := normalizeCargoPath(workspaceRecord.Dir, pattern)
		if !ok {
			issues = append(issues, fmt.Sprintf("workspace %s has an unsafe member pattern %q", workspaceRecord.Path, pattern))
			continue
		}
		glob := cargoGlobPattern(pattern)
		matched := 0
		for _, candidate := range records {
			if candidate.ParseErr != nil {
				continue
			}
			if !cargoGlobMatch(normalized, candidate.Dir) && !cargoGlobMatch(normalized, joinRel(candidate.Dir, "Cargo.toml")) {
				continue
			}
			matched++
			if candidate.Dir == workspaceRecord.Dir {
				continue
			}
			members[candidate.Dir] = true
			// An explicit path is a declared member. Cargo does not let exclude
			// remove it; exclude filters glob-expanded members and path dependencies.
			if !glob {
				explicitMembers[candidate.Dir] = true
			}
		}
		if glob && !strings.HasSuffix(normalized, "Cargo.toml") {
			for _, dir := range dirs {
				if dir == workspaceRecord.Dir {
					continue
				}
				if _, ok := byDir[dir]; ok {
					continue
				}
				if cargoGlobMatch(normalized, dir) {
					if excludeMatcher(dir) {
						continue
					}
					issues = append(issues, fmt.Sprintf("workspace %s member pattern %q matched directory %s without Cargo.toml", workspaceRecord.Path, pattern, dir))
				}
			}
		}
		if matched == 0 {
			issues = append(issues, fmt.Sprintf("workspace %s member pattern %q matched no Cargo.toml", workspaceRecord.Path, pattern))
		}
	}

	// The root package is always a member, and an explicitly listed member is not
	// removed by exclude; Cargo applies exclude to glob expansion and path
	// dependencies.
	for dir := range members {
		if dir == workspaceRecord.Dir || explicitMembers[dir] {
			continue
		}
		if excludeMatcher(dir) {
			delete(members, dir)
		}
	}

	// Cargo treats path dependencies of workspace members as implicit members when
	// they are inside the workspace root and not excluded. A [workspace.dependencies]
	// path used through `workspace = true` is a path dependency too.
	workspaceDeps := workspaceRecord.Manifest.Workspace.Dependencies
	queue := make([]string, 0, len(members))
	for dir := range members {
		queue = append(queue, dir)
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		record, ok := byDir[dir]
		if !ok {
			continue
		}
		for _, depDir := range cargoPathDependencyDirs(record, workspaceDeps, workspaceRecord.Dir) {
			if excludeMatcher(depDir) {
				continue
			}
			rel, ok := relativeTo(workspaceRecord.Dir, depDir)
			if !ok || strings.HasPrefix(rel, "../") {
				continue
			}
			if _, ok := byDir[depDir]; !ok {
				continue
			}
			if members[depDir] {
				continue
			}
			members[depDir] = true
			queue = append(queue, depDir)
		}
	}

	// package.workspace identifies a package's workspace, but Cargo still requires
	// the workspace to list or reach the package. A reference that points here while
	// the package is not a member is an invalid layout, not an implicit member.
	for _, candidate := range records {
		if candidate.ParseErr != nil || candidate.Manifest.Package == nil {
			continue
		}
		ref := strings.TrimSpace(candidate.Manifest.Package.Workspace)
		if ref == "" {
			continue
		}
		target, ok := normalizeCargoPath(candidate.Dir, ref)
		if !ok || target != workspaceRecord.Dir {
			continue
		}
		if !members[candidate.Dir] {
			issues = append(issues, fmt.Sprintf("%s package.workspace reference %q does not match workspace membership; Cargo requires the package to be listed in members or reached by a path dependency", candidate.Path, ref))
		}
	}

	workspace.Resolved = sortedUniqueStrings(mapKeys(members))
	workspace.Issues = sortedUniqueStrings(issues)
	return workspace, mapKeys(members), issues
}

// cargoGlobPattern reports whether a Cargo members entry uses glob
// metacharacters recognized by Cargo's glob expansion.
func cargoGlobPattern(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func cargoExcludeMatcher(baseDir string, patterns []string, issues *[]string) func(dir string) bool {
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		resolved, ok := normalizeCargoPath(baseDir, pattern)
		if !ok {
			*issues = append(*issues, fmt.Sprintf("unsafe Cargo exclude pattern %q", pattern))
			continue
		}
		normalized = append(normalized, resolved)
	}
	return func(dir string) bool {
		for _, prefix := range normalized {
			// Cargo exclude entries are path prefixes, not globs: excluding
			// "crates" also removes "crates/app". "." excludes every glob-expanded
			// member below the workspace root.
			if prefix == "." || dir == prefix || strings.HasPrefix(dir, prefix+"/") {
				return true
			}
		}
		return false
	}
}

func cargoPathDependencyDirs(record cargoManifestRecord, workspaceDeps map[string]any, workspaceBaseDir string) []string {
	var dirs []string
	add := func(deps map[string]any) {
		for name, value := range deps {
			path := cargoDependencyPath(value)
			base := record.Dir
			if path == "" && cargoDependencyInheritsWorkspace(value) {
				path = cargoDependencyPath(workspaceDeps[name])
				base = workspaceBaseDir
			}
			if path == "" {
				continue
			}
			if dir, ok := normalizeCargoPath(base, path); ok {
				dirs = append(dirs, dir)
			}
		}
	}
	add(record.Manifest.Dependencies)
	add(record.Manifest.DevDependencies)
	add(record.Manifest.BuildDependencies)
	for _, target := range record.Manifest.Target {
		add(target.Dependencies)
		add(target.DevDependencies)
		add(target.BuildDependencies)
	}
	return sortedUniqueStrings(dirs)
}

func cargoGlobMatch(pattern, name string) bool {
	pattern = strings.TrimPrefix(strings.ReplaceAll(pattern, "\\", "/"), "./")
	name = strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./")
	if pattern == "" {
		return name == ""
	}
	return cargoGlobSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func cargoGlobSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(name); i++ {
			if cargoGlobSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], name[0])
	if err != nil || !matched {
		return false
	}
	return cargoGlobSegments(pattern[1:], name[1:])
}

func normalizeCargoPath(baseDir, raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || strings.HasPrefix(raw, "/") || (len(raw) >= 2 && raw[1] == ':') {
		return "", false
	}
	joined := raw
	if baseDir != "." && baseDir != "" {
		joined = baseDir + "/" + raw
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(joined)))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." {
		return ".", true
	}
	if clean == "" || !SafeRelativePath(clean) {
		return "", false
	}
	return clean, true
}

func relativeTo(baseDir, target string) (string, bool) {
	rel, err := filepath.Rel(filepath.FromSlash(baseDir), filepath.FromSlash(target))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return ".", true
	}
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return "", false
	}
	return rel, true
}

func joinRel(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

func mapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cargoObligationKind(record cargoManifestRecord) string {
	if record.Manifest.Workspace != nil {
		return "rust-workspace"
	}
	return "rust-package"
}
