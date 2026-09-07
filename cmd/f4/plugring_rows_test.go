package main

import (
	"strings"
	"testing"
)

func TestBuildPlugRingRowsGroupsByCategory(t *testing.T) {
	items := []PlugRingItem{
		{ID: "zip", Name: "Zip", Category: "archive", Entrypoint: "plugin.lua"},
		{ID: "ftp", Name: "Ftp", Category: "network", Entrypoint: "plugin.lua"},
		{ID: "sftp", Name: "Sftp", Category: "network", Entrypoint: "plugin.lua"},
	}

	rows, shown := BuildPlugRingRows(items, nil)
	if len(rows) != len(shown) {
		t.Fatalf("%d rows against %d selection entries", len(rows), len(shown))
	}
	// Three plugins plus two headings.
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want three plugins under two headings", len(rows))
	}

	var headings []string
	for i, row := range rows {
		entry, ok := row.(plugRingRow)
		if !ok {
			t.Fatalf("row %d is %T", i, row)
		}
		if entry.header != "" {
			headings = append(headings, entry.header)
			if shown[i] != nil {
				t.Errorf("row %d is a heading but points at a plugin", i)
			}
			continue
		}
		if shown[i] == nil {
			t.Errorf("row %d is a plugin but points at nothing", i)
			continue
		}
		if shown[i].ID != entry.item.ID {
			t.Errorf("row %d shows %q but selects %q", i, entry.item.ID, shown[i].ID)
		}
	}

	if len(headings) != 2 || headings[0] != PlugRingCategoryTitle("archive") {
		t.Errorf("headings = %v, want archives first", headings)
	}
}

func TestBuildPlugRingRowsStatuses(t *testing.T) {
	items := []PlugRingItem{
		{ID: "a", Name: "Fresh", Version: "1.0", Entrypoint: "plugin.lua"},
		{ID: "b", Name: "Old", Version: "2.0", Entrypoint: "plugin.lua"},
		{ID: "c", Name: "New", Version: "1.0", Entrypoint: "plugin.lua"},
	}
	installed := map[string]PlugRingItem{
		"a": {ID: "a", Version: "1.0"},
		"b": {ID: "b", Version: "1.0"},
	}

	rows, shown := BuildPlugRingRows(items, installed)
	status := map[string]string{}
	for i, row := range rows {
		if shown[i] == nil {
			continue
		}
		status[shown[i].ID] = row.(plugRingRow).status
	}

	want := map[string]string{"a": "Installed", "b": "Update", "c": "Not installed"}
	for id, expected := range want {
		if status[id] != expected {
			t.Errorf("%s = %q, want %q", id, status[id], expected)
		}
	}
}

func TestBuildPlugRingRowsMarksWhatCannotRunHere(t *testing.T) {
	items := []PlugRingItem{
		{ID: "jit", Name: "Needs LuaJIT", Entrypoint: "plugin.lua", Runtimes: []string{"luajit"}},
	}

	rows, _ := BuildPlugRingRows(items, nil)
	var entry plugRingRow
	for _, row := range rows {
		if candidate := row.(plugRingRow); candidate.header == "" {
			entry = candidate
		}
	}

	if entry.status != "Unavailable" {
		t.Errorf("status = %q, want the entry marked unavailable", entry.status)
	}
	if !strings.Contains(entry.note, "luajit") {
		t.Errorf("note = %q, want it to name what is missing", entry.note)
	}
	// The reason takes the place of the description, which is of no use to
	// somebody who cannot install the thing anyway.
	if !strings.Contains(entry.GetCellText(4), "luajit") {
		t.Errorf("description column = %q, want the reason", entry.GetCellText(4))
	}
}

func TestPlugRingRowHeadingIsBlankBesidesItsTitle(t *testing.T) {
	row := plugRingRow{header: "Archives"}
	if row.GetCellText(0) != "Archives" {
		t.Errorf("heading text = %q", row.GetCellText(0))
	}
	for col := 1; col <= 4; col++ {
		if got := row.GetCellText(col); got != "" {
			t.Errorf("heading column %d = %q, want it empty", col, got)
		}
	}
}

func TestBuildPlugRingRowsEmptyCatalog(t *testing.T) {
	rows, shown := BuildPlugRingRows(nil, nil)
	if len(rows) != 0 || len(shown) != 0 {
		t.Errorf("an empty catalog produced %d rows", len(rows))
	}
}
