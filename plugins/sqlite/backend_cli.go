package sqlite

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// cliBackend reads and writes a database by running the sqlite3
// command-line tool, the way plugins/multiarc runs the host's archivers: the
// lite build links no SQLite engine (about 7 MB) and uses whatever sqlite3
// the host has (opkg install sqlite3-cli, apt install sqlite3, ...).
//
// Every call is one sqlite3 process fed a script on stdin, in ascii mode:
// fields end with 0x1F and records with 0x1E. Where f4 writes the SQL itself
// it asks for every value as typeof(v)||':'||hex(v), which keeps the type
// database/sql would have scanned and the exact bytes, and holds nothing but
// [a-z0-9A-F:] -- so neither a separator inside a value nor the way a given
// sqlite3 version escapes control characters on output (3.50 prints them as
// ^X by default) can change what comes back. A statement typed into the SQL
// box runs as typed, so its values come back as the text sqlite3 prints.
//
// What a process per call cannot do is keep state between calls: a
// transaction begun in the SQL box ends with that statement.
type cliBackend struct {
	bin  string
	path string
}

// sqlite3LookPath is exec.LookPath, replaced in tests.
var sqlite3LookPath = exec.LookPath

// errNoSQLite3 is what the lite client says when the host has no sqlite3.
var errNoSQLite3 = errors.New("the sqlite3 command-line tool is not installed (it is on PATH as sqlite3 after, for example, opkg install sqlite3-cli or apt install sqlite3)")

func openCLIBackend(ctx context.Context, path string) (*cliBackend, error) {
	bin, err := sqlite3LookPath("sqlite3")
	if err != nil {
		return nil, errNoSQLite3
	}
	return &cliBackend{bin: bin, path: path}, nil
}

func (b *cliBackend) close() {}

// cliPrelude is what every script starts with. A user's ~/.sqliterc is
// skipped (-init names an empty file instead), so none of it -- .headers,
// .changes, .echo, .mode -- can change what comes back.
const cliPrelude = ".mode ascii\n.timeout 5000\n"

// cliMarker separates a statement's own output from the count f4 asks for
// after it: an INSERT ... RETURNING prints rows of its own.
const cliMarker = "f4-sqlite-end-3c9e1d"

var (
	emptyInitOnce sync.Once
	emptyInitPath string
	emptyInitErr  error
)

// emptyInit is a file with nothing in it for -init, created once per run.
func emptyInit() (string, error) {
	emptyInitOnce.Do(func() {
		file, err := os.CreateTemp("", "f4-sqliterc-*")
		if err != nil {
			emptyInitErr = err
			return
		}
		emptyInitPath = file.Name()
		emptyInitErr = file.Close()
	})
	return emptyInitPath, emptyInitErr
}

