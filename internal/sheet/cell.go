// Package sheet implements the spreadsheet model used by f4: a grid of cells
// holding text, numbers or formulas, a formula evaluator, and importers and
// exporters for SQLite (the native format), plain text, CSV and XLSX.
//
// The feature set intentionally follows the spreadsheet built into Dos
// Navigator (cell kinds detected from content, DN calculator expression
// syntax with sum/mul and rectangular ranges, '@' absolute references, the
// same display formats and the same 256x4096 grid). The implementation is
// original Go code written from the documented behaviour; no code was taken
// from DN itself.
package sheet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Grid limits, matching the classic layout: columns A..IV and 4096 rows.
const (
	MaxColumns = 256
	MaxRows    = 4096

	// DefaultColumnWidth is the width a column gets until the user changes it.
	DefaultColumnWidth = 11
	// MinColumnWidth and MaxColumnWidth bound interactive resizing.
	MinColumnWidth = 3
	MaxColumnWidth = 60

	// DefaultDecimals is how many decimal places a new cell shows.
	DefaultDecimals = 2
)

// Formatting characters. They are variables so the UI layer can localise them.
var (
	// ThousandSeparator is inserted between groups of three integer digits.
	ThousandSeparator = ","
	// CurrencySymbol is prepended by the Currency display format.
	CurrencySymbol = "$"
	// ErrorText is displayed in cells whose formula failed to evaluate.
	ErrorText = "Error"
	// BoolFalseText and BoolTrueText render the Logical display format.
	BoolFalseText = "False"
	BoolTrueText  = "True"
)

// Kind describes how the raw text of a cell is interpreted. The kind is
// derived from the content: a leading '=' marks a formula, text that parses
// as a decimal number is a value, everything else is text.
type Kind uint8

const (
	KindText Kind = iota
	KindValue
	KindFormula
)

// Display selects how the numeric value of a cell is rendered.
type Display uint8

const (
	DisplayAsIs Display = iota
	DisplayDecimal
	DisplayComma
	DisplayExponent
	DisplayLogical
	DisplayCurrency
	DisplayPercent
	DisplayHidden
)

// Justify selects the horizontal alignment inside the column.
type Justify uint8

const (
	JustifyLeft Justify = iota
	JustifyRight
	JustifyCenter
)

// Cell is a single grid entry. Text is exactly what the user typed; Value and
// Err are recomputed by Sheet.Recalc for formula cells.
type Cell struct {
	Text      string
	Kind      Kind
	Display   Display
	Justify   Justify
	Decimals  uint8
	Protected bool

	Value float64
	Err   string
}

// Point identifies a cell position. Both coordinates are zero based, so the
// cell displayed as "A1" is Point{0, 0}.
type Point struct {
	Col, Row int
}

// Rect is an inclusive rectangular block of cells.
type Rect struct {
	Left, Top, Right, Bottom int
}

// Normalized returns the rectangle with its corners ordered and clamped to the
// grid, so callers may build it from an arbitrary anchor/cursor pair.
func (r Rect) Normalized() Rect {
	if r.Left > r.Right {
		r.Left, r.Right = r.Right, r.Left
	}
	if r.Top > r.Bottom {
		r.Top, r.Bottom = r.Bottom, r.Top
	}
	if r.Left < 0 {
		r.Left = 0
	}
	if r.Top < 0 {
		r.Top = 0
	}
	if r.Right > MaxColumns-1 {
		r.Right = MaxColumns - 1
	}
	if r.Bottom > MaxRows-1 {
		r.Bottom = MaxRows - 1
	}
	return r
}

// Contains reports whether the point lies inside the rectangle.
func (r Rect) Contains(col, row int) bool {
	r = r.Normalized()
	return col >= r.Left && col <= r.Right && row >= r.Top && row <= r.Bottom
}

// Ref is a reference to a cell as it appears inside a formula. AbsCol and
// AbsRow record the '@' markers that pin the column or the row when the
// formula is copied through the clipboard.
type Ref struct {
	Col, Row       int
	AbsCol, AbsRow bool
}

// ColumnName converts a zero based column index to its display name, using the
// classic one or two letter scheme: A..Z, AA..AZ, BA.. up to IV for column 255.
func ColumnName(col int) string {
	if col < 0 || col >= MaxColumns {
		return ""
	}
	if col < 26 {
		return string(rune('A' + col))
	}
	first := col/26 - 1
	second := col % 26
	return string([]rune{rune('A' + first), rune('A' + second)})
}

