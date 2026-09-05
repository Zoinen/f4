package sheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

const (
	xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

	nsSpreadsheet   = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRelationships = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsPackageRels   = "http://schemas.openxmlformats.org/package/2006/relationships"
)

// SaveXLSX writes the sheet as an Office Open XML workbook.
func (s *Sheet) SaveXLSX(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := s.WriteXLSX(file); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// WriteXLSX streams a workbook containing a single worksheet.
func (s *Sheet) WriteXLSX(w io.Writer) error {
	archive := zip.NewWriter(w)
	name := s.Title
	if strings.TrimSpace(name) == "" {
		name = "Sheet1"
	}
	parts := []struct {
		path    string
		content string
	}{
		{"[Content_Types].xml", contentTypesXML()},
		{"_rels/.rels", rootRelationshipsXML()},
		{"xl/workbook.xml", workbookXML(name)},
		{"xl/_rels/workbook.xml.rels", workbookRelationshipsXML()},
		{"xl/styles.xml", stylesXML()},
		{"xl/worksheets/sheet1.xml", s.worksheetXML()},
	}
	for _, part := range parts {
		writer, err := archive.Create(part.path)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(writer, part.content); err != nil {
			return err
		}
	}
	return archive.Close()
}

func contentTypesXML() string {
	return xmlHeader +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
		`</Types>`
}

func rootRelationshipsXML() string {
	return xmlHeader +
		`<Relationships xmlns="` + nsPackageRels + `">` +
		`<Relationship Id="rId1" Type="` + nsRelationships + `/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`
}

func workbookXML(name string) string {
	return xmlHeader +
		`<workbook xmlns="` + nsSpreadsheet + `" xmlns:r="` + nsRelationships + `">` +
		`<sheets><sheet name="` + escapeXML(sanitizeSheetName(name)) + `" sheetId="1" r:id="rId1"/></sheets>` +
		`</workbook>`
}

func workbookRelationshipsXML() string {
	return xmlHeader +
		`<Relationships xmlns="` + nsPackageRels + `">` +
		`<Relationship Id="rId1" Type="` + nsRelationships + `/worksheet" Target="worksheets/sheet1.xml"/>` +
		`<Relationship Id="rId2" Type="` + nsRelationships + `/styles" Target="styles.xml"/>` +
		`</Relationships>`
}

func stylesXML() string {
	return xmlHeader +
		`<styleSheet xmlns="` + nsSpreadsheet + `">` +
		`<fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>` +
		`<fills count="1"><fill><patternFill patternType="none"/></fill></fills>` +
		`<borders count="1"><border/></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>` +
		`</styleSheet>`
}

// sanitizeSheetName strips the characters Excel refuses in a tab name.
func sanitizeSheetName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', '?', '*', '[', ']', ':':
			return '_'
		}
		return r
	}, name)
	if len([]rune(name)) > 31 {
		name = string([]rune(name)[:31])
	}
	if strings.TrimSpace(name) == "" {
		return "Sheet1"
	}
	return name
}

func (s *Sheet) worksheetXML() string {
	maxCol, maxRow := s.Bounds()
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<worksheet xmlns="` + nsSpreadsheet + `">`)

	widths := s.ColumnWidths()
	if len(widths) > 0 {
		columns := make([]int, 0, len(widths))
		for col := range widths {
			columns = append(columns, col)
		}
		sort.Ints(columns)
		b.WriteString(`<cols>`)
		for _, col := range columns {
			fmt.Fprintf(&b, `<col min="%d" max="%d" width="%d" customWidth="1"/>`,
				col+1, col+1, widths[col])
		}
		b.WriteString(`</cols>`)
	}

	b.WriteString(`<sheetData>`)
	for row := 0; row <= maxRow; row++ {
		var cells strings.Builder
		for col := 0; col <= maxCol; col++ {
			cell := s.Cell(col, row)
			if cell.IsEmpty() {
				continue
			}
			cells.WriteString(cellXML(col, row, cell))
		}
		if cells.Len() == 0 {
			continue
		}
		fmt.Fprintf(&b, `<row r="%d">%s</row>`, row+1, cells.String())
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func cellXML(col, row int, cell *Cell) string {
	reference := CellName(col, row)
	switch cell.Kind {
	case KindText:
		return fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
			reference, escapeXML(cell.Text))
	case KindValue:
		return fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, reference, formatXLSXNumber(cell.Value))
	case KindFormula:
		if cell.Err != "" {
			return fmt.Sprintf(`<c r="%s" t="e"><v>#VALUE!</v></c>`, reference)
		}
		if formula, ok := formulaToExcel(cell.Formula()); ok {
			return fmt.Sprintf(`<c r="%s"><f>%s</f><v>%s</v></c>`,
				reference, escapeXML(formula), formatXLSXNumber(cell.Value))
		}
		// The expression has no Excel counterpart, so only its result travels.
		return fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, reference, formatXLSXNumber(cell.Value))
	}
	return ""
}

