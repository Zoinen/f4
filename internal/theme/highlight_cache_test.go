package theme

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestGetColorAndGetMarkerCacheUnchangedFile is part of the fix for #884.
// The reporter came back after PR #1511 (which cached the menu bar) with a
// fresh CPU profile still showing lag proportional to SSH latency, and
// specifically noted it gets worse the more files are on the panels. That
// profile put path/filepath.Match at 75% cumulative CPU, called from
// HighlightRule.Match by way of FileHighlighter.GetMarker/GetColor: every
// visible row re-ran every configured highlight rule's mask against
// filepath.Match on every single render frame, even though neither the
// file's own attributes nor the active ruleset had changed since the
// previous frame.
//
// This test stands filepath.Match in for a counter via the filepathMatchFn
// seam (the same substitution pattern internal/panel/menu_cache_test.go uses
// for BuildMenuBarItems), and checks that calling GetMarker and GetColor
// repeatedly for the same unchanged file does not call filepath.Match again
// after the first time.
func TestGetColorAndGetMarkerCacheUnchangedFile(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	oldMatch := filepathMatchFn
	calls := 0
	filepathMatchFn = func(pattern, name string) (bool, error) {
		calls++
		return filepath.Match(pattern, name)
	}
	t.Cleanup(func() { filepathMatchFn = oldMatch })

	iniData := `[Highlight_0]
Name = Sources
Mask = *.[g]o
Mark = *
NormalColor = foreground:#00FF00
`
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini.Parse(strings.NewReader(iniData)))

	item := vfs.VFSItem{Name: "main.go", Size: 42}

	if marker := highlighter.GetMarker(&item); marker != "*" {
		t.Fatalf("GetMarker = %q, want %q", marker, "*")
	}
	callsAfterFirstMarker := calls
	if callsAfterFirstMarker == 0 {
		t.Fatal("GetMarker never invoked filepath.Match at all")
	}

	color := highlighter.GetColor(&item, 0, false, false)
	fg := vtui.GetRGBFore(color)
	if fg != 0x00FF00 {
		t.Fatalf("GetColor foreground = %06X, want 00FF00", fg)
	}
	// GetColor consults the very same matched-rules trace GetMarker just
	// computed and cached for this exact file, so it must not have run
	// filepath.Match again.
	if calls != callsAfterFirstMarker {
		t.Fatalf("GetColor after GetMarker on the same unchanged file ran filepath.Match %d more time(s), want a cache hit (0 more)", calls-callsAfterFirstMarker)
	}

	// Repeat both calls: still nothing changed, still a cache hit. The rule
	// only sets NormalColor, so a cursor+selected query legitimately falls
	// back to the caller's default attribute (0) rather than reusing
	// NormalColor — that is unrelated to caching, so use the same normal
	// state here to keep this test about the cache alone.
	if marker := highlighter.GetMarker(&item); marker != "*" {
		t.Fatalf("second GetMarker = %q, want %q", marker, "*")
	}
	if color := highlighter.GetColor(&item, 0, false, false); vtui.GetRGBFore(color) != 0x00FF00 {
		t.Fatalf("second GetColor foreground = %06X, want 00FF00", vtui.GetRGBFore(color))
	}
	if calls != callsAfterFirstMarker {
		t.Fatalf("repeated GetMarker/GetColor on the same unchanged file ran filepath.Match %d more time(s), want a cache hit (0 more)", calls-callsAfterFirstMarker)
	}
}

