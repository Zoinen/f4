package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/unxed/f4/vfs"
)

func TestDatabaseSessionReadsAndWritesSQLiteValues(t *testing.T) {
	path := t.TempDir() + "/sample.db"
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE "odd""name" (id INTEGER, note TEXT, payload BLOB)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "odd""name" VALUES (1, NULL, ?)`, []byte{0, 1, 255}); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	session, tables, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if !reflect.DeepEqual(tables, []string{`odd"name`}) {
		t.Fatalf("tables = %#v, want [odd\\\"name]", tables)
	}

	result, err := session.execute(context.Background(), tableSelect(`odd"name`, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReturnsRows || !reflect.DeepEqual(result.Columns, []string{"id", "note", "payload"}) {
		t.Fatalf("query metadata = %#v", result)
	}
	wantRows := [][]string{{"1", "NULL", "x'0001ff'"}}
	if !reflect.DeepEqual(result.Rows, wantRows) {
		t.Fatalf("rows = %#v, want %#v", result.Rows, wantRows)
	}

	result, err = session.execute(context.Background(), `UPDATE "odd""name" SET note = 'changed' WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReturnsRows || result.RowsAffected != 1 {
		t.Fatalf("write result = %#v", result)
	}
}

func TestSQLiteSQLClassificationAndIdentifierQuoting(t *testing.T) {
	if got, want := tableSelect(`a"b`, 0), `SELECT * FROM "a""b" LIMIT 100`; got != want {
		t.Fatalf("tableSelect = %q, want %q", got, want)
	}
	for _, statement := range []string{
		"-- explain the query\n SELECT 1",
		"/* leading comment */ PRAGMA user_version",
		"WITH rows AS (SELECT 1) SELECT * FROM rows",
	} {
		if !statementReturnsRows(statement) {
			t.Errorf("statementReturnsRows(%q) = false", statement)
		}
	}
	for _, statement := range []string{"CREATE TABLE x (id INTEGER)", "UPDATE x SET id = 1"} {
		if statementReturnsRows(statement) {
			t.Errorf("statementReturnsRows(%q) = true", statement)
		}
	}
}

type sqliteTestRegistration struct{}

func (*sqliteTestRegistration) Unregister() {}

type sqliteTestHost struct {
	vfs.HostAPI
	vfs.ContributionHost
	command vfs.PluginCommand
}

func (host *sqliteTestHost) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.command = command
	return &sqliteTestRegistration{}, nil
}

