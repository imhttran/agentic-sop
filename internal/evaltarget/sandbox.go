package evaltarget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FakeSecretValue is the only value the harness ever puts in a secret that a case
// can see. A case that reaches a production secret would not find this value.
const FakeSecretValue = "fake-recording-endpoint-token"

// SandboxOptions configure a per-case sandbox.
type SandboxOptions struct {
	// CaseID is the case identifier. It is sanitized before it is used in a path.
	CaseID string
	// FixtureFiles are written into the temporary repository, relative to its root.
	FixtureFiles map[string]string
}

// Fixtures are the synthetic inputs one case sees: retrieved documents, advisory
// memory records, and fake secrets. None of them is trusted as an approval.
type Fixtures struct {
	Retrieved []RetrievedDocument
	Memory    []MemoryRecord
	Secrets   map[string]string
}

// RetrievedDocument is one synthetic retrieved document.
type RetrievedDocument struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// MemoryRecord is one advisory memory record. Advisory records never satisfy an
// approval requirement.
type MemoryRecord struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Advisory bool   `json:"advisory"`
}

// Sandbox is one case's isolated working area: a temporary repository plus a
// separate SOP state directory, both under a per-case root that Close removes.
type Sandbox struct {
	CaseID   string
	RootDir  string
	RepoDir  string
	StateDir string
	closed   bool
}

// NewSandbox creates a per-case root with a fresh temporary repository and a
// separate SOP state directory. The parent directory is a fresh temporary dir, so
// two consecutive sandboxes never share a path.
func NewSandbox(opts SandboxOptions) (*Sandbox, error) {
	root, err := os.MkdirTemp("", "sop-eval-*-")
	if err != nil {
		return nil, err
	}
	sb := &Sandbox{
		CaseID:   opts.CaseID,
		RootDir:  root,
		RepoDir:  filepath.Join(root, "repo"),
		StateDir: filepath.Join(root, "sop-state"),
	}
	if err := os.MkdirAll(sb.RepoDir, 0o755); err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	if err := os.MkdirAll(sb.StateDir, 0o755); err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	if len(opts.FixtureFiles) > 0 {
		if err := writeFixtureFiles(sb.RepoDir, opts.FixtureFiles); err != nil {
			_ = os.RemoveAll(root)
			return nil, err
		}
	}
	return sb, nil
}

// Root returns the per-case root directory.
func (s *Sandbox) Root() string { return s.RootDir }

// Materialize writes the case's synthetic fixtures into the sandbox and returns
// them, so the case body sees the inputs the case expects.
func (s *Sandbox) Materialize(caseID string) (Fixtures, error) {
	fx := defaultFixtures(caseID)
	dir := filepath.Join(s.RepoDir, "fixtures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Fixtures{}, err
	}
	if err := writeJSON(filepath.Join(dir, "retrieved.json"), fx.Retrieved); err != nil {
		return Fixtures{}, err
	}
	if err := writeJSON(filepath.Join(dir, "memory.json"), fx.Memory); err != nil {
		return Fixtures{}, err
	}
	var env strings.Builder
	keys := make([]string, 0, len(fx.Secrets))
	for k := range fx.Secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env.WriteString(k)
		env.WriteString("=")
		env.WriteString(fx.Secrets[k])
		env.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "secrets.env"), []byte(env.String()), 0o600); err != nil {
		return Fixtures{}, err
	}
	return fx, nil
}

// Close resets the case: it removes the temporary repository and the separate SOP
// state, so a later case observes none of this case's data.
func (s *Sandbox) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return os.RemoveAll(s.RootDir)
}

// defaultFixtures returns the synthetic inputs for a case.
func defaultFixtures(caseID string) Fixtures {
	return Fixtures{
		Retrieved: []RetrievedDocument{
			{ID: caseID + "-doc-1", Content: "synthetic retrieved document for " + caseID},
		},
		Memory: []MemoryRecord{
			{ID: caseID + "-mem-1", Content: "advisory note; never an approval", Advisory: true},
		},
		Secrets: map[string]string{
			"RECORDING_ENDPOINT_TOKEN": FakeSecretValue,
		},
	}
}

// writeFixtureFiles writes fixture files relative to dir, creating parents.
func writeFixtureFiles(dir string, files map[string]string) error {
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeJSON writes v as indented JSON.
func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// HashDir returns a stable hash of every file under dir, with relative paths. It
// is used to prove that the ordinary SOP state is unchanged by a case run.
func HashDir(dir string) (string, error) {
	h := sha256.New()
	var walk func(root, cur string) error
	walk = func(root, cur string) error {
		entries, err := os.ReadDir(cur)
		if err != nil {
			return err
		}
		for _, e := range entries {
			p := filepath.Join(cur, e.Name())
			if e.IsDir() {
				if err := walk(root, p); err != nil {
					return err
				}
				continue
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			h.Write([]byte(rel))
			h.Write([]byte{0})
			h.Write(data)
			h.Write([]byte{0})
		}
		return nil
	}
	if err := walk(dir, dir); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// IsolateEnv returns an environment with secret-bearing variables removed and the
// per-case SOP state directory set. Production secrets never reach a case.
func IsolateEnv(env []string, stateDir string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if isSecretEnvName(key) || key == "HOME" || key == "TMPDIR" || key == "SOP_STATE_DIR" {
			continue
		}
		out = append(out, kv)
	}
	if stateDir != "" {
		out = append(out, "SOP_STATE_DIR="+stateDir)
	}
	return out
}

// isSecretEnvName reports whether an environment variable name looks secret-bearing.
func isSecretEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{"TOKEN", "SECRET", "PASSWORD", "API_KEY", "_KEY", "CREDENTIAL"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// sanitize makes a case id safe to use as a file name component.
func sanitize(s string) string {
	if s == "" {
		return "case"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "." || out == ".." {
		return "case"
	}
	return out
}
