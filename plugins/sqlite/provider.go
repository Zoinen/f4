package sqlite

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostpath"
)

// sqliteHeader is how every SQLite 3 database begins: the string
// "SQLite format 3" and a NUL, the first sixteen bytes of the header described
// in section 1.3.1 of https://www.sqlite.org/fileformat.html.
//
// The file is recognized by these bytes and not by its name. A database is
// often called something other than .db -- places.sqlite, collection.anki2,
// a .gpkg map -- and a .db file is not always a database.
var sqliteHeader = []byte("SQLite format 3\x00")

// databaseProvider mounts a SQLite database in a file panel the way an archive
// is mounted: Enter or Ctrl+PgDn on the file enters it, ".." at the top of it
// leaves.
type databaseProvider struct{}

func (*databaseProvider) Name() string  { return "sqlite" }
func (*databaseProvider) Priority() int { return 20 }

func (*databaseProvider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	_, ok := localDatabasePath(ctx, parent, path)
	return ok
}

// Open reads the schema before the panel switches over, so a file that has
// the header and still is no database -- a damaged or an encrypted one -- is
// reported where the key was pressed instead of turning into an empty panel.
func (*databaseProvider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	dbPath, ok := localDatabasePath(ctx, parent, path)
	if !ok {
		return nil, fmt.Errorf("%s: %w", path, errNotADatabase)
	}
	mounted := newDatabaseVFS(parent, dbPath)
	if _, err := mounted.sessionFor(ctx); err != nil {
		_ = mounted.Close()
		return nil, err
	}
	return mounted, nil
}

// localDatabasePath answers for files on the local disk only. The driver opens
// a database by file name, and a database inside an archive or on a remote
// file system has none to give it.
func localDatabasePath(ctx context.Context, parent vfs.VFS, path string) (string, bool) {
	if ctx != nil && ctx.Err() != nil {
		return "", false
	}
	local, ok := parent.(*vfs.OSVFS)
	if !ok || local == nil {
		return "", false
	}
	abs, err := local.Abs(path)
	if err != nil || abs == "" {
		return "", false
	}
	abs = hostpath.Clean(abs)
	return abs, hasSQLiteHeader(abs)
}

// hasSQLiteHeader reads the first sixteen bytes of a regular file. Anything
// else -- a directory, a pipe that would block the read, a device -- is not a
// database, whatever its name says.
func hasSQLiteHeader(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	file, err := os.Open(path) // #nosec G304 -- the path is the file under the panel cursor, which is what the user asked to open.
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, len(sqliteHeader))
	if _, err := io.ReadFull(file, head); err != nil {
		return false
	}
	return bytes.Equal(head, sqliteHeader)
}