// ParseColumnName is the inverse of ColumnName. Case is ignored.
func ParseColumnName(name string) (int, bool) {
	switch len(name) {
	case 1:
		c := upperByte(name[0])
		if c < 'A' || c > 'Z' {
			return 0, false
		}
		return int(c - 'A'), true
	case 2:
		first, second := upperByte(name[0]), upperByte(name[1])
		if first < 'A' || first > 'Z' || second < 'A' || second > 'Z' {
			return 0, false
		}
		col := (int(first-'A')+1)*26 + int(second-'A')
		if col >= MaxColumns {
			return 0, false
		}
		return col, true
	}
	return 0, false
}

// RowName converts a zero based row index into the one based display label.
func RowName(row int) string { return strconv.Itoa(row + 1) }

// CellName returns the display name of a position, for example "B7".
func CellName(col, row int) string { return ColumnName(col) + RowName(row) }

// String renders the reference the way it has to appear in a formula,
// including the '@' markers of the absolute parts.
func (r Ref) String() string {
	var b strings.Builder
	if r.AbsCol {
		b.WriteByte('@')
	}
	b.WriteString(ColumnName(r.Col))
	if r.AbsRow {
		b.WriteByte('@')
	}
	b.WriteString(RowName(r.Row))
	return b.String()
}

// Point returns the grid position the reference points at.
func (r Ref) Point() Point { return Point{Col: r.Col, Row: r.Row} }

// ParseRef parses a single reference such as "B7", "@A@2" or "A@2". It
// returns false when the token is not a reference at all, which is how the
// tokenizer tells cell names apart from function names.
func ParseRef(token string) (Ref, bool) {
	var ref Ref
	i := 0
	if i < len(token) && token[i] == '@' {
		ref.AbsCol = true
		i++
	}
	start := i
	for i < len(token) && isLetterByte(token[i]) {
		i++
	}
	if i == start || i-start > 2 {
		return Ref{}, false
	}
	col, ok := ParseColumnName(token[start:i])
	if !ok {
		return Ref{}, false
	}
	ref.Col = col
	if i < len(token) && token[i] == '@' {
		ref.AbsRow = true
		i++
	}
	start = i
	for i < len(token) && token[i] >= '0' && token[i] <= '9' {
		i++
	}
	if i == start || i != len(token) {
		return Ref{}, false
	}
	row, err := strconv.Atoi(token[start:i])
	if err != nil || row < 1 || row > MaxRows {
		return Ref{}, false
	}
	ref.Row = row - 1
	return ref, true
}

// DeadRef is substituted for references whose target was removed by a row or
// column deletion. It does not parse, so the dependent cell reports an error.
const DeadRef = "#REF"

// RewriteRefs walks a formula and hands every reference token to fn. When fn
// returns false the reference is replaced with DeadRef. Everything that is not
// a reference (spacing, operators, function names) is preserved verbatim.
func RewriteRefs(formula string, fn func(Ref) (Ref, bool)) string {
	var out strings.Builder
	runes := []rune(formula)
	for i := 0; i < len(runes); {
		if !isRefStart(runes[i]) {
			out.WriteRune(runes[i])
			i++
			continue
		}
		start := i
		for i < len(runes) && isRefBody(runes[i]) {
			i++
		}
		token := string(runes[start:i])
		ref, ok := ParseRef(token)
		if !ok {
			out.WriteString(token)
			continue
		}
		updated, alive := fn(ref)
		if !alive {
			out.WriteString(DeadRef)
			continue
		}
		out.WriteString(updated.String())
	}
	return out.String()
}

// DetectKind classifies raw cell text the way the classic editor does.
func DetectKind(text string) Kind {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "=") {
		return KindFormula
	}
	if trimmed == "" {
		return KindText
	}
	if _, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return KindValue
	}
	return KindText
}

// NewCell builds a cell from raw text, choosing the kind and the default
// alignment: text is flushed left, numbers and formulas right.
func NewCell(text string) *Cell {
	cell := &Cell{Text: text, Kind: DetectKind(text)}
	if cell.Kind == KindText {
		cell.Justify = JustifyLeft
	} else {
		cell.Justify = JustifyRight
	}
	if cell.Kind == KindValue {
		cell.Value, _ = strconv.ParseFloat(strings.TrimSpace(text), 64)
	}
	return cell
}

