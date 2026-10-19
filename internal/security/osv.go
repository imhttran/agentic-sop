package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// osvScannerStaleMaxAge is the freshness boundary for the explicit local OSV
// database. It matches the dependency-advisory freshness window and is evidence
// only: policy decides what a stale database means.
const osvScannerStaleMaxAge = 90 * 24 * time.Hour

// osvScannerConfigName is the empty override configuration written into a
// restricted temporary directory for every scan. Passing --config to it makes
// OSV ignore repository osv-scanner.toml files, whose IgnoredVulns and
// PackageOverrides could otherwise silently suppress SOP findings or reduce
// coverage.
const osvScannerConfigName = "sop-osv-config.toml"

// osvAdapter wraps the pinned OSV-Scanner CLI. It scans explicit go.mod and
// Cargo.lock inputs against an operator-provided offline database. It never
// downloads the tool or a database, never uses the live OSV API, never runs call
// analysis (which can invoke Go/Rust toolchains), and never writes a manifest or
// lockfile.
type osvAdapter struct {
	id           string
	category     Category
	command      []string
	databasePath string
	inputs       []string
	languages    []Language
	stalePolicy  string
	version      string
	configDigest string
	exec         Executor
	timeout      time.Duration
	maxOutput    int
}

func (a *osvAdapter) declaredLanguages() []Language {
	if len(a.languages) > 0 {
		return append([]Language(nil), a.languages...)
	}
	return []Language{LanguageGo, LanguageRust}
}

func (a *osvAdapter) Descriptor() Descriptor {
	return Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       a.id,
		AdapterVersion:  a.version,
		Categories:      []Category{a.category},
		ScopeModes:      []string{"workspace", "changed-files"},
		Locality:        "local",
		Network:         "none; --offline against an explicit local OSV database; call analysis disabled",
		Languages:       a.declaredLanguages(),
	}
}

// osvInput is one explicit dependency input.
type osvInput struct {
	Path      string
	Abs       string
	Language  Language
	Ecosystem string
}

// osvDatabase is one resolved ecosystem database.
type osvDatabase struct {
	Ecosystem string
	Path      string
	Digest    string
	Size      int64
	Modified  time.Time
	Source    string
	Metadata  string
}

func (a *osvAdapter) stalePolicyValue() string {
	if strings.EqualFold(strings.TrimSpace(a.stalePolicy), StaleAllow) {
		return StaleAllow
	}
	return StaleDeny
}

func (a *osvAdapter) currentProvenance(req Request) (Provenance, error) {
	prov := Provenance{
		ScannerVersion:      a.version,
		ConfigurationDigest: a.configDigest,
		ContentIdentity:     req.Content,
		Languages:           a.declaredLanguages(),
		// The scan always uses a controlled empty config override. Its digest is
		// deterministic, so resume can compare it without creating the temp file.
		RulesIdentity: DigestBytes(nil),
	}
	plan, err := a.resolveInputPlan(req)
	if err != nil {
		return prov, err
	}
	identity, err := osvInputIdentity(req.WorkspaceRoot, plan.Inputs)
	if err != nil {
		return prov, err
	}
	prov.LockfileIdentity = identity
	prov.ObligationIdentity = plan.Detection.ObligationIdentity(a.declaredLanguages()...)
	if len(plan.Inputs) == 0 {
		return prov, nil
	}
	databases, _, err := a.resolveDatabases(req, plan.Inputs)
	if err != nil {
		return prov, err
	}
	prov.AdvisoryDatabaseIdentity = osvDatabaseIdentity(databases)
	prov.AdvisoryDatabaseMetadata = osvDatabaseMetadata(databases)
	prov.AdvisoryDatabaseStale = osvDatabaseStale(databases, time.Now().UTC())
	return prov, nil
}

// osvInputPlan is the resolved input inventory for one OSV execution. Missing
// obligations are scanned-input gaps, not clean coverage.
type osvInputPlan struct {
	Detection   Detection
	Inputs      []osvInput
	Obligations []DependencyObligation
	Missing     []DependencyObligation
}

