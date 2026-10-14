package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Project is a resolved SOP project candidate: a canonical project root plus the
// controller's identity for it.
type Project struct {
	// Root is the canonical (absolute, symlink-resolved) project directory.
	Root string
	// ID is the controller-facing project identity used in routes and stores.
	ID string
}

// Diagnostic is a skipped candidate or unreadable workspace root. Path and
// Reason are safe to log.
type Diagnostic struct {
	Path   string
	Reason string
}

// DefaultMaxDepth bounds how far below a workspace root discovery descends.
// workspace/project is depth 1; workspace/group/project is depth 2.
const DefaultMaxDepth = 4

// ignoredDirs are never descended into (VCS, dependencies, build output, caches).
var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".cache":       true,
}

// ProjectConfig is the small, non-secret subset of .agent-sdlc/config.yaml the
// controller reads.
type ProjectConfig struct {
	Name   string
	Branch string
}

// ReadProjectConfig reads `project.name` and `project.integration_branch` from
// .agent-sdlc/config.yaml with a minimal parser (no YAML dependency). A file
// without a `project` section is invalid.
func ReadProjectConfig(root string) (ProjectConfig, error) {
	path := filepath.Join(root, ".agent-sdlc", "config.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return ProjectConfig{}, err
	}
	var cfg ProjectConfig
	hasProject := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "project":
			hasProject = true
		case "name":
			if cfg.Name == "" {
				cfg.Name = value
			}
		case "integration_branch":
			cfg.Branch = value
		}
	}
	if !hasProject {
		return ProjectConfig{}, fmt.Errorf("%s: missing project section", path)
	}
	return cfg, nil
}

// ProjectID is the project's declared name, else the root's basename. Discovery
// and the store share it.
func ProjectID(root string) string {
	if cfg, err := ReadProjectConfig(root); err == nil && cfg.Name != "" {
		return cfg.Name
	}
	return filepath.Base(root)
}

// DiscoverProjects finds directories under the workspaces with a valid
// config.yaml and an initialized state.db. It is bounded by maxDepth (0 =
// DefaultMaxDepth), prunes ignoredDirs, and never follows symlinks (WalkDir
// lstats). A broken candidate yields a Diagnostic, never hides other projects.
func DiscoverProjects(workspaces []string, maxDepth int) ([]Project, []Diagnostic) {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	var projects []Project
	var diags []Diagnostic
	for _, ws := range workspaces {
		root, err := canonicalPath(ws)
		if err != nil {
			diags = append(diags, Diagnostic{Path: ws, Reason: err.Error()})
			continue
		}
		info, err := os.Stat(root)
		if err != nil {
			diags = append(diags, Diagnostic{Path: ws, Reason: "workspace root not readable: " + err.Error()})
			continue
		}
		if !info.IsDir() {
			diags = append(diags, Diagnostic{Path: ws, Reason: "workspace root is not a directory"})
			continue
		}
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// One unreadable entry must not abort the rest of the walk.
				diags = append(diags, Diagnostic{Path: path, Reason: "not readable: " + err.Error()})
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if path != root {
				if ignoredDirs[d.Name()] || depth(root, path) > maxDepth {
					return fs.SkipDir
				}
			}
			project, isProject, diag := considerProject(path)
			if diag != "" {
				diags = append(diags, Diagnostic{Path: path, Reason: diag})
				return nil
			}
			if isProject {
				projects = append(projects, project)
				// A project does not contain nested projects; do not descend.
				return fs.SkipDir
			}
			return nil
		})
		if walkErr != nil {
			diags = append(diags, Diagnostic{Path: root, Reason: "walk failed: " + walkErr.Error()})
		}
	}
	return projects, diags
}

// considerProject returns (project, true, "") for a project, ("", false, "") for
// an ordinary directory, and ("", false, reason) for an unusable one.
func considerProject(dir string) (Project, bool, string) {
	if _, err := os.Stat(filepath.Join(dir, ".agent-sdlc", "config.yaml")); err != nil {
		return Project{}, false, ""
	}
	cfg, err := ReadProjectConfig(dir)
	if err != nil {
		return Project{}, false, "invalid SOP config: " + err.Error()
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent-sdlc", "state.db")); err != nil {
		return Project{}, false, "SOP config present but no initialized state.db"
	}
	id := cfg.Name
	if id == "" {
		id = filepath.Base(dir)
	}
	return Project{Root: dir, ID: id}, true, ""
}

// ResolveProjects merges explicit roots (first, in order) with discovered ones,
// deduplicated by canonical path. Two roots claiming one identity is an error
// naming both, so the wrong repository is never exposed.
func ResolveProjects(explicit, workspaces []string, maxDepth int) ([]Project, []Diagnostic, error) {
	var out []Project
	var diags []Diagnostic
	seenRoot := map[string]bool{}
	rootByID := map[string]string{}

	add := func(root, id string) error {
		if seenRoot[root] {
			return nil
		}
		if prev, dup := rootByID[id]; dup {
			return fmt.Errorf("project identity %q is claimed by two roots: %s and %s", id, prev, root)
		}
		seenRoot[root] = true
		rootByID[id] = root
		out = append(out, Project{Root: root, ID: id})
		return nil
	}

	for _, r := range explicit {
		root, err := canonicalPath(r)
		if err != nil {
			diags = append(diags, Diagnostic{Path: r, Reason: err.Error()})
			continue
		}
		if err := add(root, ProjectID(root)); err != nil {
			return nil, diags, err
		}
	}

	discovered, discoveryDiags := DiscoverProjects(workspaces, maxDepth)
	diags = append(diags, discoveryDiags...)
	for _, p := range discovered {
		if err := add(p.Root, p.ID); err != nil {
			return nil, diags, err
		}
	}
	return out, diags, nil
}

// DiscoveryReport is a diagnostics snapshot of how the registry was resolved.
type DiscoveryReport struct {
	Explicit    []string
	Workspaces  []string
	MaxDepth    int
	Projects    []Project
	Diagnostics []Diagnostic
}

// ResolveReport resolves the registry; on an identity conflict it has no
// projects and returns the error.
func ResolveReport(explicit, workspaces []string, maxDepth int) (DiscoveryReport, error) {
	projects, diags, err := ResolveProjects(explicit, workspaces, maxDepth)
	rep := DiscoveryReport{
		Explicit:    explicit,
		Workspaces:  workspaces,
		MaxDepth:    maxDepth,
		Projects:    projects,
		Diagnostics: diags,
	}
	if err != nil {
		rep.Projects = nil
		return rep, err
	}
	return rep, nil
}

// canonicalPath is the dedupe key: absolute, symlink-resolved, cleaned.
func canonicalPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return filepath.Clean(abs), nil
}

// depth is the number of path segments from a workspace root to a directory:
// root itself is 0, workspace/project is 1, workspace/group/project is 2.
func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}
