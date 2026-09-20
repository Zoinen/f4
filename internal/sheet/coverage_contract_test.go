package sheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

type expressionCoverageEnv struct {
	value  float64
	err    error
	ranges []float64
}

func (e expressionCoverageEnv) CellValue(Ref) (float64, error) { return e.value, e.err }

func (e expressionCoverageEnv) RangeValues(Ref, Ref) ([]float64, error) {
	return append([]float64(nil), e.ranges...), e.err
}

func TestExpressionOperatorAndFunctionContracts(t *testing.T) {
	cases := []struct {
		expression string
		want       float64
	}{
		{"+2", 2}, {"2-3", -1}, {"2*3", 6}, {"8/2", 4},
		{"7\\2", 3}, {"7%3", 1}, {"7<>2", TrueValue}, {"7#7", 0},
		{"7!=7", 0}, {"2<=2", TrueValue}, {"3>=4", 0},
		{"12|5", 13}, {"12&5", 4}, {"32>>2", 8}, {"3<<2", 12},
		{"pi", math.Pi}, {"sin 0", 0}, {"cos 0", 1}, {"tan 0", 0},
		{"cotan 1", 1 / math.Tan(1)}, {"sec 0", 1}, {"cosec 1", 1 / math.Sin(1)},
		{"asin 0", 0}, {"acos 1", 0}, {"atan 0", 0}, {"actg 0", math.Pi / 2},
		{"arcsec 1", 0}, {"arccosec 1", math.Pi / 2},
		{"rad 180", math.Pi}, {"radg 200", math.Pi}, {"deg 3.141592653589793", 180}, {"grad 3.141592653589793", 200},
		{"sh 0", 0}, {"ch 0", 1}, {"th 0", 0}, {"cth 1", 1 / math.Tanh(1)},
		{"arch 1", 0}, {"ash 0", 0}, {"ath 0", 0}, {"exp 0", 1},
		{"fact 5", 120}, {"lg 100", 2}, {"ln 1", 0}, {"sqr 3", 9},
		{"sqrt 9", 3}, {"round 2.5", 3}, {"sign 4", 1}, {"sign 0", 0}, {"sign -4", -1}, {"abs -4", 4},
		{"log 10 100", 2}, {"root 2 16", 4},
	}
	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			got := evalString(t, tc.expression)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("%q = %v, want %v", tc.expression, got, tc.want)
			}
		})
	}
}

func TestExpressionErrorsAndEvaluationEdges(t *testing.T) {
	for _, expression := range []string{
		"cotan 0", "cosec 0", "asin 2", "acos 2",
		"arcsec 0", "arccosec 0", "arch 0", "ath 2", "fact -1", "lg 0", "ln 0",
		"sqrt -1", "log 1 10", "log 0 10", "log 10 0", "root 0 4", "root 2 -1",
	} {
		t.Run(expression, func(t *testing.T) {
			tree, err := Parse(expression)
			if err != nil {
				t.Fatalf("Parse(%q): %v", expression, err)
			}
			if _, err := Eval(tree, nil); err == nil {
				t.Fatalf("Eval(%q) succeeded", expression)
			}
		})
	}

	env := expressionCoverageEnv{value: 42}
	tree, err := Parse("A1")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Eval(tree, env); err != nil || got != 42 {
		t.Fatalf("reference evaluation = %v, %v", got, err)
	}
	env.err = errors.New("missing")
	if _, err := Eval(tree, env); err == nil || !strings.Contains(err.Error(), "A1") {
		t.Fatalf("reference error = %v", err)
	}

	if _, err := Eval(&rangeNode{pos: 3}, nil); err == nil {
		t.Fatal("a bare range must be rejected")
	}
	if _, err := Eval(nil, nil); err == nil {
		t.Fatal("an empty expression must be rejected")
	}
	if _, err := evalUnary(&unaryNode{op: "?", pos: 4, arg: &numberNode{value: 1}}, nil); err == nil {
		t.Fatal("an unknown unary operator must be rejected")
	}
	if _, err := evalBinary(&binaryNode{op: "?", pos: 5, left: &numberNode{value: 1}, right: &numberNode{value: 2}}, nil); err == nil {
		t.Fatal("an unknown binary operator must be rejected")
	}

	aggregate := &callNode{name: "sum", args: []node{&rangeNode{from: Ref{Col: 0, Row: 0}, to: Ref{Col: 1, Row: 1}, pos: 2}}}
	env = expressionCoverageEnv{ranges: []float64{2, 3}}
	if got, err := Eval(aggregate, env); err != nil || got != 5 {
		t.Fatalf("range aggregate = %v, %v", got, err)
	}
	env.err = errors.New("range failed")
	if _, err := Eval(aggregate, env); err == nil {
		t.Fatal("range error was lost")
	}
	if _, err := Eval(aggregate, nil); err == nil {
		t.Fatal("range without an environment must fail")
	}
	if got, err := Eval(&callNode{name: "mul", args: []node{&numberNode{value: 2}, &numberNode{value: 3}}}, nil); err != nil || got != 6 {
		t.Fatalf("scalar product = %v, %v", got, err)
	}

	for _, source := range []string{"1e-3", "1E+3", "0.5da", "2O", "2Q", "101B", ".5", "$A.F", "0Xf"} {
		if _, err := Parse(source); err != nil {
			t.Errorf("Parse(%q): %v", source, err)
		}
	}
	for _, source := range []string{".", "$", "0x", "1?"} {
		if _, err := Parse(source); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", source)
		}
	}
}

