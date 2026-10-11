package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostpath"
)

var (
	errNotADatabase = errors.New("not a SQLite database")
	errVFSClosed    = errors.New("SQLite: the database panel is closed")
)

// readOnlyError is what every change made through the panel gets. The data is
// changed in the client, which the same panel opens with Enter or F4; the
// error says so, and it is os.ErrPermission to the file operations that ask.
type readOnlyError struct{}

func (readOnlyError) Error() string {
	return sqliteText("SQLite.PanelReadOnly",
		"The database panel is read-only; press Enter or F4 on a table to change it in the SQLite client",
		"Панель базы данных только для чтения; чтобы изменить таблицу, нажмите на ней Enter или F4 — откроется клиент SQLite")
}

func (readOnlyError) Is(target error) bool { return target == os.ErrPermission }

// databaseVFS shows a SQLite database in a file panel: its tables and views
// are the files at the top of it, F3 reads one as CSV, F5 copies it out as a
// .csv file, and Enter or F4 open the SQLite client on it.
//
// The panel is the way in and the client is the editor. Nothing is written
// through the panel itself, so there is exactly one place where data changes
// and one set of rules for how it does.
//
// The database is flat on purpose: a path is the database file itself, or the
// database file joined with the name of one of its tables.
type databaseVFS struct {
	parent vfs.VFS
	dbPath string

	mu      sync.Mutex
	session *databaseSession
	closed  bool
}

func newDatabaseVFS(parent vfs.VFS, dbPath string) *databaseVFS {
	return &databaseVFS{parent: parent, dbPath: hostpath.Clean(dbPath)}
}

// sessionFor opens the database the first time it is needed. A clone starts
// without a session of its own and opens one here.
//
// The header is checked again first: the driver creates a database that is
// not there, and a panel left open on a file that has since been deleted must
// not quietly bring an empty one back.
func (v *databaseVFS) sessionFor(ctx context.Context) (*databaseSession, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil, errVFSClosed
	}
	if v.session != nil {
		return v.session, nil
	}
	if !hasSQLiteHeader(v.dbPath) {
		return nil, fmt.Errorf("%s: %w", v.dbPath, errNotADatabase)
	}
	session, _, err := openDatabase(ctx, v.dbPath)
	if err != nil {
		return nil, err
	}
	v.session = session
	return session, nil
}

func (v *databaseVFS) IsAtRoot() bool  { return true }
func (v *databaseVFS) GetPath() string { return v.dbPath }
func (v *databaseVFS) IsAbs(p string) bool {
	return hostpath.IsAbs(p)
}

// SetPath accepts the database itself and nothing else: there are no folders
// in it to go into.
func (v *databaseVFS) SetPath(p string) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	if abs != v.dbPath {
		return os.ErrNotExist
	}
	return nil
}

func (v *databaseVFS) Join(elem ...string) string { return hostpath.Join(elem...) }

func (v *databaseVFS) Abs(p string) (string, error) {
	if p == "" {
		return v.dbPath, nil
	}
	if hostpath.IsAbs(p) {
		return hostpath.Clean(p), nil
	}
	return hostpath.Join(v.dbPath, p), nil
}

func (v *databaseVFS) Base(p string) string { return hostpath.Base(hostpath.Clean(p)) }
func (v *databaseVFS) Dir(p string) string  { return hostpath.Dir(hostpath.Clean(p)) }

// tableOf is the table a path names, when it names one.
func (v *databaseVFS) tableOf(p string) (string, bool) {
	abs, err := v.Abs(p)
	if err != nil || abs == v.dbPath || hostpath.Dir(abs) != v.dbPath {
		return "", false
	}
	return tableNameFromFile(hostpath.Base(abs))
}

// existingTable is tableOf that also asks the database whether the table is
// still there.
func (v *databaseVFS) existingTable(ctx context.Context, p string) (*databaseSession, string, error) {
	table, ok := v.tableOf(p)
	if !ok {
		return nil, "", os.ErrNotExist
	}
	session, err := v.sessionFor(ctx)
	if err != nil {
		return nil, "", err
	}
	tables, err := session.listTables(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, candidate := range tables {
		if candidate == table {
			return session, table, nil
		}
	}
	return nil, "", os.ErrNotExist
}

func (v *databaseVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	if abs != v.dbPath {
		return os.ErrNotExist
	}
	session, err := v.sessionFor(ctx)
	if err != nil {
		return err
	}
	tables, err := session.listTables(ctx)
	if err != nil {
		return err
	}
	items := make([]vfs.VFSItem, 0, len(tables))
	for _, table := range tables {
		if name, ok := tableFileName(table); ok {
			items = append(items, tableItem(name))
		}
	}
	if len(items) > 0 && onChunk != nil {
		onChunk(items)
	}
	return ctx.Err()
}

// tableItem describes a table as a file. Its size is the size of its CSV,
// which is not known until the table has been read, and SQLite keeps no time
// a table last changed; both are reported as unknown rather than made up.
//
// NoExtension because a dot in a table name is part of the name: people.old
// is a table, not a file of type "old".
func tableItem(name string) vfs.VFSItem {
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit,
		Name:          name,
		NoExtension:   true,
	}
}

