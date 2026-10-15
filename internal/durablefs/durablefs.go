// Package durablefs provides a crash-safe file write: content is written to a temp
// file in the destination directory, fsynced, atomically renamed over the
// destination, and the destination directory is fsynced. After WriteFile returns
// successfully, a crash cannot leave a truncated destination and the new content is
// durable.
//
// It is the shared "committed boundary" primitive for small control files - the
// question store's document, its own marker/checkpoint files, and a run's state
// checkpoint - so a reader never observes a half-written file and a caller can
// require durability (fsync) before recording a dependent fact such as success.
package durablefs

import (
	"os"
	"path/filepath"
)

// WriteFile writes data to path durably: a temp file in the same directory is written
// and fsynced, renamed over the destination, and the directory is fsynced.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
