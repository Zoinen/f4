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
	"regexp"
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
	"NewRadioButton":    2,
	"NewText":           2,
	"NewVText":          2,
	"NewCenteredDialog": 2,
	"NewDialog":         4,
	"NewWindow":         4,
	"NewBaseWindow":     4,
	"NewGroupBox":       4,
	"NewBorderedFrame":  5,
	"NewTableDialog":    2,
	"NewVMenu":          0,
}

// uiListConstructors maps a vtui constructor to the index of its []string
// argument, every element of which is a user visible caption.
var uiListConstructors = map[string]int{
	"NewComboBox":   3,
	"NewListBox":    4,
	"NewRadioGroup": 3,
	"NewCheckGroup": 3,
	"NewMenuBar":    0,
}

// uiSetters maps a widget method that replaces a caption to the index of the
// argument carrying it. Only methods whose name cannot plausibly belong to a
// non-UI type are listed: SetText, for instance, also sets the *content* of an
// editor, which is data and not a caption.
var uiSetters = map[string]int{
	"SetTitle": 0,
}

// captionWrappers are helpers that only pad or decorate a caption they are
// given, so a literal passed to one is as visible as a literal passed straight
// to the widget. Only the first argument carries the text.
var captionWrappers = map[string]bool{
	"padLabel":   true,
	"padLabelTo": true,
}

// uiStructFields maps a vtui struct to the fields of its composite literals
// that hold a user visible caption.
var uiStructFields = map[string]map[string]bool{
	"MenuItem":    {"Text": true},
	"TableColumn": {"Title": true},
}

// skipDirs are never scanned: they contain no shipped UI code.
var skipDirs = map[string]bool{
	".git":         true,
	".diagnostics": true,
	".github":      true,
	"_git_history": true,
	"build":        true,
	"node_modules": true,
	"testdata":     true,
	"third_party":  true,
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
			out = append(out, captionFindings(rel, name, call.Args[idx], fset)...)
		}

		if idx, ok := uiSetters[name]; ok && len(call.Args) > idx {
			out = append(out, captionFindings(rel, name, call.Args[idx], fset)...)
		}

		if idx, ok := uiListConstructors[name]; ok && len(call.Args) > idx {
			if comp, ok := call.Args[idx].(*ast.CompositeLit); ok {
				for _, elt := range comp.Elts {
					out = append(out, captionFindings(rel, name, elt, fset)...)
				}
			}
		}

		return true
	})

	ast.Inspect(file, func(n ast.Node) bool {
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch t := comp.Type.(type) {
		case *ast.ArrayType:
			// []vtui.TableColumn{{Title: "Name"}, ...}: the element type is
			// written once, so the elements themselves are untyped.
			name := typeName(t.Elt)
			for _, elt := range comp.Elts {
				if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
					out = append(out, structFindings(rel, name, inner, fset)...)
				}
			}
		default:
			out = append(out, structFindings(rel, typeName(comp.Type), comp, fset)...)
		}
		return true
	})

	return out
}

// structFindings reports the hardcoded captions of one composite literal of a
// struct type that has caption carrying fields.
func structFindings(rel, name string, comp *ast.CompositeLit, fset *token.FileSet) []Finding {
	fields, ok := uiStructFields[name]
	if !ok {
		return nil
	}
	var out []Finding
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || !fields[key.Name] {
			continue
		}
		out = append(out, captionFindings(rel, name+"."+key.Name, kv.Value, fset)...)
	}
	return out
}

// typeName is the bare name of a composite literal type, with any package
// qualifier removed: both MenuItem{} and vtui.MenuItem{} yield "MenuItem".
func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
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

// captionFindings reports every hardcoded caption reachable from an
// expression that lands in a user visible slot. A plain literal is one
// caption; a concatenation contributes each of its literal parts, and a
// fmt.Sprintf call contributes its format string, which is the part a
// translator has to rewrite.
func captionFindings(rel, name string, expr ast.Expr, fset *token.FileSet) []Finding {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if f, ok := literalFinding(rel, name, e, fset); ok {
			return []Finding{f}
		}
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return nil
		}
		return append(captionFindings(rel, name, e.X, fset), captionFindings(rel, name, e.Y, fset)...)
	case *ast.ParenExpr:
		return captionFindings(rel, name, e.X, fset)
	case *ast.CallExpr:
		if len(e.Args) == 0 {
			return nil
		}
		if isSprintf(e) || captionWrappers[calleeName(e)] {
			return captionFindings(rel, name, e.Args[0], fset)
		}
	}
	return nil
}

// isSprintf reports whether a call builds a string from a format literal.
func isSprintf(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Sprintf" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "fmt"
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

// verb matches a printf placeholder, including an argument index and flags:
// %s, %d, %-8.2f, %[1]v, %%.
var verb = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*[\d.*]*[a-zA-Z%]`)

// translatable reports whether a literal carries text a translator could work
// on. Frame drawings, format strings that are nothing but placeholders and
// strings without a single letter do not.
func translatable(s string) bool {
	if strings.Contains(s, "──") {
		return false
	}
	for _, r := range verb.ReplaceAllString(s, "") {
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
	// #nosec G306 -- baseline files are source-control artifacts intended to be readable by the project team.
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
