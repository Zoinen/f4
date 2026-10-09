package sqlite

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"
)

func hexText(s string) string { return hex.EncodeToString([]byte(s)) }

func TestParseTyped(t *testing.T) {
	for _, tt := range []struct {
		field string
		want  any
	}{
		{"null:", nil},
		{"integer:2D39303037313939323534373430393933", int64(-9007199254740993)},
		{"real:312E30652B333030", 1e300},
		{"real:" + hexText("6724873095247260p944"), 1e300},
		{"real:" + hexText("4503599627370496p-1126"), 5e-324},
		{"real:" + hexText("-5066549580791808p-51"), -2.25},
		{"real:" + hexText("5404319552844596p-54"), 0.30000000000000004},
		{"real:" + hexText("Inf"), math.Inf(1)},
		{"text:D182D0B5D181D1820A1F1E", "тест\n\x1f\x1e"},
		{"text:", ""},
		{"blob:00FF1E1F", []byte{0x00, 0xff, 0x1e, 0x1f}},
		{"blob:", []byte{}},
		{"42", int64(42)},
	} {
		got, err := parseTypedOrInteger(tt.field)
		if err != nil {
			t.Errorf("parseTypedOrInteger(%q): %v", tt.field, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseTypedOrInteger(%q) = %#v, want %#v", tt.field, got, tt.want)
		}
	}
	for _, bad := range []string{"text:zz", "mystery:00", "nocolon"} {
		if _, err := parseTypedOrInteger(bad); err == nil {
			t.Errorf("parseTypedOrInteger(%q) accepted a malformed field", bad)
		}
	}
}

// sqlite3 prefixes its messages with where it failed and echoes the SQL
// under them; the client shows what the driver would have said.
func TestCLIErrorKeepsTheMessage(t *testing.T) {
	for stderr, want := range map[string]string{
		"Parse error near line 3: no such table: t\n  SELECT * FROM t;\n                ^--- error here\n": "no such table: t",
		"Runtime error near line 1: NOT NULL constraint failed: t.a (19)\n":                                "NOT NULL constraint failed: t.a (19)",
		"Error: near line 2: database is locked\n":                                                         "database is locked",
		"Error: unable to open database \"x\": unable to open database file\n":                             "unable to open database \"x\": unable to open database file",
	} {
		if got := cliError(errors.New("exit status 1"), stderr).Error(); got != want {
			t.Errorf("cliError(%q) = %q, want %q", stderr, got, want)
		}
	}
	if got := cliError(errors.New("exit status 1"), "").Error(); got != "sqlite3: exit status 1" {
		t.Errorf("cliError with no stderr = %q", got)
	}
}

func TestSQLTextRefusesNUL(t *testing.T) {
	if got, err := sqlText("O'Brien"); err != nil || got != "'O''Brien'" {
		t.Errorf("sqlText(O'Brien) = %q, %v", got, err)
	}
	if _, err := sqlText("a\x00b"); err == nil {
		t.Error("sqlText accepted a NUL byte")
	}
}

// Quote mode separates fields with commas and records with line breaks,
// except inside a string or a function call, and a field that is not a
// plain literal is kept as sqlite3 printed it.
func TestReadQuoteRecords(t *testing.T) {
	out := []byte("'a','b, c'\n1,'x''y\nz'\nX'00FF',unistr('p\\u000aq, r')\nNULL,-1.5e+300\n")
	records, err := readQuoteRecords(out)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"'a'", "'b, c'"},
		{"1", "'x''y\nz'"},
		{"X'00FF'", "unistr('p\\u000aq, r')"},
		{"NULL", "-1.5e+300"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}
	for field, want := range map[string]any{
		"'x''y\nz'":                    "x'y\nz",
		"X'00FF'":                      []byte{0x00, 0xff},
		"NULL":                         nil,
		"-1.5e+300":                    -1.5e300,
		"1":                            int64(1),
		`unistr('p\u000aq, r')`:        "p\nq, r",
		`unistr('a\\b\001fc''d')`:      "a\\b\x1fc'd",
		`unistr('\+01F600\U0001F600')`: "😀😀",
	} {
		got, ok := parseSQLLiteral(field)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("parseSQLLiteral(%q) = %#v, %v; want %#v", field, got, ok, want)
		}
	}
	for _, field := range []string{"'a'||'b'", "X'0'", `unistr('\u00')`, "unistr(42)", "char(10)"} {
		if _, ok := parseSQLLiteral(field); ok {
			t.Errorf("parseSQLLiteral(%q) took an expression for a literal", field)
		}
	}
}

// parseReal reads back what realBitsSQL wrote, m||'p'||e; either half can be
// malformed if a future sqlite3 build ever changed the format underneath it.
func TestParseRealRejectsMalformedText(t *testing.T) {
	for _, text := range []string{
		"xp5",   // mantissa is not a number
		"12pxy", // exponent is not a number
	} {
		if _, err := parseReal(text); err == nil {
			t.Errorf("parseReal(%q) accepted a malformed value", text)
		}
	}
}

// unistr's own inverse (unistr()) never emits an escape it cannot parse back,
// but the text quote mode prints is not guaranteed to be exactly that -- a
// future sqlite3 could print something wider, so an unparsable escape must
// fail rather than silently swallow bytes.
func TestUnistrRejectsInvalidCodePoints(t *testing.T) {
	for _, text := range []string{
		`\uZZZZ`,     // not hex digits
		`\U00110000`, // one past the last valid Unicode code point
	} {
		if _, ok := unistr(text); ok {
			t.Errorf("unistr(%q) accepted an invalid escape", text)
		}
	}
}

func TestAfterMarkerNotFound(t *testing.T) {
	if _, err := afterMarker([]byte("no marker was ever printed\n")); err == nil {
		t.Error("afterMarker accepted output without the marker")
	}
}

// A record that ends at EOF without its trailing 0x1E is not how sqlite3's
// own output looks, but the reader has to hand back what arrived rather than
// silently drop it if the process is ever killed mid-write.
func TestCLIRecordReaderKeepsAnUnterminatedTrailingRecord(t *testing.T) {
	r := &cliRecordReader{r: bufio.NewReader(bytes.NewReader([]byte("a\x1Fb")))}
	record, err := r.next()
	if err != nil {
		t.Fatalf("next() = %v", err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(record, want) {
		t.Errorf("next() = %#v, want %#v", record, want)
	}
	if _, err := r.next(); err != io.EOF {
		t.Errorf("next() at end = %v, want io.EOF", err)
	}
}

type erroringReader struct{ err error }

func (r erroringReader) Read([]byte) (int, error) { return 0, r.err }

// scanTable reads straight off the process's stdout pipe, so a broken pipe
// or a killed sqlite3 can hand the reader a real error, not just EOF.
func TestCLIRecordReaderPropagatesReadErrors(t *testing.T) {
	boom := errors.New("boom")
	r := &cliRecordReader{r: bufio.NewReader(io.MultiReader(bytes.NewReader([]byte("a\x1F")), erroringReader{boom}))}
	if _, err := r.next(); !errors.Is(err, boom) {
		t.Errorf("next() = %v, want %v", err, boom)
	}
}

// updateCell must not let a NUL byte anywhere near sqlite3: the client
// checks for it (sqlText) before ever touching the backend.
func TestUpdateCellRejectsNULByte(t *testing.T) {
	backend := &cliBackend{}
	if _, err := backend.updateCell(context.Background(), "t", "c", 1, "a\x00b"); err == nil {
		t.Error("updateCell accepted a value with a NUL byte")
	}
}
