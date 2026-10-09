package app

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/vtui"
)

func TestSheetNameAndPathHelpers(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{path: "book.f4s.sqlite", want: "book"},
		{path: "BOOK.F4S.SQLITE", want: "BOOK"},
		{path: "book.xlsx", want: "book"},
		{path: "book", want: "book"},
		{path: filepath.Join("dir.name", "book.csv"), want: filepath.Join("dir.name", "book")},
	}
	for _, tc := range cases {
		if got := sheetBaseName(tc.path); got != tc.want {
			t.Errorf("sheetBaseName(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}

	dir := t.TempDir()
	if got, want := sheetPathIn(dir, " report.csv "), filepath.Join(dir, "report.csv"); got != want {
		t.Fatalf("sheetPathIn relative = %q, want %q", got, want)
	}
	absolute := filepath.Join(dir, "absolute.csv")
	if got := sheetPathIn(dir, absolute); got != absolute {
		t.Fatalf("sheetPathIn absolute = %q, want %q", got, absolute)
	}
	if got := sheetPathIn(dir, ""); got != "" {
		t.Fatalf("sheetPathIn empty = %q, want empty", got)
	}
	if got := sheetPathIn("", "relative.csv"); !filepath.IsAbs(got) {
		t.Fatalf("sheetPathIn without directory = %q, want absolute", got)
	}
}

func TestSheetExportExtensions(t *testing.T) {
	for selected, want := range map[int]string{0: ".txt", 1: ".csv", 2: ".xlsx", 3: ".txt", -1: ".txt"} {
		if got := exportExtension(selected); got != want {
			t.Errorf("exportExtension(%d) = %q, want %q", selected, got, want)
		}
	}
}

func TestCompareLayoutHelpers(t *testing.T) {
	caption := "&Compare"
	wantCaption := 3 + 4 + vtui.StringWidth(action.PlainLabel(caption))
	if got := compareCaptionWidth(3, caption); got != wantCaption {
		t.Fatalf("compareCaptionWidth = %d, want %d", got, wantCaption)
	}
	items := []string{"short", "a longer caption"}
	wantRadio := 5 + 6 + vtui.StringWidth(action.PlainLabel(items[1]))
	if got := compareRadioWidth(5, items); got != wantRadio {
		t.Fatalf("compareRadioWidth = %d, want %d", got, wantRadio)
	}
	if compareCheckState(false) != 0 || compareCheckState(true) != 1 {
		t.Fatal("compareCheckState did not map booleans to checkbox states")
	}
}