// resolveInputPlan returns the explicit or detected inputs plus every in-scope
// obligation, including obligations that are missing or omitted by an explicit
// input subset. Configured inputs are validated against the inventory rather than
// treated as complete language coverage.
func (a *osvAdapter) resolveInputPlan(req Request) (osvInputPlan, error) {
	selectedLanguages := a.declaredLanguages()
	selected := map[Language]bool{}
	for _, language := range selectedLanguages {
		selected[language] = true
	}
	detection, err := DetectRepositoryScoped(req.WorkspaceRoot, req.Scope.Excluded)
	if err != nil {
		return osvInputPlan{}, fmt.Errorf("resolve OSV inputs: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if len(detection.Issues) > 0 {
		return osvInputPlan{}, fmt.Errorf("dependency layout is unsupported or ambiguous: %s", SanitizeText(strings.Join(detection.Issues, "; "), MaxDiagnosticBytes))
	}
	plan := osvInputPlan{
		Detection:   detection,
		Obligations: detection.SelectedObligations(selectedLanguages...),
	}
	matcher := NewExcludeMatcher(req.Scope.Excluded)
	obligationByInput := map[string]DependencyObligation{}
	for _, obligation := range plan.Obligations {
		obligationByInput[obligation.Input] = obligation
	}

	if len(a.inputs) > 0 {
		covered := map[string]bool{}
		for _, raw := range a.inputs {
			path, reason := NormalizePath(raw)
			if path == "" {
				return osvInputPlan{}, fmt.Errorf("configured OSV input rejected: %s", reason)
			}
			if matcher.Excluded(path) {
				return osvInputPlan{}, fmt.Errorf("configured OSV input %s is outside the declared scope", path)
			}
			obligation, ok := obligationByInput[path]
			if !ok {
				return osvInputPlan{}, fmt.Errorf("configured OSV input %s is not an in-scope dependency obligation", path)
			}
			if !selected[obligation.Language] {
				return osvInputPlan{}, fmt.Errorf("configured OSV input %s is %s but the adapter languages exclude it", path, obligation.Language)
			}
			abs := joinPath(req.WorkspaceRoot, path)
			if err := verifyOSVInput(abs, path); err != nil {
				return osvInputPlan{}, err
			}
			plan.Inputs = append(plan.Inputs, osvInput{Path: path, Abs: abs, Language: obligation.Language, Ecosystem: osvEcosystem(obligation.Language)})
			covered[path] = true
		}
		plan.Inputs = sortOSVInputs(plan.Inputs)
		for _, obligation := range plan.Obligations {
			if covered[obligation.Input] {
				continue
			}
			missing := obligation
			if missing.Present {
				missing.Issues = append(missing.Issues, "input is present but omitted by the explicit OSV inputs list")
			}
			plan.Missing = append(plan.Missing, missing)
		}
		return plan, nil
	}

	for _, obligation := range plan.Obligations {
		if !obligation.Present {
			plan.Missing = append(plan.Missing, obligation)
			continue
		}
		abs := joinPath(req.WorkspaceRoot, obligation.Input)
		if err := verifyOSVInput(abs, obligation.Input); err != nil {
			return osvInputPlan{}, err
		}
		plan.Inputs = append(plan.Inputs, osvInput{Path: obligation.Input, Abs: abs, Language: obligation.Language, Ecosystem: osvEcosystem(obligation.Language)})
	}
	plan.Inputs = sortOSVInputs(plan.Inputs)
	return plan, nil
}

func osvApplicableLabel(languages []Language) string {
	if len(languages) == 1 {
		switch languages[0] {
		case LanguageGo:
			return "Go"
		case LanguageRust:
			return "Rust"
		}
	}
	return "Go or Rust"
}

func missingObligationDiagnostic(obligation DependencyObligation) string {
	reason := fmt.Sprintf("missing dependency input %s for %s", obligation.Input, obligation.Manifest)
	if len(obligation.Issues) > 0 {
		reason += ": " + strings.Join(obligation.Issues, "; ")
	}
	return SanitizeText(reason, MaxDiagnosticBytes)
}

func osvEcosystem(language Language) string {
	if language == LanguageGo {
		return "Go"
	}
	return "crates.io"
}

func sortOSVInputs(inputs []osvInput) []osvInput {
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].Path != inputs[j].Path {
			return inputs[i].Path < inputs[j].Path
		}
		return inputs[i].Language < inputs[j].Language
	})
	return inputs
}

func verifyOSVInput(abs, rel string) error {
	info, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("OSV input %s is unavailable: %s", rel, SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("OSV input %s is a symbolic link; refusing to read outside the workspace", rel)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("OSV input %s is not a regular file", rel)
	}
	return nil
}

// osvInputIdentity is the deterministic resume key for the resolved input files.
func osvInputIdentity(root string, inputs []osvInput) (string, error) {
	if len(inputs) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		digest, err := rulesIdentity(joinPath(root, input.Path))
		if err != nil {
			return "", fmt.Errorf("OSV input identity for %s is unavailable: %s", input.Path, SanitizeText(err.Error(), MaxDiagnosticBytes))
		}
		if strings.TrimSpace(digest) == "" {
			return "", fmt.Errorf("OSV input identity for %s is empty", input.Path)
		}
		parts = append(parts, "path="+input.Path+";language="+string(input.Language)+";digest="+digest)
	}
	return strings.Join(parts, ";"), nil
}

func osvEcosystems(languages []Language) []string {
	set := map[string]bool{}
	for _, language := range languages {
		switch language {
		case LanguageGo:
			set["Go"] = true
		case LanguageRust:
			set["crates.io"] = true
		}
	}
	out := make([]string, 0, len(set))
	for ecosystem := range set {
		out = append(out, ecosystem)
	}
	sort.Strings(out)
	return out
}