func TestExpressionScannerAndNumericHelpers(t *testing.T) {
	for _, word := range []string{"div", "MOD", "shr", "ShL", "and", "OR"} {
		if !isWordOperator(word) {
			t.Errorf("isWordOperator(%q) = false", word)
		}
	}
	if isWordOperator("xor") {
		t.Fatal("xor is not part of the expression language")
	}

	for _, op := range []string{"+", "-", "*", "/", "%", "\\", "^", ":", "(", ")", ",", "~", "#", "<", ">", "|", "&", "!="} {
		if got, width := scanOperator([]rune(op), 0); got != op || width == 0 {
			t.Errorf("scanOperator(%q) = %q, %d", op, got, width)
		}
	}
	for _, op := range []string{"<=", ">=", "<>", ">>", "<<"} {
		if got, width := scanOperator([]rune(op), 0); got != op || width != 2 {
			t.Errorf("scanOperator(%q) = %q, %d", op, got, width)
		}
	}
	if got, width := scanOperator([]rune("?"), 0); got != "" || width != 0 {
		t.Fatalf("unknown operator = %q, %d", got, width)
	}

	for _, tc := range []struct {
		text string
		base int
		want float64
	}{
		{"10.1", 16, 16.0625}, {"10.1", 8, 8.125}, {"", 10, 0}, {".1", 2, 0.5},
	} {
		got, err := parseRadix(tc.text, tc.base)
		if tc.text == "" {
			if err == nil {
				t.Errorf("parseRadix(%q) unexpectedly succeeded", tc.text)
			}
			continue
		}
		if err != nil || math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("parseRadix(%q, %d) = %v, %v", tc.text, tc.base, got, err)
		}
	}
	for _, tc := range []struct {
		text string
		base int
	}{{"zz", 16}, {"2", 2}, {"1.2", 2}} {
		if _, err := parseRadix(tc.text, tc.base); err == nil {
			t.Errorf("parseRadix(%q, %d) unexpectedly succeeded", tc.text, tc.base)
		}
	}

	for _, tc := range []struct {
		value float64
		want  uint32
	}{
		{math.NaN(), 0}, {math.Inf(1), 0}, {-1, math.MaxUint32}, {math.Pow(2, 32) + 2, 2}, {3.6, 4},
	} {
		if got := toUint32(tc.value); got != tc.want {
			t.Errorf("toUint32(%v) = %d, want %d", tc.value, got, tc.want)
		}
	}
	if precedence("?") != 0 || precedence("=") != 1 || precedence("+") != 2 || precedence("*") != 3 || precedence("^") != 4 || precedence(":") != 5 {
		t.Fatal("precedence table is incomplete")
	}
}

