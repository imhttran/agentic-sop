package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cargoAuditAdapter wraps the cargo-audit CLI. It inspects explicit Cargo.lock
// files with --file, uses --no-fetch, and reads an explicit local advisory
// database with --db. It never generates or modifies a lockfile, never fetches,
// never uploads source, and never runs the experimental fix subcommand.
type cargoAuditAdapter struct {
	id           string
	category     Category
	command      []string
	databasePath string
	lockfile     string
	languages    []Language
	stalePolicy  string
	version      string
	configDigest string
	exec         Executor
	timeout      time.Duration
	maxOutput    int
	// now is the freshness clock. Production leaves it nil (time.Now); tests
	// inject a fixed clock so freshness assertions never depend on the
	// calendar date when the test runs.
	now func() time.Time
}

// cargoAuditStaleMaxAge is the freshness boundary cargo-audit itself uses for a
// stale advisory database.
const cargoAuditStaleMaxAge = 90 * 24 * time.Hour

func (a *cargoAuditAdapter) Descriptor() Descriptor {
	return Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       a.id,
		AdapterVersion:  a.version,
		Categories:      []Category{a.category},
		ScopeModes:      []string{"workspace", "changed-files"},
		Locality:        "local",
		Network:         "none; --no-fetch against an explicit local advisory database; no source upload",
		Languages:       a.languages,
	}
}

// cargoAuditReport is the JSON report cargo-audit emits. The vulnerability list
// is nested under vulnerabilities.list; the database block carries the identity
// and freshness evidence.
type cargoAuditReport struct {
	Database *struct {
		AdvisoryCount json.RawMessage `json:"advisory-count"`
		LastCommit    string          `json:"last-commit"`
		LastUpdated   string          `json:"last-updated"`
	} `json:"database"`
	Lockfile struct {
		DependencyCount int `json:"dependency-count"`
	} `json:"lockfile"`
	Vulnerabilities *struct {
		Found bool                      `json:"found"`
		Count int                       `json:"count"`
		List  []cargoAuditVulnerability `json:"list"`
	} `json:"vulnerabilities"`
	Warnings map[string][]json.RawMessage `json:"warnings"`
}

type cargoAuditVulnerability struct {
	Advisory struct {
		ID            string          `json:"id"`
		Package       string          `json:"package"`
		Title         string          `json:"title"`
		Description   string          `json:"description"`
		Date          string          `json:"date"`
		Aliases       []string        `json:"aliases"`
		Related       []string        `json:"related"`
		CVSS          json.RawMessage `json:"cvss"`
		Severity      string          `json:"severity"`
		Informational json.RawMessage `json:"informational"`
		URL           string          `json:"url"`
		References    []string        `json:"references"`
		Withdrawn     *string         `json:"withdrawn"`
		Source        json.RawMessage `json:"source"`
	} `json:"advisory"`
	Versions struct {
		Patched    []string `json:"patched"`
		Unaffected []string `json:"unaffected"`
	} `json:"versions"`
	Package struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Source  string `json:"source"`
		License string `json:"license"`
	} `json:"package"`
}

// cargoAuditDatabaseFile is a local, operator-provided metadata file for a
// pinned/fixture advisory database. Real cargo-audit databases are Git
// repositories; this file lets a deterministic fixture declare the same
// identity without depending on a live upstream repository.
const cargoAuditDatabaseFile = ".cargo-audit-metadata.json"

type cargoAuditDatabaseMeta struct {
	LastCommit    string          `json:"last-commit"`
	LastUpdated   string          `json:"last-updated"`
	AdvisoryCount json.RawMessage `json:"advisory-count"`
}

func (a *cargoAuditAdapter) stalePolicyValue() string {
	if strings.EqualFold(strings.TrimSpace(a.stalePolicy), StaleAllow) {
		return StaleAllow
	}
	return StaleDeny
}

func (a *cargoAuditAdapter) declaredLanguages() []Language {
	if len(a.languages) > 0 {
		return append([]Language(nil), a.languages...)
	}
	return []Language{LanguageRust}
}

// clock returns the current UTC time for freshness decisions. Tests replace
// a.now to remove calendar dependence; production uses the wall clock.
func (a *cargoAuditAdapter) clock() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}