func (b *cliBackend) command(ctx context.Context, script string) (*exec.Cmd, *bytes.Buffer, error) {
	initPath, err := emptyInit()
	if err != nil {
		return nil, nil, err
	}
	// #nosec G204 -- the binary is the sqlite3 found on PATH and the only other argument is the database the user opened; the script goes to stdin.
	cmd := exec.CommandContext(ctx, b.bin, "-batch", "-bail", "-init", initPath, b.path)
	cmd.Stdin = strings.NewReader(cliPrelude + script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd, &stderr, nil
}

func (b *cliBackend) run(ctx context.Context, script string) ([]byte, error) {
	cmd, stderr, err := b.command(ctx, script)
	if err != nil {
		return nil, err
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, cliError(err, stderr.String())
	}
	return out, nil
}

var cliErrorPrefix = regexp.MustCompile(`^(?:(?:Parse|Runtime) error|Error)(?:: near line \d+| near line \d+)?: `)

// cliError turns what sqlite3 printed into the message the driver would
// have given: "no such table: t" rather than "Parse error near line 3: no
// such table: t" and the SQL echoed under it.
func cliError(err error, stderr string) error {
	message := strings.TrimSpace(stderr)
	if line, _, found := strings.Cut(message, "\n"); found {
		message = strings.TrimSpace(line)
	}
	message = cliErrorPrefix.ReplaceAllString(message, "")
	if message == "" {
		return fmt.Errorf("sqlite3: %w", err)
	}
	return errors.New(message)
}

// cliRecordReader reads ascii-mode output one record at a time.
type cliRecordReader struct {
	r *bufio.Reader
}

// next returns the fields of the next record, or io.EOF after the last.
func (c *cliRecordReader) next() ([]string, error) {
	var (
		fields []string
		field  []byte
	)
	for {
		ch, err := c.r.ReadByte()
		if err == io.EOF {
			if len(fields) == 0 && len(field) == 0 {
				return nil, io.EOF
			}
			return append(fields, string(field)), nil
		}
		if err != nil {
			return nil, err
		}
		switch ch {
		case 0x1F:
			fields = append(fields, string(field))
			field = field[:0]
		case 0x1E:
			return append(fields, string(field)), nil
		default:
			field = append(field, ch)
		}
	}
}

func readRecords(out []byte) ([][]string, error) {
	reader := &cliRecordReader{r: bufio.NewReader(bytes.NewReader(out))}
	var records [][]string
	for {
		record, err := reader.next()
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
}

// typedValue is the select-list expression that reads one column in the
// typeof:hex form parseTyped takes apart. A finite non-zero real is written
// out as its exact binary value, mantissa "p" exponent (realBitsSQL): no
// decimal text survives every sqlite3 build, since the conversion both ways
// is sqlite3's own and some builds are an ulp off -- macOS's sqlite3 prints
// 1e300 as 9.999999999999999e+299 even with printf('%!.17g'). Zero and the
// infinities keep printf's text.
func typedValue(column string) string {
	quoted := quoteIdentifier(column)
	return "typeof(" + quoted + ")||':'||CASE typeof(" + quoted + ") WHEN 'real' THEN hex(" + realBitsSQL(quoted) + ") ELSE hex(" + quoted + ") END"
}

// realBitsSQL is the SQL expression that writes the real x as m||'p'||e, an
// integer m and a power of two e with x = m * 2^e exactly. A recursive CTE
// scales |x| into [1, 2) by powers of two -- 2^64 at a time, then 2 -- which
// is exact for every double, subnormals included, and then 2^52 times that
// is the 53-bit integer mantissa. The powers are built from integers, so no
// decimal literal is converted on the way. x - x = 0 is false for the
// infinities (NULL), which printf spells Inf and -Inf.
func realBitsSQL(x string) string {
	const big = "(CAST(4294967296 AS REAL) * 4294967296)"
	return "CASE WHEN " + x + " - " + x + " = 0 AND " + x + " <> 0 THEN (WITH RECURSIVE s(y, e) AS (SELECT abs(" + x + "), 0" +
		" UNION ALL SELECT CASE WHEN y >= " + big + " THEN y / " + big + " WHEN y >= 2 THEN y / 2 WHEN y * " + big + " < 1 THEN y * " + big + " ELSE y * 2 END," +
		" CASE WHEN y >= " + big + " THEN e + 64 WHEN y >= 2 THEN e + 1 WHEN y * " + big + " < 1 THEN e - 64 ELSE e - 1 END" +
		" FROM s WHERE y >= 2 OR y < 1)" +
		" SELECT CASE WHEN " + x + " < 0 THEN '-' ELSE '' END || CAST(y * 4503599627370496 AS INTEGER) || 'p' || (e - 52) FROM s WHERE y >= 1 AND y < 2)" +
		" ELSE printf('%!.17g', " + x + ") END"
}

// typedColumns is typedValue for every column, as a select list.
func typedColumns(columns []string) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = typedValue(column)
	}
	return strings.Join(parts, ", ")
}

