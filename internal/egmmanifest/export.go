package egmmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PublicCase is the reviewer-visible subset of a case. It excludes every answer-bearing
// or private field: seeded_defect, determinability_rationale, expected_finding,
// distractors, control_check, leak_checks, authored_by, and sealed_sha256.
type PublicCase struct {
	ID                     string `json:"id"`
	ArtifactPath           string `json:"artifact_path"`
	ArtifactSHA256         string `json:"artifact_sha256"`
	ExportedTreeSHA256     string `json:"exported_tree_sha256"`
	RevisionPin            string `json:"revision_pin"`
	PermittedReferences    []Ref  `json:"permitted_references,omitempty"`
	InstructionBlockSHA256 string `json:"instruction_block_sha256"`
}

// PublicManifest is the reviewer-visible subset of a manifest.
type PublicManifest struct {
	SchemaVersion int          `json:"schema_version"`
	Corpus        string       `json:"corpus"`
	Cases         []PublicCase `json:"cases"`
}

// ReviewerExport returns the canonical reviewer-visible JSON. Private fields are not
// included, so changing a private field does not change this export.
func ReviewerExport(m *Manifest) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	pm := PublicManifest{SchemaVersion: m.SchemaVersion, Corpus: m.Corpus, Cases: make([]PublicCase, 0, len(m.Cases))}
	for _, c := range m.Cases {
		refs := append([]Ref(nil), c.PermittedReferences...)
		pm.Cases = append(pm.Cases, PublicCase{
			ID:                     c.ID,
			ArtifactPath:           c.ArtifactPath,
			ArtifactSHA256:         c.ArtifactSHA256,
			ExportedTreeSHA256:     c.ExportedTreeSHA256,
			RevisionPin:            c.RevisionPin,
			PermittedReferences:    refs,
			InstructionBlockSHA256: c.InstructionBlockSHA256,
		})
	}
	return Canonical(pm)
}

// ReviewerExportDigest returns the SHA-256 of ReviewerExport.
func ReviewerExportDigest(m *Manifest) (string, error) {
	b, err := ReviewerExport(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// TreeHash returns the SHA-256 of the sorted `relative-path<TAB>blob-sha256` lines for
// every regular file under root. skipNames excludes a file or directory by base name
// (for example ".git"). A non-regular file is an error; so a symlink cannot silently
// enter an exported tree.
func TreeHash(root string, skipNames ...string) (string, error) {
	skip := make(map[string]bool, len(skipNames))
	for _, s := range skipNames {
		skip[s] = true
	}
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && skip[name] {
				return fs.SkipDir
			}
			return nil
		}
		if skip[name] {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("egmmanifest: exported tree contains a non-regular file")
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		lines = append(lines, filepath.ToSlash(rel)+"\t"+hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(lines)
	payload := strings.Join(lines, "\n")
	if len(lines) > 0 {
		payload += "\n"
	}
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:]), nil
}

// FormatProblem renders a Problem without any manifest content or vocabulary term.
func FormatProblem(p Problem) string {
	return fmt.Sprintf("case=%s kind=%s detail=%s", p.CaseID, p.Kind, p.Detail)
}
