// Package sqlite provides a small local SQLite browser/editor for f4.
package sqlite

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const sqliteCommandID = "f4.sqlite.open"

// Plugin exposes the SQLite browser/editor as an in-process f4 plugin.
type Plugin struct {
	mu           sync.Mutex
	registration vfs.Registration
	provider     *databaseProvider
	initialized  bool
}

// NewPlugin constructs the built-in SQLite plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return "SQLite" }

func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("SQLite: nil host API")
	}
	host, ok := api.(vfs.ContributionHost)
	if !ok {
		return errors.New("SQLite: host does not support plugin contributions")
	}

	p.mu.Lock()
	if p.initialized {
		p.mu.Unlock()
		return errors.New("SQLite: plugin is already initialized")
	}
	p.mu.Unlock()

	// No MenuPath and no Visible predicate.
	//
	// The main-menu row and the Ctrl+Alt+D binding belong to the host action
	// App.SQLite: a plugin command cannot own a hotkey, and two rows for one
	// command is one too many. The plugin menu and the command palette list
	// this registration as they always did.
	//
	// The predicate is gone because it asked what the panel cursor was on,
	// and the answer decided whether the command existed at all. Anywhere but
	// on a .db file it removed itself from every menu that could have led the
	// user to it. openCurrent already explains which files it takes, which is
	// the better place to say so.
	registration, err := host.RegisterPluginCommand(vfs.PluginCommand{
		ID:             sqliteCommandID,
		Location:       vfs.PluginCommandPanel,
		Label:          "SQLite client",
		LabelKey:       "SQLite.Command.Open",
		Description:    "Browse tables and execute SQL against a local SQLite database",
		DescriptionKey: "SQLite.Command.Open.Desc",
		Run:            p.openCurrent,
	})
	if err != nil {
		return fmt.Errorf("SQLite: register panel command: %w", err)
	}

	// Enter and Ctrl+PgDn on a database file mount it in the panel, the way
	// they mount an archive (#1268).
	provider := &databaseProvider{}
	api.RegisterVFSProvider(provider)

	p.mu.Lock()
	p.registration = registration
	p.provider = provider
	p.initialized = true
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	registration := p.registration
	provider := p.provider
	p.registration = nil
	p.provider = nil
	p.initialized = false
	p.mu.Unlock()
	if registration != nil {
		registration.Unregister()
	}
	if provider != nil {
		vfs.UnregisterProvider(provider)
	}
	return nil
}

func selectedSQLitePath(app vfs.App) (string, bool) {
	if app == nil {
		return "", false
	}
	fs, ok := app.GetActivePanelVFS().(*vfs.OSVFS)
	if !ok || fs == nil {
		return "", false
	}
	name := app.GetSelectedName()
	if name == "" || name == ".." || !isSQLiteFilename(name) {
		return "", false
	}
	path, err := fs.Abs(fs.Join(fs.GetPath(), name))
	if err != nil {
		return "", false
	}
	return filepath.Clean(path), true
}

func isSQLiteFilename(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".db", ".sqlite", ".sqlite3", ".db3":
		return true
	default:
		return false
	}
}

// databaseSession is one open database. What it reads and writes goes
// through a sessionBackend: the regular build links the SQLite engine
// (backend_driver.go), the lite build runs the sqlite3 command-line tool
// (backend_cli.go), and everything above the backend -- the client, the
// mounted-database panel, the CSV export -- is the same in both.
type databaseSession struct {
	backend   sessionBackend
	path      string
	closeOnce sync.Once
}

// sessionBackend is what a databaseSession needs from a database. Values
// come back typed the way database/sql scans them: nil, int64, float64,
// string or []byte.
type sessionBackend interface {
	close()
	listTables(ctx context.Context) ([]string, error)
	// exec runs a statement that returns no rows and reports how many rows
	// it changed.
	exec(ctx context.Context, statement string) (int64, error)
	// query runs a statement that returns rows, with the values as the grid
	// shows them.
	query(ctx context.Context, statement string) (queryResult, error)
	countRows(ctx context.Context, table string) (int64, error)
	insertRow(ctx context.Context, table string) (int64, error)
	// browseWithRowIDs reads one page of a table together with the rowid of
	// every row, and fails for a table that has no rowid.
	browseWithRowIDs(ctx context.Context, table string, offset int64) (queryResult, []int64, error)
	// browse reads one page of a table that has no rowid: a view, or a
	// WITHOUT ROWID table.
	browse(ctx context.Context, table string, offset int64) (queryResult, error)
	cellValue(ctx context.Context, table, column string, rowID int64) (any, error)
	updateCell(ctx context.Context, table, column string, rowID int64, value string) (int64, error)
	deleteRow(ctx context.Context, table string, rowID int64) (int64, error)
	columnDeclaredType(ctx context.Context, table, column string) (string, error)
	// scanTable reads a whole table: columns once, then row for every row.
	scanTable(ctx context.Context, table string, columns func([]string) error, row func([]any) error) error
}

