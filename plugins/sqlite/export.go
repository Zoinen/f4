package sqlite

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/vfs"
)

// exportTableToTemp writes a table as CSV into a temporary file and hands it
// over for reading. The file is removed when the reader is closed.
//
// The whole table is written before the first byte is read. The viewer and
// the copy both need the size up front, and a CSV has no size until it has
// been written.
func exportTableToTemp(ctx context.Context, session *databaseSession, table string) (vfs.ReadAtCloser, error) {
	file, err := os.CreateTemp("", "f4-sqlite-*.csv")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()
	fail := func(err error) (vfs.ReadAtCloser, error) {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return nil, err
	}
	buffered := bufio.NewWriter(file)
	if err := session.exportTableCSV(ctx, table, buffered); err != nil {
		return fail(err)
	}
	if err := buffered.Flush(); err != nil {
		return fail(err)
	}
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return &vfs.TempFileWrapper{File: file, SizeVal: info.Size(), TempPath: tempPath}, nil
}

// exportTableCSV writes a table to w as CSV: the column names, then one record
// per row, quoted as RFC 4180 has it, so that a value with a comma, a quote or
// a line break in it survives the trip.
func (s *databaseSession) exportTableCSV(ctx context.Context, table string, w io.Writer) error {
	out := csv.NewWriter(w)
	var record []string
	err := s.backend.scanTable(ctx, table,
		func(columns []string) error {
			record = make([]string, len(columns))
			return out.Write(columns)
		},
		func(values []any) error {
			for i, value := range values {
				record[i] = csvValue(value)
			}
			return out.Write(record)
		})
	if err != nil {
		return err
	}
	out.Flush()
	return out.Error()
}

// csvValue is a value as a CSV field. It is the grid's displayValue without
// the parts that only make sense on one screen line: nothing is cut short,
// and a line break stays a line break, since CSV quotes it.
//
// NULL is an empty field, as the sqlite3 shell writes it in CSV mode. Binary
// data and text that is not UTF-8 are written as the same x'...' literal the
// grid shows: raw bytes in a text file do not survive a viewer or an editor.
func csvValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case []byte:
		return "x'" + hex.EncodeToString(value) + "'"
	case time.Time:
		return value.Format(time.RFC3339Nano)
	case string:
		if !utf8.ValidString(value) {
			return "x'" + hex.EncodeToString([]byte(value)) + "'"
		}
		return value
	default:
		return fmt.Sprint(value)
	}
}
