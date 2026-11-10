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
	"path/filepath"
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

// keyExtensions denote key material. The list is shared by both profiles.
var keyExtensions = map[string]bool{
	".pem": true, ".key": true, ".p12": true, ".pfx": true, ".keystore": true, ".jks": true,
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
	case lower == ".npmrc", lower == ".netrc", lower == ".git-credentials":
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
	case lower == ".npmrc", lower == ".netrc", lower == ".git-credentials":
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