func (v *databaseVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if abs == v.dbPath {
		return vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, Name: hostpath.Base(v.dbPath), IsDir: true}, nil
	}
	if _, _, err := v.existingTable(ctx, abs); err != nil {
		return vfs.VFSItem{}, err
	}
	return tableItem(hostpath.Base(abs)), nil
}

func (v *databaseVFS) MkDir(context.Context, string) error          { return readOnlyError{} }
func (v *databaseVFS) Remove(context.Context, string) error         { return readOnlyError{} }
func (v *databaseVFS) Rename(context.Context, string, string) error { return readOnlyError{} }
func (v *databaseVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return readOnlyError{}
}
func (v *databaseVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, readOnlyError{}
}

func (v *databaseVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true}
}

func (v *databaseVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

// Open is what F3 and F5 read: the whole table as CSV.
func (v *databaseVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	session, table, err := v.existingTable(ctx, p)
	if err != nil {
		return nil, err
	}
	return exportTableToTemp(ctx, session, table)
}

// TransferName makes a table copied out of the panel a .csv file, which is
// what it is once it has left the database.
func (v *databaseVFS) TransferName(srcPath string, _ vfs.VFS) string {
	if _, ok := v.tableOf(srcPath); !ok {
		return ""
	}
	return hostpath.Base(hostpath.Clean(srcPath)) + ".csv"
}

func (v *databaseVFS) ParentVFS() vfs.VFS { return v.parent }

// databaseHandlerName is how the panel title names what opened the database.
const databaseHandlerName = "SQLite"

// PanelTitle names the database by what reads it, "SQLite:panelmodes.db",
// the way an archive is named "Zipper:7z:Far.7z" (#1383). The folder the
// file lies in is where ".." returns to.
func (v *databaseVFS) PanelTitle(p string) string {
	abs, err := v.Abs(p)
	if err != nil || abs != v.dbPath {
		return ""
	}
	return databaseHandlerName + ":" + hostpath.Base(v.dbPath)
}

// Clone gets a connection of its own, opened when it is first used, so that
// closing one panel on the database does not pull the other one's from under
// it.
func (v *databaseVFS) Clone() vfs.VFS { return newDatabaseVFS(v.parent, v.dbPath) }

func (v *databaseVFS) Close() error {
	v.mu.Lock()
	session := v.session
	v.session = nil
	v.closed = true
	v.mu.Unlock()
	session.Close()
	return nil
}

// HandlePanelAction puts the client on top of the panel: Enter and F4 on a
// table open it there. Everything else -- ".." above all, which must still
// leave the database -- is left to the panel.
//
// The cursor decides which table, not the marked files: Enter acts on the row
// the cursor is on, even when other rows are marked.
func (v *databaseVFS) HandlePanelAction(app vfs.App, action vfs.PanelAction, paths []string) bool {
	if app == nil || (action != vfs.PanelActionActivate && action != vfs.PanelActionEdit) {
		return false
	}
	target := ""
	if name := app.GetSelectedName(); name != "" {
		target = v.Join(v.dbPath, name)
	}
	if target == "" && len(paths) > 0 {
		target = paths[0]
	}
	table, ok := v.tableOf(target)
	if !ok {
		return false
	}
	openDatabaseBrowser(app, v.dbPath, table, app.RefreshAll)
	return true
}

var (
	_ vfs.VFS                  = (*databaseVFS)(nil)
	_ vfs.PanelActionHandler   = (*databaseVFS)(nil)
	_ vfs.TransferNameProvider = (*databaseVFS)(nil)
	_ vfs.PanelTitleProvider   = (*databaseVFS)(nil)
	_ vfs.VFSProvider          = (*databaseProvider)(nil)
)

// tableFileName is how a table is named in the panel. A table name may hold
// anything, and three things in one would break a path: a separator, which
// would make it two names, "." and "..", which already mean something, and a
// control character, which the panel cannot show. Those, and the escape
// character itself, are written as %XX. A name nothing can represent -- the
// empty one -- is left out of the panel; the client still reaches it.
func tableFileName(table string) (string, bool) {
	if table == "" {
		return "", false
	}
	if table == "." || table == ".." {
		return strings.Repeat("%2E", len(table)), true
	}
	var b strings.Builder
	for i := 0; i < len(table); i++ {
		c := table[i]
		if c == '%' || c == '/' || c == '\\' || c < 0x20 || c == 0x7f {
			fmt.Fprintf(&b, "%%%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String(), true
}

// tableNameFromFile undoes tableFileName. A name with a malformed escape
// was not produced by it and names no table.
func tableNameFromFile(name string) (string, bool) {
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+2 >= len(name) {
			return "", false
		}
		value, ok := unhex(name[i+1], name[i+2])
		if !ok {
			return "", false
		}
		b.WriteByte(value)
		i += 2
	}
	return b.String(), true
}

func unhex(high, low byte) (byte, bool) {
	h, ok := hexDigit(high)
	if !ok {
		return 0, false
	}
	l, ok := hexDigit(low)
	if !ok {
		return 0, false
	}
	return h<<4 | l, true
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}
