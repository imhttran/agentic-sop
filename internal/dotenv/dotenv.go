// Package dotenv reads an optional, gitignored .env file and applies it to the
// process environment.
//
// It is a convenience layer, never a second source of SOP state: it only fills
// environment variables that are not already set, so an explicitly exported
// variable always wins over the file. It is optional — a missing .env is not an
// error, and .env is never required in CI or a non-interactive environment.
//
// Secrets (API keys, tokens) may live in .env, exactly as they may live in the
// environment; this package neither reads nor persists them beyond placing them
// in the process environment, and nothing it returns is written to SOP state.
package dotenv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileName is the conventional file name this package loads from a directory.
const FileName = ".env"

// LoadDir reads <dir>/.env. A missing file returns (nil, nil): the file is
// optional. A malformed file returns a focused error naming the line.
func LoadDir(dir string) (map[string]string, error) {
	return LoadFile(filepath.Join(dir, FileName))
}

// LoadFile reads a .env file. A missing file returns (nil, nil).
func LoadFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("dotenv: read %s: %w", path, err)
	}
	vars, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("dotenv: %s: %w", path, err)
	}
	return vars, nil
}

// Parse parses .env content into key/value pairs. It accepts blank lines,
// #-comments, an optional leading "export ", and single- or double-quoted values.
// A line that is not KEY=VALUE is an error rather than a silently ignored typo.
func Parse(content string) (map[string]string, error) {
	vars := map[string]string{}
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", i+1)
		}
		key := strings.TrimSpace(line[:eq])
		if !validKey(key) {
			return nil, fmt.Errorf("line %d: invalid variable name %q", i+1, key)
		}
		value, err := unquote(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		vars[key] = value
	}
	return vars, nil
}

// Apply sets each variable into the process environment for keys that are not
// already set, so an explicit process environment variable takes precedence over
// the file. It returns the keys it set, in a deterministic order.
func Apply(vars map[string]string) []string {
	if len(vars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	applied := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, present := os.LookupEnv(k); present {
			continue
		}
		_ = os.Setenv(k, vars[k])
		applied = append(applied, k)
	}
	return applied
}

// validKey reports whether key is a portable environment variable name.
func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// unquote removes a single matching pair of surrounding quotes, unescaping the
// common sequences inside double quotes. An unquoted value is returned as-is.
func unquote(value string) (string, error) {
	if len(value) < 2 {
		return value, nil
	}
	quote := value[0]
	last := value[len(value)-1]
	if (quote != '"' && quote != '\'') || quote != last {
		return value, nil
	}
	inner := value[1 : len(value)-1]
	if quote == '\'' {
		return inner, nil
	}
	replacer := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\\`, `\`)
	return replacer.Replace(inner), nil
}
