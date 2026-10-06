// Package repoindex builds a deterministic Structural Repository Index: a stable,
// machine-readable representation of repository structure for the Context Engine to
// consume later. The deterministic harness owns indexing — what is indexed, how symbols
// are extracted, how ordering is determined, how document authority is classified, and
// how index identity works. The model never drives any of it, and building the index
// invokes no model.
//
// The index describes structure, not source bodies: it records modules, packages, files,
// Go symbols (types, structs, interfaces, functions, methods), imports, tests, and
// repository documents with their deterministic class/authority/lifecycle. It performs no
// retrieval, ranking, embeddings, or semantic search.
package repoindex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// SchemaVersion is the version of the persisted index contract.
const SchemaVersion = 1

// FileName is the index artifact, written under the SOP state directory.
const FileName = "context/index.json"

// DocumentClass classifies an indexed repository document deterministically.
type DocumentClass string

const (
	ClassPlan          DocumentClass = "plan"
	ClassSpec          DocumentClass = "spec"
	ClassHistory       DocumentClass = "history"
	ClassDocumentation DocumentClass = "documentation"
	ClassConfig        DocumentClass = "config"
	ClassOther         DocumentClass = "other"
)

// Authority is how much a document is current authority.
type Authority string

const (
	AuthorityCurrent    Authority = "current"
	AuthorityHistorical Authority = "historical"
	AuthorityUnknown    Authority = "unknown"
)

// Lifecycle is a document's planning lifecycle where known.
type Lifecycle string

const (
	LifecycleActive     Lifecycle = "active"
	LifecyclePlanned    Lifecycle = "planned"
	LifecycleComplete   Lifecycle = "complete"
	LifecycleSuperseded Lifecycle = "superseded"
	LifecycleUnknown    Lifecycle = "unknown"
)

// FileKind classifies an indexed file.
type FileKind string

const (
	FileGo     FileKind = "go"
	FileGoTest FileKind = "go_test"
	FileMD     FileKind = "markdown"
	FileYAML   FileKind = "yaml"
	FileJSON   FileKind = "json"
	FileTOML   FileKind = "toml"
	FileConfig FileKind = "config"
	FileShell  FileKind = "shell"
	FileOther  FileKind = "other"
)

// SymbolKind classifies a Go declaration.
type SymbolKind string

const (
	SymType      SymbolKind = "type"
	SymStruct    SymbolKind = "struct"
	SymInterface SymbolKind = "interface"
	SymFunction  SymbolKind = "function"
	SymMethod    SymbolKind = "method"
)

// Module is a Go module (a directory with a go.mod).
type Module struct {
	Path string `json:"path"`
	Dir  string `json:"dir"`
}

// Package is a Go package (a directory of .go files).
type Package struct {
	ImportPath string `json:"import_path"`
	Dir        string `json:"dir"`
	Name       string `json:"name"`
	Module     string `json:"module,omitempty"`
}

// File is one indexed file.
type File struct {
	Path    string   `json:"path"`
	Kind    FileKind `json:"kind"`
	Package string   `json:"package,omitempty"`
}

// Symbol is one Go declaration with a stable identity and location.
type Symbol struct {
	ID       string     `json:"id"`
	Kind     SymbolKind `json:"kind"`
	Package  string     `json:"package"`
	File     string     `json:"file"`
	Name     string     `json:"name"`
	Receiver string     `json:"receiver,omitempty"`
	Line     int        `json:"line"`
	Exported bool       `json:"exported"`
	Test     bool       `json:"test,omitempty"`
}

// Import is one import edge from a file.
type Import struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Path    string `json:"path"`
	Alias   string `json:"alias,omitempty"`
}

// Document is one indexed repository document with deterministic semantics.
type Document struct {
	Path      string        `json:"path"`
	Class     DocumentClass `json:"document_class"`
	Authority Authority     `json:"authority"`
	Lifecycle Lifecycle     `json:"lifecycle"`
	// AuthoritySource records how the authority was decided: sop | header | path | unknown.
	AuthoritySource string `json:"authority_source"`
}

// Counts summarizes the index deterministically.
type Counts struct {
	Modules    int `json:"modules"`
	Packages   int `json:"packages"`
	Files      int `json:"files"`
	Types      int `json:"types"`
	Structs    int `json:"structs"`
	Interfaces int `json:"interfaces"`
	Functions  int `json:"functions"`
	Methods    int `json:"methods"`
	Imports    int `json:"imports"`
	Tests      int `json:"tests"`
	Documents  int `json:"documents"`
	Current    int `json:"documents_current"`
	Historical int `json:"documents_historical"`
	Unknown    int `json:"documents_unknown"`
}

// Identity is the deterministic index identity: it captures the repository state the
// index corresponds to, so a caller can tell whether the index is still current.
type Identity struct {
	SchemaVersion int    `json:"schema_version"`
	Head          string `json:"head,omitempty"`
	Dirty         bool   `json:"dirty"`
	// Digest is a stable digest over the indexed files (path + content hash), so identical
	// repository state yields identical identity and any content change yields a new one.
	Digest string `json:"digest"`
}

// Index is the canonical Structural Repository Index.
type Index struct {
	SchemaVersion int        `json:"schema_version"`
	Identity      Identity   `json:"identity"`
	Modules       []Module   `json:"modules"`
	Packages      []Package  `json:"packages"`
	Files         []File     `json:"files"`
	Symbols       []Symbol   `json:"symbols"`
	Imports       []Import   `json:"imports"`
	Documents     []Document `json:"documents"`
	Counts        Counts     `json:"counts"`
}

