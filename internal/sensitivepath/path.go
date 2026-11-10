// Package sensitivepath classifies repository-relative paths that name
// credential-bearing files. It keeps the review-context vocabulary and the
// narrower model-tool read profile in one place so a path cannot be treated as
// sensitive by one surface and safe by another.
//
// The classifiers are lexical and conservative. They do not read the file and do
// not attempt general DLP; a secret in an ordinary source file is outside their
// scope.
package sensitivepath

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// reviewDirs are directories whose contents are never review context: cloud,
// SSH, and similar credential stores, plus the generic secrets directory used by
// the review-only policy.
var reviewDirs = map[string]bool{
	".aws":    true,
	".ssh":    true,
	".kube":   true,
	".gnupg":  true,
	"secrets": true,
}

// toolDirs are the well-known credential directories denied to direct model
// content tools. The generic "secrets" directory is deliberately not included:
// it is a review-only heuristic and may name ordinary source (for example
// internal/secrets/manager.go).
var toolDirs = map[string]bool{
	".aws":   true,
	".ssh":   true,
	".kube":  true,
	".gnupg": true,
}

// toolFiles are the well-known credential files denied to direct model content
// tools and hidden from the run_command sandbox. They are ordinary files, so a
// directory deny would not cover them.
var toolFiles = map[string]bool{
	".npmrc":           true,
	".netrc":           true,
	".git-credentials": true,
}

// keyExtensions denote key material. The list is shared by both profiles.
var keyExtensions = map[string]bool{
	".pem": true, ".key": true, ".p12": true, ".pfx": true, ".keystore": true, ".jks": true,
}

// HomeCredentialDirs returns the home-relative credential directories the
// run_command sandbox hides. It reuses the directories denied to the direct
// model tools and adds only command-line tool state (.config/gh,
// .config/gcloud, .azure) that is not repository source. The result is sorted
// so a sandbox profile is stable.
func HomeCredentialDirs() []string {
	dirs := make([]string, 0, len(toolDirs)+3)
	for d := range toolDirs {
		dirs = append(dirs, d)
	}
	dirs = append(dirs, ".config/gh", ".config/gcloud", ".azure")
	sort.Strings(dirs)
	return dirs
}

// HomeCredentialFiles returns the home-relative credential files the sandbox
// hides: the well-known files denied to the direct model tools plus Docker's
// registry credentials, Cargo's credential store, and pip's .pypirc. The result
// is sorted so a sandbox profile is stable.
func HomeCredentialFiles() []string {
	files := make([]string, 0, len(toolFiles)+3)
	for f := range toolFiles {
		files = append(files, f)
	}
	files = append(files, ".docker/config.json", ".cargo/credentials.toml", ".pypirc")
	sort.Strings(files)
	return files
}

// EnvCredentialDirs returns credential directories named by environment
// variables, resolved to absolute paths. A set but relative value is an error:
// the sandbox fails closed rather than guessing what it named.
func EnvCredentialDirs(getenv func(string) string) ([]string, error) {
	return envPaths(getenv, []envPath{
		{name: "GNUPGHOME"}, {name: "CLOUDSDK_CONFIG"}, {name: "GH_CONFIG_DIR"}, {name: "AZURE_CONFIG_DIR"},
		{name: "XDG_CONFIG_HOME", rels: []string{"gh", "gcloud"}},
	})
}

// EnvCredentialFiles returns credential files named by environment variables
// (including a KUBECONFIG list), resolved to absolute paths. A set but relative
// value is an error, as with EnvCredentialDirs.
func EnvCredentialFiles(getenv func(string) string) ([]string, error) {
	files, err := envPaths(getenv, []envPath{
		{name: "AWS_SHARED_CREDENTIALS_FILE"}, {name: "AWS_CONFIG_FILE"}, {name: "NPM_CONFIG_USERCONFIG"}, {name: "NETRC"},
		{name: "XDG_CONFIG_HOME", rels: []string{"git/credentials"}},
		{name: "DOCKER_CONFIG", rels: []string{"config.json"}},
		{name: "CARGO_HOME", rels: []string{"credentials.toml", "credentials"}},
	})
	if err != nil {
		return nil, err
	}
	if kube := strings.TrimSpace(getenv("KUBECONFIG")); kube != "" {
		for _, p := range filepath.SplitList(kube) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !filepath.IsAbs(p) {
				return nil, fmt.Errorf("KUBECONFIG entry is not absolute: %q", p)
			}
			real, err := resolveExisting("KUBECONFIG", p)
			if err != nil {
				return nil, err
			}
			files = append(files, real)
		}
	}
	sort.Strings(files)
	return files, nil
}

