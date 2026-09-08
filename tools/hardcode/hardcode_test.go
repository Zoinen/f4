package hardcode

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestScanFindsHardcodedCaptions(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "dialog.go"), `package sample

func build() {
	NewButton(nil, 0, "Close")
	NewButton(nil, 0, Msg("Dialog.Close"))
	vtui.NewLabel(nil, 0, "Name:")
	NewVMenu("Menu", nil)
	NewComboBox(nil, 0, 0, []string{"Never", "Daily"})
	NewLabel(nil, 0, "──────")
	NewLabel(nil, 0, "")
	NewLabel(nil, 0, "  ")
}
`)
	writeFile(t, filepath.Join(root, "dialog_test.go"), `package sample

func helper() {
	NewButton(nil, 0, "IgnoredBecauseItIsATest")
}
`)
	writeFile(t, filepath.Join(root, "tools", "gen.go"), `package main

func gen() {
	NewButton(nil, 0, "IgnoredBecauseItIsATool")
}
`)
	writeFile(t, filepath.Join(root, "third_party", "demo.go"), `package demo

func build() {
	NewButton(nil, 0, "IgnoredBecauseItIsThirdParty")
}
`)

	findings, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	got := make([]string, 0, len(findings))
	for _, f := range findings {
		if f.File != "dialog.go" {
			t.Errorf("unexpected file in findings: %s", f.File)
		}
		if f.Line == 0 {
			t.Errorf("finding %q has no line number", f.Literal)
		}
		got = append(got, f.Literal)
	}
	sort.Strings(got)

	want := []string{"Close", "Daily", "Menu", "Name:", "Never"}
	if len(got) != len(want) {
		t.Fatalf("Scan found %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Scan found %v, want %v", got, want)
		}
	}
}

func TestScanFindsWidenedCaptions(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "widgets.go"), `package sample

func build() {
	vtui.NewText(0, 0, "Ready", color)
	vtui.NewText(0, 0, Msg("Status.Ready"), color)
	vtui.NewGroupBox(0, 0, 10, 4, " Flags ")
	vtui.NewBaseWindow(0, 2, 10, 20, " Queue ")
	vtui.NewCenteredDialog(20, 8, " Prefix: "+name+" ")
	vtui.NewListBox(0, 0, 10, 4, []string{"First", Msg("List.Second")})
	vtui.NewMenuBar([]string{"&File"})
	menu.AddItem(vtui.MenuItem{Text: fmt.Sprintf("Unmount all (%d)", n), UserData: -1})
	menu.AddItem(vtui.MenuItem{Text: fmt.Sprintf(Msg("Mounts.UnmountAll"), n)})
	vtui.NewTable(0, 0, 10, 4, []vtui.TableColumn{
		{Title: "Size", Width: 10},
		{Title: Msg("FindFile.ColPath"), Width: 38},
	})
	win.SetTitle(" Attributes ")
	vtui.NewLabel(0, 0, padLabel("O&ct:"), edit)
	vtui.NewLabel(0, 0, fmt.Sprintf("%s", value), nil)
	vtui.NewEdit(0, 0, 20, "SELECT 1")
}
`)

	findings, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	got := make([]string, 0, len(findings))
	for _, f := range findings {
		got = append(got, f.Literal)
	}
	sort.Strings(got)

	want := []string{
		" Attributes ", " Flags ", " Prefix: ", " Queue ", "&File",
		"First", "O&ct:", "Ready", "Size", "Unmount all (%d)",
	}
	if len(got) != len(want) {
		t.Fatalf("Scan found %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Scan found %v, want %v", got, want)
		}
	}
}

func TestBaselineRoundTrip(t *testing.T) {
	findings := []Finding{
		{File: "b.go", Line: 3, Func: "NewButton", Literal: "Two\tTabbed"},
		{File: "a.go", Line: 1, Func: "NewLabel", Literal: "One"},
		{File: "a.go", Line: 9, Func: "NewLabel", Literal: "One"},
	}

	ids := IDs(findings)
	if len(ids) != 2 {
		t.Fatalf("IDs did not de-duplicate: %v", ids)
	}
	if ids[0] >= ids[1] {
		t.Fatalf("IDs are not sorted: %v", ids)
	}

	path := filepath.Join(t.TempDir(), "baseline.txt")
	if err := WriteBaseline(path, findings); err != nil {
		t.Fatalf("WriteBaseline failed: %v", err)
	}

	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("LoadBaseline returned %d entries, want 2", len(loaded))
	}
	for _, id := range ids {
		if !loaded[id] {
			t.Errorf("baseline lost entry %q", id)
		}
	}
}

func TestLoadBaselineReportsMissingFile(t *testing.T) {
	_, err := LoadBaseline(filepath.Join(t.TempDir(), "absent.txt"))
	if !os.IsNotExist(err) {
		t.Fatalf("LoadBaseline returned %v, want a not-exist error", err)
	}
}