func formatXLSXNumber(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0"
	}
	return strconv.FormatFloat(value, 'G', 15, 64)
}

func escapeXML(text string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(text))
	return buffer.String()
}

// LoadXLSX reads the first worksheet of a workbook into a new sheet.
func LoadXLSX(name string) (*Sheet, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return ReadXLSX(file, info.Size())
}

// ReadXLSX parses a workbook from any random access reader.
func ReadXLSX(r io.ReaderAt, size int64) (*Sheet, error) {
	archive, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		files[path.Clean(file.Name)] = file
	}

	sharedStrings, err := readSharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	sheetPath, sheetName, err := firstWorksheetPath(files)
	if err != nil {
		return nil, err
	}
	worksheet, ok := files[sheetPath]
	if !ok {
		return nil, fmt.Errorf("worksheet %s is missing from the workbook", sheetPath)
	}

	result := New()
	result.SuspendUndo()
	result.AutoRecalc = false
	result.Title = sheetName
	if err := readWorksheet(worksheet, sharedStrings, result); err != nil {
		return nil, err
	}
	result.AutoRecalc = true
	result.Recalc()
	result.ResumeUndo()
	result.Modified = false
	return result, nil
}

// firstWorksheetPath resolves the target of the first sheet listed in the
// workbook, falling back to the conventional location.
func firstWorksheetPath(files map[string]*zip.File) (string, string, error) {
	workbook, ok := files["xl/workbook.xml"]
	if !ok {
		if _, ok := files["xl/worksheets/sheet1.xml"]; ok {
			return "xl/worksheets/sheet1.xml", "Sheet1", nil
		}
		return "", "", fmt.Errorf("not a workbook: xl/workbook.xml is missing")
	}

	reader, err := workbook.Open()
	if err != nil {
		return "", "", err
	}
	defer func() { _ = reader.Close() }()

	var sheetName, relationID string
	decoder := xml.NewDecoder(reader)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "sheet" {
			continue
		}
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "name":
				sheetName = attr.Value
			case "id":
				relationID = attr.Value
			}
		}
		break
	}
	if relationID == "" {
		return "xl/worksheets/sheet1.xml", sheetName, nil
	}

	target, err := resolveRelationship(files["xl/_rels/workbook.xml.rels"], relationID)
	if err != nil || target == "" {
		return "xl/worksheets/sheet1.xml", sheetName, nil
	}
	if strings.HasPrefix(target, "/") {
		return path.Clean(strings.TrimPrefix(target, "/")), sheetName, nil
	}
	return path.Clean(path.Join("xl", target)), sheetName, nil
}

func resolveRelationship(file *zip.File, id string) (string, error) {
	if file == nil {
		return "", nil
	}
	reader, err := file.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()

	decoder := xml.NewDecoder(reader)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		var currentID, target string
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "Id":
				currentID = attr.Value
			case "Target":
				target = attr.Value
			}
		}
		if currentID == id {
			return target, nil
		}
	}
}

func readSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	var (
		strs    []string
		current strings.Builder
		inItem  bool
		inText  bool
	)
	decoder := xml.NewDecoder(reader)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "si":
				inItem, inText = true, false
				current.Reset()
			case "t":
				inText = true
			}
		case xml.CharData:
			if inItem && inText {
				current.Write(element)
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "t":
				inText = false
			case "si":
				strs = append(strs, current.String())
				inItem = false
			}
		}
	}
	return strs, nil
}

