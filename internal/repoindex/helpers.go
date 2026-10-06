package repoindex

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultExcludedDirs are directory names the index never descends into. They match the
// repository's own conventions (state, VCS, dependencies, build output, editor scratch).
var defaultExcludedDirs = map[string]bool{
	".git": true, ".agent-sdlc": true, "vendor": true, "node_modules": true,
	"dist": true, "build": true, "tmp": true, ".agents": true, ".claude": true,
	".playwright-mcp": true,
}

// indexableExt reports whether a file name is one the index reads.
func indexableExt(name string) bool {
	switch {
	case strings.HasSuffix(name, ".go"),
		strings.HasSuffix(name, ".md"),
		strings.HasSuffix(name, ".yaml"),
		strings.HasSuffix(name, ".yml"),
		strings.HasSuffix(name, ".json"),
		strings.HasSuffix(name, ".toml"),
		strings.HasSuffix(name, ".sh"),
		name == "go.mod",
		name == "go.sum",
		name == ".env.example":
		return true
	}
	return false
}

// walkIndexable returns the repository-relative, slash-separated paths of every indexable
// file, deterministically sorted.
func walkIndexable(root, stateDir string) ([]string, error) {
	excluded := map[string]bool{}
	for k, v := range defaultExcludedDirs {
		excluded[k] = v
	}
	excluded[stateDir] = true

	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if excluded[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || excluded[d.Name()] {
			return nil
		}
		if !indexableExt(d.Name()) {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("repoindex: walk: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

// classifyFile maps a path to its file kind.
func classifyFile(rel string) FileKind {
	base := filepath.Base(rel)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return FileGoTest
	case strings.HasSuffix(base, ".go"):
		return FileGo
	case strings.HasSuffix(base, ".md"):
		return FileMD
	case strings.HasSuffix(base, ".yaml"), strings.HasSuffix(base, ".yml"):
		return FileYAML
	case strings.HasSuffix(base, ".json"):
		return FileJSON
	case strings.HasSuffix(base, ".toml"):
		return FileTOML
	case base == ".env.example", base == "go.mod", base == "go.sum":
		return FileConfig
	case strings.HasSuffix(base, ".sh"):
		return FileShell
	}
	return FileOther
}

// classifyDocument maps a non-Go path to its document class. It returns "" only for a
// path the caller should not treat as a document.
func classifyDocument(rel string) DocumentClass {
	base := filepath.Base(rel)
	switch {
	case strings.HasPrefix(rel, "docs/plans/"), strings.HasPrefix(rel, "docs/history/plans/"):
		return ClassPlan
	case strings.HasPrefix(rel, "docs/specs/"):
		return ClassSpec
	case strings.HasPrefix(rel, "docs/history/"):
		return ClassHistory
	case strings.HasPrefix(rel, "docs/"):
		if strings.HasSuffix(base, ".md") {
			return ClassDocumentation
		}
		return ClassConfig
	}
	if strings.HasSuffix(base, ".md") {
		if strings.HasPrefix(base, "PLAN") || strings.HasPrefix(base, "PHASE") {
			return ClassPlan
		}
		return ClassDocumentation
	}
	return ClassConfig
}

// classifyAuthority decides a document's authority and lifecycle. The order of authority
// is: explicit SOP lifecycle metadata, then an explicit document header, then a well-known
// repository path, then unknown. Filesystem placement never overrides SOP lifecycle state.
func classifyAuthority(rel string, class DocumentClass, sop sopMeta) Document {
	doc := Document{Path: rel, Class: class, Authority: AuthorityUnknown, Lifecycle: LifecycleUnknown, AuthoritySource: "unknown"}

	if sop.activeSource != "" && rel == sop.activeSource {
		doc.Authority, doc.Lifecycle, doc.AuthoritySource = AuthorityCurrent, LifecycleActive, "sop"
		return doc
	}
	if disp, ok := sop.archived[rel]; ok {
		doc.Authority, doc.Lifecycle, doc.AuthoritySource = AuthorityHistorical, dispositionLifecycle(disp), "sop"
		return doc
	}
	if life, auth, ok := readDocHeader(filepath.Join(sop.root, filepath.FromSlash(rel))); ok {
		doc.Authority, doc.Lifecycle, doc.AuthoritySource = auth, life, "header"
		return doc
	}
	if strings.HasPrefix(rel, "docs/history/plans/") {
		doc.Authority, doc.AuthoritySource = AuthorityHistorical, "path"
		return doc
	}
	return doc
}

// dispositionLifecycle maps an archive disposition to a lifecycle value.
func dispositionLifecycle(disp string) Lifecycle {
	switch strings.ToLower(strings.TrimSpace(disp)) {
	case "complete":
		return LifecycleComplete
	case "superseded":
		return LifecycleSuperseded
	}
	return LifecycleComplete
}

// readDocHeader reads a lightweight Markdown status header from the top of a document.
func readDocHeader(path string) (Lifecycle, Authority, bool) {
	f, err := os.Open(path)
	if err != nil {
		return LifecycleUnknown, AuthorityUnknown, false
	}
	defer f.Close()

	life, auth := LifecycleUnknown, AuthorityUnknown
	found := false
	sc := bufio.NewScanner(f)
	for i := 0; i < 15 && sc.Scan(); i++ {
		line := sc.Text()
		if strings.Contains(line, "Authority:") {
			switch {
			case strings.Contains(line, "historical"):
				auth, found = AuthorityHistorical, true
			case strings.Contains(line, "current"):
				auth, found = AuthorityCurrent, true
			}
		}
		if strings.Contains(line, "Lifecycle:") {
			switch {
			case strings.Contains(line, "complete"):
				life = LifecycleComplete
			case strings.Contains(line, "superseded"):
				life = LifecycleSuperseded
			case strings.Contains(line, "active"):
				life = LifecycleActive
			case strings.Contains(line, "planned"):
				life = LifecyclePlanned
			}
		}
	}
	return life, auth, found
}

// parseGo parses one Go file and extracts its package name, import path, symbols, and
// imports. It uses go/parser with object resolution skipped, so it needs no type
// information and holds no transient AST identity.
func parseGo(root, rel, dir string, modByDir map[string]string) (impPath, pkgName string, syms []Symbol, imps []Import, err error) {
	fset := token.NewFileSet()
	f, perr := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.SkipObjectResolution)
	if perr != nil {
		return "", "", nil, nil, fmt.Errorf("repoindex: parse %s: %w", rel, perr)
	}
	impPath = importPathOf(dir, modByDir)
	pkgName = f.Name.Name
	isTest := strings.HasSuffix(rel, "_test.go")

	for _, imp := range f.Imports {
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		imps = append(imps, Import{Package: impPath, File: rel, Path: strings.Trim(imp.Path.Value, `"`), Alias: alias})
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				kind := SymType
				switch ts.Type.(type) {
				case *ast.StructType:
					kind = SymStruct
				case *ast.InterfaceType:
					kind = SymInterface
				}
				syms = append(syms, Symbol{
					ID: symbolID(kind, impPath, "", ts.Name.Name), Kind: kind, Package: impPath,
					File: rel, Name: ts.Name.Name, Line: fset.Position(ts.Pos()).Line,
					Exported: ts.Name.IsExported(), Test: isTest,
				})
			}
		case *ast.FuncDecl:
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := receiverName(d.Recv.List[0].Type)
				syms = append(syms, Symbol{
					ID: symbolID(SymMethod, impPath, recv, d.Name.Name), Kind: SymMethod, Package: impPath,
					File: rel, Name: d.Name.Name, Receiver: recv, Line: fset.Position(d.Pos()).Line,
					Exported: d.Name.IsExported(), Test: isTest,
				})
				continue
			}
			syms = append(syms, Symbol{
				ID: symbolID(SymFunction, impPath, "", d.Name.Name), Kind: SymFunction, Package: impPath,
				File: rel, Name: d.Name.Name, Line: fset.Position(d.Pos()).Line,
				Exported: d.Name.IsExported(), Test: isTest,
			})
		}
	}
	return impPath, pkgName, syms, imps, nil
}