func TestPluginRegistersLocalizedPanelCommand(t *testing.T) {
	host := &sqliteTestHost{}
	plugin := NewPlugin()
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.command.ID != sqliteCommandID || host.command.Location != vfs.PluginCommandPanel || host.command.Run == nil {
		t.Fatalf("command metadata = %#v", host.command)
	}
	// The main-menu row belongs to the host action App.SQLite, and the
	// command is offered wherever the cursor happens to be: a predicate on
	// the panel selection took the entry out of the menus that lead to it.
	if host.command.MenuPath != "" || host.command.Visible != nil {
		t.Fatalf("command hides itself: MenuPath = %q, Visible set = %t", host.command.MenuPath, host.command.Visible != nil)
	}
	if host.command.Label != "SQLite client" || host.command.LabelKey != "SQLite.Command.Open" || host.command.DescriptionKey != "SQLite.Command.Open.Desc" {
		t.Fatalf("localization metadata = %#v", host.command)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDatabasePathIsResolvedAgainstThePanel(t *testing.T) {
	dir := t.TempDir()
	app := &sqliteTestApp{fs: vfs.NewOSVFS(dir)}
	if err := app.fs.SetPath(dir); err != nil {
		t.Fatal(err)
	}

	got := databasePathIn(app, "new.sqlite")
	if want := filepath.Join(dir, "new.sqlite"); got != want {
		t.Fatalf("databasePathIn = %q, want %q", got, want)
	}

	absolute := filepath.Join(dir, "elsewhere.sqlite")
	if got := databasePathIn(app, absolute); got != absolute {
		t.Fatalf("databasePathIn(absolute) = %q, want %q", got, absolute)
	}

	// A database the client created is a database it can open again.
	session, tables, err := openDatabase(context.Background(), filepath.Join(dir, "new.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if len(tables) != 0 {
		t.Fatalf("a fresh database reported tables: %#v", tables)
	}
	if _, err := session.execute(context.Background(), `CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	tables, err = session.listTables(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0] != "t" {
		t.Fatalf("tables after CREATE TABLE = %#v", tables)
	}
}

// sqliteTestApp is the slice of vfs.App this plugin reads: the active panel.
type sqliteTestApp struct {
	vfs.App
	fs *vfs.OSVFS
}

func (a *sqliteTestApp) GetActivePanelVFS() vfs.VFS { return a.fs }

func TestBrowseTableCarriesRowIDsAndEditsACell(t *testing.T) {
	path := t.TempDir() + "/edit.db"
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, note TEXT, size INTEGER)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO notes VALUES (1, 'first', 10), (2, NULL, 20)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE VIEW big AS SELECT * FROM notes WHERE size > 5`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	session, _, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	browse, err := session.browseTable(context.Background(), "notes", 0)
	if err != nil {
		t.Fatal(err)
	}
	// The rowid alias is the browser's business and never reaches the screen.
	if !reflect.DeepEqual(browse.result.Columns, []string{"id", "note", "size"}) {
		t.Fatalf("columns = %#v", browse.result.Columns)
	}
	if !reflect.DeepEqual(browse.rowIDs, []int64{1, 2}) || !browse.writable {
		t.Fatalf("rowIDs = %#v, writable = %t", browse.rowIDs, browse.writable)
	}

	// A view has no rowid: it comes back readable and unwritable.
	view, err := session.browseTable(context.Background(), "big", 0)
	if err != nil {
		t.Fatal(err)
	}
	if view.rowIDs != nil || view.writable || len(view.result.Rows) != 2 {
		t.Fatalf("view browse = %d row(s), rowIDs %#v, writable %t", len(view.result.Rows), view.rowIDs, view.writable)
	}

	affected, err := session.updateCell(context.Background(), "notes", "note", 2, "second")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("updateCell affected %d rows, want 1", affected)
	}
	value, err := session.cellValue(context.Background(), "notes", "note", 2)
	if err != nil {
		t.Fatal(err)
	}
	if text, ok := editableText(value); !ok || text != "second" {
		t.Fatalf("cell after the edit = %q (editable %t), want %q", text, ok, "second")
	}

	// Column affinity turns a typed number back into one.
	if _, err := session.updateCell(context.Background(), "notes", "size", 2, "42"); err != nil {
		t.Fatal(err)
	}
	value, err = session.cellValue(context.Background(), "notes", "size", 2)
	if err != nil {
		t.Fatal(err)
	}
	if number, ok := value.(int64); !ok || number != 42 {
		t.Fatalf("size after the edit = %#v, want int64(42)", value)
	}
}

func TestEditableTextRefusesWhatALineBoxWouldCorrupt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    any
		want     string
		editable bool
	}{
		{"NULL edits as an empty line", nil, "", true},
		{"text passes through", "hello", "hello", true},
		{"a number is written out", int64(42), "42", true},
		{"binary is refused", []byte{0, 1, 255}, "", false},
		{"line breaks are refused", "two\nlines", "", false},
	} {
		got, editable := editableText(tc.value)
		if got != tc.want || editable != tc.editable {
			t.Errorf("%s: editableText(%#v) = %q, %t; want %q, %t", tc.name, tc.value, got, editable, tc.want, tc.editable)
		}
	}
}

func TestInsertRowAddsADefaultRowAndReportsRefusals(t *testing.T) {
	path := t.TempDir() + "/insert.db"
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE strict (id INTEGER PRIMARY KEY, note TEXT NOT NULL)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	session, _, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	// An empty table is still writable: that is where a first row is wanted.
	empty, err := session.browseTable(context.Background(), "notes", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !empty.writable || len(empty.rowIDs) != 0 || len(empty.result.Rows) != 0 {
		t.Fatalf("empty browse = %d row(s), rowIDs %#v, writable %t", len(empty.result.Rows), empty.rowIDs, empty.writable)
	}

	rowID, err := session.insertRow(context.Background(), "notes")
	if err != nil {
		t.Fatal(err)
	}
	filled, err := session.browseTable(context.Background(), "notes", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(filled.rowIDs, []int64{rowID}) {
		t.Fatalf("rowIDs after the insert = %#v, want [%d]", filled.rowIDs, rowID)
	}
	value, err := session.cellValue(context.Background(), "notes", "note", rowID)
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Fatalf("the new row's note = %#v, want NULL", value)
	}

	// NOT NULL without a default cannot take a row of defaults, and the error
	// is what the user is shown instead of a guess at the value.
	if _, err := session.insertRow(context.Background(), "strict"); err == nil {
		t.Fatal("a NOT NULL column accepted a default row")
	}
}

func TestDeleteRowRemovesOneRowByRowID(t *testing.T) {
	path := t.TempDir() + "/delete.db"
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO notes VALUES (1, 'first'), (2, 'second')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	session, _, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	affected, err := session.deleteRow(context.Background(), "notes", 1)
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("deleteRow affected %d rows, want 1", affected)
	}
	browse, err := session.browseTable(context.Background(), "notes", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(browse.rowIDs, []int64{2}) {
		t.Fatalf("rowIDs after the delete = %#v, want [2]", browse.rowIDs)
	}

	// Deleting the same row again is not an error, it is a row that is gone.
	affected, err = session.deleteRow(context.Background(), "notes", 1)
	if err != nil {
		t.Fatal(err)
	}
	if affected != 0 {
		t.Fatalf("deleting a missing row affected %d rows, want 0", affected)
	}
}

func TestBrowsePagesAreStableAndClamped(t *testing.T) {
	path := t.TempDir() + "/pages.db"
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE numbers (value INTEGER)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	for i := 1; i <= 150; i++ {
		if _, err := db.Exec(`INSERT INTO numbers VALUES (?)`, i); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	session, _, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	first, err := session.browseTable(context.Background(), "numbers", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.result.Rows) != browsePageSize || first.offset != 0 || first.total != 150 {
		t.Fatalf("first page: %d rows, offset %d, total %d", len(first.result.Rows), first.offset, first.total)
	}

	second, err := session.browseTable(context.Background(), "numbers", browsePageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.result.Rows) != 50 || second.offset != browsePageSize {
		t.Fatalf("second page: %d rows at offset %d", len(second.result.Rows), second.offset)
	}
	// Pages are two halves of the same table, not two draws from it: ordered
	// by rowid, the second page starts where the first one stopped.
	if second.rowIDs[0] != first.rowIDs[browsePageSize-1]+1 {
		t.Fatalf("second page starts at rowid %d after a first page ending at %d", second.rowIDs[0], first.rowIDs[browsePageSize-1])
	}

	// An offset past the end lands on the last page that has rows.
	past, err := session.browseTable(context.Background(), "numbers", 100000)
	if err != nil {
		t.Fatal(err)
	}
	if past.offset != browsePageSize || len(past.result.Rows) != 50 {
		t.Fatalf("clamped page: %d rows at offset %d, want 50 at %d", len(past.result.Rows), past.offset, browsePageSize)
	}
}

func TestOffsetArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name          string
		offset, total int64
		want          int64
	}{
		{"empty table", 500, 0, 0},
		{"negative goes to the start", -100, 150, 0},
		{"inside stays put on its page", 100, 150, 100},
		{"past the end lands on the last page", 700, 150, 100},
		{"exactly full pages", 300, 300, 200},
	} {
		if got := clampOffset(tc.offset, tc.total); got != tc.want {
			t.Errorf("%s: clampOffset(%d, %d) = %d, want %d", tc.name, tc.offset, tc.total, got, tc.want)
		}
	}
	for _, tc := range []struct{ total, want int64 }{{0, 0}, {1, 0}, {100, 0}, {101, 100}, {250, 200}} {
		if got := lastPageOffset(tc.total); got != tc.want {
			t.Errorf("lastPageOffset(%d) = %d, want %d", tc.total, got, tc.want)
		}
	}
}

func TestTypeAffinityFollowsTheFiveRules(t *testing.T) {
	for _, tc := range []struct{ declared, want string }{
		{"INT", "INTEGER"},
		{"int", "INTEGER"},
		{"BIGINT", "INTEGER"},
		{"POINT", "INTEGER"}, // INT anywhere wins, per the documentation
		{"VARCHAR(80)", "TEXT"},
		{"clob", "TEXT"},
		{"TEXT", "TEXT"},
		{"", "BLOB"},
		{"BLOB", "BLOB"},
		{"REAL", "REAL"},
		{"DOUBLE PRECISION", "REAL"},
		{"FLOAT", "REAL"},
		{"DECIMAL(10,5)", "NUMERIC"},
		{"BOOLEAN", "NUMERIC"},
		{"DATE", "NUMERIC"},
	} {
		if got := typeAffinity(tc.declared); got != tc.want {
			t.Errorf("typeAffinity(%q) = %q, want %q", tc.declared, got, tc.want)
		}
	}
}

func TestStoredAsTextInsteadFlagsOnlyRealMismatches(t *testing.T) {
	for _, tc := range []struct {
		affinity, value string
		want            bool
	}{
		{"INTEGER", "42", false},
		{"INTEGER", " 42 ", false},
		{"INTEGER", "-4.2e1", false},
		{"INTEGER", "тест", true},
		{"INTEGER", "", true},
		{"INTEGER", "0x1A", true}, // SQLite does not convert hex text either
		{"REAL", "3.14", false},
		{"NUMERIC", "abc", true},
		{"TEXT", "тест", false},
		{"BLOB", "whatever", false},
	} {
		if got := storedAsTextInstead(tc.affinity, tc.value); got != tc.want {
			t.Errorf("storedAsTextInstead(%q, %q) = %t, want %t", tc.affinity, tc.value, got, tc.want)
		}
	}
}

func TestColumnDeclaredTypeReadsThePragma(t *testing.T) {
	path := t.TempDir() + "/types.db"
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE things (id int, note, "weird name" VARCHAR(5))`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	session, _, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	// The driver reports the declared type in its own casing -- "int" comes
	// back as "INT" -- and the casing is no part of what the feature needs:
	// affinity derivation uppercases whatever it is given. So the test asks
	// for the type, not for its spelling.
	for _, tc := range []struct{ column, want string }{
		{"id", "int"},
		{"ID", "int"}, // column names compare without case
		{"note", ""},
		{"weird name", "VARCHAR(5)"},
	} {
		declared, err := session.columnDeclaredType(context.Background(), "things", tc.column)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.EqualFold(declared, tc.want) {
			t.Errorf("columnDeclaredType(%q) = %q, want %q up to case", tc.column, declared, tc.want)
		}
	}
}