func readWorksheet(file *zip.File, sharedStrings []string, target *Sheet) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	decoder := xml.NewDecoder(reader)
	var (
		cellRef   string
		cellType  string
		value     strings.Builder
		formula   strings.Builder
		inline    strings.Builder
		inValue   bool
		inFormula bool
		inInline  bool
		inText    bool
	)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "col":
				applyColumnWidth(element, target)
			case "c":
				cellRef, cellType = "", ""
				value.Reset()
				formula.Reset()
				inline.Reset()
				for _, attr := range element.Attr {
					switch attr.Name.Local {
					case "r":
						cellRef = attr.Value
					case "t":
						cellType = attr.Value
					}
				}
			case "v":
				inValue = true
			case "f":
				inFormula = true
			case "is":
				inInline = true
			case "t":
				inText = true
			}
		case xml.CharData:
			switch {
			case inValue:
				value.Write(element)
			case inFormula:
				formula.Write(element)
			case inInline && inText:
				inline.Write(element)
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "v":
				inValue = false
			case "f":
				inFormula = false
			case "is":
				inInline = false
			case "t":
				inText = false
			case "c":
				storeImportedCell(target, cellRef, cellType,
					value.String(), formula.String(), inline.String(), sharedStrings)
			}
		}
	}
	return nil
}

func applyColumnWidth(element xml.StartElement, target *Sheet) {
	var min, max, width int
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "min":
			min, _ = strconv.Atoi(attr.Value)
		case "max":
			max, _ = strconv.Atoi(attr.Value)
		case "width":
			if parsed, err := strconv.ParseFloat(attr.Value, 64); err == nil {
				width = int(math.Round(parsed))
			}
		}
	}
	if min <= 0 || width <= 0 {
		return
	}
	if max < min {
		max = min
	}
	for col := min - 1; col <= max-1 && col < MaxColumns; col++ {
		if width < MinColumnWidth {
			width = MinColumnWidth
		}
		if width > MaxColumnWidth {
			width = MaxColumnWidth
		}
		target.widths[col] = width
	}
}

