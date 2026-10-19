package security

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
)

// Language identifies a source language detected from repository manifests.
type Language string

const (
	// LanguageGo is selected by a go.mod at or below the detection root.
	LanguageGo Language = "go"
	// LanguageRust is selected by a Cargo.toml at or below the detection root.
	LanguageRust Language = "rust"
)

// CapabilityStatus is the availability of a configured adapter candidate. The
// vocabulary is exactly available, unavailable, unsupported, and proposed. It is
// computed from the adapter descriptor and configuration only; detection never
// starts a scanner executable.
type CapabilityStatus string

const (
	// CapabilityAvailable reports an adapter that can run for the detected content.
	CapabilityAvailable CapabilityStatus = "available"
	// CapabilityUnavailable reports a built-in adapter whose executable cannot be
	// resolved by the availability probe.
	CapabilityUnavailable CapabilityStatus = "unavailable"
	// CapabilityUnsupported reports an adapter whose declared categories or
	// language scope cannot cover the detected content, or an unknown built-in id.
	CapabilityUnsupported CapabilityStatus = "unsupported"
	// CapabilityProposed reports an adapter that the selection function proposes
	// from detected content but that is not configured yet.
	CapabilityProposed CapabilityStatus = "proposed"
)

// isKnownCapabilityStatus reports whether s is one of the four statuses.
func isKnownCapabilityStatus(s CapabilityStatus) bool {
	switch s {
	case CapabilityAvailable, CapabilityUnavailable, CapabilityUnsupported, CapabilityProposed:
		return true
	}
	return false
}

// Manifest records one detected manifest file as a repository-relative path.
type Manifest struct {
	// Path is the repository-relative manifest path with forward slashes.
	Path string `json:"path"`
	// Language is the language the manifest declares.
	Language Language `json:"language"`
	// Kind is the manifest flavor: "go.mod" or "cargo.toml".
	Kind string `json:"kind"`
}

// Lockfile records one lockfile location. Present is true when the file exists;
// Present is false when the lockfile is expected but missing. A missing lockfile
// is reported explicitly instead of being assumed present.
type Lockfile struct {
	// Path is the repository-relative lockfile path with forward slashes.
	Path string `json:"path"`
	// Language is the language the lockfile belongs to.
	Language Language `json:"language"`
	// Present reports whether the file actually exists in the tree.
	Present bool `json:"present"`
	// Manifests names the manifest paths (sorted) the lockfile belongs to.
	Manifests []string `json:"manifests,omitempty"`
	// Owner is the owning workspace/package manifest for a Rust lockfile.
	Owner string `json:"owner,omitempty"`
	// Kind is "rust-workspace" or "rust-package" for a Rust lockfile.
	Kind string `json:"kind,omitempty"`
}

// DependencyObligation is one in-scope dependency analysis input. A Go module's
// input is its go.mod; a Rust workspace/package input is its Cargo.lock. Present
// distinguishes a real input from a detected but unsupported/missing obligation.
type DependencyObligation struct {
	Language  Language `json:"language"`
	Kind      string   `json:"kind"`
	Root      string   `json:"root"`
	Manifest  string   `json:"manifest,omitempty"`
	Input     string   `json:"input"`
	Present   bool     `json:"present"`
	Workspace string   `json:"workspace,omitempty"`
	Issues    []string `json:"issues,omitempty"`
}

// CargoWorkspace records a parsed Cargo workspace. The Members slice lists the
// declared member patterns as Cargo declares them; Resolved lists the normalized
// repository-relative member directories. Both are sorted and stable.
type CargoWorkspace struct {
	// Manifest is the workspace root Cargo.toml path.
	Manifest string `json:"manifest"`
	// Members lists the declared member patterns, sorted.
	Members []string `json:"members,omitempty"`
	// Excluded lists the declared exclude patterns, sorted.
	Excluded []string `json:"excluded,omitempty"`
	// Resolved lists the resolved member directories, sorted.
	Resolved []string `json:"resolved,omitempty"`
	// Issues lists unsupported or ambiguous layout facts for this workspace.
	Issues []string `json:"issues,omitempty"`
}

// AdapterCandidate is one adapter selected or proposed for the detected content.
type AdapterCandidate struct {
	// ID is the stable built-in adapter id.
	ID string `json:"id"`
	// Category is the adapter's category (Descriptor.Categories).
	Category Category `json:"category"`
	// Languages lists the languages this candidate can serve, sorted.
	Languages []Language `json:"languages,omitempty"`
	// Rules is the rules path the candidate requires, if any.
	Rules string `json:"rules,omitempty"`
	// Status is the capability status for this candidate.
	Status CapabilityStatus `json:"status"`
	// Reason explains a missing prerequisite or unsupported capability without
	// changing the status vocabulary.
	Reason string `json:"reason,omitempty"`
	// Disabled reports that configuration disabled this adapter. A disabled
	// adapter is visible but never counted as passed coverage.
	Disabled bool `json:"disabled,omitempty"`
	// DisabledReason explains the effective disable (global, category, adapter).
	DisabledReason string `json:"disabled_reason,omitempty"`
	// Selection records explicit or automatic selection.
	Selection string `json:"selection,omitempty"`
}

