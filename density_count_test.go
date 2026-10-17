package ionomaps_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rootDensity counts the root package's checks and functions as P10-05
// counts them (densityOf), over every file outside tests.
func rootDensity(t *testing.T) (checks, funcs int) {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		c, n := densityOf(t, string(src))
		checks, funcs = checks+c, funcs+n
	}
	return checks, funcs
}

// densityOf is a file's checks and functions as P10-05 counts them: every
// function and method, and each guard - an if with no init and no else whose
// condition makes no call but len or cap and whose body is one return.
func densityOf(t *testing.T, src string) (checks, funcs int) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		funcs++
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if s, ok := n.(*ast.IfStmt); ok && s.Init == nil && s.Else == nil && callFree(s.Cond) && singleReturn(s.Body) {
				checks++
			}
			return true
		})
	}
	return checks, funcs
}

// callFree is whether an expression makes no call but len or cap.
func callFree(e ast.Expr) bool {
	free := true
	ast.Inspect(e, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return free
		}
		if id, ok := c.Fun.(*ast.Ident); ok && (id.Name == "len" || id.Name == "cap") {
			return true
		}
		free = false
		return false
	})
	return free
}

// singleReturn is whether a block is one return statement.
func singleReturn(b *ast.BlockStmt) bool {
	if len(b.List) != 1 {
		return false
	}
	_, ok := b.List[0].(*ast.ReturnStmt)
	return ok
}