func storeImportedCell(target *Sheet, reference, cellType, value, formula, inline string, sharedStrings []string) {
	if reference == "" {
		return
	}
	ref, ok := ParseRef(reference)
	if !ok {
		return
	}

	text := ""
	switch cellType {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 0 || index >= len(sharedStrings) {
			return
		}
		text = sharedStrings[index]
	case "inlineStr":
		text = inline
	case "str", "e":
		text = value
	case "b":
		if strings.TrimSpace(value) == "1" {
			text = "1"
		} else {
			text = "0"
		}
	default:
		text = strings.TrimSpace(value)
	}

	if formula != "" {
		if translated, ok := formulaFromExcel(formula); ok {
			text = "=" + translated
		}
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	target.setTextNoUndo(ref.Col, ref.Row, text)
}

// formulaToExcel rewrites an expression of this dialect into Excel syntax. It
// reports false for constructs Excel has no equivalent for (bitwise and
// integer operators, the time operator), in which case only the computed
// value is exported.
func formulaToExcel(expression string) (string, bool) {
	tree, err := Parse(expression)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	if !writeExcel(&b, tree) {
		return "", false
	}
	return b.String(), true
}

func writeExcel(b *strings.Builder, tree node) bool {
	switch n := tree.(type) {
	case *numberNode:
		b.WriteString(strconv.FormatFloat(n.value, 'G', 15, 64))
		return true
	case *refNode:
		b.WriteString(excelRef(n.ref))
		return true
	case *rangeNode:
		b.WriteString(excelRef(n.from))
		b.WriteByte(':')
		b.WriteString(excelRef(n.to))
		return true
	case *unaryNode:
		if n.op == "~" {
			return false
		}
		b.WriteString(n.op)
		b.WriteByte('(')
		if !writeExcel(b, n.arg) {
			return false
		}
		b.WriteByte(')')
		return true
	case *binaryNode:
		operator, ok := excelOperator(n.op)
		if !ok {
			return false
		}
		b.WriteByte('(')
		if !writeExcel(b, n.left) {
			return false
		}
		b.WriteString(operator)
		if !writeExcel(b, n.right) {
			return false
		}
		b.WriteByte(')')
		return true
	case *callNode:
		return writeExcelCall(b, n)
	}
	return false
}

func excelRef(ref Ref) string {
	var b strings.Builder
	if ref.AbsCol {
		b.WriteByte('$')
	}
	b.WriteString(ColumnName(ref.Col))
	if ref.AbsRow {
		b.WriteByte('$')
	}
	b.WriteString(RowName(ref.Row))
	return b.String()
}

func excelOperator(op string) (string, bool) {
	switch op {
	case "+", "-", "*", "/", "^", "<", ">", "<=", ">=", "=":
		return op, true
	case "#", "!=", "<>":
		return "<>", true
	}
	return "", false
}

func writeExcelCall(b *strings.Builder, call *callNode) bool {
	writeArgs := func(name string) bool {
		b.WriteString(name)
		b.WriteByte('(')
		for i, arg := range call.args {
			if i > 0 {
				b.WriteByte(',')
			}
			if !writeExcel(b, arg) {
				return false
			}
		}
		b.WriteByte(')')
		return true
	}

	switch call.name {
	case "sum":
		return writeArgs("SUM")
	case "mul":
		return writeArgs("PRODUCT")
	case "if":
		b.WriteString("IF((")
		if !writeExcel(b, call.args[0]) {
			return false
		}
		b.WriteString(")<>0,")
		if !writeExcel(b, call.args[1]) {
			return false
		}
		b.WriteByte(',')
		if !writeExcel(b, call.args[2]) {
			return false
		}
		b.WriteByte(')')
		return true
	case "sin", "cos", "tan", "exp", "ln", "sqrt", "sign", "abs", "fact":
		return writeArgs(strings.ToUpper(call.name))
	case "tg":
		return writeArgs("TAN")
	case "lg":
		return writeArgs("LOG10")
	case "round":
		b.WriteString("ROUND(")
		if !writeExcel(b, call.args[0]) {
			return false
		}
		b.WriteString(",0)")
		return true
	case "sqr":
		b.WriteString("POWER(")
		if !writeExcel(b, call.args[0]) {
			return false
		}
		b.WriteString(",2)")
		return true
	case "pi":
		b.WriteString("PI()")
		return true
	case "log":
		// log(base, x) in this dialect, LOG(x, base) in Excel.
		b.WriteString("LOG(")
		if !writeExcel(b, call.args[1]) {
			return false
		}
		b.WriteByte(',')
		if !writeExcel(b, call.args[0]) {
			return false
		}
		b.WriteByte(')')
		return true
	case "root":
		b.WriteString("POWER(")
		if !writeExcel(b, call.args[1]) {
			return false
		}
		b.WriteString(",1/(")
		if !writeExcel(b, call.args[0]) {
			return false
		}
		b.WriteString("))")
		return true
	}
	return false
}

// formulaFromExcel converts an Excel formula into this dialect. The result is
// validated by parsing it, so anything unsupported is rejected and the
// importer falls back to the cached value.
func formulaFromExcel(formula string) (string, bool) {
	converted := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(formula), "="))
	if converted == "" {
		return "", false
	}
	converted = strings.ReplaceAll(converted, "$", "@")
	converted = strings.ReplaceAll(converted, ";", ",")
	// Only the spellings this dialect does not share with Excel need to be
	// rewritten; every other function name is matched case-insensitively by
	// the parser itself.
	replacements := []struct{ from, to string }{
		{"PRODUCT(", "mul("},
		{"LOG10(", "lg("},
		{"POWER(", "power("},
		{"PI()", "pi"},
		{"SUM(", "sum("},
		{"IF(", "if("},
	}
	for _, replacement := range replacements {
		converted = replaceFold(converted, replacement.from, replacement.to)
	}
	if strings.Contains(converted, "power(") {
		// POWER(x, y) has no direct spelling here; rewrite it as x^(y).
		converted = rewritePower(converted)
	}
	if _, err := Parse(converted); err != nil {
		return "", false
	}
	return converted, true
}

// replaceFold replaces every case-insensitive occurrence of from with to in a
// single forward pass, so a replacement that folds back to its own pattern
// cannot make the scan restart forever.
func replaceFold(text, from, to string) string {
	if from == "" {
		return text
	}
	upper := strings.ToUpper(text)
	target := strings.ToUpper(from)
	var out strings.Builder
	for pos := 0; ; {
		index := strings.Index(upper[pos:], target)
		if index < 0 {
			out.WriteString(text[pos:])
			return out.String()
		}
		index += pos
		out.WriteString(text[pos:index])
		out.WriteString(to)
		pos = index + len(from)
	}
}

// rewritePower turns power(a,b) into (a)^(b) for the simple, non nested case.
func rewritePower(expression string) string {
	for {
		index := strings.Index(expression, "power(")
		if index < 0 {
			return expression
		}
		depth := 0
		end := -1
		comma := -1
		for i := index + len("power(") - 1; i < len(expression); i++ {
			switch expression[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = i
				}
			case ',':
				if depth == 1 && comma < 0 {
					comma = i
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 || comma < 0 {
			return expression
		}
		base := expression[index+len("power(") : comma]
		exponent := expression[comma+1 : end]
		expression = expression[:index] + "(" + base + ")^(" + exponent + ")" + expression[end+1:]
	}
}