// Detection is the deterministic result of inspecting repository content. Every
// slice is sorted and no timestamp or map is embedded, so repeated detection of
// the same tree produces equal values.
type Detection struct {
	// Root is the repository-relative detection root. It is recorded as "." so
	// persisted evidence never carries a host absolute path.
	Root string `json:"root"`
	// rootAbs is the absolute root used internally for path resolution. It is
	// never serialized.
	rootAbs string
	// Languages lists the detected languages, sorted.
	Languages []Language `json:"languages,omitempty"`
	// Manifests lists detected manifest files, sorted by path.
	Manifests []Manifest `json:"manifests,omitempty"`
	// Lockfiles lists every actual and expected lockfile location, sorted by path.
	Lockfiles []Lockfile `json:"lockfiles,omitempty"`
	// Workspaces lists parsed Cargo workspaces, sorted by manifest path.
	Workspaces []CargoWorkspace `json:"workspaces,omitempty"`
	// Obligations lists every in-scope Go/Rust dependency input, including
	// detected inputs that are missing or unsupported. Sorted by input/language.
	Obligations []DependencyObligation `json:"obligations,omitempty"`
	// Issues lists unsupported or ambiguous dependency layouts. An issue makes
	// dependency coverage non-clean rather than inferred complete.
	Issues []string `json:"issues,omitempty"`
}

// workspaceRoot returns the absolute root used for safe path resolution.
func (d Detection) workspaceRoot() string {
	if d.rootAbs != "" {
		return d.rootAbs
	}
	return d.Root
}

// HasLanguage reports whether the detection found language.
func (d Detection) HasLanguage(language Language) bool {
	for _, have := range d.Languages {
		if have == language {
			return true
		}
	}
	return false
}

