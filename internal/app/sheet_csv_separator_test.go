package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/i18n"
)

// f4#1892 (nz256): TSV and semicolon files open in columns, the sheet keeps
// their delimiter for export, and the spreadsheet menu (F9) has the item that
// changes it.
func TestSheetOpensTSVAndSemicolonFilesInColumns(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"table.tsv": "name\tvalue\nalpha\t42\n",
		"table.csv": "name;value\nalpha;42\n",
		"plain.csv": "name,value\nalpha,42\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		doc, err := loadSheetCSV(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := doc.Cell(1, 1).DisplayText(); got != "42" {
			t.Errorf("%s: B2 = %q, want 42", name, got)
		}
		if doc.Modified {
			t.Errorf("%s: a file just opened is marked modified", name)
		}
		if !sheetIsCSVPath(path) {
			t.Errorf("%s: not opened as CSV", name)
		}
	}

	// Read again with another delimiter: one column.
	doc, err := loadSheetCSVWith(filepath.Join(dir, "table.csv"), ',')
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Cell(0, 1).DisplayText(); got != "alpha;42" || doc.Cell(1, 1) != nil {
		t.Errorf("read with comma: A2 = %q, B2 = %v; want alpha;42 and nothing", got, doc.Cell(1, 1))
	}

	found := false
	for _, entry := range sheetMenuEntries() {
		if entry.label == i18n.Msg("Sheet.MenuCSVSeparator") {
			found = true
		}
	}
	if !found {
		t.Error("the spreadsheet menu has no CSV separator item")
	}
}

// f4#1892: Save (F2) of a sheet opened from a CSV file writes CSV with the
// sheet's delimiter, not the native format under the .csv name.
func TestSheetSaveKeepsCSVFilesCSV(t *testing.T) {
	sf := newSheetFrameForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "table.csv")
	if err := os.WriteFile(path, []byte("name;value\nalpha;42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sheetOpen(sf, path)
	if sf.doc == nil || sf.doc.Cell(1, 1).DisplayText() != "42" {
		t.Fatal("the CSV file did not open in columns")
	}
	sf.doc.SetText(1, 1, "43")
	sheetSave(sf, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "name;value\nalpha;43\n" {
		t.Errorf("saved file = %q, want the CSV with ; and the new value", got)
	}

	tsv := filepath.Join(dir, "new.tsv")
	sheetSave(sf, tsv)
	data, err = os.ReadFile(tsv)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "name;value\nalpha;43\n" {
		t.Errorf("saved .tsv = %q, want the sheet's own delimiter kept", got)
	}
}