// Formula returns the expression of a formula cell without its leading '='.
func (c *Cell) Formula() string {
	if c == nil || c.Kind != KindFormula {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(c.Text), "=")
}

// Clone returns an independent copy of the cell.
func (c *Cell) Clone() *Cell {
	if c == nil {
		return nil
	}
	copied := *c
	return &copied
}

// IsEmpty reports whether the cell holds nothing at all.
func (c *Cell) IsEmpty() bool {
	return c == nil || strings.TrimSpace(c.Text) == ""
}

// DisplayText renders the cell according to its display format. Formula cells
// that failed to evaluate render as ErrorText.
func (c *Cell) DisplayText() string {
	if c == nil {
		return ""
	}
	if c.Display == DisplayHidden {
		return ""
	}
	if c.Kind == KindFormula && c.Err != "" {
		return ErrorText
	}
	switch c.Display {
	case DisplayAsIs:
		if c.Kind == KindFormula {
			return generalNumber(c.Value)
		}
		return c.Text
	case DisplayDecimal:
		if c.Kind == KindText {
			return c.Text
		}
		return strconv.FormatFloat(c.Value, 'f', int(c.Decimals), 64)
	case DisplayComma:
		if c.Kind == KindText {
			return c.Text
		}
		return groupThousands(generalNumber(c.Value))
	case DisplayExponent:
		if c.Kind == KindText {
			return c.Text
		}
		return strconv.FormatFloat(c.Value, 'E', -1, 64)
	case DisplayLogical:
		if c.Kind == KindText {
			return c.Text
		}
		if c.Value == 0 {
			return BoolFalseText
		}
		return BoolTrueText
	case DisplayCurrency:
		if c.Kind == KindText {
			return c.Text
		}
		decimals := int(c.Decimals)
		if decimals == 0 {
			decimals = 2
		}
		return CurrencySymbol + groupThousands(strconv.FormatFloat(c.Value, 'f', decimals, 64))
	case DisplayPercent:
		if c.Kind == KindText {
			return c.Text
		}
		return groupThousands(strconv.FormatFloat(c.Value*100, 'f', 2, 64)) + "%"
	}
	return c.Text
}

// NumericValue returns the value a formula would see when referring to this
// cell: empty cells count as zero, text cells are an error.
func (c *Cell) NumericValue() (float64, error) {
	if c.IsEmpty() {
		return 0, nil
	}
	if c.Kind == KindText {
		return 0, fmt.Errorf("reference to a text cell")
	}
	if c.Err != "" {
		return 0, fmt.Errorf("%s", c.Err)
	}
	return c.Value, nil
}

// generalNumber prints a float without an exponent where practical and
// without trailing zeros, which is what the "as is" and comma formats use.
func generalNumber(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ErrorText
	}
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatFloat(value, 'f', 0, 64)
	}
	// Fifteen significant digits is the widest a float64 can carry without
	// exposing binary rounding noise such as 1234567.8910000001.
	text := strconv.FormatFloat(value, 'g', 15, 64)
	if strings.ContainsAny(text, "eE") {
		text = strconv.FormatFloat(value, 'f', -1, 64)
	}
	if strings.Contains(text, ".") {
		text = strings.TrimRight(text, "0")
		text = strings.TrimSuffix(text, ".")
	}
	if text == "" || text == "-" {
		return "0"
	}
	return text
}

// groupThousands inserts ThousandSeparator into the integer part of a printed
// number, leaving any sign and fractional part untouched.
func groupThousands(text string) string {
	sign := ""
	if strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+") {
		sign, text = text[:1], text[1:]
	}
	integer, fraction := text, ""
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		integer, fraction = text[:dot], text[dot:]
	}
	for i := len(integer) - 3; i > 0; i -= 3 {
		integer = integer[:i] + ThousandSeparator + integer[i:]
	}
	return sign + integer + fraction
}

func upperByte(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 'a' + 'A'
	}
	return b
}

func isLetterByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isRefStart(r rune) bool {
	return r == '@' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isRefBody(r rune) bool {
	return isRefStart(r) || (r >= '0' && r <= '9')
}
