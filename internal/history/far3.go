package history

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3/driver"
)

// Far3History is a read-only snapshot of the three portable Far history kinds.
type Far3History struct {
	Commands, Folders []HistoryRecord
	Files             []ViewerEditorRecord
	Skipped           int
}

// ReadFar3History reads committed SQLite data, including Far's live WAL. It
// never runs commands or changes Far's database. Kinds and record types follow
// https://github.com/FarGroup/FarManager/blob/master/far/history.hpp.
func ReadFar3History(ctx context.Context, path string) (Far3History, error) {
	var result Far3History
	abs, err := filepath.Abs(path)
	if err != nil {
		return result, err
	}
	u := url.URL{Path: filepath.ToSlash(abs)}
	db, err := driver.Open("file:" + u.EscapedPath() + "?mode=ro")
	if err != nil {
		return result, fmt.Errorf("open Far3 history: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT kind, type, lock, name, time, guid, data
		FROM history WHERE kind IN (0, 1, 2) AND key = '' ORDER BY time DESC, id DESC`)
	if err != nil {
		return result, fmt.Errorf("read Far3 history %q: %w", path, err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, typ, locked int
		var ticks int64
		var name, guid, data string
		if err := rows.Scan(&kind, &typ, &locked, &name, &ticks, &guid, &data); err != nil {
			return Far3History{}, fmt.Errorf("decode Far3 history: %w", err)
		}
		if name == "" || (guid != "" && strings.Trim(guid, "{}") != "00000000-0000-0000-0000-000000000000") ||
			(kind == 2 && typ != 0 && typ != 1 && typ != 4) {
			result.Skipped++
			continue
		}
		stamp := time.Time{}
		if ticks > 0 {
			stamp = time.Unix(ticks/10000000-11644473600, ticks%10000000*100).UTC()
		}
		record := HistoryRecord{Name: name, Timestamp: stamp, Lock: locked != 0}
		switch kind {
		case 0:
			record.Dir = data
			result.Commands = append(result.Commands, record)
		case 1:
			result.Folders = append(result.Folders, record)
		case 2:
			mode := HistoryModeView
			if typ == 1 || typ == 4 {
				mode = HistoryModeEdit
			}
			result.Files = append(result.Files, ViewerEditorRecord{Path: name, Display: name,
				Mode: mode, Local: true, VFSType: "*vfs.OSVFS", Timestamp: stamp, Lock: record.Lock})
		}
	}
	if err := rows.Err(); err != nil {
		return Far3History{}, fmt.Errorf("read Far3 history: %w", err)
	}
	return result, nil
}
