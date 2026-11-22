package egmmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Problem is one verification failure. Detail never contains manifest contents or
// vocabulary terms.
type Problem struct {
	CaseID string
	Kind   string
	Detail string
}

// Report is the result of Verify. OK reports whether every case verified.
type Report struct {
	Cases    int
	Verified int
	Problems []Problem
}

// OK reports whether the manifest verified with no problems.
func (r *Report) OK() bool {
	return r != nil && len(r.Problems) == 0
}

// Options controls the optional artifact and leakage checks.
type Options struct {
	// ArtifactRoot, when set, is the directory against which artifact_path and
	// permitted_references paths are resolved and hashed.
	ArtifactRoot string
	// TreeRoot, when set, is a single-case exported reviewer tree whose deterministic
	// hash is compared with exported_tree_sha256.
	TreeRoot string
	// Vocabulary, when set, scans artifact and reference files for forbidden terms.
	Vocabulary *Vocabulary
}

// Verify validates the manifest, recomputes every case digest, and performs the optional
// artifact, reference, tree, and leakage checks. A schema failure returns an error; other
// failures are reported as Problems. Any problem means verification failed closed.
func Verify(m *Manifest, opts Options) (*Report, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	rep := &Report{Cases: len(m.Cases)}
	for i := range m.Cases {
		c := &m.Cases[i]
		want, err := CaseDigest(*c)
		if err != nil {
			return nil, err
		}
		if c.SealedSHA256 != want {
			rep.Problems = append(rep.Problems, Problem{c.ID, "digest-mismatch", "sealed_sha256 does not match the canonical record"})
			continue
		}
		rep.Verified++
		if opts.ArtifactRoot != "" {
			if err := verifyArtifacts(c, opts, rep); err != nil {
				return nil, err
			}
		}
	}
	if opts.TreeRoot != "" {
		if len(m.Cases) != 1 {
			return nil, errors.New("egmmanifest: TreeRoot requires exactly one case")
		}
		got, err := TreeHash(opts.TreeRoot)
		if err != nil {
			return nil, err
		}
		if got != m.Cases[0].ExportedTreeSHA256 {
			rep.Problems = append(rep.Problems, Problem{m.Cases[0].ID, "tree-mismatch", "exported_tree_sha256 does not match the exported tree"})
		}
	}
	return rep, nil
}

func verifyArtifacts(c *Case, opts Options, rep *Report) error {
	art, err := safeJoin(opts.ArtifactRoot, c.ArtifactPath)
	if err != nil {
		rep.Problems = append(rep.Problems, Problem{c.ID, "invalid-path", "artifact_path escapes the artifact root"})
		return nil
	}
	if err := checkFile(art, c.ArtifactSHA256, "artifact", c.ID, opts.Vocabulary, rep); err != nil {
		return err
	}
	for _, ref := range c.PermittedReferences {
		p, err := safeJoin(opts.ArtifactRoot, ref.Path)
		if err != nil {
			rep.Problems = append(rep.Problems, Problem{c.ID, "invalid-path", "a permitted reference path escapes the artifact root"})
			continue
		}
		if err := checkFile(p, ref.BlobSHA256, "reference", c.ID, opts.Vocabulary, rep); err != nil {
			return err
		}
	}
	return nil
}

func checkFile(path, wantHash, kind, caseID string, vocab *Vocabulary, rep *Report) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		rep.Problems = append(rep.Problems, Problem{caseID, kind + "-missing", kind + " file is missing"})
		return nil
	}
	if err != nil {
		rep.Problems = append(rep.Problems, Problem{caseID, kind + "-unreadable", kind + " file is unreadable"})
		return nil
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != wantHash {
		rep.Problems = append(rep.Problems, Problem{caseID, kind + "-mismatch", kind + "_sha256 does not match"})
	}
	if vocab != nil {
		if hits := vocab.Scan(data); len(hits) > 0 {
			rep.Problems = append(rep.Problems, Problem{caseID, "leak", fmt.Sprintf("%d forbidden-term hit(s) in the %s", len(hits), kind)})
		}
	}
	return nil
}

func safeJoin(root, rel string) (string, error) {
	if !safeRelativePath(rel) {
		return "", fmt.Errorf("unsafe relative path %q", rel)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	p := filepath.Join(absRoot, filepath.FromSlash(rel))
	absP, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if !within(absP, absRoot) {
		return "", fmt.Errorf("path escapes root")
	}
	return absP, nil
}
