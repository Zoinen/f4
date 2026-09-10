package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestDisplayValueFormatsSQLiteValuesAndBoundsOutput(t *testing.T) {
	stamp := time.Date(2026, time.January, 2, 3, 4, 5, 678000000, time.UTC)
	long := strings.Repeat("x", 513)
	for _, tc := range []struct {
		name, want string
		value      any
	}{
		{"NULL", "NULL", nil},
		{"binary", "x'0001ff'", []byte{0, 1, 255}},
		{"time", stamp.Format(time.RFC3339Nano), stamp},
		{"invalid UTF-8", "x'fffe'", string([]byte{0xff, 0xfe})},
		{"escaped text", "line\\r\\nnext\\tend", "line\r\nnext\tend"},
		{"ordinary text", "hello", "hello"},
		{"number", "42", int64(42)},
		{"truncated", strings.Repeat("x", 511) + "\u2026", long},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := displayValue(tc.value); got != tc.want {
				t.Fatalf("displayValue(%#v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

type sqliteSelectedApp struct {
	sqliteTestApp
	selected string
}

func (a *sqliteSelectedApp) GetSelectedName() string { return a.selected }

func TestSelectedSQLitePathRejectsUnsafeSelections(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if err := fs.SetPath(dir); err != nil {
		t.Fatal(err)
	}
	app := &sqliteSelectedApp{sqliteTestApp: sqliteTestApp{fs: fs}, selected: "data.DB"}
	want := filepath.Join(dir, "data.DB")
	if got, ok := selectedSQLitePath(app); !ok || got != want {
		t.Fatalf("selectedSQLitePath = %q, %t; want %q, true", got, ok, want)
	}
	for _, name := range []string{"", "..", "notes.txt", "folder.db/child"} {
		app.selected = name
		if got, ok := selectedSQLitePath(app); ok || got != "" {
			t.Errorf("selectedSQLitePath(%q) = %q, %t; want empty, false", name, got, ok)
		}
	}
	if got, ok := selectedSQLitePath(nil); ok || got != "" {
		t.Fatalf("selectedSQLitePath(nil) = %q, %t; want empty, false", got, ok)
	}
}

func TestBrowserStateEdgesAndF4Gesture(t *testing.T) {
	b, app, fm := newSQLiteBrowserTest(t, true)
	if got := b.browseStatus("notes", tableBrowse{}); got == "" {
		t.Fatal("browseStatus returned an empty status for an empty result")
	}

	if column, rowID, ok := b.cellUnderCursor(); !ok || column != "id" || rowID != 1 {
		t.Fatalf("cellUnderCursor() = %q, %d, %t; want id, 1, true", column, rowID, ok)
	}
	b.resultTable.SelectPos = -1
	if _, _, ok := b.cellUnderCursor(); ok {
		t.Fatal("cellUnderCursor accepted a negative row")
	}
	b.resultTable.SelectPos = 0
	b.writable = false
	if _, _, ok := b.cellUnderCursor(); ok {
		t.Fatal("cellUnderCursor accepted a read-only result")
	}
	b.writable = true

	before := app.progressCalls
	b.turnPage(-browsePageSize)
	b.turnPage(browsePageSize)
	if app.progressCalls != before {
		t.Fatalf("turnPage started work at both page boundaries: %d calls, want %d", app.progressCalls, before)
	}
	b.closed = true
	b.turnPage(browsePageSize)
	b.closed = false

	b.dialog.SetFocusedItem(b.resultTable)
	if !b.frame.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_F4}) {
		t.Fatal("F4 was not handled")
	}
	if fm.GetTopFrame() == b.frame {
		t.Fatal("F4 did not open the cell editor")
	}
	dismissSQLiteTopFrame(fm)
}

func TestBrowserRefreshesAnEmptySchema(t *testing.T) {
	oldFrameManager := vtui.FrameManager
	fm := vtui.NewFrameManager()
	fm.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager = fm

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.sqlite")
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	session, tables, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	app := &sqliteBrowserApp{sqliteTestApp: sqliteTestApp{fs: vfs.NewOSVFS(dir)}}
	b := newBrowser(app, session, tables)
	fm.Push(b.frame)
	t.Cleanup(func() {
		if fm.GetTopFrame() == b.frame {
			b.dialog.Close()
			fm.Pop()
		}
		fm.Shutdown()
		vtui.FrameManager = oldFrameManager
	})
	if b.currentTable != "" || len(b.tables) != 0 {
		t.Fatalf("fresh browser state = table %q, tables %#v", b.currentTable, b.tables)
	}

	if _, err := session.execute(context.Background(), "CREATE TABLE created (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	b.refresh()
	if b.currentTable != "created" || !containsString(b.tables, "created") {
		t.Fatalf("schema after CREATE = table %q, tables %#v", b.currentTable, b.tables)
	}
	if _, err := session.execute(context.Background(), "DROP TABLE created"); err != nil {
		t.Fatal(err)
	}
	b.refresh()
	if b.currentTable != "" || len(b.tables) != 0 || len(b.resultTable.Rows) != 0 {
		t.Fatalf("schema after DROP = table %q, tables %#v, rows %d", b.currentTable, b.tables, len(b.resultTable.Rows))
	}
}

func TestPluginCloseIsIdempotent(t *testing.T) {
	plugin := NewPlugin()
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
}
