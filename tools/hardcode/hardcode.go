// Package hardcode finds UI captions that are baked into Go sources as string
// literals instead of coming from the localization table.
//
// It backs both the tools/find_hardcoded.go command line utility and the
// TestNoNewHardcodedUIStrings guard in the root package, so that CI fails as
// soon as a new hardcoded caption appears. See L10N_PLAN.md, stage S0.
package hardcode

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// uiConstructors maps a vtui constructor to the index of the argument that
// carries a user visible caption.
var uiConstructors = map[string]int{
	"NewLabel":          2,
	"NewButton":         2,
	"NewCheckbox":       2,
	"NewCenteredDialog": 2,
	"NewVMenu":          0,
	"NewText":           0,
}

// comboBoxOptionsArg is the index of the options slice of NewComboBox.
const comboBoxOptionsArg = 3

// skipDirs are never scanned: they contain no shipped UI code.
var skipDirs = map[string]bool{
	".git":         true,
	".github":      true,
	"_git_history": true,
	"build":        true,
	"node_modules": true,
	"testdata":     true,
	"tools":        true,
	"vendor":       true,
}

const baselineHeader = "" +
	"# Hardcoded UI captions that predate the localization audit.\n" +
	"# One entry per line: file, constructor and the quoted literal, tab separated.\n" +
	"# The list may only shrink. See L10N_PLAN.md, stages S0 and S6.\n"

// Finding is a single hardcoded caption.
type Finding struct {
	File    string
	Line    int
	Func    string
	Literal string
}

// ID is the line-number independent identity of a finding, used in baselines.
func (f Finding) ID() string {
	return fmt.Sprintf("%s\t%s\t%s", f.File, f.Func, strconv.Quote(f.Literal))
}

// String renders a finding for human consumption.
func (f Finding) String() string {
	return fmt.Sprintf("[%s:%d] %s() has a hardcoded caption: %s", f.File, f.Line, f.Func, strconv.Quote(f.Literal))
}

// Scan walks root and reports every hardcoded caption it can see. Paths in the
// result are slash separated and relative to root, so that baselines generated
// on Windows and on Unix are identical.
func Scan(root string) ([]Finding, error) {
	fset := token.NewFileSet()
	var out []Finding

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			// A file we cannot parse is not our problem: the compiler will
			// complain about it far more clearly than we could.
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		out = append(out, inspectFile(filepath.ToSlash(rel), file, fset)...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

func inspectFile(rel string, file *ast.File, fset *token.FileSet) []Finding {
	var out []Finding

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calleeName(call)

		if idx, ok := uiConstructors[name]; ok && len(call.Args) > idx {
			if f, ok := literalFinding(rel, name, call.Args[idx], fset); ok {
				out = append(out, f)
			}
		}

		if name == "NewComboBox" && len(call.Args) > comboBoxOptionsArg {
			if comp, ok := call.Args[comboBoxOptionsArg].(*ast.CompositeLit); ok {
				for _, elt := range comp.Elts {
					if f, ok := literalFinding(rel, name, elt, fset); ok {
						out = append(out, f)
					}
				}
			}
		}

		return true
	})

	return out
}

func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func literalFinding(rel, name string, expr ast.Expr, fset *token.FileSet) (Finding, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return Finding{}, false
	}
	val, err := strconv.Unquote(lit.Value)
	if err != nil {
		return Finding{}, false
	}
	if !translatable(val) {
		return Finding{}, false
	}
	return Finding{
		File:    rel,
		Line:    fset.Position(lit.ValuePos).Line,
		Func:    name,
		Literal: val,
	}, true
}

// translatable reports whether a literal carries text a translator could work
// on. Frame drawings and strings without a single letter do not.
func translatable(s string) bool {
	if strings.Contains(s, "──") {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// IDs returns the sorted and de-duplicated identities of the findings.
func IDs(findings []Finding) []string {
	seen := make(map[string]bool, len(findings))
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		id := f.ID()
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// LoadBaseline reads a baseline file into a set of finding identities.
func LoadBaseline(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, nil
}

// WriteBaseline stores the findings as a baseline file.
func WriteBaseline(path string, findings []Finding) error {
	var b strings.Builder
	b.WriteString(baselineHeader)
	for _, id := range IDs(findings) {
		b.WriteString(id)
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