// symbolID is a stable symbol identity: same repository structure yields the same id.
func symbolID(kind SymbolKind, impPath, recv, name string) string {
	if recv != "" {
		return string(kind) + ":" + impPath + "." + recv + "." + name
	}
	return string(kind) + ":" + impPath + "." + name
}

// receiverName renders a receiver type expression to its base type name.
func receiverName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	}
	return ""
}

// withinModule reports whether dir is inside module directory md (md "" is the repo root).
func withinModule(dir, md string) bool {
	if md == "" {
		return true
	}
	return dir == md || strings.HasPrefix(dir, md+"/")
}

// importPathOf resolves a package directory to its import path using the nearest module.
func importPathOf(dir string, modByDir map[string]string) string {
	best, bestLen := "\x00", -1
	for md := range modByDir {
		if withinModule(dir, md) && len(md) > bestLen {
			best, bestLen = md, len(md)
		}
	}
	if best == "\x00" {
		return dir
	}
	mp := modByDir[best]
	if dir == best {
		return mp
	}
	if best == "" {
		return mp + "/" + dir
	}
	return mp + "/" + strings.TrimPrefix(dir, best+"/")
}

// moduleOf returns the import path of the module containing dir.
func moduleOf(dir string, modByDir map[string]string) string {
	best, bestLen := "\x00", -1
	for md := range modByDir {
		if withinModule(dir, md) && len(md) > bestLen {
			best, bestLen = md, len(md)
		}
	}
	if best == "\x00" {
		return ""
	}
	return modByDir[best]
}

