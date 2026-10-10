package sqlite

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func createTestDatabase(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// mountTestDatabase creates a database and enters it the way Enter on the
// file does.
func mountTestDatabase(t *testing.T, statements ...string) (*databaseVFS, *vfs.OSVFS, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	createTestDatabase(t, path, statements...)
	parent := vfs.NewOSVFS(dir)
	opened, err := (&databaseProvider{}).Open(context.Background(), parent, path)
	if err != nil {
		t.Fatal(err)
	}
	mounted, ok := opened.(*databaseVFS)
	if !ok {
		t.Fatalf("provider opened %T, want *databaseVFS", opened)
	}
	t.Cleanup(func() { _ = mounted.Close() })
	return mounted, parent, path
}

func TestPluginRegistersTheDatabaseProvider(t *testing.T) {
	host := &sqliteTestHost{}
	plugin := NewPlugin()
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if _, ok := host.provider.(*databaseProvider); !ok {
		t.Fatalf("registered provider = %T, want *databaseProvider", host.provider)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
}

// The file is a database because of what is in it, not because of its name:
// the issue asks for SQLite to be recognized by its format.
func TestProviderRecognizesADatabaseByItsContent(t *testing.T) {
	dir := t.TempDir()
	parent := vfs.NewOSVFS(dir)
	database := filepath.Join(dir, "settings.bin")
	createTestDatabase(t, database, "CREATE TABLE t (id INTEGER)")
	impostor := filepath.Join(dir, "notes.db")
	if err := os.WriteFile(impostor, []byte("this is not a database at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "folder.db")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}

	provider := &databaseProvider{}
	ctx := context.Background()
	if !provider.CanOpen(ctx, parent, database) {
		t.Fatal("a database without a database extension was not recognized")
	}
	for _, path := range []string{impostor, empty, folder} {
		if provider.CanOpen(ctx, parent, path) {
			t.Errorf("CanOpen(%s) = true for something that is not a database", filepath.Base(path))
		}
	}
	if provider.CanOpen(ctx, vfs.NewNullVFS(0), database) {
		t.Error("CanOpen accepted a file outside the local file system")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if provider.CanOpen(cancelled, parent, database) {
		t.Error("CanOpen answered on a cancelled context")
	}
	if _, err := provider.Open(ctx, parent, impostor); err == nil {
		t.Error("Open mounted a file that is not a database")
	}
}

func TestDatabaseVFSListsTablesAndViewsAsFiles(t *testing.T) {
	mounted, parent, path := mountTestDatabase(t,
		"CREATE TABLE people (id INTEGER PRIMARY KEY, name TEXT)",
		"CREATE VIEW adults AS SELECT * FROM people",
		`CREATE TABLE "a/b" (x)`,
		`CREATE TABLE ".." (x)`,
		`CREATE TABLE "old.people" (x)`,
	)
	if !mounted.IsAtRoot() || mounted.GetPath() != path || mounted.ParentVFS() != vfs.VFS(parent) {
		t.Fatalf("mount = root %t, path %q, parent %v", mounted.IsAtRoot(), mounted.GetPath(), mounted.ParentVFS())
	}

	var items []vfs.VFSItem
	if err := mounted.ReadDir(context.Background(), mounted.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	want := []struct{ file, table string }{
		{"%2E%2E", ".."},
		{"a%2Fb", "a/b"},
		{"adults", "adults"},
		{"old.people", "old.people"},
		{"people", "people"},
	}
	if len(items) != len(want) {
		t.Fatalf("listing = %#v, want %d entries", items, len(want))
	}
	for i, item := range items {
		if item.Name != want[i].file || item.IsDir || !item.NoExtension {
			t.Errorf("entry %d = %q (dir %t, no extension %t), want file %q", i, item.Name, item.IsDir, item.NoExtension, want[i].file)
		}
		entry := mounted.Join(mounted.GetPath(), item.Name)
		if table, ok := mounted.tableOf(entry); !ok || table != want[i].table {
			t.Errorf("tableOf(%q) = %q, %t; want %q", item.Name, table, ok, want[i].table)
		}
		if mounted.Dir(entry) != path || mounted.Base(entry) != item.Name {
			t.Errorf("%q: Dir %q, Base %q", item.Name, mounted.Dir(entry), mounted.Base(entry))
		}
		if _, err := mounted.Stat(context.Background(), entry); err != nil {
			t.Errorf("Stat(%q): %v", item.Name, err)
		}
	}

	// The title names what opened the file, not where it lies (#1383).
	if got := mounted.PanelTitle(mounted.GetPath()); got != "SQLite:test.db" {
		t.Errorf("panel title = %q, want SQLite:test.db", got)
	}
	if got := mounted.PanelTitle(filepath.Dir(path)); got != "" {
		t.Errorf("a path outside the database got the title %q", got)
	}

	root, err := mounted.Stat(context.Background(), path)
	if err != nil || !root.IsDir {
		t.Fatalf("Stat(root) = %#v, %v", root, err)
	}
	if _, err := mounted.Stat(context.Background(), mounted.Join(path, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(missing) error = %v, want os.ErrNotExist", err)
	}
	if err := mounted.SetPath(path); err != nil {
		t.Fatalf("SetPath(root): %v", err)
	}
	if err := mounted.SetPath(mounted.Join(path, "people")); err == nil {
		t.Fatal("SetPath went into a table as if it were a folder")
	}
}

func TestDatabaseVFSReadsATableAsCSV(t *testing.T) {
	mounted, _, path := mountTestDatabase(t,
		"CREATE TABLE notes (id INTEGER, note TEXT, payload BLOB)",
		`INSERT INTO notes VALUES (1, 'plain', NULL), (2, 'comma, "quote"' || char(10) || 'line', x'00ff')`,
	)
	reader, err := mounted.Open(context.Background(), mounted.Join(path, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	wrapper, ok := reader.(*vfs.TempFileWrapper)
	if !ok {
		_ = reader.Close()
		t.Fatalf("Open returned %T", reader)
	}
	data := make([]byte, reader.Size())
	if n, err := reader.ReadAt(context.Background(), data, 0); err != nil && !errors.Is(err, io.EOF) || n != len(data) {
		_ = reader.Close()
		t.Fatalf("ReadAt = %d, %v; size %d", n, err, len(data))
	}
	want := "id,note,payload\n" +
		"1,plain,\n" +
		"2,\"comma, \"\"quote\"\"\nline\",x'00ff'\n"
	if string(data) != want {
		t.Errorf("CSV =\n%s\nwant\n%s", data, want)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wrapper.TempPath); !os.IsNotExist(err) {
		t.Fatalf("temporary CSV %s is still there after Close: %v", wrapper.TempPath, err)
	}

	if _, err := mounted.Open(context.Background(), mounted.Join(path, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open(missing) error = %v, want os.ErrNotExist", err)
	}
}

func TestDatabaseVFSCopiesATableOutAsACSVFile(t *testing.T) {
	mounted, _, path := mountTestDatabase(t, "CREATE TABLE people (id INTEGER)")
	if got := mounted.TransferName(mounted.Join(path, "people"), nil); got != "people.csv" {
		t.Fatalf("TransferName(table) = %q, want people.csv", got)
	}
	if got := mounted.TransferName(path, nil); got != "" {
		t.Fatalf("TransferName(database) = %q, want the host's default", got)
	}
}

func TestDatabaseVFSRefusesChangesMadeThroughThePanel(t *testing.T) {
	mounted, _, path := mountTestDatabase(t, "CREATE TABLE people (id INTEGER)")
	ctx := context.Background()
	table := mounted.Join(path, "people")
	_, createErr := mounted.Create(ctx, mounted.Join(path, "new"))
	for name, err := range map[string]error{
		"Create": createErr,
		"Remove": mounted.Remove(ctx, table),
		"MkDir":  mounted.MkDir(ctx, mounted.Join(path, "dir")),
		"Rename": mounted.Rename(ctx, table, mounted.Join(path, "other")),
	} {
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("%s error = %v, want a read-only refusal", name, err)
		}
	}
	if mounted.GetCapabilities().HasWrite {
		t.Error("the database panel claims to be writable")
	}
}

func TestTableFileNamesRoundTrip(t *testing.T) {
	for _, table := range []string{"people", "a/b", `a\b`, "100%", "tab\there", ".", "..", "...", "old.people", "Пользователи"} {
		name, ok := tableFileName(table)
		if !ok {
			t.Errorf("tableFileName(%q) refused", table)
			continue
		}
		back, ok := tableNameFromFile(name)
		if !ok || back != table {
			t.Errorf("%q -> %q -> %q, %t", table, name, back, ok)
		}
	}
	if _, ok := tableFileName(""); ok {
		t.Error("an empty table name got a file name")
	}
	for _, name := range []string{"", ".", "..", "%2", "%zz", "50%"} {
		if table, ok := tableNameFromFile(name); ok {
			t.Errorf("tableNameFromFile(%q) = %q, want no table", name, table)
		}
	}
}

// sqliteVFSApp is the panel the database is mounted in.
type sqliteVFSApp struct {
	sqliteBrowserApp
	selected  string
	refreshed int
}

func (a *sqliteVFSApp) GetSelectedName() string { return a.selected }
func (a *sqliteVFSApp) RefreshAll()             { a.refreshed++ }

// Enter on a table opens the client on that table: the editor sits on top of
// the database panel. Closing it reads the panel again, since the SQL box can
// create and drop tables.
func TestEnterOnATableOpensTheClientOnIt(t *testing.T) {
	oldFrameManager := vtui.FrameManager
	fm := vtui.NewFrameManager()
	fm.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager = fm
	t.Cleanup(func() {
		for fm.GetTopFrame() != nil {
			fm.GetTopFrame().SetExitCode(-1)
			fm.Pop()
		}
		fm.Shutdown()
		vtui.FrameManager = oldFrameManager
	})

	mounted, _, _ := mountTestDatabase(t,
		"CREATE TABLE alpha (id INTEGER)",
		"CREATE TABLE beta (id INTEGER)",
		"INSERT INTO beta VALUES (7)",
	)
	app := &sqliteVFSApp{selected: "beta"}

	before := fm.GetTopFrame()
	app.selected = ".."
	if mounted.HandlePanelAction(app, vfs.PanelActionActivate, nil) || fm.GetTopFrame() != before {
		t.Fatal("Enter on \"..\" did not leave the way out of the database to the panel")
	}
	app.selected = "beta"
	if mounted.HandlePanelAction(app, vfs.PanelActionDelete, nil) {
		t.Fatal("F8 was taken over; it belongs to the panel")
	}

	for _, action := range []vfs.PanelAction{vfs.PanelActionActivate, vfs.PanelActionEdit} {
		refreshed := app.refreshed
		if !mounted.HandlePanelAction(app, action, nil) {
			t.Fatalf("action %d on a table was not handled", action)
		}
		window, ok := fm.GetTopFrame().(*browserWindow)
		if !ok {
			t.Fatalf("action %d opened %T, want the SQLite client", action, fm.GetTopFrame())
		}
		if window.browser.currentTable != "beta" || len(window.browser.resultTable.Rows) != 1 {
			t.Fatalf("client shows table %q with %d rows, want beta with 1", window.browser.currentTable, len(window.browser.resultTable.Rows))
		}
		window.browser.dialog.Close()
		fm.Pop()
		if app.refreshed != refreshed+1 {
			t.Fatalf("closing the client refreshed the panel %d times, want once", app.refreshed-refreshed)
		}
	}
}
