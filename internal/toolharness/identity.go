package toolharness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/imhttran/agentic-sop/internal/sensitivepath"
)

// The approved LLM02 Option C Phase 1 policy adds a bounded protected-inode
// registry to the existing lexical and symlink credential-path denial. A
// hardlink is a second directory entry for the same inode, so the lexical
// classifier cannot see the denied name behind an ordinary alias. The registry
// records the identity of every lexically denied credential file in the
// authorized roots, and the content tools verify the opened descriptor against
// it before returning content.
//
// This is deliberately not a second authorization framework and not general
// DLP: it reuses sensitivepath.IsToolReadDenied for the vocabulary, the existing
// root boundary for the scan, and the existing model-input redaction unchanged.
const (
	maxIdentityScanEntries = 200000
	maxIdentityEntries     = 100000
)

var (
	// ErrIdentityRegistryUnavailable reports that a content tool refused to
	// return content because the protected-identity registry could not be
	// established within its bound. It is a policy refusal, not a tool error.
	ErrIdentityRegistryUnavailable = errors.New("refusing to expose content: hardlink identity registry unavailable")
	errIdentityRegistryFull        = errors.New("hardlink identity registry entry bound exceeded")
)

// protectedIdentity is one lexically denied file's filesystem identity. The
// relative path is retained for the denial detail; it never contains content.
type protectedIdentity struct {
	root string
	rel  string
	info fs.FileInfo
}

// identityRegistry is built once per harness and merged when a content-write
// tool creates or replaces a denied path. It is safe for concurrent use.
type identityRegistry struct {
	once    sync.Once
	mu      sync.RWMutex
	entries []protectedIdentity
	err     error
}

// ensure builds the registry once. A construction failure is retained and
// returned to every content tool, so the failure is fail-closed.
func (r *identityRegistry) ensure(roots []authorizedRoot) error {
	r.once.Do(func() {
		entries, err := scanProtectedIdentities(roots, maxIdentityScanEntries, maxIdentityEntries)
		r.mu.Lock()
		defer r.mu.Unlock()
		if err != nil {
			r.err = fmt.Errorf("%w: %v", ErrIdentityRegistryUnavailable, err)
			return
		}
		r.entries = entries
	})
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.err
}

// match reports whether info identifies a protected credential file.
func (r *identityRegistry) match(info fs.FileInfo) (protectedIdentity, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if os.SameFile(entry.info, info) {
			return entry, true
		}
	}
	return protectedIdentity{}, false
}

// merge adds identities discovered after the initial scan. Entries are never
// removed: a denied name can disappear while its inode is still reachable
// through a hardlink, and forgetting it would reopen the gap. Duplicate
// identities are ignored.
func (r *identityRegistry) merge(entries []protectedIdentity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	for _, entry := range entries {
		if identityPresent(r.entries, entry.info) {
			continue
		}
		if len(r.entries) >= maxIdentityEntries {
			r.err = fmt.Errorf("%w: %v", ErrIdentityRegistryUnavailable, errIdentityRegistryFull)
			return r.err
		}
		r.entries = append(r.entries, entry)
	}
	return nil
}

func identityPresent(entries []protectedIdentity, info fs.FileInfo) bool {
	for _, entry := range entries {
		if os.SameFile(entry.info, info) {
			return true
		}
	}
	return false
}

// scanProtectedIdentities walks each authorized root once and records the
// identity of every regular file whose lexical path is denied by the approved
// credential vocabulary. A walk error, an unreadable entry, or exceeding either
// bound is a construction failure, not a silent partial registry.
func scanProtectedIdentities(roots []authorizedRoot, maxScan, maxProtected int) ([]protectedIdentity, error) {
	var identities []protectedIdentity
	scanned := 0
	for _, root := range roots {
		err := filepath.WalkDir(root.path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("scan %s: %w", path, walkErr)
			}
			scanned++
			if scanned > maxScan {
				return fmt.Errorf("workspace scan exceeded %d entries", maxScan)
			}
			if entry.IsDir() {
				if path != root.path && (entry.Name() == ".git" || entry.Name() == stateDirRel) {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			rel, err := filepath.Rel(root.path, path)
			if err != nil {
				return fmt.Errorf("resolve %s relative to the authorized root: %w", path, err)
			}
			rel = filepath.ToSlash(rel)
			if !sensitivepath.IsToolReadDenied(rel) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("inspect %s: %w", path, err)
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			identities = append(identities, protectedIdentity{root: root.path, rel: rel, info: info})
			if len(identities) > maxProtected {
				return fmt.Errorf("protected credential identity bound of %d exceeded", maxProtected)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return identities, nil
}

// ensureIdentityRegistry builds the registry on first content read. Reads fail
// closed when it cannot be built.
func (h *Harness) ensureIdentityRegistry() error {
	if h.identity == nil {
		return fmt.Errorf("%w: registry is not initialized", ErrIdentityRegistryUnavailable)
	}
	return h.identity.ensure(h.roots)
}

// openVerifiedContent opens a resolved path and verifies the opened descriptor
// against the protected-identity registry before returning it. The caller reads
// from the same descriptor, so the identity decision and the read cannot be
// separated by a path replacement.
func (h *Harness) openVerifiedContent(abs, display string) (*os.File, error) {
	if err := h.ensureIdentityRegistry(); err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("verify %s: %w", display, err)
	}
	if protected, ok := h.identity.match(info); ok {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %s (identity matches %s)", ErrSensitivePath, display, protected.rel)
	}
	return f, nil
}

// refreshIdentity merges a newly written denied path into the registry. It is
// called only after a successful write, and it never removes old identities.
func (h *Harness) refreshIdentity(p resolvedPath) {
	if h.identity == nil {
		return
	}
	rel, err := filepath.Rel(p.root, p.abs)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if !sensitivepath.IsToolReadDenied(rel) && !sensitivepath.IsToolReadDenied(p.requested) {
		return
	}
	info, err := os.Stat(p.abs)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	if err := h.ensureIdentityRegistry(); err != nil {
		return
	}
	_ = h.identity.merge([]protectedIdentity{{root: p.root, rel: rel, info: info}})
}