// Options configures Build. Head and Dirty describe the repository state; when empty,
// Build derives them from git if the repository is a working tree.
type Options struct {
	Root  string
	Head  string
	Dirty bool
	// StateDir overrides the SOP state directory name (default ".agent-sdlc").
	StateDir string
}

// Build indexes the repository at opts.Root deterministically. It is model-free and
// read-only. Only the files it indexes affect the result; collections are fully ordered.
func Build(opts Options) (Index, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return Index{}, errors.New("repoindex: root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Index{}, fmt.Errorf("repoindex: resolve root: %w", err)
	}
	stateDir := opts.StateDir
	if stateDir == "" {
		stateDir = ".agent-sdlc"
	}

	sop := readSOPMetadata(abs, stateDir)

	paths, err := walkIndexable(abs, stateDir)
	if err != nil {
		return Index{}, err
	}

	idx := Index{SchemaVersion: SchemaVersion}
	digest := sha256.New()

	// Modules: every go.mod directory (the module root's own go.mod first, then others).
	modByDir := map[string]string{}
	var moduleDirs []string
	for _, rel := range paths {
		if filepath.Base(rel) == "go.mod" {
			mp, err := modulePath(filepath.Join(abs, rel))
			if err != nil {
				return Index{}, err
			}
			dir := filepath.ToSlash(filepath.Dir(rel))
			if dir == "." {
				dir = ""
			}
			modByDir[dir] = mp
			moduleDirs = append(moduleDirs, dir)
		}
	}
	sort.Strings(moduleDirs)
	for _, dir := range moduleDirs {
		idx.Modules = append(idx.Modules, Module{Path: modByDir[dir], Dir: dir})
	}

	// Go files: parse and extract packages, symbols, imports.
	type goFile struct {
		path    string
		pkgName string
		impPath string
		syms    []Symbol
		imps    []Import
	}
	var goFiles []goFile
	for _, rel := range paths {
		if !strings.HasSuffix(rel, ".go") {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		if dir == "." {
			dir = ""
		}
		impPath, pkgName, syms, imps, err := parseGo(abs, rel, dir, modByDir)
		if err != nil {
			return Index{}, err
		}
		goFiles = append(goFiles, goFile{path: rel, pkgName: pkgName, impPath: impPath, syms: syms, imps: imps})
	}

	// Packages grouped by directory.
	pkgByDir := map[string]*Package{}
	for _, gf := range goFiles {
		dir := filepath.ToSlash(filepath.Dir(gf.path))
		if dir == "." {
			dir = ""
		}
		p := pkgByDir[dir]
		if p == nil {
			p = &Package{ImportPath: gf.impPath, Dir: dir, Name: gf.pkgName, Module: moduleOf(dir, modByDir)}
			pkgByDir[dir] = p
		}
	}
	var pkgDirs []string
	for dir := range pkgByDir {
		pkgDirs = append(pkgDirs, dir)
	}
	sort.Strings(pkgDirs)
	for _, dir := range pkgDirs {
		idx.Packages = append(idx.Packages, *pkgByDir[dir])
	}

	// Files, symbols, imports.
	for _, rel := range paths {
		h, err := hashFile(filepath.Join(abs, rel))
		if err != nil {
			return Index{}, err
		}
		_, _ = io.WriteString(digest, rel)
		digest.Write([]byte{0})
		_, _ = io.WriteString(digest, h)
		digest.Write([]byte{0})

		kind := classifyFile(rel)
		f := File{Path: rel, Kind: kind}
		if kind == FileGo || kind == FileGoTest {
			dir := filepath.ToSlash(filepath.Dir(rel))
			if dir == "." {
				dir = ""
			}
			if p := pkgByDir[dir]; p != nil {
				f.Package = p.ImportPath
			}
		}
		idx.Files = append(idx.Files, f)

		if kind == FileGo || kind == FileGoTest {
			for _, gf := range goFiles {
				if gf.path != rel {
					continue
				}
				idx.Symbols = append(idx.Symbols, gf.syms...)
				idx.Imports = append(idx.Imports, gf.imps...)
				break
			}
			continue
		}

		// Non-Go documents.
		class := classifyDocument(rel)
		if class == "" {
			continue
		}
		doc := classifyAuthority(rel, class, sop)
		idx.Documents = append(idx.Documents, doc)
	}

	// Sort every collection deterministically.
	sort.Slice(idx.Symbols, func(i, j int) bool { return idx.Symbols[i].ID < idx.Symbols[j].ID })
	sort.Slice(idx.Imports, func(i, j int) bool {
		a, b := idx.Imports[i], idx.Imports[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Alias < b.Alias
	})
	sort.Slice(idx.Documents, func(i, j int) bool { return idx.Documents[i].Path < idx.Documents[j].Path })

	idx.Counts = countIndex(idx)
	idx.Identity = Identity{
		SchemaVersion: SchemaVersion,
		Head:          strings.TrimSpace(opts.Head),
		Dirty:         opts.Dirty,
		Digest:        hex.EncodeToString(digest.Sum(nil))[:16],
	}
	return idx, nil
}

// Marshal renders the index as canonical JSON (stable field order, trailing newline).
func (idx Index) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