// openSessionBackend opens the backend this build uses; tests replace it to
// run the same checks against the other one.
var openSessionBackend = openDefaultBackend

func openDatabase(ctx context.Context, path string) (*databaseSession, []string, error) {
	backend, err := openSessionBackend(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	session := &databaseSession{backend: backend, path: path}
	tables, err := session.listTables(ctx)
	if err != nil {
		session.Close()
		return nil, nil, err
	}
	return session, tables, nil
}

func (s *databaseSession) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.backend != nil {
			s.backend.close()
		}
	})
}

func (s *databaseSession) listTables(ctx context.Context) ([]string, error) {
	return s.backend.listTables(ctx)
}

type queryResult struct {
	Columns      []string
	Rows         [][]string
	RowsAffected int64
	ReturnsRows  bool
}

func (s *databaseSession) execute(ctx context.Context, statement string) (queryResult, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return queryResult{}, errors.New("SQL statement is empty")
	}
	if !statementReturnsRows(statement) {
		rowsAffected, err := s.backend.exec(ctx, statement)
		if err != nil {
			return queryResult{}, err
		}
		return queryResult{RowsAffected: rowsAffected}, nil
	}
	return s.backend.query(ctx, statement)
}

// listTablesSQL lists what the client and the database panel show: tables
// and views, without SQLite's own.
const listTablesSQL = `
		SELECT name
		FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name`

// rowIDColumn is the alias a table browse gives sqlite's rowid. It is dropped
// before the result is shown: the user sees the table's own columns, while the
// browser keeps the identifiers that make a cell writable.
const rowIDColumn = "_f4_rowid"

// browseTable reads a table for the panel on the right, together with the
// rowid of every row it returns.
//
// A view and a WITHOUT ROWID table have no rowid, and the query for one fails;
// that is not an error but the answer that this table can only be read, so the
// plain browse runs instead and the rowids come back nil.
func (s *databaseSession) browseTable(ctx context.Context, table string, offset int64) (tableBrowse, error) {
	total, err := s.countRows(ctx, table)
	if err != nil {
		return tableBrowse{}, err
	}
	offset = clampOffset(offset, total)

	result, rowIDs, err := s.browseWithRowIDs(ctx, table, offset)
	if err == nil {
		// Writable is reported separately from the rowids themselves: an
		// empty table also has none, and it is the one place a new row is
		// most likely to be wanted.
		return tableBrowse{result: result, rowIDs: rowIDs, writable: true, offset: offset, total: total}, nil
	}
	result, err = s.backend.browse(ctx, table, offset)
	return tableBrowse{result: result, offset: offset, total: total}, err
}

// countRows is what makes paging possible: the page to show has to be chosen
// against the number of rows there actually are.
func (s *databaseSession) countRows(ctx context.Context, table string) (int64, error) {
	return s.backend.countRows(ctx, table)
}

// lastPageOffset is where a row appended to the end of a table can be found.
func lastPageOffset(total int64) int64 {
	if total <= 0 {
		return 0
	}
	return ((total - 1) / browsePageSize) * browsePageSize
}

// tableBrowse is everything one reading of a table produces: the rows to show,
// the rowid each of them came from, and whether they can be written back.
type tableBrowse struct {
	result   queryResult
	rowIDs   []int64
	writable bool
	// offset is the page this reading covers, and total the number of rows in
	// the table it came from, so the client can say which hundred of how many
	// is on screen.
	offset int64
	total  int64
}

// insertRow adds a row of defaults, which is the part a dialog cannot do
// better: the columns are filled in afterwards with F4, one cell at a time,
// through the same path that edits an existing row.
//
// A table whose columns are NOT NULL without defaults refuses this, and
// SQLite names the column that refused; that message is worth showing rather
// than guessing at values on the user's behalf.
func (s *databaseSession) insertRow(ctx context.Context, table string) (int64, error) {
	return s.backend.insertRow(ctx, table)
}

func (s *databaseSession) browseWithRowIDs(ctx context.Context, table string, offset int64) (queryResult, []int64, error) {
	return s.backend.browseWithRowIDs(ctx, table, offset)
}

// cellValue reads one cell as it is stored, not as it is displayed: the shown
// text is escaped and cut at 512 characters, and writing that back would
// truncate the value it came from.
func (s *databaseSession) cellValue(ctx context.Context, table, column string, rowID int64) (any, error) {
	return s.backend.cellValue(ctx, table, column, rowID)
}

