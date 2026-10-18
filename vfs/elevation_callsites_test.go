package vfs

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every place in this package that may ask for the sudo password checks the
// operation first (mayElevate or ElevationAllowed), so that one refusal ends
// elevation for a whole delete, move, copy or scan (f4#1998). A new elevated
// call that skips the check asks again after the user said no; this test
// names it.
func TestEveryElevatedCallChecksTheOperation(t *testing.T) {
	// Functions whose elevated calls are checked by their callers, and the
	// one call that has no operation to belong to: entering a folder.
	exempt := map[string]bool{
		"sudoOpenCancellable":      true,
		"sudoReadDirCancellable":   true,
		"resolvePathWithElevation": true,
	}
	// Calls that never ask for a password.
	harmless := map[string]bool{"IsAvailable": true, "WithdrawPrompt": true, "RunOnUI": true}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "sudo_") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || exempt[fn.Name.Name] {
				continue
			}
			var stack []ast.Node
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if n == nil {
					stack = stack[:len(stack)-1]
					return false
				}
				stack = append(stack, n)
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "globalSudoClient" || harmless[sel.Sel.Name] {
					return true
				}
				checked++
				if !underOperationCheck(stack) {
					t.Errorf("%s: %s calls globalSudoClient.%s outside an `if mayElevate(ctx, err)` check",
						fset.Position(call.Pos()), fn.Name.Name, sel.Sel.Name)
				}
				return true
			})
		}
	}
	if checked < 10 {
		t.Fatalf("found only %d elevated calls; the test no longer sees them", checked)
	}
}

// underOperationCheck reports whether one of the enclosing if statements
// tests mayElevate or ElevationAllowed.
func underOperationCheck(stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		ifs, ok := stack[i].(*ast.IfStmt)
		if !ok {
			continue
		}
		found := false
		ast.Inspect(ifs.Cond, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && (id.Name == "mayElevate" || id.Name == "ElevationAllowed") {
					found = true
				}
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// After a refusal in the operation, a refused call is not retried through
// sudo, whatever it was.
func TestMayElevateRefusesAfterTheOperationRefused(t *testing.T) {
	op := WithElevationScope(context.Background())
	elevationScopeOf(op).declined.Store(true)
	if mayElevate(op, os.ErrPermission) {
		t.Error("an operation whose password was refused tried sudo again")
	}
	if mayElevate(context.Background(), os.ErrNotExist) {
		t.Error("only a refusal of rights is retried through sudo")
	}
}