// currentProvenance fingerprints the local database before execution so resume
// can compare an unchanged database without running the scanner.
func (a *cargoAuditAdapter) currentProvenance(req Request) (Provenance, error) {
	prov := Provenance{
		ScannerVersion:      a.version,
		ConfigurationDigest: a.configDigest,
		ContentIdentity:     req.Content,
		Languages:           a.declaredLanguages(),
	}
	plan, err := a.resolveInputPlan(req)
	if err != nil {
		return prov, err
	}
	lockfileIdentity, err := cargoLockfileIdentity(req.WorkspaceRoot, plan.Lockfiles)
	if err != nil {
		return prov, err
	}
	prov.LockfileIdentity = lockfileIdentity
	prov.ObligationIdentity = plan.Detection.ObligationIdentity(LanguageRust)
	if len(plan.Lockfiles) == 0 {
		return prov, nil
	}
	dir, err := a.resolveDatabasePath(req)
	if err != nil {
		return prov, err
	}
	identity, err := cargoDatabaseIdentity(dir)
	if err != nil {
		return prov, err
	}
	prov.AdvisoryDatabaseIdentity = identity
	metadata, stale := cargoDatabaseFreshness(dir, a.clock())
	prov.AdvisoryDatabaseMetadata = metadata
	prov.AdvisoryDatabaseStale = stale
	return prov, nil
}