// TestHighlightCacheInvalidatesOnThemeReload covers the risk called out
// explicitly for #884: a stale cache serving the wrong marker/color after a
// real change would be worse than the original slowness. Reloading the
// ruleset (LoadThemeRules/LoadUserRules -> CombineRules, exactly what a
// theme switch or an INI reload does) must produce a fresh, correct result
// for the very same file, not the old cached one.
func TestHighlightCacheInvalidatesOnThemeReload(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	oldCfg := config.App
	config.App.EnforceColorCorrection = false
	defer func() { config.App = oldCfg }()

	// Everything comes from ThemeRules (as a real theme switch would set
	// it) so the second LoadThemeRules below is the only rule in play, with
	// nothing from UserRules left over to shadow it.
	highlighter := &FileHighlighter{}
	highlighter.LoadThemeRules(ini.Parse(strings.NewReader(`[Highlight_0]
Name = Sources
Mask = *.go
Mark = *
NormalColor = foreground:#00FF00
`)))

	item := vfs.VFSItem{Name: "main.go"}

	if marker := highlighter.GetMarker(&item); marker != "*" {
		t.Fatalf("GetMarker before reload = %q, want %q", marker, "*")
	}
	if fg := vtui.GetRGBFore(highlighter.GetColor(&item, 0, false, false)); fg != 0x00FF00 {
		t.Fatalf("GetColor before reload foreground = %06X, want 00FF00", fg)
	}

	// A theme switch loads a new set of theme rules and recombines. The
	// same file's mask still matches *.go, but the mark and colour it
	// resolves to must come from the new rule, not from whatever the cache
	// remembers from before.
	highlighter.LoadThemeRules(ini.Parse(strings.NewReader(`[Highlight_0]
Name = SourcesReloaded
Mask = *.go
Mark = #
NormalColor = foreground:#FF00FF
`)))

	if marker := highlighter.GetMarker(&item); marker != "#" {
		t.Fatalf("GetMarker after theme reload = %q, want %q (stale cache would return the old mark)", marker, "#")
	}
	if fg := vtui.GetRGBFore(highlighter.GetColor(&item, 0, false, false)); fg != 0xFF00FF {
		t.Fatalf("GetColor after theme reload foreground = %06X, want FF00FF (stale cache would return the old colour)", fg)
	}
}

// TestHighlightCacheInvalidatesOnAttributeChange covers the other real
// change #884 called out: a file that actually changes (renamed, or a real
// refresh picks up new attributes) must not keep showing a color/marker
// resolved for its old state, just because a VFSItem with the old name or
// attributes once passed through this exact FileHighlighter.
func TestHighlightCacheInvalidatesOnAttributeChange(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	oldCfg := config.App
	config.App.EnforceColorCorrection = false
	defer func() { config.App = oldCfg }()

	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini.Parse(strings.NewReader(`[Highlight_0]
Name = BigFiles
SizeAbove = 1000
Mark = !
NormalColor = foreground:#FF0000

[Highlight_1]
Name = SmallFiles
SizeBelow = 999
Mark = .
NormalColor = foreground:#00FF00
`)))

	item := vfs.VFSItem{Name: "data.bin", Size: 2000}
	if marker := highlighter.GetMarker(&item); marker != "!" {
		t.Fatalf("GetMarker for big file = %q, want %q", marker, "!")
	}
	if fg := vtui.GetRGBFore(highlighter.GetColor(&item, 0, false, false)); fg != 0xFF0000 {
		t.Fatalf("GetColor for big file foreground = %06X, want FF0000", fg)
	}

	// The very same VFSItem is mutated in place, exactly as a real refresh
	// (re-stat) would update it: same name, new size. The cache key must
	// follow the size, not just the name, or this would keep reporting the
	// stale "big file" result forever.
	item.Size = 10
	if marker := highlighter.GetMarker(&item); marker != "." {
		t.Fatalf("GetMarker for the same file after shrinking = %q, want %q (stale cache would return %q)", marker, ".", "!")
	}
	if fg := vtui.GetRGBFore(highlighter.GetColor(&item, 0, false, false)); fg != 0x00FF00 {
		t.Fatalf("GetColor for the same file after shrinking foreground = %06X, want 00FF00 (stale cache would return FF0000)", fg)
	}

	// And an mtime-only change (a real refresh with the same size) must
	// also be picked up, not just Size.
	highlighter2 := &FileHighlighter{}
	highlighter2.LoadFromIni(ini.Parse(strings.NewReader(`[Highlight_0]
Name = Recent
DateAfter = 2999-01-01 00:00:00
Mark = N
NormalColor = foreground:#0000FF
`)))
	old := vfs.VFSItem{Name: "note.txt", MTime: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)}
	if marker := highlighter2.GetMarker(&old); marker != "" {
		t.Fatalf("GetMarker for old mtime = %q, want no match", marker)
	}
	fresh := old
	fresh.MTime = time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)
	if marker := highlighter2.GetMarker(&fresh); marker != "N" {
		t.Fatalf("GetMarker after mtime changed = %q, want %q (stale cache would return no match)", marker, "N")
	}
}
