//go:build lite

package sheet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestJSONStoreIsHumanReadable checks the shape store_lite.go actually
// writes to disk, independently of the Save/Load round trip that
// TestSQLiteRoundTrip (sheet_test.go) already exercises under both build
// tags. It is the format-specific complement that test needs: something
// that inspects the bytes on disk rather than only going back through the
// same package's own Load.
func TestJSONStoreIsHumanReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.f4s")
	s := New()
	s.Title = "Budget"
	s.Separators = true
	s.SetColumnWidth(2, 20)
	s.SetText(0, 0, "item")
	s.SetText(2, 0, "yes")

	ctx := context.Background()
	if err := s.Save(ctx, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("saved sheet is not a JSON object: %v", err)
	}
	for _, key := range []string{"schema", "title", "separators", "columns", "cells"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("saved sheet is missing top-level key %q: %s", key, data)
		}
	}
	if generic["title"] != "Budget" {
		t.Errorf("title = %v, want %q", generic["title"], "Budget")
	}
}

// TestJSONLoadRejectsGarbage mirrors the sqlite store opening a file that is
// not a database at all: Load must return an error rather than panicking or
// silently producing an empty sheet.
func TestJSONLoadRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.f4s")
	if err := os.WriteFile(path, []byte("not json at all {{{"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(context.Background(), path); err == nil {
		t.Error("Load must reject a file that is not valid JSON")
	}
}

// TestJSONLoadRejectsNewerSchema mirrors store.go's own schema check: a file
// stamped with a schema version newer than this build understands must be
// refused rather than silently misread.
func TestJSONLoadRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.f4s")
	doc := fmt.Sprintf(`{"schema":%d,"title":"","separators":false,"columns":{},"cells":[]}`, SchemaVersion+1)
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(context.Background(), path); err == nil {
		t.Error("Load must reject a sheet written by a newer schema version")
	}
}

// TestJSONLoadRejectsInvalidCellFormatting mirrors store.go's own bounds
// check on the Display/Justify/Decimals enums: a hand-edited or corrupted
// file that puts an out-of-range value in one of those fields must be
// refused, the same way an out-of-range value read from the sqlite columns
// is refused.
func TestJSONLoadRejectsInvalidCellFormatting(t *testing.T) {
	cases := []string{
		`{"col":0,"row":0,"text":"x","display":99,"justify":0,"decimals":0,"protected":false}`,
		`{"col":0,"row":0,"text":"x","display":0,"justify":99,"decimals":0,"protected":false}`,
		`{"col":0,"row":0,"text":"x","display":0,"justify":0,"decimals":256,"protected":false}`,
		`{"col":0,"row":0,"text":"x","display":0,"justify":0,"decimals":-1,"protected":false}`,
	}
	for _, cell := range cases {
		path := filepath.Join(t.TempDir(), "book.f4s")
		doc := `{"schema":1,"title":"","separators":false,"columns":{},"cells":[` + cell + `]}`
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := Load(context.Background(), path); err == nil {
			t.Errorf("Load must reject invalid cell formatting: %s", cell)
		}
	}
}

// TestJSONLoadSkipsOutOfGridCells mirrors store.go's own behaviour: a cell
// coordinate outside the grid is silently dropped rather than rejecting the
// whole file, since the grid limits (MaxColumns/MaxRows) can shrink between
// versions in a way an individual out-of-range cell should not be fatal to.
func TestJSONLoadSkipsOutOfGridCells(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.f4s")
	doc := `{"schema":1,"title":"","separators":false,"columns":{},"cells":[` +
		`{"col":-1,"row":0,"text":"x","display":0,"justify":0,"decimals":0,"protected":false},` +
		`{"col":0,"row":999999,"text":"y","display":0,"justify":0,"decimals":0,"protected":false},` +
		`{"col":0,"row":0,"text":"kept","display":0,"justify":0,"decimals":0,"protected":false}` +
		`]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cell := loaded.Cell(0, 0); cell == nil || cell.Text != "kept" {
		t.Errorf("in-range cell was lost: %+v", cell)
	}
	count := 0
	loaded.Cells(func(Point, *Cell) { count++ })
	if count != 1 {
		t.Errorf("out-of-grid cells were not dropped, got %d cells", count)
	}
}

// TestJSONIsSheetFileRejectsNonSheetJSON checks the lite build's
// content-sniffing rejects JSON documents that are not sheet files, the
// same way the sqlite build's IsSheetFile rejects a database without a
// f4_sheet_cells table.
func TestJSONIsSheetFileRejectsNonSheetJSON(t *testing.T) {
	cases := map[string]string{
		"empty object":        `{}`,
		"unrelated document":  `{"name":"not a sheet","values":[1,2,3]}`,
		"not json at all":     `just some text`,
		"json array, not obj": `[1,2,3]`,
	}
	for name, content := range cases {
		path := filepath.Join(t.TempDir(), "maybe.f4s")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if IsSheetFile(context.Background(), path) {
			t.Errorf("%s: IsSheetFile must reject %q", name, content)
		}
	}
}

// TestJSONIsSheetFileMissingFile checks IsSheetFile fails closed, rather
// than panicking, when the path does not exist at all.
func TestJSONIsSheetFileMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.f4s")
	if IsSheetFile(context.Background(), path) {
		t.Error("IsSheetFile must report false for a missing file")
	}
}
