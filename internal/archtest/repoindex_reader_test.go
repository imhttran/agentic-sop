package archtest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPersistedRepoIndexReadersVerifyRows guards OWASP LLM04. The repository index is
// written to context/index.json by `sop index`, and each row carries a digest that only
// repoindex.VerifyRows checks. Nothing reads the file back today, so the digests are
// latent. This fails if a file reads the persisted index without calling VerifyRows,
// so a future reader cannot skip the check silently.
func TestPersistedRepoIndexReadersVerifyRows(t *testing.T) {
	root := moduleRoot(t)
	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", ".claude", "archive", "testdata", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		readsIndex := strings.Contains(src, "repoindex.FileName") &&
			(strings.Contains(src, "os.ReadFile") || strings.Contains(src, "os.Open"))
		if readsIndex && !strings.Contains(src, "repoindex.VerifyRows(") {
			rel, _ := filepath.Rel(root, path)
			violations = append(violations, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("files read the persisted repository index without repoindex.VerifyRows: %s", strings.Join(violations, ", "))
	}
}