func TestXLSXHelpersAndFormulaContracts(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "Sheet1"}, {"  ", "Sheet1"}, {"a/b:c*d?e[f]", "a_b_c_d_e_f_"},
		{strings.Repeat("x", 40), strings.Repeat("x", 31)},
	} {
		if got := sanitizeSheetName(tc.input); got != tc.want {
			t.Errorf("sanitizeSheetName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	for _, tc := range []struct {
		value float64
		want  string
	}{{math.NaN(), "0"}, {math.Inf(1), "0"}, {12.5, "12.5"}} {
		if got := formatXLSXNumber(tc.value); got != tc.want {
			t.Errorf("formatXLSXNumber(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
	if got := escapeXML(`<a x="1">&`); got != "&lt;a x=&#34;1&#34;&gt;&amp;" {
		t.Errorf("escapeXML = %q", got)
	}

	for _, tc := range []struct{ expression, want string }{
		{"sin(A1)", "SIN(A1)"}, {"tg(A1)", "TAN(A1)"}, {"lg(A1)", "LOG10(A1)"},
		{"round(A1)", "ROUND(A1,0)"}, {"sqr(A1)", "POWER(A1,2)"}, {"pi", "PI()"},
		{"log(2,A1)", "LOG(A1,2)"}, {"root(2,A1)", "POWER(A1,1/(2))"},
		{"sum(A1:B2)", "SUM(A1:B2)"}, {"A1#B1", "(A1<>B1)"},
	} {
		if got, ok := formulaToExcel(tc.expression); !ok || got != tc.want {
			t.Errorf("formulaToExcel(%q) = %q, %v; want %q", tc.expression, got, ok, tc.want)
		}
	}
	for _, expression := range []string{"~A1", "A1 shl 2", "1:2", "not a formula"} {
		if _, ok := formulaToExcel(expression); ok {
			t.Errorf("formulaToExcel(%q) unexpectedly succeeded", expression)
		}
	}
	for _, tc := range []struct{ formula, want string }{
		{"=SUM($A$1;B2)", "sum(@A@1,B2)"},
		{"POWER(2,3)", "(2)^(3)"}, {"LOG10(100)", "lg(100)"}, {"PI()", "pi"},
	} {
		if got, ok := formulaFromExcel(tc.formula); !ok || got != tc.want {
			t.Errorf("formulaFromExcel(%q) = %q, %v; want %q", tc.formula, got, ok, tc.want)
		}
	}
	for _, formula := range []string{"", "=", "VLOOKUP(A1,B:C,2,FALSE)", "POWER(1)"} {
		if _, ok := formulaFromExcel(formula); ok {
			t.Errorf("formulaFromExcel(%q) unexpectedly succeeded", formula)
		}
	}
	if got := replaceFold("Power(2,3) POWER(4,5)", "POWER(", "power("); got != "power(2,3) power(4,5)" {
		t.Errorf("replaceFold = %q", got)
	}
	for _, tc := range []struct{ expression, want string }{
		{"power(2,3)", "(2)^(3)"}, {"xpower(2,3)", "x(2)^(3)"}, {"power(1,power(2,3))", "(1)^((2)^(3))"},
		{"power(2)", "power(2)"},
	} {
		if got := rewritePower(tc.expression); got != tc.want {
			t.Errorf("rewritePower(%q) = %q, want %q", tc.expression, got, tc.want)
		}
	}
}

func TestXLSXImportedCellVariantsAndColumns(t *testing.T) {
	s := New()
	storeImportedCell(s, "A1", "inlineStr", "", "", "inline", nil)
	storeImportedCell(s, "A2", "str", "string", "", "", nil)
	storeImportedCell(s, "A3", "e", "#VALUE!", "", "", nil)
	storeImportedCell(s, "A4", "b", "1", "", "", nil)
	storeImportedCell(s, "A5", "b", "0", "", "", nil)
	storeImportedCell(s, "A6", "", " 12.5 ", "", "", nil)
	storeImportedCell(s, "A7", "s", "0", "", "", []string{"shared"})
	storeImportedCell(s, "A8", "", "99", "SUM(A6;A7)", "", nil)
	storeImportedCell(s, "A9", "", "48", "unsupported(A1)", "", nil)
	for row, want := range map[int]string{0: "inline", 1: "string", 2: "#VALUE!", 3: "1", 4: "0", 5: "12.5", 6: "shared", 7: "=sum(A6,A7)", 8: "48"} {
		if got := s.Cell(0, row).Text; got != want {
			t.Errorf("A%d = %q, want %q", row+1, got, want)
		}
	}
	for _, args := range [][6]string{{"", "", "", "", "", ""}, {"A10", "s", "9", "", "", ""}, {"bad", "", "1", "", "", ""}} {
		storeImportedCell(s, args[0], args[1], args[2], args[3], args[4], nil)
	}
	if s.Cell(0, 9) != nil {
		t.Fatal("an invalid shared-string cell was imported")
	}

	for _, tc := range []struct {
		attrs []xml.Attr
		want  map[int]int
	}{
		{[]xml.Attr{{Name: xml.Name{Local: "min"}, Value: "2"}, {Name: xml.Name{Local: "max"}, Value: "4"}, {Name: xml.Name{Local: "width"}, Value: "3.5"}}, map[int]int{1: 4, 2: 4, 3: 4}},
		{[]xml.Attr{{Name: xml.Name{Local: "min"}, Value: "5"}, {Name: xml.Name{Local: "max"}, Value: "4"}, {Name: xml.Name{Local: "width"}, Value: "999"}}, map[int]int{4: MaxColumnWidth}},
	} {
		target := New()
		applyColumnWidth(xml.StartElement{Name: xml.Name{Local: "col"}, Attr: tc.attrs}, target)
		for col, want := range tc.want {
			if got := target.ColumnWidth(col); got != want {
				t.Errorf("column %d width = %d, want %d", col, got, want)
			}
		}
	}
	target := New()
	applyColumnWidth(xml.StartElement{Attr: []xml.Attr{{Name: xml.Name{Local: "min"}, Value: "0"}, {Name: xml.Name{Local: "width"}, Value: "bad"}}}, target)
	if len(target.ColumnWidths()) != 0 {
		t.Fatal("invalid column width was accepted")
	}
}

func makeCoverageZip(t *testing.T, entries map[string]string) (*zip.Reader, []byte) {
	t.Helper()
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	for name, content := range entries {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return reader, data.Bytes()
}

func zipFileMap(reader *zip.Reader) map[string]*zip.File {
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[file.Name] = file
	}
	return files
}

func TestXLSXImportVariantsAndFallbacks(t *testing.T) {
	if _, err := ReadXLSX(bytes.NewReader([]byte("not a zip")), 9); err == nil {
		t.Fatal("invalid XLSX was accepted")
	}
	if _, _, err := firstWorksheetPath(map[string]*zip.File{}); err == nil {
		t.Fatal("an empty archive was accepted as a workbook")
	}
	reader, _ := makeCoverageZip(t, map[string]string{"xl/worksheets/sheet1.xml": `<worksheet/>`})
	if path, name, err := firstWorksheetPath(zipFileMap(reader)); err != nil || path != "xl/worksheets/sheet1.xml" || name != "Sheet1" {
		t.Fatalf("sheet fallback = %q, %q, %v", path, name, err)
	}

	entries := map[string]string{
		"xl/workbook.xml":            `<workbook><sheets><sheet name="Data" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/custom.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><t>shared</t></si></sst>`,
		"xl/worksheets/custom.xml": `<worksheet><cols><col min="1" max="2" width="20"/></cols><sheetData>` +
			`<row><c r="A1" t="s"><v>0</v></c><c r="A2" t="inlineStr"><is><t>inline</t></is></c>` +
			`<c r="A3" t="b"><v>1</v></c><c r="A4"><v>12.5</v></c>` +
			`<c r="B1"><f>SUM(A3;A4)</f><v>0</v></c></row></sheetData></worksheet>`,
	}
	_, data := makeCoverageZip(t, entries)
	got, err := ReadXLSX(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	if got.Title != "Data" || got.ColumnWidth(0) != 20 || got.Cell(0, 0).Text != "shared" || got.Cell(0, 1).Text != "inline" || got.Cell(0, 2).Value != 1 || got.Cell(0, 3).Value != 12.5 {
		t.Fatalf("imported workbook = title %q width %d cells %#v %#v %#v %#v", got.Title, got.ColumnWidth(0), got.Cell(0, 0), got.Cell(0, 1), got.Cell(0, 2), got.Cell(0, 3))
	}
	if got.Cell(1, 0).Text != "=sum(A3,A4)" {
		t.Fatalf("imported formula = %q", got.Cell(1, 0).Text)
	}

	reader, data = makeCoverageZip(t, map[string]string{
		"xl/workbook.xml":          `<workbook><sheets><sheet name="Fallback"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet/>`,
	})
	if got, err := ReadXLSX(bytes.NewReader(data), int64(len(data))); err != nil || got.Title != "Fallback" {
		t.Fatalf("no-rId workbook fallback = %#v, %v", got, err)
	}
	if path, name, err := firstWorksheetPath(zipFileMap(reader)); err != nil || path != "xl/worksheets/sheet1.xml" || name != "Fallback" {
		t.Fatalf("no-rId fallback = %q, %q, %v", path, name, err)
	}
}
