//go:build !lite

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/ncruces/go-sqlite3/driver"
)

// openDefaultBackend is the regular build's backend: the SQLite engine
// linked into f4 (github.com/ncruces/go-sqlite3). backend_default_lite.go
// swaps in the sqlite3 command-line tool.
func openDefaultBackend(ctx context.Context, path string) (sessionBackend, error) {
	backend, err := openDriverBackend(ctx, path)
	if err != nil {
		return nil, err
	}
	return backend, nil
}

// driverBackend talks to the database through database/sql over one
// connection, so a transaction begun in the SQL box stays open across
// statements.
type driverBackend struct {
	db *sql.DB
}

func openDriverBackend(ctx context.Context, path string) (*driverBackend, error) {
	db, err := driver.Open(path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &driverBackend{db: db}, nil
}

func (b *driverBackend) close() {
	if b.db != nil {
		_ = b.db.Close()
	}
}

func (b *driverBackend) listTables(ctx context.Context) ([]string, error) {
	rows, err := b.db.QueryContext(ctx, listTablesSQL)
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

func (b *driverBackend) exec(ctx context.Context, statement string) (int64, error) {
	result, err := b.db.ExecContext(ctx, statement)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (b *driverBackend) query(ctx context.Context, statement string) (queryResult, error) {
	rows, err := b.db.QueryContext(ctx, statement)
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

func (b *driverBackend) countRows(ctx context.Context, table string) (int64, error) {
	var total int64
	if err := b.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdentifier(table)).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// insertRow adds a row of defaults, which is the part a dialog cannot do
// better: the columns are filled in afterwards with F4, one cell at a time,
// through the same path that edits an existing row.
//
// A table whose columns are NOT NULL without defaults refuses this, and
// SQLite names the column that refused; that message is worth showing rather
// than guessing at values on the user's behalf.
func (b *driverBackend) insertRow(ctx context.Context, table string) (int64, error) {
	// #nosec G202 -- SQLite cannot bind identifiers; quoteIdentifier escapes every embedded quote.
	result, err := b.db.ExecContext(ctx, "INSERT INTO "+quoteIdentifier(table)+" DEFAULT VALUES")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (b *driverBackend) browseWithRowIDs(ctx context.Context, table string, offset int64) (queryResult, []int64, error) {
	// Ordered by rowid so that paging is stable: without an order, two pages
	// of the same table are not guaranteed to be two different halves of it.
	// #nosec G202 -- the table name is identifier-quoted and both numeric clauses are generated from typed integers.
	statement := "SELECT rowid AS " + quoteIdentifier(rowIDColumn) + ", * FROM " + quoteIdentifier(table) +
		" ORDER BY rowid LIMIT " + strconv.Itoa(browsePageSize) + " OFFSET " + strconv.FormatInt(offset, 10)
	rows, err := b.db.QueryContext(ctx, statement)
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

func (b *driverBackend) browse(ctx context.Context, table string, offset int64) (queryResult, error) {
	return b.query(ctx, tableSelect(table, offset))
}

func (b *driverBackend) cellValue(ctx context.Context, table, column string, rowID int64) (any, error) {
	statement := "SELECT " + quoteIdentifier(column) + " FROM " + quoteIdentifier(table) + " WHERE rowid = ?"
	var value any
	if err := b.db.QueryRowContext(ctx, statement, rowID).Scan(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// updateCell binds the value as a parameter, so nothing the user types is
// ever parsed as SQL.
func (b *driverBackend) updateCell(ctx context.Context, table, column string, rowID int64, value string) (int64, error) {
	// #nosec G202 -- SQLite cannot bind identifiers; both identifiers are escaped, while values remain bound parameters.
	statement := "UPDATE " + quoteIdentifier(table) + " SET " + quoteIdentifier(column) + " = ? WHERE rowid = ?"
	result, err := b.db.ExecContext(ctx, statement, value, rowID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (b *driverBackend) deleteRow(ctx context.Context, table string, rowID int64) (int64, error) {
	// #nosec G202 -- SQLite cannot bind identifiers; quoteIdentifier escapes the table name and rowID is a bound parameter.
	statement := "DELETE FROM " + quoteIdentifier(table) + " WHERE rowid = ?"
	result, err := b.db.ExecContext(ctx, statement, rowID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (b *driverBackend) columnDeclaredType(ctx context.Context, table, column string) (string, error) {
	rows, err := b.db.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
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

func (b *driverBackend) scanTable(ctx context.Context, table string, columns func([]string) error, row func([]any) error) error {
	// #nosec G202 -- SQLite cannot bind identifiers; quoteIdentifier escapes every embedded quote.
	rows, err := b.db.QueryContext(ctx, "SELECT * FROM "+quoteIdentifier(table))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	names, err := rows.Columns()
	if err != nil {
		return err
	}
	if err := columns(names); err != nil {
		return err
	}
	values := make([]any, len(names))
	destinations := make([]any, len(names))
	for i := range values {
		destinations[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(destinations...); err != nil {
			return err
		}
		if err := row(values); err != nil {
			return err
		}
	}
	return rows.Err()
}