// updateCell writes one cell. The value is never parsed as SQL, and column
// affinity turns "42" back into a number in a column that stores numbers.
func (s *databaseSession) updateCell(ctx context.Context, table, column string, rowID int64, value string) (int64, error) {
	return s.backend.updateCell(ctx, table, column, rowID, value)
}

// deleteRow removes one row by rowid.
func (s *databaseSession) deleteRow(ctx context.Context, table string, rowID int64) (int64, error) {
	return s.backend.deleteRow(ctx, table, rowID)
}

// columnDeclaredType is the type a column was declared with, empty when the
// column was declared without one.
func (s *databaseSession) columnDeclaredType(ctx context.Context, table, column string) (string, error) {
	return s.backend.columnDeclaredType(ctx, table, column)
}

// typeAffinity is the affinity SQLite derives from a declared type, by the
// five rules of its documentation, in their order: INT anywhere in the name
// wins, then CHAR, CLOB and TEXT, then BLOB or no type at all, then REAL,
// FLOA and DOUB, and NUMERIC for everything else.
func typeAffinity(declared string) string {
	upper := strings.ToUpper(declared)
	switch {
	case strings.Contains(upper, "INT"):
		return "INTEGER"
	case strings.Contains(upper, "CHAR"), strings.Contains(upper, "CLOB"), strings.Contains(upper, "TEXT"):
		return "TEXT"
	case upper == "", strings.Contains(upper, "BLOB"):
		return "BLOB"
	case strings.Contains(upper, "REAL"), strings.Contains(upper, "FLOA"), strings.Contains(upper, "DOUB"):
		return "REAL"
	default:
		return "NUMERIC"
	}
}

// storedAsTextInstead reports whether a value typed for this column would be
// kept as text although the column was declared for numbers.
//
// SQLite types are affinities, not checks: a column declared int converts
// "42" to the number 42 and stores anything that does not read as a number --
// "тест", say -- as the text it is, without a word. That is the documented
// behaviour of the database and the client does not forbid it; it asks first,
// because a typed value that misses the column's declared type is usually a
// slip, and a STRICT table would have refused it outright.
func storedAsTextInstead(affinity, value string) bool {
	switch affinity {
	case "INTEGER", "REAL", "NUMERIC":
	default:
		return false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	_, err := strconv.ParseFloat(trimmed, 64)
	return err != nil
}

// editableText is the value as a line the user can edit, and whether editing
// it in a one line box is safe at all.
func editableText(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", true
	case []byte:
		// Binary: a text box would corrupt it on the way back.
		return "", false
	case string:
		if strings.ContainsAny(typed, "\r\n") {
			return "", false
		}
		return typed, true
	case time.Time:
		return typed.Format(time.RFC3339Nano), true
	default:
		return fmt.Sprint(typed), true
	}
}

func statementReturnsRows(statement string) bool {
	statement = stripSQLComments(strings.TrimSpace(statement))
	if statement == "" {
		return false
	}
	keyword := strings.ToLower(strings.Fields(statement)[0])
	switch keyword {
	case "select", "pragma", "explain", "values":
		return true
	case "with":
		// CTEs may end in SELECT/VALUES or in a write with RETURNING. Treat
		// them as row-producing statements; SQLite will report a useful error
		// for a write without RETURNING rather than silently dropping it.
		return true
	default:
		return false
	}
}

func stripSQLComments(statement string) string {
	for {
		trimmed := strings.TrimSpace(statement)
		switch {
		case strings.HasPrefix(trimmed, "--"):
			if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
				statement = trimmed[newline+1:]
				continue
			}
			return ""
		case strings.HasPrefix(trimmed, "/*"):
			if end := strings.Index(trimmed[2:], "*/"); end >= 0 {
				statement = trimmed[end+4:]
				continue
			}
			return ""
		default:
			return trimmed
		}
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// browsePageSize is how many rows one page of a table browse holds.
const browsePageSize = 100

func tableSelect(table string, offset int64) string {
	statement := "SELECT * FROM " + quoteIdentifier(table) + " LIMIT " + strconv.Itoa(browsePageSize)
	if offset > 0 {
		statement += " OFFSET " + strconv.FormatInt(offset, 10)
	}
	return statement
}

// clampOffset keeps a page offset inside a table that may have shrunk under
// it: a row deleted from the last page, or a whole page emptied by a DELETE
// from the SQL box, lands on the last page that still has rows rather than
// past the end.
func clampOffset(offset, total int64) int64 {
	if offset <= 0 || total <= 0 {
		return 0
	}
	if offset < total {
		return offset - offset%browsePageSize
	}
	return ((total - 1) / browsePageSize) * browsePageSize
}

func displayValue(value any) string {
	var text string
	switch value := value.(type) {
	case nil:
		return "NULL"
	case []byte:
		text = "x'" + hex.EncodeToString(value) + "'"
	case time.Time:
		text = value.Format(time.RFC3339Nano)
	case string:
		if !utf8.ValidString(value) {
			text = "x'" + hex.EncodeToString([]byte(value)) + "'"
		} else {
			text = value
		}
	default:
		text = fmt.Sprint(value)
	}
	text = strings.NewReplacer("\r", "\\r", "\n", "\\n", "\t", "\\t").Replace(text)
	const maxRunes = 512
	runes := []rune(text)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes-1]) + "…"
	}
	return text
}