// modulePath reads the module path from a go.mod.
func modulePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("repoindex: read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("repoindex: %s: no module directive", path)
}

// hashFile returns a truncated content hash, so index identity tracks content, not mtime.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("repoindex: open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("repoindex: hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// countIndex summarizes the index.
func countIndex(idx Index) Counts {
	c := Counts{
		Modules: len(idx.Modules), Packages: len(idx.Packages), Files: len(idx.Files),
		Imports: len(idx.Imports), Documents: len(idx.Documents),
	}
	for _, s := range idx.Symbols {
		switch s.Kind {
		case SymType:
			c.Types++
		case SymStruct:
			c.Structs++
		case SymInterface:
			c.Interfaces++
		case SymFunction:
			c.Functions++
		case SymMethod:
			c.Methods++
		}
		if s.Test {
			c.Tests++
		}
	}
	for _, d := range idx.Documents {
		switch d.Authority {
		case AuthorityCurrent:
			c.Current++
		case AuthorityHistorical:
			c.Historical++
		default:
			c.Unknown++
		}
	}
	return c
}

// sopMeta is the explicit SOP lifecycle metadata the index consumes, so SOP authority
// overrides filesystem placement.
type sopMeta struct {
	root         string
	activeSource string
	archived     map[string]string
}

// readSOPMetadata reads the recorded active plan and the archived plans (with their
// dispositions) from the SOP state directory. It is read-only and model-free.
func readSOPMetadata(root, stateDir string) sopMeta {
	m := sopMeta{root: root, archived: map[string]string{}}
	dir := filepath.Join(root, stateDir)
	if src := readMetaSource(filepath.Join(dir, "plan.meta.json")); src != "" {
		m.activeSource = src
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "archive"))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		adir := filepath.Join(dir, "archive", e.Name())
		src := readMetaSource(filepath.Join(adir, "plan.meta.json"))
		if src == "" {
			continue
		}
		disp := "complete"
		if data, err := os.ReadFile(filepath.Join(adir, "lifecycle.json")); err == nil {
			var life struct {
				Disposition string `json:"disposition"`
			}
			if json.Unmarshal(data, &life) == nil && strings.TrimSpace(life.Disposition) != "" {
				disp = strings.ToLower(strings.TrimSpace(life.Disposition))
			}
		}
		m.archived[src] = disp
	}
	return m
}

// readMetaSource reads a plan.meta.json's recorded source as a slash path.
func readMetaSource(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var meta struct {
		Source string `json:"source"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(meta.Source)))
}
