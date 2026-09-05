package main

import (
	"go/ast"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// frameManagerCaptureAllowlist holds background readers that are allowed to
// read vtui.FrameManager from inside the work they start, with the reason why.
// It is empty, and the point of the test is that it stays that way.
var frameManagerCaptureAllowlist = map[string]string{}

// TestBackgroundWorkCapturesTheFrameManager is a static audit, and it exists
// because one mistake in this shape cost five CI rounds in a row.
//
// vtui.FrameManager is process-wide. Production assigns it once, so reading it
// from a goroutine is harmless there; the tests replace it between cases, so a
// goroutine that outlives its test reads it while the next test writes it. The
// detector then blames the test that did the writing, which is never the test
// that leaked the reader, and every report had to be traced back by hand.
//
// The rule that avoids all of it: read the frame manager where the work is
// started, on the goroutine that owns it, and use that value inside. This
// checks the two ways background work begins -- a go statement with a function
// literal, and time.AfterFunc -- across the whole module.
func TestBackgroundWorkCapturesTheFrameManager(t *testing.T) {
	var found []string

	for _, source := range commandPaletteParseProductionGo(t) {
		aliases, dotImport := commandPaletteVTUIImportAliases(source.file)
		if len(aliases) == 0 && !dotImport {
			continue
		}

		var walk func(node ast.Node, background bool)
		walk = func(node ast.Node, background bool) {
			ast.Inspect(node, func(child ast.Node) bool {
				if child == nil {
					return false
				}
				// Only the outermost block is descended into with background
				// set: an inner goroutine within one that already captured
				// reads that capture, not the global.
				if body := backgroundFuncBody(child); body != nil && !background {
					walk(body, true)
					return false
				}
				if background && isFrameManagerSelector(child, aliases, dotImport) {
					found = append(found, source.path+":"+strconv.Itoa(source.fset.Position(child.Pos()).Line))
				}
				return true
			})
		}
		walk(source.file, false)
	}

	var unexpected []string
	for _, site := range found {
		if _, allowed := frameManagerCaptureAllowlist[site]; !allowed {
			unexpected = append(unexpected, site)
		}
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		t.Errorf("background work reads vtui.FrameManager from inside itself at %s\n"+
			"read it where the work is started and use that value inside, the way newDialogReporter does",
			strings.Join(unexpected, ", "))
	}
	for site := range frameManagerCaptureAllowlist {
		if !slices.Contains(found, site) {
			t.Errorf("stale allowlist entry %q: nothing reads the frame manager there any more", site)
		}
	}
}

// backgroundFuncBody returns the body of a background block: the function
// literal of a go statement, or the callback handed to time.AfterFunc.
func backgroundFuncBody(node ast.Node) ast.Node {
	switch typed := node.(type) {
	case *ast.GoStmt:
		if literal, ok := typed.Call.Fun.(*ast.FuncLit); ok {
			return literal.Body
		}
	case *ast.CallExpr:
		selector, ok := typed.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "AfterFunc" {
			return nil
		}
		if pkg, ok := selector.X.(*ast.Ident); !ok || pkg.Name != "time" {
			return nil
		}
		for _, argument := range typed.Args {
			if literal, ok := argument.(*ast.FuncLit); ok {
				return literal.Body
			}
		}
	}
	return nil
}

// isFrameManagerSelector reports whether node is the vtui.FrameManager
// selector itself, under whichever name the file imports the package by.
func isFrameManagerSelector(node ast.Node, aliases map[string]bool, dotImport bool) bool {
	switch typed := node.(type) {
	case *ast.SelectorExpr:
		if typed.Sel.Name != "FrameManager" {
			return false
		}
		pkg, ok := typed.X.(*ast.Ident)
		return ok && aliases[pkg.Name]
	case *ast.Ident:
		return dotImport && typed.Name == "FrameManager"
	}
	return false
}