func (p *Plugin) openCurrent(app vfs.App) {
	if mounted, ok := app.GetActivePanelVFS().(*databaseVFS); ok && mounted != nil {
		// Inside a mounted database the command opens that database, on
		// the table under the cursor.
		table, _ := mounted.tableOf(mounted.Join(mounted.GetPath(), app.GetSelectedName()))
		openDatabaseBrowser(app, mounted.GetPath(), table, app.RefreshAll)
		return
	}
	path, ok := selectedSQLitePath(app)
	if !ok {
		// Nothing usable under the cursor is not a dead end: ask for a name.
		// SQLite creates a database on first open, so the same prompt covers
		// starting an empty one and reaching a file the extension list would
		// have turned away.
		app.InputBox(
			sqliteText("SQLite.Title", " SQLite ", " SQLite "),
			sqliteText("SQLite.PathPrompt", "Database file to open or create:", "Файл базы данных (открыть или создать):"),
			sqliteText("SQLite.NewFileName", "database.sqlite", "database.sqlite"),
			func(answer string) {
				if answer = strings.TrimSpace(answer); answer != "" {
					p.openPath(app, databasePathIn(app, answer))
				}
			})
		return
	}
	p.openPath(app, path)
}

// databasePathIn resolves a typed name against the directory of the active
// panel, so a bare name means a database next to what the user is looking at.
func databasePathIn(app vfs.App, path string) string {
	if !filepath.IsAbs(path) {
		if fs, ok := app.GetActivePanelVFS().(*vfs.OSVFS); ok && fs != nil {
			if abs, err := fs.Abs(fs.Join(fs.GetPath(), path)); err == nil {
				return filepath.Clean(abs)
			}
		}
		if abs, err := filepath.Abs(path); err == nil {
			return filepath.Clean(abs)
		}
	}
	return filepath.Clean(path)
}

func (p *Plugin) openPath(app vfs.App, path string) {
	openDatabaseBrowser(app, path, "", nil)
}

// openDatabaseBrowser opens the client on a database, showing table when it
// is one of the database's tables and the first table otherwise. onClose, when
// set, runs once the client has closed; the database panel uses it to read
// the tables again, since the SQL box can create and drop them.
func openDatabaseBrowser(app vfs.App, path, table string, onClose func()) {
	var (
		session *databaseSession
		tables  []string
	)
	app.RunProgressTask(sqliteText("SQLite.Title", " SQLite ", " SQLite "), sqliteText("SQLite.OpeningDatabase", "Opening database...", "Открытие базы данных..."), false,
		func(ctx context.Context, update func(string, int)) error {
			update(sqliteText("SQLite.ReadingSchema", "Reading database schema...", "Чтение схемы базы данных..."), -1)
			var err error
			session, tables, err = openDatabase(ctx, path)
			return err
		},
		func(err error) {
			if err != nil {
				showSQLiteMessage(app, sqliteText("SQLite.Title", " SQLite ", " SQLite "), fmt.Sprintf(sqliteText("SQLite.OpenFailed", "Could not open %s:\n\n%v", "Не удалось открыть %s:\n\n%v"), path, err))
				return
			}
			if session == nil || vtui.FrameManager == nil {
				if session != nil {
					session.Close()
				}
				return
			}
			browser := newBrowserAt(app, session, tables, table)
			browser.onClose = onClose
			vtui.FrameManager.Push(browser.frame)
		})
}

func showSQLiteMessage(app vfs.App, title, message string) {
	if vtui.FrameManager == nil {
		return
	}
	if anchor, ok := app.(vtui.Frame); ok {
		vtui.ShowMessageOn(anchor, title, message, []string{sqliteText("SQLite.OK", "&OK", "&ОК")})
		return
	}
	vtui.ShowMessage(title, message, []string{sqliteText("SQLite.OK", "&OK", "&ОК")})
}