// envPath names one credential location: the variable's value itself, or, when
// rels is set, each rel joined under that value.
type envPath struct {
	name string
	rels []string
}

// envPaths resolves each set variable in specs; unset ones are skipped.
func envPaths(getenv func(string) string, specs []envPath) ([]string, error) {
	var out []string
	for _, s := range specs {
		p, err := absoluteEnv(getenv, s.name)
		if err != nil {
			return nil, err
		}
		if p == "" {
			continue
		}
		if len(s.rels) == 0 {
			out = append(out, p)
		}
		for _, rel := range s.rels {
			out = append(out, filepath.Join(p, rel))
		}
	}
	return out, nil
}

// absoluteEnv returns the resolved absolute value of name, or "" when unset.
func absoluteEnv(getenv func(string) string, name string) (string, error) {
	v := strings.TrimSpace(getenv(name))
	if v == "" {
		return "", nil
	}
	if !filepath.IsAbs(v) {
		return "", fmt.Errorf("%s is not absolute: %q", name, v)
	}
	return resolveExisting(name, v)
}

// resolveExisting resolves symlinks in p before any lexical cleaning:
// filepath.Clean would apply "link/.." to the text, naming a different object
// than the kernel opens. A path that does not exist is returned unchanged (the
// caller skips missing entries); any other resolution error fails closed.
func resolveExisting(name, p string) (string, error) {
	real, err := filepath.EvalSymlinks(p)
	if err == nil {
		return real, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	return "", fmt.Errorf("%s cannot be resolved: %q: %v", name, p, err)
}

// IsReviewSensitive reports whether a repository-relative path must be excluded
// from review context. It preserves the existing review policy: a credential
// directory (including "secrets"), a credential file, key material, or a name
// beginning with "secret".
func IsReviewSensitive(path string) bool {
	segments := split(path)
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments[:len(segments)-1] {
		if reviewDirs[strings.ToLower(segment)] {
			return true
		}
	}
	base := segments[len(segments)-1]
	lower := strings.ToLower(base)
	if reviewDirs[lower] || keyExtensions[strings.ToLower(filepath.Ext(base))] {
		return true
	}
	switch {
	case lower == ".env", strings.HasPrefix(lower, ".env."):
		return true
	case toolFiles[lower]:
		return true
	case strings.HasPrefix(lower, "credentials"):
		return true
	case strings.HasPrefix(lower, "id_rsa"), strings.HasPrefix(lower, "id_dsa"),
		strings.HasPrefix(lower, "id_ecdsa"), strings.HasPrefix(lower, "id_ed25519"):
		return true
	case strings.HasPrefix(lower, "secret"), strings.HasSuffix(lower, ".secret"),
		strings.Contains(lower, ".secret."):
		return true
	}
	return false
}

// IsToolReadDenied reports whether a direct model content tool must refuse a
// repository-relative path. This is the approved narrow profile: well-known
// credential directories and files plus key material. It deliberately preserves
// .env.example and .env.sample, and it does not apply the review-only generic
// "secrets" directory or "secret*" source-name heuristics.
func IsToolReadDenied(path string) bool {
	segments := split(path)
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments[:len(segments)-1] {
		if toolDirs[strings.ToLower(segment)] {
			return true
		}
	}
	base := segments[len(segments)-1]
	lower := strings.ToLower(base)
	if toolDirs[lower] || keyExtensions[strings.ToLower(filepath.Ext(base))] {
		return true
	}
	switch {
	case lower == ".env.example", lower == ".env.sample":
		return false
	case lower == ".env", strings.HasPrefix(lower, ".env."):
		return true
	case toolFiles[lower]:
		return true
	case strings.HasPrefix(lower, "credentials"):
		return true
	case strings.HasPrefix(lower, "id_rsa"), strings.HasPrefix(lower, "id_dsa"),
		strings.HasPrefix(lower, "id_ecdsa"), strings.HasPrefix(lower, "id_ed25519"):
		return true
	}
	return false
}

// split returns the slash-separated, cleaned path segments. An empty or
// root-only path has no segments.
func split(path string) []string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "" || clean == "." {
		return nil
	}
	return strings.Split(clean, "/")
}
