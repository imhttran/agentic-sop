package orchestration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenLifecycleImports are mutating lifecycle paths the orchestration
// package must never import: importing them would create a second authority or
// allow direct lifecycle mutation.
var forbiddenLifecycleImports = []string{
	"internal/scheduler",
	"internal/run",
}

// TestNoSecondLifecycleAuthority is the guard that keeps exactly one SOP
// lifecycle authority (internal/domain) in place. It fails if orchestration
// declares its own status/transition vocabulary or imports a path that mutates
// lifecycle state.
func TestNoSecondLifecycleAuthority(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		// No orchestration source may import a mutating lifecycle path.
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			for _, bad := range forbiddenLifecycleImports {
				if strings.Contains(path, bad) {
					t.Fatalf("%s imports mutating lifecycle path %q", name, path)
				}
			}
		}
		// No orchestration source may declare a status/transition vocabulary name.
		ast.Inspect(f, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.FuncDecl:
				for _, forbidden := range []string{"Transition", "CanTransitionTo"} {
					if strings.Contains(decl.Name.Name, forbidden) {
						t.Fatalf("%s declares lifecycle method %q; transitions belong to internal/domain", name, decl.Name.Name)
					}
				}
			case *ast.TypeSpec:
				if decl.Name.Name == "TaskStatus" || decl.Name.Name == "BlockedReason" || decl.Name.Name == "transitions" {
					t.Fatalf("%s declares lifecycle type %q; vocabulary belongs to internal/domain", name, decl.Name.Name)
				}
			}
			return true
		})
	}
}