// MissingLockfiles returns the expected-but-absent lockfiles, sorted by path.
func (d Detection) MissingLockfiles() []Lockfile {
	var out []Lockfile
	for _, lf := range d.Lockfiles {
		if !lf.Present {
			out = append(out, lf)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// PresentLockfiles returns the lockfiles that actually exist, sorted by path.
func (d Detection) PresentLockfiles() []Lockfile {
	var out []Lockfile
	for _, lf := range d.Lockfiles {
		if lf.Present {
			out = append(out, lf)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// HasPresentLockfile reports whether the detected language has an actual
// lockfile. A Go lockfile must never satisfy a Rust dependency capability.
func (d Detection) HasPresentLockfile(language Language) bool {
	for _, lf := range d.Lockfiles {
		if lf.Language == language && lf.Present {
			return true
		}
	}
	return false
}

// MissingObligations returns the in-scope obligations with no usable input,
// sorted by language/input. A missing obligation is never clean coverage.
func (d Detection) MissingObligations(languages ...Language) []DependencyObligation {
	selected := map[Language]bool{}
	for _, language := range languages {
		selected[language] = true
	}
	var out []DependencyObligation
	for _, obligation := range d.Obligations {
		if len(selected) > 0 && !selected[obligation.Language] {
			continue
		}
		if !obligation.Present {
			out = append(out, obligation)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		return out[i].Input < out[j].Input
	})
	return out
}

// SelectedObligations returns the in-scope obligations for a language set.
func (d Detection) SelectedObligations(languages ...Language) []DependencyObligation {
	selected := map[Language]bool{}
	for _, language := range languages {
		selected[language] = true
	}
	var out []DependencyObligation
	for _, obligation := range d.Obligations {
		if len(selected) > 0 && !selected[obligation.Language] {
			continue
		}
		out = append(out, obligation)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		return out[i].Input < out[j].Input
	})
	return out
}

// ObligationIdentity is a deterministic resume key over the resolved obligation
// inventory, including missing/unsupported state.
func (d Detection) ObligationIdentity(languages ...Language) string {
	obligations := d.SelectedObligations(languages...)
	parts := make([]string, 0, len(obligations))
	for _, obligation := range obligations {
		parts = append(parts, fmt.Sprintf("language=%s;kind=%s;root=%s;manifest=%s;input=%s;present=%t;workspace=%s;issues=%s",
			obligation.Language, obligation.Kind, obligation.Root, obligation.Manifest, obligation.Input,
			obligation.Present, obligation.Workspace, strings.Join(obligation.Issues, "|")))
	}
	sort.Strings(parts)
	return DigestStrings(parts...)
}

// Built-in adapter ids that selection can propose.
const (
	// AdapterIDSemgrep is the built-in SAST adapter id.
	AdapterIDSemgrep = "semgrep"
	// AdapterIDGitleaks is the built-in secret adapter id.
	AdapterIDGitleaks = "gitleaks"
	// AdapterIDCargoAudit is the built-in Rust dependency adapter id.
	AdapterIDCargoAudit = "cargo-audit"
	// AdapterIDOSVScanner is the built-in Go/Rust dependency adapter id.
	AdapterIDOSVScanner = "osv-scanner"
)

// languageManifests maps a manifest file name to the language it declares.
var languageManifests = map[string]Language{
	"go.mod":     LanguageGo,
	"Cargo.toml": LanguageRust,
}

// manifestLockfile names the lockfile each language expects beside its manifest.
var manifestLockfile = map[Language]string{
	LanguageGo:   "go.sum",
	LanguageRust: "Cargo.lock",
}

// DetectRepository inspects the repository under root with the default VCS and
// SOP-state exclusions.
func DetectRepository(root string) (Detection, error) {
	return DetectRepositoryScoped(root, nil)
}

// DetectRepositoryScoped inspects the repository under root and returns a
// deterministic detection result. It only reads content: no scanner executable
// runs, no network is used, and no path outside root is read. Every reported
// path is repository-relative and sorted. Declared exclusions are applied to the
// same content boundary the scanners use.
func DetectRepositoryScoped(root string, excludes []string) (Detection, error) {
	abs, err := ensureWorkspaceRoot(root)
	if err != nil {
		return Detection{}, err
	}
	result := Detection{Root: ".", rootAbs: abs}
	matcher := NewExcludeMatcher(append([]string{".git", ".agent-sdlc"}, excludes...))

	languageSet := map[Language]bool{}
	// manifestDirs records Go module directories. A go module expects go.sum.
	manifestDirs := map[Language]map[string][]string{}
	found := map[string]bool{}
	var cargoRecords []cargoManifestRecord

	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		rel, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel != "." && matcher.Excluded(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if matcher.Excluded(rel) {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		language, ok := languageManifests[name]
		if !ok {
			// Source files are a language signal even when the repository has no
			// manifest (for example a source-only fixture or a GOPATH-style tree).
			switch strings.ToLower(filepath.Ext(name)) {
			case ".go":
				languageSet[LanguageGo] = true
			case ".rs":
				languageSet[LanguageRust] = true
			}
			return nil
		}
		if !SafeRelativePath(rel) {
			return fmt.Errorf("security: detection rejected unsafe manifest path %q", rel)
		}
		if found[rel] {
			return nil
		}
		found[rel] = true
		result.Manifests = append(result.Manifests, Manifest{Path: rel, Language: language, Kind: strings.ToLower(name)})
		languageSet[language] = true
		dir := repoDir(rel)
		if manifestDirs[language] == nil {
			manifestDirs[language] = map[string][]string{}
		}
		manifestDirs[language][dir] = append(manifestDirs[language][dir], rel)

		if language == LanguageRust {
			data, rerr := os.ReadFile(filepath.Join(abs, filepath.FromSlash(rel)))
			if rerr != nil {
				return rerr
			}
			manifest, perr := parseCargoManifest(data)
			cargoRecords = append(cargoRecords, cargoManifestRecord{Path: rel, Dir: dir, Manifest: manifest, ParseErr: perr})
		}
		return nil
	})
	if err != nil {
		return Detection{}, err
	}

	// Cargo membership and obligations come from parsed Cargo.toml content, never
	// from directory ancestry. A workspace member shares the workspace root's
	// Cargo.lock; an excluded or undeclared package owns its own lockfile.
	workspaces, rustLockfiles, cargoIssues := resolveCargoWorkspaces(abs, cargoRecords)
	result.Workspaces = workspaces
	result.Issues = append(result.Issues, cargoIssues...)

	// Lockfile expectations: a Go module expects go.sum, and each resolved Rust
	// workspace/package expects Cargo.lock. Every expectation is reported, present
	// or not.
	result.Lockfiles = detectLockfiles(abs, map[Language]map[string][]string{LanguageGo: manifestDirs[LanguageGo]})
	result.Lockfiles = append(result.Lockfiles, rustLockfiles...)
	result.Obligations = dependencyObligations(result)
	if languageSet[LanguageGo] {
		hasGoModule := false
		for _, obligation := range result.Obligations {
			if obligation.Language == LanguageGo && obligation.Present {
				hasGoModule = true
				break
			}
		}
		if !hasGoModule {
			result.Obligations = append(result.Obligations, DependencyObligation{
				Language: LanguageGo,
				Kind:     "go-module",
				Root:     ".",
				Input:    "go.mod",
				Present:  false,
				Issues:   []string{"Go source detected without go.mod"},
			})
		}
	}
	result.Obligations = sortDependencyObligations(result.Obligations)
	result.Workspaces = mergeWorkspaces(result.Workspaces)
	result.Issues = sortedUniqueStrings(result.Issues)
	result.Languages = sortedLanguages(languageSet)
	sort.Slice(result.Manifests, func(i, j int) bool { return result.Manifests[i].Path < result.Manifests[j].Path })
	sort.Slice(result.Lockfiles, func(i, j int) bool {
		if result.Lockfiles[i].Path != result.Lockfiles[j].Path {
			return result.Lockfiles[i].Path < result.Lockfiles[j].Path
		}
		return result.Lockfiles[i].Language < result.Lockfiles[j].Language
	})
	return result, nil
}

// detectLockfiles builds the sorted lockfile expectations for the detected
// manifest directories.
func detectLockfiles(root string, manifestDirs map[Language]map[string][]string) []Lockfile {
	type key struct {
		language Language
		dir      string
	}
	seen := map[key]bool{}
	var out []Lockfile
	for language, dirs := range manifestDirs {
		lockName, ok := manifestLockfile[language]
		if !ok {
			continue
		}
		for dir, manifests := range dirs {
			k := key{language: language, dir: dir}
			if seen[k] {
				continue
			}
			seen[k] = true
			repoPath := lockName
			if dir != "." && dir != "" {
				repoPath = dir + "/" + lockName
			}
			if !SafeRelativePath(repoPath) {
				continue
			}
			present := false
			if info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(repoPath))); err == nil && info.Mode().IsRegular() {
				present = true
			}
			sortedManifests := append([]string(nil), manifests...)
			sort.Strings(sortedManifests)
			out = append(out, Lockfile{Path: repoPath, Language: language, Present: present, Manifests: sortedManifests})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Language < out[j].Language
	})
	return out
}

// mergeWorkspaces deduplicates and sorts the parsed Cargo workspaces by path.
func mergeWorkspaces(in []CargoWorkspace) []CargoWorkspace {
	if len(in) == 0 {
		return nil
	}
	out := append([]CargoWorkspace(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest < out[j].Manifest })
	for i := range out {
		sort.Strings(out[i].Members)
		sort.Strings(out[i].Excluded)
		sort.Strings(out[i].Resolved)
		sort.Strings(out[i].Issues)
	}
	return out
}

// sortedLanguages returns the language set in a stable order.
func sortedLanguages(set map[Language]bool) []Language {
	if len(set) == 0 {
		return nil
	}
	out := make([]Language, 0, len(set))
	for language := range set {
		out = append(out, language)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// repoDir returns the slash-separated directory of a repository-relative path.
// A path at the repository root yields ".".
func repoDir(rel string) string {
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if dir == "" {
		return "."
	}
	return dir
}

// dependencyObligations derives the deterministic in-scope input inventory from
// detected Go manifests and resolved Rust lockfile obligations.
func dependencyObligations(detection Detection) []DependencyObligation {
	var out []DependencyObligation
	for _, manifest := range detection.Manifests {
		if manifest.Language != LanguageGo {
			continue
		}
		out = append(out, DependencyObligation{
			Language: LanguageGo,
			Kind:     "go-module",
			Root:     repoDir(manifest.Path),
			Manifest: manifest.Path,
			Input:    manifest.Path,
			Present:  true,
		})
	}
	for _, lockfile := range detection.Lockfiles {
		if lockfile.Language != LanguageRust {
			continue
		}
		obligation := DependencyObligation{
			Language:  LanguageRust,
			Kind:      lockfile.Kind,
			Root:      repoDir(lockfile.Owner),
			Manifest:  lockfile.Owner,
			Input:     lockfile.Path,
			Present:   lockfile.Present,
			Workspace: lockfile.Owner,
		}
		if !lockfile.Present {
			obligation.Issues = []string{"Cargo.lock is missing for " + lockfile.Owner}
		}
		out = append(out, obligation)
	}
	return sortDependencyObligations(out)
}

// dependencyInventoryGap reports why the selected dependency inputs cannot
// account for every in-scope obligation for the given languages. It is a
// read-only projection for capability and readiness reporting; it never changes
// scan execution, and it never treats an omitted or missing input as clean.
func dependencyInventoryGap(detection Detection, languages []Language, explicitInputs []string, explicitLockfile string) string {
	if len(languages) == 0 {
		languages = []Language{LanguageGo, LanguageRust}
	}
	if len(detection.Issues) > 0 {
		return "dependency layout is unsupported or ambiguous: " + detection.Issues[0]
	}
	obligations := detection.SelectedObligations(languages...)
	if len(obligations) == 0 {
		return ""
	}
	byInput := make(map[string]DependencyObligation, len(obligations))
	for _, obligation := range obligations {
		byInput[obligation.Input] = obligation
	}
	covered := map[string]bool{}
	addInput := func(raw, label string) string {
		if strings.TrimSpace(raw) == "" {
			return ""
		}
		path, reason := NormalizePath(raw)
		if path == "" {
			return "configured " + label + " rejected: " + reason
		}
		obligation, ok := byInput[path]
		if !ok {
			return "configured " + label + " " + path + " is not an in-scope dependency obligation"
		}
		if !containsLanguage(languages, obligation.Language) {
			return "configured " + label + " " + path + " is outside the adapter language scope"
		}
		info, err := os.Lstat(joinPath(detection.workspaceRoot(), path))
		if err != nil {
			return "configured " + label + " " + path + " is unavailable"
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "configured " + label + " " + path + " is not a regular file"
		}
		covered[path] = true
		return ""
	}
	if reason := addInput(explicitLockfile, "lockfile"); reason != "" {
		return reason
	}
	for _, raw := range explicitInputs {
		if reason := addInput(raw, "dependency input"); reason != "" {
			return reason
		}
	}
	for _, obligation := range obligations {
		if !obligation.Present {
			return missingObligationDiagnostic(obligation)
		}
		if len(covered) > 0 && !covered[obligation.Input] {
			return "input is present but omitted by the explicit dependency input list: " + obligation.Input
		}
	}
	return ""
}

func sortDependencyObligations(values []DependencyObligation) []DependencyObligation {
	out := append([]DependencyObligation(nil), values...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		return out[i].Input < out[j].Input
	})
	return out
}

// sortedUniqueStrings sorts and deduplicates values.
func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

// SelectCandidates maps a detection result to candidate built-in adapters. It is
// content-based only and deterministic: identical detections yield identically
// ordered candidates. A Rust-only repository never receives a Go-only Semgrep
// ruleset, and cargo-audit is proposed only when Rust content and a lockfile
// expectation exist.
func SelectCandidates(detection Detection, configured []config.SecurityAdapterConfig) []AdapterCandidate {
	return SelectCandidatesWithDatabase(detection, configured, "")
}

// SelectCandidatesWithDatabase is SelectCandidates with the global advisory
// database path available to the dependency capability classifier.
func SelectCandidatesWithDatabase(detection Detection, configured []config.SecurityAdapterConfig, databasePath string) []AdapterCandidate {
	return SelectCandidatesWithDatabases(detection, configured, databasePath, "")
}

// SelectCandidatesWithDatabases is SelectCandidates with the separate OSV offline
// database cache root available to the OSV capability classifier. The advisory
// database path and the OSV cache are deliberately not interchangeable.
func SelectCandidatesWithDatabases(detection Detection, configured []config.SecurityAdapterConfig, advisoryDatabasePath, osvDatabasePath string) []AdapterCandidate {
	if len(detection.Languages) == 0 {
		return nil
	}
	byID := configuredAdaptersByID(configured)
	languages := append([]Language(nil), detection.Languages...)
	var out []AdapterCandidate

	// SAST: Semgrep is proposed when Go or Rust source exists. Its coverage is
	// derived from the local rules' declared languages, never from a path guess.
	sast := AdapterCandidate{ID: AdapterIDSemgrep, Category: CategorySAST, Languages: languages}
	rules := ""
	if raw, ok := byID[AdapterIDSemgrep]; ok {
		rules = strings.TrimSpace(raw.Rules)
	}
	if rules == "" {
		sast.Status = CapabilityUnsupported
	} else if covered, err := coveredLanguages(rules, languages); err != nil || len(covered) == 0 {
		sast.Status = CapabilityUnsupported
	} else {
		// Only a ruleset that actually declares a detected language is offered.
		sast.Rules = rules
		if raw, ok := byID[AdapterIDSemgrep]; ok && !executableResolvable(raw.Command) {
			sast.Status = CapabilityUnavailable
		} else if ok {
			sast.Status = CapabilityAvailable
		} else {
			sast.Status = CapabilityProposed
		}
	}
	out = append(out, sast)

	// Secrets: Gitleaks is language-agnostic and is proposed when content exists.
	secrets := AdapterCandidate{ID: AdapterIDGitleaks, Category: CategorySecrets, Languages: languages}
	if raw, ok := byID[AdapterIDGitleaks]; ok {
		if executableResolvable(raw.Command) {
			secrets.Status = CapabilityAvailable
		} else {
			secrets.Status = CapabilityUnavailable
		}
	} else {
		secrets.Status = CapabilityProposed
	}
	out = append(out, secrets)

	// Dependencies: cargo-audit is relevant only for Rust content. A missing
	// lockfile or database is reported as unsupported, never as clean coverage.
	if detection.HasLanguage(LanguageRust) {
		deps := AdapterCandidate{ID: AdapterIDCargoAudit, Category: CategoryDependencies, Languages: []Language{LanguageRust}}
		raw, configured := byID[AdapterIDCargoAudit]
		db := strings.TrimSpace(raw.Database)
		if db == "" {
			db = strings.TrimSpace(advisoryDatabasePath)
		}
		if reason := dependencyInventoryGap(detection, []Language{LanguageRust}, nil, raw.Lockfile); reason != "" {
			deps.Status = CapabilityUnsupported
			deps.Reason = reason
		} else if !detection.HasPresentLockfile(LanguageRust) {
			deps.Status = CapabilityUnsupported
		} else if db == "" {
			deps.Status = CapabilityUnsupported
		} else if !databaseAvailable(db, detection.workspaceRoot()) {
			deps.Status = CapabilityUnsupported
		} else if configured && !executableResolvable(raw.Command) {
			deps.Status = CapabilityUnavailable
		} else if configured {
			deps.Status = CapabilityAvailable
		} else {
			deps.Status = CapabilityProposed
			if strings.TrimSpace(raw.Version) == "" {
				deps.Reason = "cargo-audit requires an exact version pin"
			}
		}
		out = append(out, deps)
	}

	// OSV-Scanner covers Go go.mod files and Rust Cargo.lock files. It is proposed
	// separately from cargo-audit so overlapping Rust coverage is visible to the
	// operator instead of being selected silently.
	if detection.HasLanguage(LanguageGo) || detection.HasLanguage(LanguageRust) {
		osv := AdapterCandidate{ID: AdapterIDOSVScanner, Category: CategoryDependencies}
		raw, configured := byID[AdapterIDOSVScanner]
		for _, language := range detection.Languages {
			switch language {
			case LanguageGo:
				if osvInputAvailable(detection, LanguageGo) {
					osv.Languages = append(osv.Languages, LanguageGo)
				}
			case LanguageRust:
				if osvInputAvailable(detection, LanguageRust) {
					osv.Languages = append(osv.Languages, LanguageRust)
				}
			}
		}
		db := strings.TrimSpace(raw.Database)
		if db == "" {
			db = strings.TrimSpace(osvDatabasePath)
		}
		declared := configuredLanguages(raw.Languages)
		if len(declared) == 0 {
			declared = []Language{LanguageGo, LanguageRust}
		}
		if reason := dependencyInventoryGap(detection, declared, raw.Inputs, ""); reason != "" {
			osv.Status = CapabilityUnsupported
			osv.Reason = reason
		} else if len(osv.Languages) == 0 {
			osv.Status = CapabilityUnsupported
		} else if db == "" {
			osv.Status = CapabilityUnsupported
		} else if !osvDatabaseAvailable(db, detection.workspaceRoot(), osv.Languages) {
			osv.Status = CapabilityUnsupported
		} else if configured && !executableResolvable(raw.Command) {
			osv.Status = CapabilityUnavailable
		} else if configured {
			osv.Status = CapabilityAvailable
		} else {
			osv.Status = CapabilityProposed
			if strings.TrimSpace(raw.Version) == "" {
				osv.Reason = "osv-scanner requires an exact version pin"
			}
		}
		out = append(out, osv)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AutoAdapterConfigs adds the built-in adapters that detection proposes when an
// explicit category is missing. Explicit adapters are never replaced or
// removed; their own adapter-level guard still prevents a Go ruleset from being
// applied to Rust source.
func AutoAdapterConfigs(detection Detection, configured []config.SecurityAdapterConfig, databasePath string) []config.SecurityAdapterConfig {
	out := append([]config.SecurityAdapterConfig(nil), configured...)
	byID := configuredAdaptersByID(out)
	categories := map[string]bool{}
	for _, raw := range out {
		category := strings.ToLower(strings.TrimSpace(raw.Category))
		if category == "" {
			switch strings.ToLower(strings.TrimSpace(raw.ID)) {
			case AdapterIDSemgrep:
				category = config.SecurityCategorySAST
			case AdapterIDGitleaks:
				category = config.SecurityCategorySecrets
			case AdapterIDCargoAudit:
				category = config.SecurityCategoryDependencies
			case AdapterIDOSVScanner:
				category = config.SecurityCategoryDependencies
			}
		}
		if category != "" {
			categories[category] = true
		}
	}
	if len(detection.Languages) == 0 {
		return out
	}
	if !categories[config.SecurityCategorySecrets] {
		out = append(out, config.SecurityAdapterConfig{
			ID: AdapterIDGitleaks, Category: config.SecurityCategorySecrets,
			Protocol: config.SecurityProtocolBuiltin, Command: []string{"gitleaks"},
		})
	}
	if !categories[config.SecurityCategorySAST] {
		rules := ""
		if raw, ok := byID[AdapterIDSemgrep]; ok {
			rules = raw.Rules
		}
		out = append(out, config.SecurityAdapterConfig{
			ID: AdapterIDSemgrep, Category: config.SecurityCategorySAST,
			Protocol: config.SecurityProtocolBuiltin, Command: []string{"semgrep"}, Rules: rules,
		})
	}
	if detection.HasLanguage(LanguageRust) && !categories[config.SecurityCategoryDependencies] {
		database := databasePath
		if raw, ok := byID[AdapterIDCargoAudit]; ok && strings.TrimSpace(raw.Database) != "" {
			database = raw.Database
		}
		out = append(out, config.SecurityAdapterConfig{
			ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
			Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
			Database: database, Stale: "",
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func configuredAdaptersByID(configured []config.SecurityAdapterConfig) map[string]config.SecurityAdapterConfig {
	byID := make(map[string]config.SecurityAdapterConfig, len(configured))
	for _, raw := range configured {
		id := strings.ToLower(strings.TrimSpace(raw.ID))
		if id == "" {
			continue
		}
		if _, ok := byID[id]; !ok {
			byID[id] = raw
		}
	}
	return byID
}

// ClassifyCapabilities maps each configured adapter to a capability status. It
// uses the adapter descriptor, repository detection, and executable resolution:
// no scanner executable runs.
func ClassifyCapabilities(detection Detection, configured []config.SecurityAdapterConfig) []AdapterCandidate {
	return ClassifyCapabilitiesWithDatabase(detection, configured, "")
}

// ClassifyCapabilitiesWithDatabase is ClassifyCapabilities with the global
// advisory database path available.
func ClassifyCapabilitiesWithDatabase(detection Detection, configured []config.SecurityAdapterConfig, databasePath string) []AdapterCandidate {
	return ClassifyCapabilitiesWithDatabases(detection, configured, databasePath, "")
}

// ClassifyCapabilitiesWithDatabases is ClassifyCapabilities with the separate
// cargo-audit advisory database and OSV offline cache roots available.
func ClassifyCapabilitiesWithDatabases(detection Detection, configured []config.SecurityAdapterConfig, advisoryDatabasePath, osvDatabasePath string) []AdapterCandidate {
	out := make([]AdapterCandidate, 0, len(configured))
	proposed := SelectCandidatesWithDatabases(detection, configured, advisoryDatabasePath, osvDatabasePath)
	proposedByID := map[string]AdapterCandidate{}
	for _, candidate := range proposed {
		proposedByID[candidate.ID] = candidate
	}
	for _, raw := range configured {
		id := strings.ToLower(strings.TrimSpace(raw.ID))
		if id == "" {
			continue
		}
		candidate := AdapterCandidate{ID: id, Category: Category(strings.ToLower(strings.TrimSpace(raw.Category)))}
		descriptor, err := configuredDescriptor(raw)
		if err != nil {
			candidate.Status = CapabilityUnsupported
			out = append(out, candidate)
			continue
		}
		candidate.Category = descriptor.Categories[0]
		if len(candidate.Languages) == 0 {
			candidate.Languages = append([]Language(nil), descriptor.Languages...)
		}
		if suggestion, ok := proposedByID[id]; ok {
			candidate.Languages = suggestion.Languages
			candidate.Rules = suggestion.Rules
		}
		switch id {
		case AdapterIDSemgrep:
			candidate.Status = capabilityForSemgrep(detection, raw)
		case AdapterIDCargoAudit:
			candidate.Status = capabilityForCargoAudit(detection, raw, advisoryDatabasePath)
		case AdapterIDOSVScanner:
			candidate.Status = capabilityForOSV(detection, raw, osvDatabasePath)
		default:
			if !candidateSupportedFor(id, detection) {
				candidate.Status = CapabilityUnsupported
			} else if !executableResolvable(raw.Command) {
				candidate.Status = CapabilityUnavailable
			} else {
				candidate.Status = CapabilityAvailable
			}
		}
		if candidate.Status != CapabilityAvailable {
			candidate.Reason = capabilityReason(id, raw, detection, advisoryDatabasePath, osvDatabasePath)
		}
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func capabilityReason(id string, raw config.SecurityAdapterConfig, detection Detection, advisoryDatabasePath, osvDatabasePath string) string {
	switch id {
	case AdapterIDSemgrep:
		if len(detection.Languages) == 0 {
			return "no Go or Rust source detected"
		}
		if strings.TrimSpace(raw.Rules) == "" {
			return "local Semgrep rules are not configured"
		}
		if covered, err := coveredLanguages(raw.Rules, detection.Languages); err != nil || len(covered) == 0 {
			return "local Semgrep rules do not cover the detected source languages"
		}
		if !executableResolvable(raw.Command) {
			return "scanner executable is unavailable"
		}
	case AdapterIDCargoAudit:
		if reason := dependencyInventoryGap(detection, []Language{LanguageRust}, nil, raw.Lockfile); reason != "" {
			return reason
		}
		if !detection.HasLanguage(LanguageRust) || !detection.HasPresentLockfile(LanguageRust) {
			return "no Rust content with a Cargo.lock"
		}
		if strings.TrimSpace(raw.Version) == "" {
			return "cargo-audit requires an exact version pin"
		}
		database := firstNonEmpty(raw.Database, advisoryDatabasePath)
		if strings.TrimSpace(database) == "" || !databaseAvailable(database, detection.workspaceRoot()) {
			return "cargo-audit advisory database is not configured or unavailable"
		}
		if !executableResolvable(raw.Command) {
			return "scanner executable is unavailable"
		}
	case AdapterIDOSVScanner:
		declared := configuredLanguages(raw.Languages)
		if len(declared) == 0 {
			declared = []Language{LanguageGo, LanguageRust}
		}
		if reason := dependencyInventoryGap(detection, declared, raw.Inputs, ""); reason != "" {
			return reason
		}
		if len(selectedOSVLanguages(detection, raw)) == 0 {
			return "no go.mod or Cargo.lock inputs for the selected languages"
		}
		if strings.TrimSpace(raw.Version) == "" {
			return "osv-scanner requires an exact version pin"
		}
		database := firstNonEmpty(raw.Database, osvDatabasePath)
		if strings.TrimSpace(database) == "" || !osvDatabaseAvailable(database, detection.workspaceRoot(), selectedOSVLanguages(detection, raw)) {
			return "OSV offline database is not configured or unavailable"
		}
		if !executableResolvable(raw.Command) {
			return "scanner executable is unavailable"
		}
	default:
		if !candidateSupportedFor(id, detection) {
			return "detected content is not supported by this adapter"
		}
		if !executableResolvable(raw.Command) {
			return "scanner executable is unavailable"
		}
	}
	return ""
}

func capabilityForSemgrep(detection Detection, raw config.SecurityAdapterConfig) CapabilityStatus {
	if len(detection.Languages) == 0 {
		return CapabilityUnsupported
	}
	if strings.TrimSpace(raw.Rules) == "" {
		return CapabilityUnsupported
	}
	covered, err := coveredLanguages(raw.Rules, detection.Languages)
	if err != nil || len(covered) == 0 {
		return CapabilityUnsupported
	}
	if !executableResolvable(raw.Command) {
		return CapabilityUnavailable
	}
	return CapabilityAvailable
}

func capabilityForCargoAudit(detection Detection, raw config.SecurityAdapterConfig, databasePath string) CapabilityStatus {
	if dependencyInventoryGap(detection, []Language{LanguageRust}, nil, raw.Lockfile) != "" {
		return CapabilityUnsupported
	}
	if !detection.HasLanguage(LanguageRust) || !detection.HasPresentLockfile(LanguageRust) {
		return CapabilityUnsupported
	}
	if strings.TrimSpace(raw.Version) == "" {
		return CapabilityUnsupported
	}
	database := firstNonEmpty(raw.Database, databasePath)
	if strings.TrimSpace(database) == "" || !databaseAvailable(database, detection.workspaceRoot()) {
		return CapabilityUnsupported
	}
	if !executableResolvable(raw.Command) {
		return CapabilityUnavailable
	}
	return CapabilityAvailable
}

func capabilityForOSV(detection Detection, raw config.SecurityAdapterConfig, databasePath string) CapabilityStatus {
	declared := configuredLanguages(raw.Languages)
	if len(declared) == 0 {
		declared = []Language{LanguageGo, LanguageRust}
	}
	if dependencyInventoryGap(detection, declared, raw.Inputs, "") != "" {
		return CapabilityUnsupported
	}
	languages := selectedOSVLanguages(detection, raw)
	if len(languages) == 0 {
		return CapabilityUnsupported
	}
	if strings.TrimSpace(raw.Version) == "" {
		return CapabilityUnsupported
	}
	database := firstNonEmpty(raw.Database, databasePath)
	if strings.TrimSpace(database) == "" || !osvDatabaseAvailable(database, detection.workspaceRoot(), languages) {
		return CapabilityUnsupported
	}
	if !executableResolvable(raw.Command) {
		return CapabilityUnavailable
	}
	return CapabilityAvailable
}

func selectedOSVLanguages(detection Detection, raw config.SecurityAdapterConfig) []Language {
	declared := configuredLanguages(raw.Languages)
	if len(declared) == 0 {
		declared = []Language{LanguageGo, LanguageRust}
	}
	var out []Language
	for _, language := range declared {
		if osvInputAvailable(detection, language) {
			out = append(out, language)
		}
	}
	return out
}

// osvInputAvailable reports whether detect found the explicit input OSV needs for
// a language. Go uses go.mod; Rust uses a present Cargo.lock. Missing input is not
// clean coverage.
func osvInputAvailable(detection Detection, language Language) bool {
	switch language {
	case LanguageGo:
		for _, manifest := range detection.Manifests {
			if manifest.Language == LanguageGo {
				return true
			}
		}
		return false
	case LanguageRust:
		return detection.HasPresentLockfile(LanguageRust)
	default:
		return false
	}
}

// osvDatabaseAvailable reports whether every required ecosystem database exists
// under the explicit OSV cache root. It never downloads or updates a database.
func osvDatabaseAvailable(database, root string, languages []Language) bool {
	path := strings.TrimSpace(database)
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	base, _, ok := osvDatabaseLayout(path)
	if !ok {
		return false
	}
	for _, ecosystem := range osvEcosystems(languages) {
		if !isRegularFile(filepath.Join(base, ecosystem, "all.zip")) {
			return false
		}
	}
	return true
}

// databaseAvailable reports whether a configured local advisory database exists
// as a directory. It never reads or updates the database.
func databaseAvailable(database, root string) bool {
	path := strings.TrimSpace(database)
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// configuredDescriptor builds the descriptor for one configured adapter without
// starting it. An unknown built-in adapter id or category is an error, which the
// classifier reports as UNSUPPORTED.
func configuredDescriptor(raw config.SecurityAdapterConfig) (Descriptor, error) {
	id := strings.ToLower(strings.TrimSpace(raw.ID))
	category := Category(strings.ToLower(strings.TrimSpace(raw.Category)))
	protocol := strings.ToLower(strings.TrimSpace(raw.Protocol))
	if protocol == "" {
		protocol = config.SecurityProtocolBuiltin
	}
	switch protocol {
	case config.SecurityProtocolBuiltin:
		switch id {
		case AdapterIDSemgrep:
			if category == "" {
				category = CategorySAST
			}
		case AdapterIDGitleaks:
			if category == "" {
				category = CategorySecrets
			}
		case AdapterIDCargoAudit:
			if category == "" {
				category = CategoryDependencies
			}
		case AdapterIDOSVScanner:
			if category == "" {
				category = CategoryDependencies
			}
		default:
			return Descriptor{}, fmt.Errorf("security: unknown built-in adapter %q", id)
		}
	case config.SecurityProtocolExternal:
		if category == "" {
			return Descriptor{}, fmt.Errorf("security: adapter %q has no category", id)
		}
	default:
		return Descriptor{}, fmt.Errorf("security: adapter %q has unknown protocol %q", id, raw.Protocol)
	}
	descriptor := Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       id,
		AdapterVersion:  strings.TrimSpace(raw.Version),
		Categories:      []Category{category},
		ScopeModes:      []string{"workspace"},
		Locality:        "local",
		Languages:       configuredLanguages(raw.Languages),
	}
	if raw.RepositoryWide != nil {
		descriptor.RepositoryWide = *raw.RepositoryWide
	}
	if category == CategorySecrets {
		descriptor.RepositoryWide = true
	}
	if err := descriptor.Validate(); err != nil {
		return Descriptor{}, err
	}
	return descriptor, nil
}

// candidateSupportedFor reports whether an adapter id can cover the detected
// content. Built-in scanner adapters need at least one detected language.
func candidateSupportedFor(id string, detection Detection) bool {
	switch id {
	case AdapterIDSemgrep, AdapterIDGitleaks:
		return len(detection.Languages) > 0
	case AdapterIDCargoAudit:
		return detection.HasLanguage(LanguageRust) && detection.HasPresentLockfile(LanguageRust)
	case AdapterIDOSVScanner:
		return osvInputAvailable(detection, LanguageGo) || osvInputAvailable(detection, LanguageRust)
	default:
		return true
	}
}

// executableResolvable reports whether the adapter's executable name resolves on
// PATH. It never runs the executable, so classification cannot execute a scanner.
func executableResolvable(command []string) bool {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return false
	}
	name := strings.TrimSpace(command[0])
	if isAbsPath(name) || strings.ContainsAny(name, "/\\") {
		info, err := os.Stat(name)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(name)
	return err == nil
}