// resolveDatabasePath returns the explicit local advisory database directory. An
// empty path is a fail-closed configuration error; it never triggers a download.
func (a *cargoAuditAdapter) resolveDatabasePath(req Request) (string, error) {
	raw := strings.TrimSpace(a.databasePath)
	if raw == "" {
		return "", fmt.Errorf("advisory database path is not configured; implicit fetch and update are disabled")
	}
	dir := raw
	if !isAbsPath(dir) {
		dir = joinPath(req.WorkspaceRoot, dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("advisory database is unavailable: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if !info.IsDir() {
		return "", fmt.Errorf("advisory database is unavailable: %q is not a directory", filepath.ToSlash(raw))
	}
	return dir, nil
}

// cargoAuditInputPlan is the resolved Rust input inventory for one execution.
type cargoAuditInputPlan struct {
	Detection   Detection
	Lockfiles   []string
	Obligations []DependencyObligation
	Missing     []DependencyObligation
}

// resolveInputPlan returns the explicit or detected Cargo.lock inputs plus every
// in-scope Rust obligation. A configured lockfile is validated against the
// inventory; an omitted obligation is a coverage gap, not an implicit subset.
func (a *cargoAuditAdapter) resolveInputPlan(req Request) (cargoAuditInputPlan, error) {
	detection, err := DetectRepositoryScoped(req.WorkspaceRoot, req.Scope.Excluded)
	if err != nil {
		return cargoAuditInputPlan{}, fmt.Errorf("resolve Cargo lockfiles: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
	}
	if len(detection.Issues) > 0 {
		return cargoAuditInputPlan{}, fmt.Errorf("dependency layout is unsupported or ambiguous: %s", SanitizeText(strings.Join(detection.Issues, "; "), MaxDiagnosticBytes))
	}
	plan := cargoAuditInputPlan{
		Detection:   detection,
		Obligations: detection.SelectedObligations(LanguageRust),
	}
	byInput := map[string]DependencyObligation{}
	for _, obligation := range plan.Obligations {
		byInput[obligation.Input] = obligation
	}

	if raw := strings.TrimSpace(a.lockfile); raw != "" {
		path, reason := NormalizePath(raw)
		if path == "" {
			return cargoAuditInputPlan{}, fmt.Errorf("configured lockfile rejected: %s", reason)
		}
		obligation, ok := byInput[path]
		if !ok {
			return cargoAuditInputPlan{}, fmt.Errorf("configured lockfile %s is not an in-scope Cargo.lock obligation", path)
		}
		abs := joinPath(req.WorkspaceRoot, path)
		info, err := os.Lstat(abs)
		if err != nil {
			return cargoAuditInputPlan{}, fmt.Errorf("configured lockfile %s is unavailable: %s", path, SanitizeText(err.Error(), MaxDiagnosticBytes))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cargoAuditInputPlan{}, fmt.Errorf("configured lockfile %s is a symbolic link; refusing to read outside the workspace", path)
		}
		if !info.Mode().IsRegular() {
			return cargoAuditInputPlan{}, fmt.Errorf("configured lockfile %s is not a regular file", path)
		}
		plan.Lockfiles = []string{path}
		for _, other := range plan.Obligations {
			if other.Input == obligation.Input {
				continue
			}
			missing := other
			if missing.Present {
				missing.Issues = append(missing.Issues, "lockfile is present but omitted by the explicit lockfile setting")
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
		info, err := os.Lstat(abs)
		if err != nil {
			return cargoAuditInputPlan{}, fmt.Errorf("Cargo lockfile %s is unavailable: %s", obligation.Input, SanitizeText(err.Error(), MaxDiagnosticBytes))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cargoAuditInputPlan{}, fmt.Errorf("Cargo lockfile %s is a symbolic link; refusing to read outside the workspace", obligation.Input)
		}
		if !info.Mode().IsRegular() {
			return cargoAuditInputPlan{}, fmt.Errorf("Cargo lockfile %s is not a regular file", obligation.Input)
		}
		plan.Lockfiles = append(plan.Lockfiles, obligation.Input)
	}
	sort.Strings(plan.Lockfiles)
	return plan, nil
}

// cargoLockfileIdentity returns a deterministic digest of the resolved lockfiles.
// It is recorded as provenance and used as a resume key, so a changed lockfile
// invalidates reused evidence even when the advisory database is unchanged.
func cargoLockfileIdentity(root string, lockfiles []string) (string, error) {
	if len(lockfiles) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(lockfiles))
	for _, rel := range lockfiles {
		digest, err := rulesIdentity(joinPath(root, rel))
		if err != nil {
			return "", fmt.Errorf("lockfile identity for %s is unavailable: %s", rel, SanitizeText(err.Error(), MaxDiagnosticBytes))
		}
		if strings.TrimSpace(digest) == "" {
			return "", fmt.Errorf("lockfile identity for %s is empty", rel)
		}
		parts = append(parts, "path="+rel+";digest="+digest)
	}
	return strings.Join(parts, ";"), nil
}

// verifyToolVersion executes the configured command with --version and requires
// the parsed version to equal the pinned configuration value. A missing pin, an
// unrecognized version string, or a mismatch fails closed before any scan runs.
func (a *cargoAuditAdapter) verifyToolVersion(ctx context.Context, workDir string) (string, error) {
	pinned := normalizeCargoAuditVersion(a.version)
	if pinned == "" {
		return "", fmt.Errorf("cargo-audit version is not pinned; configure an exact version so an incompatible tool cannot produce a clean result")
	}
	argv := append([]string(nil), a.command...)
	if len(argv) == 0 {
		argv = []string{"cargo", "audit"}
	}
	argv = append(argv, "--version")
	runCtx, cancel := scanContext(ctx, a.timeout)
	defer cancel()
	stdout, _, _, err := a.exec.Run(runCtx, workDir, argv, nil, a.maxOutput)
	if err != nil {
		return "", err
	}
	actual := normalizeCargoAuditVersion(string(stdout))
	if actual == "" {
		return "", fmt.Errorf("cargo-audit version output is unrecognized: %s", SanitizeText(strings.TrimSpace(string(stdout)), MaxDiagnosticBytes))
	}
	if actual != pinned {
		return "", fmt.Errorf("cargo-audit version mismatch: found %s, want %s", actual, pinned)
	}
	return actual, nil
}

// normalizeCargoAuditVersion extracts a comparable semantic version from version
// output such as "cargo-audit 0.22.2" or "cargo-audit-audit 0.22.2". An
// unrecognized value yields an empty string instead of a guess.
func normalizeCargoAuditVersion(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	fields := strings.Fields(value)
	value = strings.TrimPrefix(fields[len(fields)-1], "v")
	if !isCargoVersion(value) {
		return ""
	}
	return value
}

func isCargoVersion(value string) bool {
	core := value
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	segments := strings.Split(core, ".")
	if len(segments) != 3 {
		return false
	}
	for _, segment := range segments {
		if segment == "" {
			return false
		}
		for _, r := range segment {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// Scan audits every resolved lockfile once. Each failure is reported
// independently, and any failure makes the execution non-clean.
func (a *cargoAuditAdapter) Scan(ctx context.Context, req Request) (Execution, error) {
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
	exec.Provenance.ObligationIdentity = plan.Detection.ObligationIdentity(LanguageRust)
	lockfiles := plan.Lockfiles
	if len(lockfiles) == 0 {
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
		exec.Diagnostics = []string{"no applicable Rust dependency content; scanner was not executed"}
		sanitizeExecution(&exec)
		return finish(exec, nil)
	}
	databaseDir, err := a.resolveDatabasePath(req)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	lockfileIdentity, err := cargoLockfileIdentity(req.WorkspaceRoot, lockfiles)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	exec.Provenance.LockfileIdentity = lockfileIdentity
	identity, err := cargoDatabaseIdentity(databaseDir)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{"advisory database identity is unavailable: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	exec.Provenance.AdvisoryDatabaseIdentity = identity

	workDir, err := os.MkdirTemp("", "sop-cargo-audit-*")
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{"create restricted cargo-audit directory: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	defer os.RemoveAll(workDir)

	// The configured version is a pin, not a label: verify the executable before
	// any scan so an incompatible tool can never produce a clean dependency result.
	actualVersion, versionErr := a.verifyToolVersion(ctx, workDir)
	if versionErr != nil {
		switch {
		case errors.Is(versionErr, ErrUnavailable):
			exec.Status = StatusUnavailable
			exec.Diagnostics = append(exec.Diagnostics, "cargo-audit executable is unavailable")
		case errors.Is(versionErr, context.DeadlineExceeded):
			exec.Status = StatusTimedOut
			exec.Diagnostics = append(exec.Diagnostics, "cargo-audit version check exceeded the scan deadline")
		case errors.Is(versionErr, context.Canceled):
			exec.Status = StatusCancelled
			exec.Diagnostics = append(exec.Diagnostics, "cargo-audit version check was cancelled")
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
		rawReports []json.RawMessage
		metadata   []string
		failures   int
		successes  int
		anyStale   bool
	)
	for _, rel := range lockfiles {
		abs := joinPath(req.WorkspaceRoot, rel)
		argv := append([]string(nil), a.command...)
		if len(argv) == 0 {
			argv = []string{"cargo", "audit"}
		}
		argv = append(argv,
			"--file", abs,
			"--json",
			"--no-fetch",
			"--db", databaseDir,
		)
		if a.stalePolicyValue() == StaleAllow {
			argv = append(argv, "--stale")
		}

		runCtx, cancel := scanContext(ctx, a.timeout)
		stdout, stderr, exitCode, runErr := a.exec.Run(runCtx, workDir, argv, nil, a.maxOutput)
		cancel()
		exec.ExitCode = exitCode
		if len(stderr) > 0 {
			exec.Diagnostics = append(exec.Diagnostics, SanitizeText(string(stderr), MaxDiagnosticBytes))
		}
		if runErr != nil {
			switch {
			case errors.Is(runErr, ErrUnavailable):
				exec.Status = StatusUnavailable
				exec.Coverage = CoverageUnknown
				exec.Diagnostics = append(exec.Diagnostics, "cargo-audit executable is unavailable")
				sanitizeExecution(&exec)
				return finish(exec, nil)
			case errors.Is(runErr, context.DeadlineExceeded):
				exec.Status = StatusTimedOut
				exec.Coverage = CoverageUnknown
				exec.Diagnostics = append(exec.Diagnostics, "cargo-audit exceeded the scan deadline")
				sanitizeExecution(&exec)
				return finish(exec, nil)
			case errors.Is(runErr, context.Canceled):
				exec.Status = StatusCancelled
				exec.Coverage = CoverageUnknown
				exec.Diagnostics = append(exec.Diagnostics, "cargo-audit was cancelled")
				sanitizeExecution(&exec)
				return finish(exec, nil)
			default:
				failures++
				exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit failed for %s: %s", rel, SanitizeText(runErr.Error(), MaxDiagnosticBytes)))
				continue
			}
		}
		if len(stdout) == 0 {
			failures++
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit returned empty output for %s", rel))
			continue
		}
		if a.maxOutput > 0 && len(stdout) >= a.maxOutput {
			failures++
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit output for %s reached the %d-byte limit", rel, a.maxOutput))
			continue
		}
		if exitCode != 0 && exitCode != 1 {
			failures++
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit exited with code %d for %s", exitCode, rel))
			continue
		}

		var report cargoAuditReport
		if err := json.Unmarshal(stdout, &report); err != nil {
			failures++
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit returned malformed JSON for %s: %s", rel, SanitizeText(err.Error(), MaxDiagnosticBytes)))
			continue
		}
		if report.Database == nil {
			failures++
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit report for %s has no advisory database identity", rel))
			continue
		}
		if report.Vulnerabilities == nil {
			failures++
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit report for %s has no vulnerability evidence", rel))
			continue
		}
		if report.Vulnerabilities.Found != (len(report.Vulnerabilities.List) > 0) {
			// An inconsistent report is not evidence of a clean dependency graph.
			failures++
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit report for %s has inconsistent vulnerability count/list evidence", rel))
			continue
		}
		dbMeta := cargoAuditDatabaseMeta{
			LastCommit:    SanitizeText(report.Database.LastCommit, 128),
			LastUpdated:   SanitizeText(report.Database.LastUpdated, 128),
			AdvisoryCount: report.Database.AdvisoryCount,
		}
		metadata = append(metadata, cargoAuditMetadataString(dbMeta))
		stale, freshKnown := cargoDatabaseStale(dbMeta.LastUpdated, a.clock())
		if !freshKnown {
			// Missing freshness evidence is not clean. Treat it as stale so the
			// explicit policy decides and required mode cannot pass.
			stale = true
			exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit database freshness is unavailable for %s; treated as stale", rel))
		}
		if stale {
			anyStale = true
		}

		for _, vulnerability := range report.Vulnerabilities.List {
			exec.Findings = append(exec.Findings, a.normalizeVulnerability(vulnerability, rel))
		}
		warnings := 0
		for _, list := range report.Warnings {
			warnings += len(list)
		}
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("cargo-audit %s: %d dependency(ies), %d advisory(ies), %d warning(s)",
			rel, report.Lockfile.DependencyCount, len(report.Vulnerabilities.List), warnings))
		rawReports = append(rawReports, append(json.RawMessage(nil), stdout...))
		successes++
	}

	if len(metadata) > 0 {
		exec.Provenance.AdvisoryDatabaseMetadata = strings.Join(sortedUniqueStrings(metadata), "; ")
	}
	exec.Provenance.AdvisoryDatabaseStale = anyStale
	if successes > 0 {
		exec.Languages = a.declaredLanguages()
	}
	missingObligations := len(plan.Missing) > 0
	if missingObligations {
		for _, obligation := range plan.Missing {
			exec.Diagnostics = append(exec.Diagnostics, missingObligationDiagnostic(obligation))
		}
	}
	if anyStale {
		if a.stalePolicyValue() == StaleAllow {
			exec.Coverage = CoveragePartial
			exec.Diagnostics = append(exec.Diagnostics, "advisory database is stale; coverage is PARTIAL (policy: allow)")
		} else {
			exec.Diagnostics = append(exec.Diagnostics, "advisory database is stale (policy: deny)")
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
		exec.Diagnostics = append(exec.Diagnostics, "no cargo-audit report was produced")
	default:
		if exec.Coverage == "" || exec.Coverage == CoverageUnknown {
			exec.Coverage = CoverageComplete
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

// normalizeVulnerability maps one advisory into a normalized dependency finding.
// It preserves the advisory ID, affected package and version, severity when
// available, CVSS, aliases/CVEs, patched/unaffected versions, and references.
func (a *cargoAuditAdapter) normalizeVulnerability(v cargoAuditVulnerability, lockfile string) Finding {
	advisory := v.Advisory
	pkg := firstNonEmpty(v.Package.Name, advisory.Package)
	version := v.Package.Version
	severity, original, score := cargoSeverity(advisory.Severity, advisory.CVSS, advisory.Informational)

	meta := map[string]string{
		"package":  SanitizeText(pkg, 128),
		"lockfile": SanitizeText(lockfile, 256),
	}
	if version != "" {
		meta["version"] = SanitizeText(version, 128)
	}
	if len(advisory.Aliases) > 0 {
		meta["aliases"] = SanitizeText(strings.Join(advisory.Aliases, ","), MaxMessageBytes)
	}
	if len(advisory.Related) > 0 {
		meta["related"] = SanitizeText(strings.Join(advisory.Related, ","), MaxMessageBytes)
	}
	if len(v.Versions.Patched) > 0 {
		meta["patched_versions"] = SanitizeText(strings.Join(v.Versions.Patched, ","), MaxMessageBytes)
	}
	if len(v.Versions.Unaffected) > 0 {
		meta["unaffected_versions"] = SanitizeText(strings.Join(v.Versions.Unaffected, ","), MaxMessageBytes)
	}
	if vector := rawString(advisory.CVSS); vector != "" {
		meta["cvss"] = SanitizeText(vector, 256)
	}
	if score != "" {
		meta["cvss_score"] = score
	}
	if advisory.URL != "" {
		meta["url"] = SanitizeText(advisory.URL, MaxMessageBytes)
	}
	if len(advisory.References) > 0 {
		meta["references"] = SanitizeText(strings.Join(advisory.References, ","), MaxMessageBytes)
	}
	if advisory.Date != "" {
		meta["advisory_date"] = SanitizeText(advisory.Date, 64)
	}
	if info := rawString(advisory.Informational); info != "" {
		meta["informational"] = SanitizeText(info, 128)
	}
	if advisory.Withdrawn != nil && strings.TrimSpace(*advisory.Withdrawn) != "" {
		meta["withdrawn"] = SanitizeText(*advisory.Withdrawn, 64)
	}
	if v.Package.License != "" {
		meta["license"] = SanitizeText(v.Package.License, 128)
	}
	if v.Package.Source != "" {
		meta["source"] = SanitizeText(v.Package.Source, MaxMessageBytes)
	}

	message := strings.TrimSpace(firstNonEmpty(advisory.Title, advisory.ID))
	if pkg != "" {
		message = pkg
		if version != "" {
			message += " " + version
		}
		message += ": " + strings.TrimSpace(firstNonEmpty(advisory.Title, advisory.ID))
	}
	if len(v.Versions.Patched) > 0 {
		message += " (patched: " + strings.Join(v.Versions.Patched, ", ") + ")"
	}

	fingerprintKey := advisory.ID
	if pkg != "" || version != "" {
		fingerprintKey += ":" + pkg + "@" + version
	}
	finding := Finding{
		AdapterID:        a.id,
		Provider:         "cargo-audit",
		Category:         CategoryDependencies,
		RuleID:           SanitizeText(firstNonEmpty(advisory.ID, "RUSTSEC-UNKNOWN"), MaxMessageBytes),
		OriginalSeverity: SanitizeText(original, 128),
		Severity:         severity,
		Message:          SanitizeText(message, MaxMessageBytes),
		Path:             lockfile,
		Fingerprint:      StableFingerprint("cargo-audit", fingerprintKey, lockfile, 0, 0),
		Metadata:         meta,
		Confidence:       "HIGH",
	}
	for _, alias := range advisory.Aliases {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(alias)), "CVE-") {
			finding.CVE = append(finding.CVE, SanitizeText(alias, 64))
		}
	}
	return finding
}

// cargoSeverity maps the advisory's explicit severity when present, otherwise a
// CVSS 3.x base score, otherwise INFO for an informational advisory, otherwise
// UNKNOWN. Unknown is explicit and is never silently treated as low.
func cargoSeverity(explicit string, cvss json.RawMessage, informational json.RawMessage) (Severity, string, string) {
	if strings.TrimSpace(explicit) != "" {
		return NormalizeSeverity("cargo-audit", explicit), explicit, ""
	}
	vector := rawString(cvss)
	if vector != "" {
		if score, ok := cvss31BaseScore(vector); ok {
			mapped := severityForCVSS(score)
			return mapped, fmt.Sprintf("%s (%.1f)", vector, score), strconv.FormatFloat(score, 'f', 1, 64)
		}
		return SeverityUnknown, vector, ""
	}
	if rawString(informational) != "" {
		return SeverityInfo, "informational", ""
	}
	return SeverityUnknown, "unknown", ""
}

func severityForCVSS(score float64) Severity {
	switch {
	case score >= 9.0:
		return SeverityCritical
	case score >= 7.0:
		return SeverityHigh
	case score >= 4.0:
		return SeverityMedium
	case score > 0:
		return SeverityLow
	default:
		return SeverityInfo
	}
}

// cvss31BaseScore computes the CVSS v3.1 base score from a vector string. It
// returns false for a malformed or unsupported vector rather than guessing.
func cvss31BaseScore(vector string) (float64, bool) {
	vector = strings.TrimSpace(vector)
	vector = strings.TrimPrefix(vector, "CVSS:3.1/")
	vector = strings.TrimPrefix(vector, "CVSS:3.0/")
	metrics := map[string]string{}
	for _, part := range strings.Split(vector, "/") {
		key, value, ok := strings.Cut(part, ":")
		if !ok || key == "" || value == "" {
			return 0, false
		}
		metrics[key] = value
	}
	av, ok := map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}[metrics["AV"]]
	if !ok {
		return 0, false
	}
	ac, ok := map[string]float64{"L": 0.77, "H": 0.44}[metrics["AC"]]
	if !ok {
		return 0, false
	}
	ui, ok := map[string]float64{"N": 0.85, "R": 0.62}[metrics["UI"]]
	if !ok {
		return 0, false
	}
	scopeChanged := metrics["S"] == "C"
	if metrics["S"] != "U" && !scopeChanged {
		return 0, false
	}
	prTable := map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}
	if scopeChanged {
		prTable = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.5}
	}
	pr, ok := prTable[metrics["PR"]]
	if !ok {
		return 0, false
	}
	impactValue := func(value string) (float64, bool) {
		v, ok := map[string]float64{"H": 0.56, "L": 0.22, "N": 0.0}[value]
		return v, ok
	}
	c, ok := impactValue(metrics["C"])
	if !ok {
		return 0, false
	}
	i, ok := impactValue(metrics["I"])
	if !ok {
		return 0, false
	}
	a, ok := impactValue(metrics["A"])
	if !ok {
		return 0, false
	}
	iss := 1 - ((1 - c) * (1 - i) * (1 - a))
	var impact float64
	if scopeChanged {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}
	exploitability := 8.22 * av * ac * pr * ui
	if impact <= 0 {
		return 0, true
	}
	base := impact + exploitability
	if scopeChanged {
		base = 1.08 * base
	}
	if base > 10 {
		base = 10
	}
	return math.Ceil(base*10) / 10, true
}

// cargoDatabaseIdentity returns a deterministic local fingerprint for resume.
// A fixture may declare metadata explicitly; a real Git database is identified
// by its HEAD commit; any other directory falls back to a content digest.
func cargoDatabaseIdentity(dir string) (string, error) {
	if meta, ok := readCargoDatabaseMeta(dir); ok {
		// Scanner metadata alone is not content-sensitive: an edited advisory file
		// with unchanged metadata must not reuse earlier evidence. Include the local
		// content digest beside the declared identity.
		digest, err := rulesIdentity(dir)
		if err != nil {
			return "", fmt.Errorf("advisory database content digest is unavailable: %s", SanitizeText(err.Error(), MaxDiagnosticBytes))
		}
		if strings.TrimSpace(digest) == "" {
			return "", fmt.Errorf("empty advisory database content digest")
		}
		identity := cargoAuditMetadataString(meta)
		if identity == "" {
			return "digest=" + digest, nil
		}
		return identity + ";digest=" + digest, nil
	}
	if commit := cargoGitHead(dir); commit != "" {
		// Include the content digest as well as HEAD so uncommitted changes to a
		// local database cannot reuse evidence from the earlier contents.
		if digest, err := rulesIdentity(dir); err == nil && strings.TrimSpace(digest) != "" {
			return "git-commit=" + commit + ";digest=" + digest, nil
		}
		return "git-commit=" + commit, nil
	}
	digest, err := rulesIdentity(dir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(digest) == "" {
		return "", fmt.Errorf("empty advisory database digest")
	}
	return "digest=" + digest, nil
}

func readCargoDatabaseMeta(dir string) (cargoAuditDatabaseMeta, bool) {
	data, err := os.ReadFile(filepath.Join(dir, cargoAuditDatabaseFile))
	if err != nil {
		return cargoAuditDatabaseMeta{}, false
	}
	var meta cargoAuditDatabaseMeta
	if json.Unmarshal(data, &meta) != nil {
		return cargoAuditDatabaseMeta{}, false
	}
	return meta, true
}

func cargoGitHead(dir string) string {
	gitDir := filepath.Join(dir, ".git")
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(head))
	if !strings.HasPrefix(value, "ref: ") {
		if isHexCommit(value) {
			return value
		}
		return ""
	}
	ref := strings.TrimSpace(strings.TrimPrefix(value, "ref: "))
	refPath := filepath.Join(gitDir, filepath.FromSlash(ref))
	if data, err := os.ReadFile(refPath); err == nil {
		commit := strings.TrimSpace(string(data))
		if isHexCommit(commit) {
			return commit
		}
	}
	if packed, err := os.ReadFile(filepath.Join(gitDir, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(packed), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[1] == ref && isHexCommit(fields[0]) {
				return fields[0]
			}
		}
	}
	return ""
}

func isHexCommit(value string) bool {
	if len(value) < 7 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func cargoAuditMetadataString(meta cargoAuditDatabaseMeta) string {
	parts := make([]string, 0, 3)
	if commit := strings.TrimSpace(meta.LastCommit); commit != "" {
		parts = append(parts, "last-commit="+SanitizeText(commit, 128))
	}
	if updated := strings.TrimSpace(meta.LastUpdated); updated != "" {
		parts = append(parts, "last-updated="+SanitizeText(updated, 128))
	}
	if count := rawScalar(meta.AdvisoryCount); count != "" {
		parts = append(parts, "advisories="+SanitizeText(count, 64))
	}
	return strings.Join(parts, ";")
}

// cargoDatabaseStale reports whether last-updated is older than the stale
// boundary. A missing or unparsable timestamp is reported as not fresh-known.
func cargoDatabaseStale(lastUpdated string, now time.Time) (stale bool, known bool) {
	value := strings.TrimSpace(lastUpdated)
	if value == "" {
		return false, false
	}
	updated, err := time.Parse(time.RFC3339, value)
	if err != nil {
		if updated, err = time.Parse("2006-01-02T15:04:05Z", value); err != nil {
			return false, false
		}
	}
	return now.Sub(updated) > cargoAuditStaleMaxAge, true
}

// cargoDatabaseFreshness rechecks a local database without running the scanner.
// It is used by resume so an unchanged but aged database invalidates reused
// evidence. Scanner-reported metadata wins; otherwise the newest file
// modification time is a conservative local freshness signal.
func cargoDatabaseFreshness(dir string, now time.Time) (metadata string, stale bool) {
	if meta, ok := readCargoDatabaseMeta(dir); ok {
		metadata = cargoAuditMetadataString(meta)
		if stale, known := cargoDatabaseStale(meta.LastUpdated, now); known {
			return metadata, stale
		}
	}
	modified := newestModTime(dir)
	if modified.IsZero() {
		return metadata, true
	}
	meta := "modified=" + modified.Format(time.RFC3339)
	if metadata != "" {
		metadata += "; " + meta
	} else {
		metadata = meta
	}
	return metadata, now.Sub(modified) > cargoAuditStaleMaxAge
}

// newestModTime returns the newest regular-file modification time under dir. A
// missing or unreadable tree yields the zero time, which callers treat as stale.
func newestModTime(dir string) time.Time {
	var newest time.Time
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	var v any
	if json.Unmarshal(raw, &v) == nil {
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func rawScalar(raw json.RawMessage) string {
	return rawString(raw)
}

func marshalReports(reports []json.RawMessage) []byte {
	wrapper := struct {
		Reports []json.RawMessage `json:"reports"`
	}{Reports: reports}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return append(data, '\n')
}
