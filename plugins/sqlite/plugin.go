// Package sqlite provides a small local SQLite browser/editor for f4.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const sqliteCommandID = "f4.sqlite.open"

// Plugin exposes the SQLite browser/editor as an in-process f4 plugin.
type Plugin struct {
	mu           sync.Mutex
	registration vfs.Registration
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

	p.mu.Lock()
	p.registration = registration
	p.initialized = true
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	registration := p.registration
	p.registration = nil
	p.initialized = false
	p.mu.Unlock()
	if registration != nil {
		registration.Unregister()
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

type databaseSession struct {
	db        *sql.DB
	path      string
	closeOnce sync.Once
}

func openDatabase(ctx context.Context, path string) (*databaseSession, []string, error) {
	db, err := driver.Open(path)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	session := &databaseSession{db: db, path: path}
	if err := db.PingContext(ctx); err != nil {
		session.Close()
		return nil, nil, err
	}
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
		if s.db != nil {
			_ = s.db.Close()
		}
	})
}

func (s *databaseSession) listTables(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
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
		result, err := s.db.ExecContext(ctx, statement)
		if err != nil {
			return queryResult{}, err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return queryResult{}, err
		}
		return queryResult{RowsAffected: rowsAffected}, nil
	}

	rows, err := s.db.QueryContext(ctx, statement)
	if err != nil {
		return queryResult{}, err
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return queryResult{}, err
	}
	result := queryResult{Columns: columns, ReturnsRows: true}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(values))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return queryResult{}, err
		}
		cells := make([]string, len(values))
		for i, value := range values {
			cells[i] = displayValue(value)
		}
		result.Rows = append(result.Rows, cells)
	}
	if err := rows.Err(); err != nil {
		return queryResult{}, err
	}
	return result, nil
}

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
	result, err = s.execute(ctx, tableSelect(table, offset))
	return tableBrowse{result: result, offset: offset, total: total}, err
}

// countRows is what makes paging possible: the page to show has to be chosen
// against the number of rows there actually are.
func (s *databaseSession) countRows(ctx context.Context, table string) (int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table)).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
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
	// #nosec G202 -- SQLite cannot bind identifiers; quoteIdentifier escapes every embedded quote.
	result, err := s.db.ExecContext(ctx, "INSERT INTO "+quoteIdentifier(table)+" DEFAULT VALUES")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *databaseSession) browseWithRowIDs(ctx context.Context, table string, offset int64) (queryResult, []int64, error) {
	// Ordered by rowid so that paging is stable: without an order, two pages
	// of the same table are not guaranteed to be two different halves of it.
	// #nosec G202 -- the table name is identifier-quoted and both numeric clauses are generated from typed integers.
	statement := "SELECT rowid AS " + quoteIdentifier(rowIDColumn) + ", * FROM " + quoteIdentifier(table) +
		" ORDER BY rowid LIMIT " + strconv.Itoa(browsePageSize) + " OFFSET " + strconv.FormatInt(offset, 10)
	rows, err := s.db.QueryContext(ctx, statement)
	if err != nil {
		return queryResult{}, nil, err
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return queryResult{}, nil, err
	}
	if len(columns) < 2 {
		return queryResult{}, nil, errors.New("SQLite: table has no columns of its own")
	}

	result := queryResult{Columns: columns[1:], ReturnsRows: true}
	var rowIDs []int64
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(values))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return queryResult{}, nil, err
		}
		rowID, ok := values[0].(int64)
		if !ok {
			return queryResult{}, nil, errors.New("SQLite: rows have no usable rowid")
		}
		cells := make([]string, len(columns)-1)
		for i, value := range values[1:] {
			cells[i] = displayValue(value)
		}
		rowIDs = append(rowIDs, rowID)
		result.Rows = append(result.Rows, cells)
	}
	if err := rows.Err(); err != nil {
		return queryResult{}, nil, err
	}
	return result, rowIDs, nil
}

// cellValue reads one cell as it is stored, not as it is displayed: the shown
// text is escaped and cut at 512 characters, and writing that back would
// truncate the value it came from.
func (s *databaseSession) cellValue(ctx context.Context, table, column string, rowID int64) (any, error) {
	statement := "SELECT " + quoteIdentifier(column) + " FROM " + quoteIdentifier(table) + " WHERE rowid = ?"
	var value any
	if err := s.db.QueryRowContext(ctx, statement, rowID).Scan(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// updateCell writes one cell. The value travels as a parameter, so nothing the
// user types is ever parsed as SQL, and column affinity turns "42" back into a
// number in a column that stores numbers.
func (s *databaseSession) updateCell(ctx context.Context, table, column string, rowID int64, value string) (int64, error) {
	// #nosec G202 -- SQLite cannot bind identifiers; both identifiers are escaped, while values remain bound parameters.
	statement := "UPDATE " + quoteIdentifier(table) + " SET " + quoteIdentifier(column) + " = ? WHERE rowid = ?"
	result, err := s.db.ExecContext(ctx, statement, value, rowID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// deleteRow removes one row by rowid.
func (s *databaseSession) deleteRow(ctx context.Context, table string, rowID int64) (int64, error) {
	// #nosec G202 -- SQLite cannot bind identifiers; quoteIdentifier escapes the table name and rowID is a bound parameter.
	statement := "DELETE FROM " + quoteIdentifier(table) + " WHERE rowid = ?"
	result, err := s.db.ExecContext(ctx, statement, rowID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// columnDeclaredType is the type a column was declared with, empty when the
// column was declared without one.
func (s *databaseSession) columnDeclaredType(ctx context.Context, table, column string) (string, error) {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			cid       int
			name      string
			declared  string
			notNull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &declared, &notNull, &dfltValue, &pk); err != nil {
			return "", err
		}
		if strings.EqualFold(name, column) {
			return declared, nil
		}
	}
	return "", rows.Err()
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
			browser := newBrowser(app, session, tables)
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