// parseTyped turns typeof:hex back into the value database/sql would have
// scanned: nil, int64, float64, string or []byte. hex() of a number is hex
// of the number's text, which is what the number is read back from.
func parseTyped(field string) (any, error) {
	kind, encoded, found := strings.Cut(field, ":")
	if !found {
		return nil, fmt.Errorf("sqlite3: unexpected value %q in output", field)
	}
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("sqlite3: unexpected value %q in output: %w", field, err)
	}
	switch kind {
	case "null":
		return nil, nil
	case "integer":
		return strconv.ParseInt(string(raw), 10, 64)
	case "real":
		return parseReal(string(raw))
	case "text":
		return string(raw), nil
	case "blob":
		return raw, nil
	}
	return nil, fmt.Errorf("sqlite3: unexpected type %q in output", kind)
}

// parseReal reads what realBitsSQL wrote: m||'p'||e, or printf's text for
// zero and the infinities.
func parseReal(text string) (float64, error) {
	mantissa, exponent, found := strings.Cut(text, "p")
	if !found {
		return strconv.ParseFloat(text, 64)
	}
	m, err := strconv.ParseInt(mantissa, 10, 64)
	if err != nil {
		return 0, err
	}
	e, err := strconv.Atoi(exponent)
	if err != nil {
		return 0, err
	}
	// m has at most 53 bits, so float64(m) is exact, and so is scaling it
	// back by a power of two to the double it came from.
	return math.Ldexp(float64(m), e), nil
}

