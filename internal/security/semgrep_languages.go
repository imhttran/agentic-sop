package security

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// detectSourceLanguages returns the Go and Rust source languages present in the
// requested scope. It reads repository content only; it never starts a scanner,
// downloads rules, or reaches the network.
func detectSourceLanguages(req Request) ([]Language, error) {
	root := strings.TrimSpace(req.WorkspaceRoot)
	if root == "" {
		return nil, nil
	}
	matcher := NewExcludeMatcher(req.Scope.Excluded)
	set := map[Language]bool{}

	addFile := func(rel string) {
		if matcher.Excluded(rel) {
			return
		}
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".go":
			set[LanguageGo] = true
		case ".rs":
			set[LanguageRust] = true
		}
	}

	if req.Scope.Mode == "changed-files" {
		for _, file := range req.Scope.Files {
			if path, _ := NormalizePath(file); path != "" {
				addFile(path)
			}
		}
	} else {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if entry.IsDir() {
				if rel != "." && (entry.Name() == ".git" || entry.Name() == ".agent-sdlc" || matcher.Excluded(rel)) {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type().IsRegular() && SafeRelativePath(rel) {
				addFile(rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		// A manifest is a durable language signal even when a workspace uses
		// generated or vendored source files.
		detection, derr := DetectRepositoryScoped(root, req.Scope.Excluded)
		if derr != nil {
			return nil, derr
		}
		for _, language := range detection.Languages {
			set[language] = true
		}
	}

	out := make([]Language, 0, len(set))
	for language := range set {
		out = append(out, language)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// semgrepLanguageCoverage is the pre-execution decision for a ruleset. It
// refuses a scan when no detected source language is covered, and marks coverage
// PARTIAL when only part of the detected source can be analyzed.
type semgrepLanguageCoverage struct {
	Covered     map[Language]bool
	Unsupported []Language
	Detected    []Language
}

// evaluateSemgrepLanguageCoverage parses the local rules' declared languages and
// compares them with the source languages in scope. It never infers coverage
// from a path name or from another language's rules.
func evaluateSemgrepLanguageCoverage(req Request, rulesPath string) (semgrepLanguageCoverage, error) {
	detected, err := detectSourceLanguages(req)
	if err != nil {
		return semgrepLanguageCoverage{}, err
	}
	covered, err := coveredLanguages(rulesPath, detected)
	if err != nil {
		return semgrepLanguageCoverage{}, err
	}
	return semgrepLanguageCoverage{
		Covered:     covered,
		Unsupported: unsupportedLanguages(detected, covered),
		Detected:    detected,
	}, nil
}

// unsupportedSourceDiagnostic renders the explicit UNSUPPORTED evidence for a
// language the local rules cannot analyze.
func unsupportedSourceDiagnostic(languages []Language) string {
	return "semgrep: local rules do not cover detected source language(s): " + languageListText(languages) + "; analysis is UNSUPPORTED"
}

// sourceLanguageDiagnostic renders the detected language set for evidence.
func sourceLanguageDiagnostic(languages []Language) string {
	if len(languages) == 0 {
		return "semgrep: no Go or Rust source detected; language coverage guard not applied"
	}
	return "semgrep: detected source language(s): " + languageListText(languages)
}
