package egmmanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SealResult is what a successful seal reports. It carries no manifest content.
type SealResult struct {
	OutputPath string
	SHA256     string
	Cases      int
}

// SealBytes parses an unsigned manifest, populates every case's sealed_sha256 from the
// registered digest rule, and returns the sealed manifest together with its canonical
// output bytes: the canonical manifest plus exactly one trailing newline.
//
// The only field it fills in is sealed_sha256. A case whose sealed_sha256 is already set
// must match the recomputed digest; a mismatch means the record changed after sealing, and
// resealing it silently would launder that change.
func SealBytes(data []byte) (*Manifest, []byte, error) {
	m, err := parseManifest(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	if err := m.validate(false); err != nil {
		return nil, nil, err
	}
	for i := range m.Cases {
		c := &m.Cases[i]
		digest, err := CaseDigest(*c)
		if err != nil {
			return nil, nil, fmt.Errorf("egmmanifest: case %q: digest: %w", c.ID, err)
		}
		if c.SealedSHA256 != "" && c.SealedSHA256 != digest {
			return nil, nil, fmt.Errorf("egmmanifest: case %q: %w", c.ID, errSealedMismatch)
		}
		c.SealedSHA256 = digest
	}
	// Full validation now that the derived field is present.
	if err := m.Validate(); err != nil {
		return nil, nil, err
	}
	canonical, err := Canonical(m)
	if err != nil {
		return nil, nil, fmt.Errorf("egmmanifest: canonicalize sealed manifest: %w", err)
	}
	out := make([]byte, 0, len(canonical)+1)
	out = append(out, canonical...)
	out = append(out, '\n')
	return m, out, nil
}

var errSealedMismatch = errors.New("sealed_sha256 does not match the record; refusing to reseal a modified record")

// SealFile reads an unsigned manifest from inPath, seals it, and writes the canonical
// sealed manifest to outPath. The output is created exclusively: an existing file, a
// symlink, or a dangling symlink at outPath is an error. On any failure no output and no
// temporary file is left behind.
func SealFile(inPath, outPath string) (SealResult, error) {
	var zero SealResult
	if inPath == "" || outPath == "" {
		return zero, errors.New("egmmanifest: seal requires an input and an output path")
	}

	inAbs, err := filepath.Abs(inPath)
	if err != nil {
		return zero, err
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return zero, err
	}
	// Compare resolved paths so a symlink or a redundant component cannot alias them.
	realIn, err := resolveMaybe(inAbs)
	if err != nil {
		return zero, err
	}
	realOut, err := resolveMaybe(outAbs)
	if err != nil {
		return zero, err
	}
	if realIn == realOut {
		return zero, errors.New("egmmanifest: input and output are the same path")
	}
	// A pre-existing output is never overwritten, even by a hard link alias.
	if _, err := os.Lstat(outAbs); err == nil {
		return zero, errors.New("egmmanifest: output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return zero, err
	}

	data, err := os.ReadFile(inAbs)
	if err != nil {
		return zero, fmt.Errorf("egmmanifest: read input: %w", err)
	}
	m, out, err := SealBytes(data)
	if err != nil {
		return zero, err
	}

	sum := sha256.Sum256(out)
	digest := hex.EncodeToString(sum[:])
	if err := writeExclusive(outAbs, out); err != nil {
		return zero, err
	}
	return SealResult{OutputPath: outAbs, SHA256: digest, Cases: len(m.Cases)}, nil
}

// writeExclusive publishes data at path atomically. The bytes are written to a temporary
// file in the destination directory and linked into place; the link fails if the target
// exists, so this can neither overwrite nor expose a partially written file. The temporary
// file is removed on every path.
func writeExclusive(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".egmmanifest-seal-*")
	if err != nil {
		return fmt.Errorf("egmmanifest: create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := writeAndSync(tmp, data); err != nil {
		return fmt.Errorf("egmmanifest: write temporary output: %w", err)
	}
	// 0400 before publication, so the sealed file is never group/other readable.
	if err := os.Chmod(tmpName, 0o400); err != nil {
		return fmt.Errorf("egmmanifest: restrict temporary output: %w", err)
	}
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("egmmanifest: output already exists")
		}
		return fmt.Errorf("egmmanifest: publish output: %w", err)
	}
	return nil
}

// writeAndSync writes data to f, syncs it, and closes it.
func writeAndSync(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