// unistr decodes the escapes of SQLite's unistr(): \\ for a backslash and
// \XXXX, \uXXXX, \+XXXXXX or \UXXXXXXXX for a code point in hex.
func unistr(text string) (any, bool) {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' {
			b.WriteByte(text[i])
			continue
		}
		rest := text[i+1:]
		digits := 4
		switch {
		case strings.HasPrefix(rest, `\`):
			b.WriteByte('\\')
			i++
			continue
		case strings.HasPrefix(rest, "u"):
			rest, i = rest[1:], i+1
		case strings.HasPrefix(rest, "+"):
			rest, i, digits = rest[1:], i+1, 6
		case strings.HasPrefix(rest, "U"):
			rest, i, digits = rest[1:], i+1, 8
		}
		if len(rest) < digits {
			return nil, false
		}
		code, err := strconv.ParseUint(rest[:digits], 16, 32)
		if err != nil || code > utf8.MaxRune {
			return nil, false
		}
		b.WriteRune(rune(code))
		i += digits
	}
	return b.String(), true
}

// sqlText is value as an SQL string literal. A string literal stores the
// same TEXT a bound parameter does, so column affinity treats both alike.
func sqlText(value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		return "", errors.New("SQLite: the sqlite3 tool cannot pass a value with a NUL byte")
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}

// afterMarker is what a script printed after cliMarker.
func afterMarker(out []byte) ([]byte, error) {
	i := bytes.LastIndex(out, []byte(cliMarker))
	if i < 0 {
		return nil, errors.New("sqlite3: output ended early")
	}
	rest := out[i+len(cliMarker):]
	rest = bytes.TrimLeft(rest, "\r\n")
	return rest, nil
}

// countAfter runs statement and then reads the one number count asks for:
// changes() or last_insert_rowid().
func (b *cliBackend) countAfter(ctx context.Context, statement, count string) (int64, error) {
	out, err := b.run(ctx, statement+"\n;\n.print "+cliMarker+"\nSELECT "+count+";\n")
	if err != nil {
		return 0, err
	}
	rest, err := afterMarker(out)
	if err != nil {
		return 0, err
	}
	records, err := readRecords(rest)
	if err != nil {
		return 0, err
	}
	if len(records) != 1 || len(records[0]) != 1 {
		return 0, fmt.Errorf("sqlite3: unexpected output for %s", count)
	}
	return strconv.ParseInt(strings.TrimSpace(records[0][0]), 10, 64)
}

// typed runs a statement whose every column is typedValue (or a plain
// integer such as rowid) and returns the values.
func (b *cliBackend) typed(ctx context.Context, statement string) ([][]any, error) {
	out, err := b.run(ctx, statement+"\n;\n")
	if err != nil {
		return nil, err
	}
	records, err := readRecords(out)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(records))
	for _, record := range records {
		values := make([]any, len(record))
		for i, field := range record {
			if values[i], err = parseTypedOrInteger(field); err != nil {
				return nil, err
			}
		}
		rows = append(rows, values)
	}
	return rows, nil
}

// parseTypedOrInteger reads a typeof:hex field, or a bare integer such as a
// rowid or a count.
func parseTypedOrInteger(field string) (any, error) {
	if n, err := strconv.ParseInt(field, 10, 64); err == nil {
		return n, nil
	}
	return parseTyped(field)
}

func (b *cliBackend) listTables(ctx context.Context) ([]string, error) {
	rows, err := b.typed(ctx, strings.Replace(listTablesSQL, "SELECT name", "SELECT "+typedValue("name"), 1))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		name, ok := row[0].(string)
		if !ok {
			return nil, errors.New("sqlite3: unexpected table name in output")
		}
		names = append(names, name)
	}
	return names, nil
}

func (b *cliBackend) exec(ctx context.Context, statement string) (int64, error) {
	return b.countAfter(ctx, statement, "changes()")
}

// query runs a statement typed into the SQL box. It runs in quote mode,
// where sqlite3 prints every value as an SQL literal -- NULL, 42, 1.5,
// 'text', X'00FF', and in newer versions unistr('...') for text holding
// control characters -- so NULL, numbers and blobs come back as what they
// are; ascii mode would print a blob as text, cut at its first NUL byte. A
// real comes back as the text sqlite3 printed, which some versions round.
// A field that is none of these is shown as sqlite3 printed it: these
// values are only ever displayed.
func (b *cliBackend) query(ctx context.Context, statement string) (queryResult, error) {
	out, err := b.run(ctx, ".mode quote\n.headers on\n"+statement+"\n;\n")
	if err != nil {
		return queryResult{}, err
	}
	records, err := readQuoteRecords(out)
	if err != nil {
		return queryResult{}, err
	}
	result := queryResult{ReturnsRows: true}
	if len(records) == 0 {
		// sqlite3 prints the header with the first row, so a statement that
		// returned nothing leaves no column names behind either.
		return result, nil
	}
	result.Columns = make([]string, len(records[0]))
	for i, field := range records[0] {
		result.Columns[i] = field
		if name, ok := parseSQLLiteral(field); ok {
			if text, isText := name.(string); isText {
				result.Columns[i] = text
			}
		}
	}
	for _, record := range records[1:] {
		cells := make([]string, len(record))
		for i, field := range record {
			if value, ok := parseSQLLiteral(field); ok {
				cells[i] = displayValue(value)
			} else {
				cells[i] = displayValue(field)
			}
		}
		result.Rows = append(result.Rows, cells)
	}
	return result, nil
}

// readQuoteRecords splits quote-mode output into records and fields:
// commas between fields and line breaks between records, except inside a
// quoted string or inside parentheses.
func readQuoteRecords(out []byte) ([][]string, error) {
	var (
		records [][]string
		fields  []string
		field   []byte
		depth   int
	)
	for i := 0; i < len(out); i++ {
		ch := out[i]
		switch {
		case ch == '\'':
			field = append(field, ch)
			for {
				i++
				if i >= len(out) {
					return nil, errors.New("sqlite3: unterminated string in output")
				}
				field = append(field, out[i])
				if out[i] == '\'' {
					if i+1 < len(out) && out[i+1] == '\'' {
						i++
						field = append(field, '\'')
						continue
					}
					break
				}
			}
		case ch == '(':
			depth++
			field = append(field, ch)
		case ch == ')' && depth > 0:
			depth--
			field = append(field, ch)
		case ch == ',' && depth == 0:
			fields = append(fields, string(field))
			field = field[:0]
		case ch == '\r' && depth == 0 && i+1 < len(out) && out[i+1] == '\n':
		case ch == '\n' && depth == 0:
			records = append(records, append(fields, string(field)))
			fields, field = nil, field[:0]
		default:
			field = append(field, ch)
		}
	}
	if len(fields) > 0 || len(field) > 0 {
		records = append(records, append(fields, string(field)))
	}
	return records, nil
}

// parseSQLLiteral reads one plain SQL literal -- NULL, an integer, a real,
// 'text', unistr('text') or X'hex' -- into the value database/sql would
// have scanned.
func parseSQLLiteral(literal string) (any, bool) {
	switch {
	case literal == "NULL":
		return nil, true
	case strings.HasPrefix(literal, "unistr(") && strings.HasSuffix(literal, ")"):
		inner, ok := parseSQLLiteral(literal[len("unistr(") : len(literal)-1])
		text, isText := inner.(string)
		if !ok || !isText {
			return nil, false
		}
		return unistr(text)
	case len(literal) >= 2 && literal[0] == '\'' && literal[len(literal)-1] == '\'':
		body := literal[1 : len(literal)-1]
		if strings.Contains(strings.ReplaceAll(body, "''", ""), "'") {
			return nil, false
		}
		return strings.ReplaceAll(body, "''", "'"), true
	case len(literal) >= 3 && (literal[0] == 'X' || literal[0] == 'x') && literal[1] == '\'' && literal[len(literal)-1] == '\'':
		raw, err := hex.DecodeString(literal[2 : len(literal)-1])
		return raw, err == nil
	}
	if n, err := strconv.ParseInt(literal, 10, 64); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(literal, 64); err == nil {
		return f, true
	}
	return nil, false
}

func (b *cliBackend) countRows(ctx context.Context, table string) (int64, error) {
	rows, err := b.typed(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table))
	if err != nil {
		return 0, err
	}
	if len(rows) != 1 || len(rows[0]) != 1 {
		return 0, errors.New("sqlite3: unexpected output for COUNT(*)")
	}
	total, ok := rows[0][0].(int64)
	if !ok {
		return 0, errors.New("sqlite3: unexpected output for COUNT(*)")
	}
	return total, nil
}

func (b *cliBackend) insertRow(ctx context.Context, table string) (int64, error) {
	return b.countAfter(ctx, "INSERT INTO "+quoteIdentifier(table)+" DEFAULT VALUES", "last_insert_rowid()")
}

// tableColumns lists a table's or a view's columns with their declared
// types, from PRAGMA table_info: cid, name, type, notnull, dflt_value, pk.
func (b *cliBackend) tableColumns(ctx context.Context, table string) (names, types []string, err error) {
	out, err := b.run(ctx, "PRAGMA table_info("+quoteIdentifier(table)+");\n")
	if err != nil {
		return nil, nil, err
	}
	records, err := readRecords(out)
	if err != nil {
		return nil, nil, err
	}
	for _, record := range records {
		if len(record) < 3 {
			return nil, nil, errors.New("sqlite3: unexpected output for PRAGMA table_info")
		}
		names = append(names, record[1])
		types = append(types, record[2])
	}
	return names, types, nil
}

func (b *cliBackend) browseWithRowIDs(ctx context.Context, table string, offset int64) (queryResult, []int64, error) {
	columns, _, err := b.tableColumns(ctx, table)
	if err != nil {
		return queryResult{}, nil, err
	}
	if len(columns) == 0 {
		return queryResult{}, nil, errors.New("SQLite: table has no columns of its own")
	}
	// Ordered by rowid so that paging is stable, as backend_driver.go's is.
	statement := "SELECT rowid, " + typedColumns(columns) + " FROM " + quoteIdentifier(table) +
		" ORDER BY rowid LIMIT " + strconv.Itoa(browsePageSize) + " OFFSET " + strconv.FormatInt(offset, 10)
	rows, err := b.typed(ctx, statement)
	if err != nil {
		return queryResult{}, nil, err
	}
	result := queryResult{Columns: columns, ReturnsRows: true}
	var rowIDs []int64
	for _, values := range rows {
		rowID, ok := values[0].(int64)
		if !ok {
			return queryResult{}, nil, errors.New("SQLite: rows have no usable rowid")
		}
		cells := make([]string, len(values)-1)
		for i, value := range values[1:] {
			cells[i] = displayValue(value)
		}
		rowIDs = append(rowIDs, rowID)
		result.Rows = append(result.Rows, cells)
	}
	return result, rowIDs, nil
}

// browse reads a page of a table without rowids: a view, or a WITHOUT
// ROWID table. Typed like browseWithRowIDs, which a plain SELECT typed into
// the SQL box would not be.
func (b *cliBackend) browse(ctx context.Context, table string, offset int64) (queryResult, error) {
	columns, _, err := b.tableColumns(ctx, table)
	if err != nil {
		return queryResult{}, err
	}
	if len(columns) == 0 {
		return queryResult{}, fmt.Errorf("no such table: %s", table)
	}
	statement := "SELECT " + typedColumns(columns) + " FROM " + quoteIdentifier(table) + " LIMIT " + strconv.Itoa(browsePageSize)
	if offset > 0 {
		statement += " OFFSET " + strconv.FormatInt(offset, 10)
	}
	rows, err := b.typed(ctx, statement)
	if err != nil {
		return queryResult{}, err
	}
	result := queryResult{Columns: columns, ReturnsRows: true}
	for _, values := range rows {
		cells := make([]string, len(values))
		for i, value := range values {
			cells[i] = displayValue(value)
		}
		result.Rows = append(result.Rows, cells)
	}
	return result, nil
}

func (b *cliBackend) cellValue(ctx context.Context, table, column string, rowID int64) (any, error) {
	rows, err := b.typed(ctx, "SELECT "+typedValue(column)+" FROM "+quoteIdentifier(table)+
		" WHERE rowid = "+strconv.FormatInt(rowID, 10))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("sql: no rows in result set")
	}
	return rows[0][0], nil
}

func (b *cliBackend) updateCell(ctx context.Context, table, column string, rowID int64, value string) (int64, error) {
	literal, err := sqlText(value)
	if err != nil {
		return 0, err
	}
	return b.countAfter(ctx, "UPDATE "+quoteIdentifier(table)+" SET "+quoteIdentifier(column)+" = "+literal+
		" WHERE rowid = "+strconv.FormatInt(rowID, 10), "changes()")
}

func (b *cliBackend) deleteRow(ctx context.Context, table string, rowID int64) (int64, error) {
	return b.countAfter(ctx, "DELETE FROM "+quoteIdentifier(table)+" WHERE rowid = "+strconv.FormatInt(rowID, 10), "changes()")
}

func (b *cliBackend) columnDeclaredType(ctx context.Context, table, column string) (string, error) {
	names, types, err := b.tableColumns(ctx, table)
	if err != nil {
		return "", err
	}
	for i, name := range names {
		if strings.EqualFold(name, column) {
			return types[i], nil
		}
	}
	return "", nil
}

// scanTable streams the table rather than reading it whole: an export can
// be much larger than a page.
func (b *cliBackend) scanTable(ctx context.Context, table string, columns func([]string) error, row func([]any) error) (err error) {
	names, _, err := b.tableColumns(ctx, table)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no such table: %s", table)
	}
	if err := columns(names); err != nil {
		return err
	}
	cmd, stderr, err := b.command(ctx, "SELECT "+typedColumns(names)+" FROM "+quoteIdentifier(table)+";\n")
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		if waitErr := cmd.Wait(); waitErr != nil && err == nil {
			err = cliError(waitErr, stderr.String())
		}
	}()
	reader := &cliRecordReader{r: bufio.NewReader(stdout)}
	values := make([]any, len(names))
	for {
		record, err := reader.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			_, _ = io.Copy(io.Discard, stdout)
			return err
		}
		if len(record) != len(names) {
			_, _ = io.Copy(io.Discard, stdout)
			return errors.New("sqlite3: unexpected row in output")
		}
		for i, field := range record {
			if values[i], err = parseTyped(field); err != nil {
				_, _ = io.Copy(io.Discard, stdout)
				return err
			}
		}
		if err := row(values); err != nil {
			_, _ = io.Copy(io.Discard, stdout)
			return err
		}
	}
}