// osvDatabaseLayout resolves the cache layout once. OSV v2.7 reads
// <cache>/osv-scalibr/<ecosystem>/all.zip; the older documented
// <cache>/osv-scanner/<ecosystem>/all.zip is also accepted. Mixed layouts are
// rejected because the scanner receives one environment cache root.
func osvDatabaseLayout(root string) (base, cacheRoot string, ok bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", "", false
	}
	if base := strings.ToLower(filepath.Base(root)); base == "osv-scalibr" || base == "osv-scanner" {
		return root, filepath.Dir(root), true
	}
	for _, layout := range []string{"osv-scalibr", "osv-scanner"} {
		candidate := filepath.Join(root, layout)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, root, true
		}
	}
	return "", "", false
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// resolveDatabases resolves every ecosystem database required by the inputs and
// returns one shared cache root. It never downloads or updates a database.
func (a *osvAdapter) resolveDatabases(req Request, inputs []osvInput) ([]osvDatabase, string, error) {
	raw := strings.TrimSpace(a.databasePath)
	if raw == "" {
		return nil, "", fmt.Errorf("OSV database path is not configured; implicit download and the live OSV API are disabled")
	}
	root := raw
	if !isAbsPath(root) {
		root = filepath.Join(req.WorkspaceRoot, filepath.FromSlash(root))
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, "", fmt.Errorf("OSV database cache is unavailable: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("OSV database cache %q is not a directory", filepath.ToSlash(raw))
	}
	languages := map[Language]bool{}
	for _, input := range inputs {
		languages[input.Language] = true
	}
	orderedLanguages := make([]Language, 0, len(languages))
	for language := range languages {
		orderedLanguages = append(orderedLanguages, language)
	}
	ecosystems := osvEcosystems(orderedLanguages)
	if len(ecosystems) == 0 {
		return nil, "", nil
	}
	// Resolve the layout once so every ecosystem comes from the same cache root.
	base, layoutRoot, ok := osvDatabaseLayout(root)
	if !ok {
		return nil, "", fmt.Errorf("OSV database cache %q has no osv-scalibr or osv-scanner layout", filepath.ToSlash(raw))
	}
	var databases []osvDatabase
	for _, ecosystem := range ecosystems {
		path := filepath.Join(base, ecosystem, "all.zip")
		if !isRegularFile(path) {
			return nil, "", fmt.Errorf("OSV database for ecosystem %s is missing; expected %s", ecosystem, filepath.ToSlash(path))
		}
		db, err := inspectOSVDatabase(ecosystem, path)
		if err != nil {
			return nil, "", err
		}
		databases = append(databases, db)
	}
	sort.Slice(databases, func(i, j int) bool { return databases[i].Ecosystem < databases[j].Ecosystem })
	return databases, layoutRoot, nil
}

func inspectOSVDatabase(ecosystem, path string) (osvDatabase, error) {
	info, err := os.Stat(path)
	if err != nil {
		return osvDatabase{}, fmt.Errorf("OSV database for %s is unavailable: %s", ecosystem, SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	digest, err := rulesIdentity(path)
	if err != nil || strings.TrimSpace(digest) == "" {
		return osvDatabase{}, fmt.Errorf("OSV database digest for %s is unavailable", ecosystem)
	}
	db := osvDatabase{
		Ecosystem: ecosystem,
		Path:      path,
		Digest:    digest,
		Size:      info.Size(),
		Modified:  info.ModTime().UTC(),
	}
	if data, err := os.ReadFile(path + ".metadata.json"); err == nil {
		var meta struct {
			Source       string `json:"source"`
			SHA256       string `json:"sha256"`
			DownloadedAt string `json:"downloaded_at"`
		}
		if json.Unmarshal(data, &meta) == nil {
			if strings.TrimSpace(meta.SHA256) != "" && !strings.EqualFold(strings.TrimSpace(meta.SHA256), digest) {
				return osvDatabase{}, fmt.Errorf("OSV database digest for %s does not match its acquisition metadata", ecosystem)
			}
			db.Source = SanitizeText(meta.Source, MaxMessageBytes)
			parts := []string{"ecosystem=" + ecosystem, "digest=" + digest,
				fmt.Sprintf("size=%d", db.Size), "modified=" + db.Modified.Format(time.RFC3339)}
			if db.Source != "" {
				parts = append(parts, "source="+db.Source)
			}
			if strings.TrimSpace(meta.DownloadedAt) != "" {
				parts = append(parts, "downloaded_at="+SanitizeText(meta.DownloadedAt, 128))
			}
			db.Metadata = strings.Join(parts, ";")
			return db, nil
		}
	}
	db.Metadata = fmt.Sprintf("ecosystem=%s;digest=%s;size=%d;modified=%s", ecosystem, digest, db.Size, db.Modified.Format(time.RFC3339))
	return db, nil
}

func osvDatabaseIdentity(databases []osvDatabase) string {
	parts := make([]string, 0, len(databases))
	for _, db := range databases {
		parts = append(parts, fmt.Sprintf("ecosystem=%s;digest=%s;size=%d;modified=%s",
			db.Ecosystem, db.Digest, db.Size, db.Modified.Format(time.RFC3339)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func osvDatabaseMetadata(databases []osvDatabase) string {
	parts := make([]string, 0, len(databases))
	for _, db := range databases {
		if strings.TrimSpace(db.Metadata) != "" {
			parts = append(parts, db.Metadata)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

// osvDatabaseStale reports whether any required database is older than the
// documented freshness boundary. Missing freshness evidence is treated as stale.
func osvDatabaseStale(databases []osvDatabase, now time.Time) bool {
	for _, db := range databases {
		if db.Modified.IsZero() || now.Sub(db.Modified) > osvScannerStaleMaxAge {
			return true
		}
	}
	return false
}

func normalizeOSVVersion(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if !isCargoVersion(value) {
		return ""
	}
	return value
}

// parseOSVVersion extracts the scanner version from `osv-scanner --version`.
// Unrecognized output yields an empty value instead of a guess; the output also
// contains an osv-scalibr version and build metadata.
func parseOSVVersion(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if !strings.Contains(lower, "osv-scanner") || !strings.Contains(lower, "version") {
			continue
		}
		value := line
		if index := strings.IndexAny(value, ":="); index >= 0 {
			value = value[index+1:]
		} else if fields := strings.Fields(value); len(fields) > 0 {
			value = fields[len(fields)-1]
		}
		if version := normalizeOSVVersion(value); version != "" {
			return version
		}
	}
	return ""
}

func (a *osvAdapter) verifyToolVersion(ctx context.Context, workDir, cacheRoot string) (string, error) {
	pinned := normalizeOSVVersion(a.version)
	if pinned == "" {
		return "", fmt.Errorf("osv-scanner version is not pinned; configure an exact version so an incompatible tool cannot produce a clean result")
	}
	argv := append([]string(nil), a.command...)
	if len(argv) == 0 {
		argv = []string{"osv-scanner"}
	}
	argv = append(argv, "--version")
	runCtx, cancel := scanContext(ctx, a.timeout)
	defer cancel()
	stdout, _, _, err := a.runEnv(runCtx, workDir, argv, cacheRoot, nil)
	if err != nil {
		return "", err
	}
	actual := parseOSVVersion(string(stdout))
	if actual == "" {
		return "", fmt.Errorf("osv-scanner version output is unrecognized: %s", SanitizeText(strings.TrimSpace(string(stdout)), MaxDiagnosticBytes))
	}
	if actual != pinned {
		return "", fmt.Errorf("osv-scanner version mismatch: found %s, want %s", actual, pinned)
	}
	return actual, nil
}

func (a *osvAdapter) runEnv(ctx context.Context, dir string, argv []string, cacheRoot string, stdin []byte) ([]byte, []byte, int, error) {
	executor, ok := a.exec.(EnvExecutor)
	if !ok {
		return nil, nil, -1, fmt.Errorf("executor does not support an explicit environment; offline OSV mode cannot be guaranteed")
	}
	env := []string{}
	if strings.TrimSpace(cacheRoot) != "" {
		env = append(env, "OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY="+cacheRoot)
	}
	return executor.RunEnv(ctx, dir, argv, env, stdin, a.maxOutput)
}

type osvReport struct {
	Results *[]osvResult `json:"results"`
}

type osvResult struct {
	Source struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"source"`
	Packages []osvPackage `json:"packages"`
}

type osvPackage struct {
	Package struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Ecosystem string `json:"ecosystem"`
		PURL      string `json:"purl"`
	} `json:"package"`
	Vulnerabilities []osvVulnerability `json:"vulnerabilities"`
	Groups          []osvGroup         `json:"groups"`
}

type osvVulnerability struct {
	ID               string         `json:"id"`
	Aliases          []string       `json:"aliases"`
	Summary          string         `json:"summary"`
	Details          string         `json:"details"`
	Published        string         `json:"published"`
	Modified         string         `json:"modified"`
	Severity         []osvSeverity  `json:"severity"`
	Affected         []osvAffected  `json:"affected"`
	References       []osvReference `json:"references"`
	DatabaseSpecific map[string]any `json:"database_specific"`
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type osvAffected struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
		PURL      string `json:"purl"`
	} `json:"package"`
	Ranges []struct {
		Type   string              `json:"type"`
		Events []map[string]string `json:"events"`
	} `json:"ranges"`
	Versions         []string       `json:"versions"`
	DatabaseSpecific map[string]any `json:"database_specific"`
}

type osvReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type osvGroup struct {
	IDs                  []string               `json:"ids"`
	Aliases              []string               `json:"aliases"`
	MaxSeverity          string                 `json:"max_severity"`
	ExperimentalAnalysis map[string]osvAnalysis `json:"experimentalAnalysis"`
	ExperimentalSnake    map[string]osvAnalysis `json:"experimental_analysis"`
}

type osvAnalysis struct {
	Called *bool `json:"called"`
}

// Scan runs OSV-Scanner once per explicit input so one input's failure cannot
// hide another input's result. It uses a controlled empty --config so repository
// scanner configuration cannot suppress SOP findings.
func (a *osvAdapter) Scan(ctx context.Context, req Request) (Execution, error) {
	exec := newExecution(a.id, a.category, a.version, a.configDigest, "", req)
	exec.Provenance.ScannerVersion = a.version

	plan, err := a.resolveInputPlan(req)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	exec.Obligations = plan.Obligations
	exec.Provenance.ObligationIdentity = plan.Detection.ObligationIdentity(a.declaredLanguages()...)
	inputs := plan.Inputs
	if len(inputs) == 0 {
		if len(plan.Missing) > 0 {
			exec.Status = StatusFailed
			exec.Coverage = CoverageUnknown
			for _, obligation := range plan.Missing {
				exec.Diagnostics = append(exec.Diagnostics, missingObligationDiagnostic(obligation))
			}
			sanitizeExecution(&exec)
			return finish(exec, nil)
		}
		exec.Status = StatusCompleted
		exec.Coverage = CoverageComplete
		exec.NoApplicableContent = true
		exec.Provenance.LockfileIdentity = ""
		exec.Diagnostics = []string{"no applicable " + osvApplicableLabel(a.declaredLanguages()) + " dependency content; scanner was not executed"}
		sanitizeExecution(&exec)
		return finish(exec, nil)
	}
	identity, err := osvInputIdentity(req.WorkspaceRoot, inputs)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	exec.Provenance.LockfileIdentity = identity

	databases, cacheRoot, err := a.resolveDatabases(req, inputs)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	exec.Provenance.AdvisoryDatabaseIdentity = osvDatabaseIdentity(databases)
	exec.Provenance.AdvisoryDatabaseMetadata = osvDatabaseMetadata(databases)
	exec.Provenance.AdvisoryDatabaseStale = osvDatabaseStale(databases, time.Now().UTC())

	workDir, err := os.MkdirTemp("", "sop-osv-scanner-*")
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{"create restricted osv-scanner directory: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	defer os.RemoveAll(workDir)
	configPath := filepath.Join(workDir, osvScannerConfigName)
	if err := os.WriteFile(configPath, []byte{}, 0o600); err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{"write controlled osv-scanner config: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	configIdentity, err := rulesIdentity(configPath)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{"controlled osv-scanner config identity is unavailable: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	exec.Provenance.RulesIdentity = configIdentity

	actualVersion, versionErr := a.verifyToolVersion(ctx, workDir, cacheRoot)
	if versionErr != nil {
		switch {
		case errors.Is(versionErr, ErrUnavailable):
			exec.Status = StatusUnavailable
			exec.Diagnostics = append(exec.Diagnostics, "osv-scanner executable is unavailable")
		case errors.Is(versionErr, context.DeadlineExceeded):
			exec.Status = StatusTimedOut
			exec.Diagnostics = append(exec.Diagnostics, "osv-scanner version check exceeded the scan deadline")
		case errors.Is(versionErr, context.Canceled):
			exec.Status = StatusCancelled
			exec.Diagnostics = append(exec.Diagnostics, "osv-scanner version check was cancelled")
		default:
			exec.Status = StatusFailed
			exec.Diagnostics = append(exec.Diagnostics, SanitizeText(versionErr.Error(), MaxDiagnosticBytes))
		}
		exec.Coverage = CoverageUnknown
		sanitizeExecution(&exec)
		return finish(exec, nil)
	}
	exec.AdapterVersion = actualVersion
	exec.Provenance.ScannerVersion = actualVersion

	var (
		rawReports    []json.RawMessage
		successes     int
		failures      int
		covered       map[Language]bool
		advisoryTotal int
	)
	covered = map[Language]bool{}
	for _, input := range inputs {
		report, stdout, exitCode, diagnostics, scanErr := a.scanInput(ctx, workDir, configPath, cacheRoot, input)
		exec.ExitCode = exitCode
		exec.Diagnostics = append(exec.Diagnostics, diagnostics...)
		if scanErr != nil {
			switch {
			case errors.Is(scanErr, ErrUnavailable):
				exec.Status = StatusUnavailable
				exec.Coverage = CoverageUnknown
				exec.Diagnostics = append(exec.Diagnostics, "osv-scanner executable is unavailable")
				sanitizeExecution(&exec)
				return finish(exec, nil)
			case errors.Is(scanErr, context.DeadlineExceeded):
				exec.Status = StatusTimedOut
				exec.Coverage = CoverageUnknown
				exec.Diagnostics = append(exec.Diagnostics, "osv-scanner exceeded the scan deadline")
				sanitizeExecution(&exec)
				return finish(exec, nil)
			case errors.Is(scanErr, context.Canceled):
				exec.Status = StatusCancelled
				exec.Coverage = CoverageUnknown
				exec.Diagnostics = append(exec.Diagnostics, "osv-scanner was cancelled")
				sanitizeExecution(&exec)
				return finish(exec, nil)
			default:
				failures++
				exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("osv-scanner failed for %s: %s", input.Path, SanitizeText(scanErr.Error(), MaxDiagnosticBytes)))
				continue
			}
		}
		findings := normalizeOSVReport(report, input)
		advisoryTotal += len(findings)
		exec.Findings = append(exec.Findings, findings...)
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("osv-scanner %s: %d package(s), %d advisory group(s)", input.Path, osvPackageCount(report), len(findings)))
		covered[input.Language] = true
		rawReports = append(rawReports, append(json.RawMessage(nil), sanitizeOSVNativeReport(stdout, req.WorkspaceRoot, inputs)...))
		successes++
	}

	exec.Languages = sortedLanguages(covered)
	missingObligations := len(plan.Missing) > 0
	if missingObligations {
		for _, obligation := range plan.Missing {
			exec.Diagnostics = append(exec.Diagnostics, missingObligationDiagnostic(obligation))
		}
	}
	switch {
	case failures > 0 && successes == 0:
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
	case failures > 0:
		exec.Status = StatusFailed
		exec.Coverage = CoveragePartial
	case missingObligations && successes == 0:
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
	case missingObligations:
		exec.Status = StatusFailed
		exec.Coverage = CoveragePartial
	case successes == 0:
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = append(exec.Diagnostics, "no osv-scanner report was produced")
	default:
		exec.Coverage = CoverageComplete
	}

	if exec.Provenance.AdvisoryDatabaseStale {
		if a.stalePolicyValue() == StaleAllow {
			if exec.Coverage == CoverageComplete {
				exec.Coverage = CoveragePartial
			}
			exec.Diagnostics = append(exec.Diagnostics, "OSV database is stale; coverage is PARTIAL (policy: allow)")
		} else {
			exec.Diagnostics = append(exec.Diagnostics, "OSV database is stale (policy: deny)")
		}
	}
	if req.Scope.Mode == "changed-files" {
		if exec.Coverage == CoverageComplete {
			exec.Coverage = CoveragePartial
		}
		exec.Diagnostics = append(exec.Diagnostics, "changed-files scope: dependency audit is workspace-wide and was not limited to changed files")
	}

	if len(rawReports) > 0 {
		native := rawReports[0]
		if len(rawReports) > 1 {
			native = marshalReports(rawReports)
		}
		sanitized, digest, serr := SanitizeNativeReport(native)
		if serr == nil {
			exec.NativeReport = sanitized
			exec.NativeReportDigest = digest
			exec.Provenance.SanitizedReportDigest = digest
		} else {
			exec.Status = StatusFailed
			exec.Coverage = CoverageUnknown
			exec.Diagnostics = append(exec.Diagnostics, "native report sanitization failed: "+SanitizeText(serr.Error(), MaxDiagnosticBytes))
		}
	}
	exec.Diagnostics = sanitizeList(exec.Diagnostics)
	sanitizeExecution(&exec)
	return finish(exec, nil)
}

func (a *osvAdapter) scanInput(ctx context.Context, workDir, configPath, cacheRoot string, input osvInput) (osvReport, []byte, int, []string, error) {
	argv := append([]string(nil), a.command...)
	if len(argv) == 0 {
		argv = []string{"osv-scanner"}
	}
	argv = append(argv,
		"scan", "source",
		"--format", "json",
		"--offline",
		"--all-packages",
		"--no-resolve",
		"--no-call-analysis=all",
		"--config", configPath,
		"--lockfile", input.Abs,
	)
	runCtx, cancel := scanContext(ctx, a.timeout)
	defer cancel()
	stdout, stderr, exitCode, runErr := a.runEnv(runCtx, workDir, argv, cacheRoot, nil)
	var diagnostics []string
	if len(stderr) > 0 {
		diagnostics = append(diagnostics, SanitizeText(sanitizeOSVWorkspacePaths(string(stderr), input.Abs, input.Path), MaxDiagnosticBytes))
	}
	if runErr != nil {
		return osvReport{}, stdout, exitCode, diagnostics, runErr
	}
	if len(stdout) == 0 {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner returned empty output for %s", input.Path)
	}
	if a.maxOutput > 0 && len(stdout) >= a.maxOutput {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner output for %s reached the %d-byte limit", input.Path, a.maxOutput)
	}
	if exitCode != 0 && exitCode != 1 {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner exited with code %d for %s", exitCode, input.Path)
	}
	var report osvReport
	if err := json.Unmarshal(stdout, &report); err != nil {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner returned malformed JSON for %s: %s", input.Path, SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if report.Results == nil {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner report for %s has no results field", input.Path)
	}
	found := false
	for _, result := range *report.Results {
		if result.Source.Path == "" || !osvSourceMatches(result.Source.Path, input.Abs, input.Path) {
			continue
		}
		found = true
	}
	if len(*report.Results) > 0 && !found {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner report for %s does not identify that input", input.Path)
	}
	hasVulnerabilities := osvReportHasVulnerabilities(report)
	if exitCode == 1 && !hasVulnerabilities {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner exited with code 1 for %s but reported no vulnerabilities", input.Path)
	}
	if exitCode == 0 && hasVulnerabilities {
		return osvReport{}, stdout, exitCode, diagnostics, fmt.Errorf("osv-scanner exited with code 0 for %s but reported vulnerabilities", input.Path)
	}
	return report, stdout, exitCode, diagnostics, nil
}

func osvSourceMatches(source, abs, rel string) bool {
	if filepath.IsAbs(source) {
		return filepath.Clean(source) == filepath.Clean(abs)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(source)))
	return clean == rel || clean == "./"+rel
}

func osvReportHasVulnerabilities(report osvReport) bool {
	if report.Results == nil {
		return false
	}
	for _, result := range *report.Results {
		for _, pkg := range result.Packages {
			if len(pkg.Vulnerabilities) > 0 || len(pkg.Groups) > 0 {
				return true
			}
		}
	}
	return false
}

func osvPackageCount(report osvReport) int {
	if report.Results == nil {
		return 0
	}
	total := 0
	for _, result := range *report.Results {
		total += len(result.Packages)
	}
	return total
}

// normalizeOSVReport maps OSV's package results into normalized dependency
// findings. Groups are OSV's explicit alias grouping; one finding is emitted per
// group and input. Different packages, versions, and inputs are never merged.
func normalizeOSVReport(report osvReport, input osvInput) []Finding {
	if report.Results == nil {
		return nil
	}
	var findings []Finding
	for _, result := range *report.Results {
		for _, pkg := range result.Packages {
			findings = append(findings, normalizeOSVPackage(pkg, input)...)
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Fingerprint < findings[j].Fingerprint
	})
	return findings
}

func normalizeOSVPackage(pkg osvPackage, input osvInput) []Finding {
	byID := map[string]osvVulnerability{}
	for _, vulnerability := range pkg.Vulnerabilities {
		byID[vulnerability.ID] = vulnerability
	}
	var out []Finding
	emitted := map[string]bool{}
	if len(pkg.Groups) > 0 {
		for _, group := range pkg.Groups {
			if len(group.IDs) == 0 {
				continue
			}
			var vulnerabilities []osvVulnerability
			for _, id := range group.IDs {
				if vulnerability, ok := byID[id]; ok {
					vulnerabilities = append(vulnerabilities, vulnerability)
					emitted[id] = true
				}
			}
			primary := group.IDs[0]
			if primary == "" {
				continue
			}
			out = append(out, buildOSVFinding(primary, group, vulnerabilities, pkg, input))
		}
	}
	for _, vulnerability := range pkg.Vulnerabilities {
		if emitted[vulnerability.ID] || vulnerability.ID == "" {
			continue
		}
		group := osvGroup{IDs: []string{vulnerability.ID}, Aliases: append([]string(nil), vulnerability.Aliases...)}
		out = append(out, buildOSVFinding(vulnerability.ID, group, []osvVulnerability{vulnerability}, pkg, input))
	}
	return out
}

func buildOSVFinding(primary string, group osvGroup, vulnerabilities []osvVulnerability, pkg osvPackage, input osvInput) Finding {
	name := strings.TrimSpace(pkg.Package.Name)
	version := strings.TrimSpace(pkg.Package.Version)
	ecosystem := strings.TrimSpace(pkg.Package.Ecosystem)
	if ecosystem == "" {
		ecosystem = input.Ecosystem
	}
	aliases := uniqueStrings(append(append([]string(nil), group.Aliases...), group.IDs...))
	var summary string
	var references []string
	var advisoryIDs []string
	var fixed []string
	var affectedRanges []string
	var severityEvidence []string
	var cweIDs []string
	var analysisNotes []string
	severity := SeverityUnknown
	for _, vulnerability := range vulnerabilities {
		advisoryIDs = append(advisoryIDs, vulnerability.ID)
		aliases = append(aliases, vulnerability.Aliases...)
		summary = firstNonEmpty(summary, vulnerability.Summary, vulnerability.Details)
		for _, reference := range vulnerability.References {
			if strings.TrimSpace(reference.URL) != "" {
				references = append(references, reference.URL)
			}
		}
		for _, affected := range vulnerability.Affected {
			if affected.Package.Name != "" && !strings.EqualFold(affected.Package.Name, name) {
				continue
			}
			if affected.Package.Ecosystem != "" && !strings.EqualFold(affected.Package.Ecosystem, ecosystem) {
				continue
			}
			for _, r := range affected.Ranges {
				affectedRanges = append(affectedRanges, renderOSVRange(r.Type, r.Events))
				for _, event := range r.Events {
					if value := strings.TrimSpace(event["fixed"]); value != "" {
						fixed = append(fixed, value)
					}
				}
			}
			for _, value := range affected.Versions {
				if strings.TrimSpace(value) != "" {
					affectedRanges = append(affectedRanges, "version:"+value)
				}
			}
			if values := databaseStringList(affected.DatabaseSpecific, "cwe_ids", "cwe"); len(values) > 0 {
				cweIDs = append(cweIDs, values...)
			}
		}
		for _, entry := range vulnerability.Severity {
			score := osvSeverityScore(entry)
			if score == "" {
				continue
			}
			severityEvidence = append(severityEvidence, entry.Type+":"+score)
			if mapped := osvSeverityFromScore(score); rankSeverity(mapped) > rankSeverity(severity) {
				severity = mapped
			}
		}
		if value := databaseString(vulnerability.DatabaseSpecific, "severity"); value != "" {
			severityEvidence = append(severityEvidence, "database_specific:"+value)
			if mapped := NormalizeSeverity("osv-scanner", value); rankSeverity(mapped) > rankSeverity(severity) {
				severity = mapped
			}
		}
		if values := databaseStringList(vulnerability.DatabaseSpecific, "cwe_ids", "cwe"); len(values) > 0 {
			cweIDs = append(cweIDs, values...)
		}
	}
	if strings.TrimSpace(group.MaxSeverity) != "" {
		severityEvidence = append(severityEvidence, "group_max_severity:"+group.MaxSeverity)
		if mapped := osvSeverityFromScore(group.MaxSeverity); rankSeverity(mapped) > rankSeverity(severity) {
			severity = mapped
		}
	}
	for id, analysis := range group.ExperimentalAnalysis {
		if analysis.Called != nil {
			analysisNotes = append(analysisNotes, fmt.Sprintf("%s.called=%t", id, *analysis.Called))
		}
	}
	for id, analysis := range group.ExperimentalSnake {
		if analysis.Called != nil {
			analysisNotes = append(analysisNotes, fmt.Sprintf("%s.called=%t", id, *analysis.Called))
		}
	}
	aliases = uniqueStrings(aliases)
	references = uniqueStrings(references)
	fixed = uniqueStrings(fixed)
	affectedRanges = uniqueStrings(affectedRanges)
	severityEvidence = uniqueStrings(severityEvidence)
	cweIDs = uniqueStrings(cweIDs)
	analysisNotes = uniqueStrings(analysisNotes)

	meta := map[string]string{
		"package":    SanitizeText(name, 256),
		"ecosystem":  SanitizeText(ecosystem, 128),
		"input":      SanitizeText(input.Path, 256),
		"input_type": SanitizeText(filepath.Base(filepath.FromSlash(input.Path)), 64),
		// Reachability is not inferred from a version match. Call analysis is
		// deliberately disabled, so the absence of evidence is explicit.
		"reachability": "not-evaluated",
	}
	if version != "" {
		meta["version"] = SanitizeText(version, 128)
	}
	if pkg.Package.PURL != "" {
		meta["purl"] = SanitizeText(pkg.Package.PURL, MaxMessageBytes)
	}
	if len(aliases) > 0 {
		meta["aliases"] = SanitizeText(strings.Join(aliases, ","), MaxMessageBytes)
	}
	if len(advisoryIDs) > 0 {
		meta["advisory_ids"] = SanitizeText(strings.Join(uniqueStrings(advisoryIDs), ","), MaxMessageBytes)
	}
	if len(fixed) > 0 {
		meta["fixed_versions"] = SanitizeText(strings.Join(fixed, ","), MaxMessageBytes)
	}
	if len(affectedRanges) > 0 {
		meta["affected_ranges"] = SanitizeText(strings.Join(affectedRanges, ";"), MaxMessageBytes)
	}
	if len(references) > 0 {
		meta["references"] = SanitizeText(strings.Join(references, ","), MaxMessageBytes)
		meta["url"] = SanitizeText(references[0], MaxMessageBytes)
	}
	if len(severityEvidence) > 0 {
		meta["severity_evidence"] = SanitizeText(strings.Join(severityEvidence, ";"), MaxMessageBytes)
	}
	if len(cweIDs) > 0 {
		meta["cwe_ids"] = SanitizeText(strings.Join(cweIDs, ","), MaxMessageBytes)
	}
	if len(analysisNotes) > 0 {
		meta["reachability"] = SanitizeText(strings.Join(analysisNotes, ","), MaxMessageBytes)
	}

	message := strings.TrimSpace(summary)
	if message == "" {
		message = primary
	}
	if name != "" {
		label := name
		if version != "" {
			label += "@" + version
		}
		if ecosystem != "" {
			label += " (" + ecosystem + ")"
		}
		message = label + ": " + message
	}
	if len(fixed) > 0 {
		message += " (fixed: " + strings.Join(fixed, ", ") + ")"
	}

	finding := Finding{
		AdapterID:        "osv-scanner",
		Provider:         "osv-scanner",
		Category:         CategoryDependencies,
		RuleID:           SanitizeText(primary, MaxMessageBytes),
		OriginalSeverity: SanitizeText(strings.Join(severityEvidence, ";"), 256),
		Severity:         severity,
		Message:          SanitizeText(message, MaxMessageBytes),
		Path:             input.Path,
		Fingerprint:      StableFingerprint("osv-scanner", primary+"|"+ecosystem+"|"+name+"|"+version, input.Path, 0, 0),
		Metadata:         meta,
		Confidence:       "HIGH",
	}
	for _, alias := range aliases {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(alias)), "CVE-") {
			finding.CVE = append(finding.CVE, SanitizeText(alias, 64))
		}
	}
	finding.CVE = uniqueStrings(finding.CVE)
	if len(cweIDs) > 0 {
		finding.CWE = cweIDs
	}
	return finding
}

func renderOSVRange(kind string, events []map[string]string) string {
	var parts []string
	for _, event := range events {
		for _, key := range []string{"introduced", "fixed", "last_affected", "limit"} {
			if value := strings.TrimSpace(event[key]); value != "" {
				parts = append(parts, key+"="+value)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if kind != "" {
		return kind + ":" + strings.Join(parts, ",")
	}
	return strings.Join(parts, ",")
}

func osvSeverityScore(entry osvSeverity) string {
	score := strings.TrimSpace(entry.Score)
	if score == "" {
		return ""
	}
	if _, err := strconv.ParseFloat(score, 64); err == nil {
		return score
	}
	return score
}

func osvSeverityFromScore(raw string) Severity {
	value := strings.TrimSpace(raw)
	if value == "" {
		return SeverityUnknown
	}
	if score, err := strconv.ParseFloat(value, 64); err == nil {
		return severityForCVSS(score)
	}
	if score, ok := cvss31BaseScore(value); ok {
		return severityForCVSS(score)
	}
	return SeverityUnknown
}

func rankSeverity(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 5
	case SeverityHigh:
		return 4
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

func databaseString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	if value, ok := values[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func databaseStringList(values map[string]any, keys ...string) []string {
	if len(values) == 0 {
		return nil
	}
	var out []string
	for _, key := range keys {
		switch value := values[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				out = append(out, value)
			}
		case []any:
			for _, item := range value {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
		}
	}
	return uniqueStrings(out)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// sanitizeOSVNativeReport rewrites absolute input paths to repository-relative
// paths before the native report is persisted.
func sanitizeOSVNativeReport(raw []byte, root string, inputs []osvInput) []byte {
	var report osvReport
	if json.Unmarshal(raw, &report) != nil || report.Results == nil {
		return raw
	}
	absToRel := map[string]string{}
	for _, input := range inputs {
		absToRel[filepath.Clean(input.Abs)] = input.Path
	}
	for i := range *report.Results {
		result := &(*report.Results)[i]
		if result.Source.Path == "" {
			continue
		}
		clean := filepath.Clean(result.Source.Path)
		if rel, ok := absToRel[clean]; ok {
			result.Source.Path = rel
			continue
		}
		if filepath.IsAbs(result.Source.Path) {
			if rel, reason := relativeFindingPath(root, result.Source.Path); rel != "" {
				result.Source.Path = rel
			} else if reason != "" {
				result.Source.Path = "."
			}
		}
	}
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return raw
	}
	return append(out, '\n')
}

// sanitizeOSVWorkspacePaths removes an absolute workspace/input path from
// scanner diagnostics. Report text remains untrusted but must not leak host paths.
func sanitizeOSVWorkspacePaths(text, abs, rel string) string {
	text = strings.ReplaceAll(text, abs, rel)
	return strings.ReplaceAll(text, filepath.ToSlash(abs), rel)
}
