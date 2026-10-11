//go:build !lite

package sqlite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ncruces/go-sqlite3/driver"
)

// The lite build's client reads databases through the sqlite3 tool
// (backend_cli.go) and the regular build's through the linked engine
// (backend_driver.go). These tests run the same operations through both,
// each on its own copy of one database, and require the same answers.

func createConformanceDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "conformance.sqlite")
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, statement := range []string{
		`CREATE TABLE t (a INT, b TEXT, c BLOB, d REAL)`,
		`INSERT INTO t VALUES (1, 'plain', NULL, 1.5)`,
		`INSERT INTO t VALUES (NULL, 'it''s' || char(10) || 'two lines', x'00ff1e1f', -2.25)`,
		`INSERT INTO t VALUES (3, 'sep' || char(31) || 'inside' || char(30) || 'value', x'', NULL)`,
		// 1e300 is the value macOS's own sqlite3 prints one ulp off, even
		// with printf('%!.17g'); typedValue reads it as mantissa and power
		// of two, so the CLI backend still gets it exactly.
		`INSERT INTO t VALUES (-9007199254740993, 'тест', 'text in blob', 1e300)`,
		`INSERT INTO t VALUES (6, 'more reals', 123456789.125, -1.7976931348623157e308)`,
		`INSERT INTO t VALUES (5, 'reals', 0.1 + 0.2, 4.9e-324)`,
		`CREATE VIEW v AS SELECT a, d FROM t WHERE a IS NOT NULL`,
		`CREATE TABLE w (k TEXT PRIMARY KEY, n INT) WITHOUT ROWID`,
		`INSERT INTO w VALUES ('x', 1), ('y', 2)`,
		`CREATE TABLE "odd ""name""" (x INT DEFAULT 7, "y z" TEXT)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	return path
}

// conformance is everything one backend answered.
type conformance struct {
	tables      []string
	count       int64
	browse      tableBrowse
	viewBrowse  tableBrowse
	noRowID     tableBrowse
	cells       []any
	declared    []string
	csv         string
	query       queryResult
	queryErr    string
	updated     int64
	afterUpdate []any
	deleted     int64
	insertedID  int64
	oddBrowse   tableBrowse
	execChanges int64
}

func collectConformance(t *testing.T, open func(context.Context, string) (sessionBackend, error)) conformance {
	t.Helper()
	previous := openSessionBackend
	openSessionBackend = open
	defer func() { openSessionBackend = previous }()

	ctx := context.Background()
	session, tables, err := openDatabase(ctx, createConformanceDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	var c conformance
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	c.tables = tables
	c.count, err = session.countRows(ctx, "t")
	must(err)
	c.browse, err = session.browseTable(ctx, "t", 0)
	must(err)
	c.viewBrowse, err = session.browseTable(ctx, "v", 0)
	must(err)
	c.noRowID, err = session.browseTable(ctx, "w", 0)
	must(err)
	for _, rowID := range c.browse.rowIDs {
		for _, column := range []string{"a", "b", "c", "d"} {
			value, err := session.cellValue(ctx, "t", column, rowID)
			must(err)
			c.cells = append(c.cells, value)
		}
	}
	for _, column := range []string{"a", "b", "c", "d", "missing"} {
		declared, err := session.columnDeclaredType(ctx, "t", column)
		must(err)
		c.declared = append(c.declared, declared)
	}
	var csv bytes.Buffer
	must(session.exportTableCSV(ctx, "t", &csv))
	c.csv = csv.String()
	// A real typed into the SQL box comes back as the text sqlite3 prints,
	// and some versions round the last digit on the way, so this leaves
	// the reals out; f4's own reads get them exactly (realBitsSQL).
	c.query, err = session.execute(ctx, "SELECT a, b, c, a IS NULL AS \"x, y\" FROM t ORDER BY rowid")
	must(err)
	if _, err := session.execute(ctx, "SELECT * FROM nosuch"); err != nil {
		c.queryErr = err.Error()
	}

	first := c.browse.rowIDs[0]
	c.updated, err = session.updateCell(ctx, "t", "a", first, "42")
	must(err)
	second := c.browse.rowIDs[1]
	_, err = session.updateCell(ctx, "t", "b", second, "O'Brien -- ; DROP TABLE t")
	must(err)
	_, err = session.updateCell(ctx, "t", "a", second, "тест")
	must(err)
	for _, rowID := range []int64{first, second} {
		for _, column := range []string{"a", "b"} {
			value, err := session.cellValue(ctx, "t", column, rowID)
			must(err)
			c.afterUpdate = append(c.afterUpdate, value)
		}
	}
	c.deleted, err = session.deleteRow(ctx, "t", c.browse.rowIDs[2])
	must(err)
	c.insertedID, err = session.insertRow(ctx, `odd "name"`)
	must(err)
	c.oddBrowse, err = session.browseTable(ctx, `odd "name"`, 0)
	must(err)
	result, err := session.execute(ctx, "UPDATE t SET d = 0 WHERE d IS NULL OR d < 2 -- trailing comment")
	must(err)
	c.execChanges = result.RowsAffected
	return c
}

func requireSQLite3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 command-line tool not available")
	}
}

func TestCLIBackendMatchesDriverBackend(t *testing.T) {
	requireSQLite3(t)
	want := collectConformance(t, openDefaultBackend)
	got := collectConformance(t, func(ctx context.Context, path string) (sessionBackend, error) {
		return openCLIBackend(ctx, path)
	})

	if !reflect.DeepEqual(got.tables, want.tables) {
		t.Errorf("tables = %q, want %q", got.tables, want.tables)
	}
	if got.count != want.count {
		t.Errorf("count = %d, want %d", got.count, want.count)
	}
	for _, pair := range []struct {
		name      string
		got, want tableBrowse
	}{
		{"table", got.browse, want.browse},
		{"view", got.viewBrowse, want.viewBrowse},
		{"without rowid", got.noRowID, want.noRowID},
		{"quoted names after insert", got.oddBrowse, want.oddBrowse},
	} {
		if !reflect.DeepEqual(pair.got, pair.want) {
			t.Errorf("%s browse =\n\t%#v\nwant\n\t%#v", pair.name, pair.got, pair.want)
		}
	}
	if !reflect.DeepEqual(got.cells, want.cells) {
		t.Errorf("cells = %#v, want %#v", got.cells, want.cells)
	}
	if !reflect.DeepEqual(got.declared, want.declared) {
		t.Errorf("declared types = %q, want %q", got.declared, want.declared)
	}
	if got.csv != want.csv {
		t.Errorf("CSV =\n%q\nwant\n%q", got.csv, want.csv)
	}
	if !reflect.DeepEqual(got.query, want.query) {
		t.Errorf("query = %#v, want %#v", got.query, want.query)
	}
	if !strings.Contains(got.queryErr, "no such table: nosuch") || !strings.Contains(want.queryErr, "no such table: nosuch") {
		t.Errorf("query error = %q, driver said %q", got.queryErr, want.queryErr)
	}
	if got.updated != want.updated || got.deleted != want.deleted || got.insertedID != want.insertedID || got.execChanges != want.execChanges {
		t.Errorf("counts = %d/%d/%d/%d, want %d/%d/%d/%d",
			got.updated, got.deleted, got.insertedID, got.execChanges,
			want.updated, want.deleted, want.insertedID, want.execChanges)
	}
	if !reflect.DeepEqual(got.afterUpdate, want.afterUpdate) {
		t.Errorf("after update = %#v, want %#v", got.afterUpdate, want.afterUpdate)
	}
}

// A statement typed into the SQL box that returns no rows has no header
// to read columns from; the client shows an empty result, not an error.
func TestCLIBackendEmptyResult(t *testing.T) {
	requireSQLite3(t)
	backend, err := openCLIBackend(context.Background(), createConformanceDB(t))
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.query(context.Background(), "SELECT * FROM t WHERE 0")
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReturnsRows || len(result.Rows) != 0 {
		t.Errorf("empty result = %#v", result)
	}
}

func TestCLIBackendWithoutSQLite3(t *testing.T) {
	previous := sqlite3LookPath
	sqlite3LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { sqlite3LookPath = previous }()

	if _, err := openCLIBackend(context.Background(), "whatever.sqlite"); !errors.Is(err, errNoSQLite3) {
		t.Fatalf("openCLIBackend without sqlite3 = %v, want errNoSQLite3", err)
	}
}

// A cancelled context has to stop every operation before it runs its
// sqlite3 process, not just the ones a test happened to exercise before.
func TestCLIBackendCancelledContextFailsEveryOperation(t *testing.T) {
	requireSQLite3(t)
	backend, err := openCLIBackend(context.Background(), createConformanceDB(t))
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	for name, call := range map[string]func() error{
		"listTables":         func() error { _, err := backend.listTables(cancelled); return err },
		"countRows":          func() error { _, err := backend.countRows(cancelled, "t"); return err },
		"columnDeclaredType": func() error { _, err := backend.columnDeclaredType(cancelled, "t", "a"); return err },
		"browseWithRowIDs":   func() error { _, _, err := backend.browseWithRowIDs(cancelled, "t", 0); return err },
		"browse":             func() error { _, err := backend.browse(cancelled, "v", 0); return err },
		"cellValue":          func() error { _, err := backend.cellValue(cancelled, "t", "a", 1); return err },
		"updateCell":         func() error { _, err := backend.updateCell(cancelled, "t", "a", 1, "1"); return err },
		"deleteRow":          func() error { _, err := backend.deleteRow(cancelled, "t", 1); return err },
		"insertRow":          func() error { _, err := backend.insertRow(cancelled, "t"); return err },
		"query":              func() error { _, err := backend.query(cancelled, "SELECT 1"); return err },
		"exec":               func() error { _, err := backend.exec(cancelled, "SELECT 1"); return err },
		"scanTable": func() error {
			return backend.scanTable(cancelled, "t", func([]string) error { return nil }, func([]any) error { return nil })
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s on a cancelled context returned no error", name)
		}
	}
}

// A table that is not there answers the same way as one with no columns of
// its own: PRAGMA table_info of a missing table is empty rows, not an error.
func TestCLIBackendMissingTableFailsCleanly(t *testing.T) {
	requireSQLite3(t)
	backend, err := openCLIBackend(context.Background(), createConformanceDB(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err := backend.browseWithRowIDs(ctx, "no_such_table", 0); err == nil {
		t.Error("browseWithRowIDs on a missing table returned no error")
	}
	if _, err := backend.browse(ctx, "no_such_table", 0); err == nil {
		t.Error("browse on a missing table returned no error")
	}
	if err := backend.scanTable(ctx, "no_such_table", func([]string) error { return nil }, func([]any) error { return nil }); err == nil {
		t.Error("scanTable on a missing table returned no error")
	}
}

// cellValue on a rowid nothing holds is not a driver error -- database/sql
// itself calls this "no rows in result set" -- so the CLI backend has to
// spell it out the same way rather than returning a zero value.
func TestCLIBackendCellValueOnMissingRow(t *testing.T) {
	requireSQLite3(t)
	backend, err := openCLIBackend(context.Background(), createConformanceDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.cellValue(context.Background(), "t", "a", 999999); err == nil {
		t.Error("cellValue on a missing row returned no error")
	}
}

// scanTable must stop as soon as either callback says so, and never start
// reading rows if the columns callback already failed.
func TestCLIBackendScanTableStopsOnCallbackError(t *testing.T) {
	requireSQLite3(t)
	backend, err := openCLIBackend(context.Background(), createConformanceDB(t))
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")

	if err := backend.scanTable(context.Background(), "t",
		func([]string) error { return boom },
		func([]any) error {
			t.Fatal("row callback ran after the columns callback had already failed")
			return nil
		}); !errors.Is(err, boom) {
		t.Errorf("scanTable columns error = %v, want %v", err, boom)
	}

	rows := 0
	if err := backend.scanTable(context.Background(), "t",
		func([]string) error { return nil },
		func([]any) error { rows++; return boom }); !errors.Is(err, boom) {
		t.Errorf("scanTable row error = %v, want %v", err, boom)
	}
	if rows != 1 {
		t.Errorf("row callback ran %d times after failing, want exactly 1", rows)
	}
}

// browse is what a view or a WITHOUT ROWID table pages through; past the
// first page it has to add its own OFFSET, unlike browseWithRowIDs' query
// which always carries one.
func TestCLIBackendBrowsePastTheFirstPage(t *testing.T) {
	requireSQLite3(t)
	path := filepath.Join(t.TempDir(), "paged.sqlite")
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE w (k TEXT PRIMARY KEY, n INT) WITHOUT ROWID`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < browsePageSize+5; i++ {
		if _, err := db.Exec(`INSERT INTO w VALUES (?, ?)`, fmt.Sprintf("k%03d", i), i); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	backend, err := openCLIBackend(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.browse(context.Background(), "w", browsePageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 5 {
		t.Fatalf("rows past the first page = %d, want 5", len(result.Rows))
	}
	if want := fmt.Sprintf("k%03d", browsePageSize); result.Rows[0][0] != want {
		t.Errorf("first row past the offset = %q, want %q", result.Rows[0][0], want)
	}
}
