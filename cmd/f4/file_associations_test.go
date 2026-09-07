package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMatchMask_BasicGlob(t *testing.T) {
	cases := []struct {
		name string
		mask string
		want bool
	}{
		{"pic.png", "*.png", true},
		{"pic.PNG", "*.png", false}, // case-sensitive by default
		{"pic.jpg", "*.png", false},
		{"pic.png", "*.png,*.jpg", true},
		{"pic.jpg", "*.png,*.jpg", true},
		{"pic.gif", "*.png,*.jpg", false},
		{"pic.png", "*.png;*.jpg", true},   // ';' also works as OR
		{"pic.jpg", "*.png ; *.jpg", true}, // whitespace tolerated
		{"README", "*.*", true},            // *.* is "match anything"
		{"anything", "*", true},
		{"", "*.png", false},
		{"pic.png", "", false},
	}
	for _, c := range cases {
		got := MatchMask(c.name, c.mask, false)
		if got != c.want {
			t.Errorf("MatchMask(%q, %q, false) = %v, want %v", c.name, c.mask, got, c.want)
		}
	}
}

func TestMatchMask_IgnoreCase(t *testing.T) {
	if !MatchMask("PIC.PNG", "*.png", true) {
		t.Error("expected case-insensitive match")
	}
	if !MatchMask("pic.png", "*.PNG", true) {
		t.Error("expected case-insensitive match (mask upper, name lower)")
	}
}

func TestMatchMask_Exclude(t *testing.T) {
	// Include OR-list, exclude vetoes.
	if !MatchMask("pic.png", "*.png,*.jpg", false) {
		t.Fatal("include should match without exclude")
	}
	if MatchMask("thumb_pic.png", "*.png,*.jpg|thumb_*", false) {
		t.Error("exclude thumb_* should veto thumb_pic.png")
	}
	if !MatchMask("normal.png", "*.png,*.jpg|thumb_*", false) {
		t.Error("normal.png should survive the exclude filter")
	}
	if MatchMask("weird.png", "*.png|*.png", false) {
		t.Error("exclude equal to include should veto")
	}
}

func TestMatchMask_Regex(t *testing.T) {
	// /^Makefile.*$/ matches names starting with Makefile.
	if !MatchMask("Makefile", "/^Makefile.*$/", false) {
		t.Error("regex should match Makefile")
	}
	if !MatchMask("Makefile.in", "/^Makefile.*$/", false) {
		t.Error("regex should match Makefile.in")
	}
	if MatchMask("something-else", "/^Makefile.*$/", false) {
		t.Error("regex should not match something-else")
	}
	// Regex with commas stays a single mask when wrapped in slashes.
	if !MatchMask("v1_2", "/v\\d+_\\d+/", false) {
		t.Error("regex with commas or backrefs must survive the splitter")
	}
	// Ignore-case in regex mode via (?i) prefix.
	if !MatchMask("MAKEFILE", "/^makefile$/", true) {
		t.Error("regex + ignoreCase should match uppercase")
	}
	// Bad regex never matches — no panic, no crash.
	if MatchMask("x", "/[/", false) {
		t.Error("malformed regex should return false, not panic")
	}
}

func TestSaveLoadAssociations_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "associations.ini")

	in := []FileAssoc{
		{
			Mask:        "*.png,*.jpg",
			Description: "Image viewer",
			Commands: [assocKindCount]string{
				AssocExecute: `eog "!.!"`,
				AssocView:    `feh "!.!"`,
			},
			Enabled: [assocKindCount]bool{
				AssocExecute: true,
				AssocView:    true,
			},
		},
		{
			Mask:        "*.md",
			Description: "Markdown",
			Commands: [assocKindCount]string{
				AssocEdit: "vim !.!",
			},
			Enabled: [assocKindCount]bool{AssocEdit: true},
		},
	}

	if err := SaveAssociations(path, in); err != nil {
		t.Fatalf("SaveAssociations: %v", err)
	}

	// File must exist on disk (atomic tmp+rename actually landed).
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist after save: %v", path, err)
	}

	out, err := LoadAssociations(path)
	if err != nil {
		t.Fatalf("LoadAssociations: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("round-trip mismatch\n in: %+v\nout: %+v", in, out)
	}
}

func TestLoadAssociations_MissingFile(t *testing.T) {
	out, err := LoadAssociations(filepath.Join(t.TempDir(), "nope.ini"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("missing file should yield empty slice, got %d entries", len(out))
	}
}

func TestLoadAssociations_StopsAtFirstGap(t *testing.T) {
	// far2l's loader stops at the first Type%d section it can't find,
	// even if later sections exist. Mirror that behaviour so a manually
	// hand-edited file that deletes Type1 doesn't quietly load Type2+.
	dir := t.TempDir()
	path := filepath.Join(dir, "associations.ini")
	content := `[Associations/Type0]
Mask=*.txt
Description=Text
Execute=cat !.!
State=1

[Associations/Type2]
Mask=*.md
Description=Markdown
Execute=less !.!
State=1
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := LoadAssociations(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 entry (stops at Type1 gap), got %d", len(out))
	}
	if out[0].Mask != "*.txt" {
		t.Errorf("expected Type0 loaded, got %q", out[0].Mask)
	}
}

func TestLoadAssociations_StatePreservesEnabledBits(t *testing.T) {
	// State=5 = bits 0 (Execute) and 2 (View).
	dir := t.TempDir()
	path := filepath.Join(dir, "associations.ini")
	content := `[Associations/Type0]
Mask=*.png
Description=Image
Execute=eog !.!
AltExec=
View=feh !.!
AltView=
Edit=
AltEdit=
State=5
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := LoadAssociations(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	got := out[0].Enabled
	want := [assocKindCount]bool{
		AssocExecute: true,
		AssocView:    true,
	}
	if got != want {
		t.Errorf("Enabled=%v, want %v", got, want)
	}
}

func TestMatchingAssociations_SkipsDisabledAndEmpty(t *testing.T) {
	list := []FileAssoc{
		{
			Mask:     "*.png",
			Commands: [assocKindCount]string{AssocView: "feh"},
			Enabled:  [assocKindCount]bool{AssocView: true},
		},
		{
			// Enabled but empty command — must be skipped.
			Mask:    "*.png",
			Enabled: [assocKindCount]bool{AssocView: true},
		},
		{
			// Command present but disabled — must be skipped.
			Mask:     "*.png",
			Commands: [assocKindCount]string{AssocView: "eog"},
		},
	}
	got := MatchingAssociations(list, "pic.png", AssocView)
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}
	if got[0].Commands[AssocView] != "feh" {
		t.Errorf("expected feh match, got %q", got[0].Commands[AssocView])
	}
}

func TestMatchingAssociations_OrderPreserved(t *testing.T) {
	list := []FileAssoc{
		{
			Mask:        "*.png",
			Description: "First",
			Commands:    [assocKindCount]string{AssocView: "feh"},
			Enabled:     [assocKindCount]bool{AssocView: true},
		},
		{
			Mask:        "*.png",
			Description: "Second",
			Commands:    [assocKindCount]string{AssocView: "eog"},
			Enabled:     [assocKindCount]bool{AssocView: true},
		},
	}
	got := MatchingAssociations(list, "pic.png", AssocView)
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	if got[0].Description != "First" || got[1].Description != "Second" {
		t.Errorf("order not preserved: %+v", got)
	}
}
