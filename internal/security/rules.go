package security

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// rulesLanguageSet parses local Semgrep rules and returns the source languages
// the rules declare. It never downloads or executes a ruleset.
func rulesLanguageSet(path string) (map[string]bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	languages := map[string]bool{}
	if !info.IsDir() {
		return languages, parseRuleLanguages(path, languages)
	}
	err = filepath.WalkDir(path, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".yaml", ".yml":
			return parseRuleLanguages(p, languages)
		default:
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	return languages, nil
}

func parseRuleLanguages(path string, out map[string]bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	collectRuleLanguages(document, out)
	return nil
}

func collectRuleLanguages(value any, out map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		if raw, ok := v["languages"]; ok {
			for _, language := range stringSlice(raw) {
				language = strings.ToLower(strings.TrimSpace(language))
				if language != "" {
					out[language] = true
				}
			}
		}
		for key, child := range v {
			if key == "languages" {
				continue
			}
			collectRuleLanguages(child, out)
		}
	case []any:
		for _, child := range v {
			collectRuleLanguages(child, out)
		}
	}
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

// coveredLanguages maps the detected source languages a ruleset actually covers.
// A language with no matching rule is absent from the result; it is never
// assumed covered.
func coveredLanguages(rulesPath string, detected []Language) (map[Language]bool, error) {
	parsed, err := rulesLanguageSet(rulesPath)
	if err != nil {
		return nil, err
	}
	covered := map[Language]bool{}
	for _, language := range detected {
		switch language {
		case LanguageGo:
			if parsed["go"] {
				covered[language] = true
			}
		case LanguageRust:
			if parsed["rust"] {
				covered[language] = true
			}
		}
	}
	return covered, nil
}

func unsupportedLanguages(detected []Language, covered map[Language]bool) []Language {
	var out []Language
	for _, language := range detected {
		if !covered[language] {
			out = append(out, language)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func languageListText(languages []Language) string {
	values := make([]string, 0, len(languages))
	for _, language := range languages {
		values = append(values, string(language))
	}
	return strings.Join(values, ", ")
}
