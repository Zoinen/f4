package theme

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestFileHighlighter_CombineRules_ThemeWinsPriority covers the
// HighlightPriority==1 ("Theme wins") branch of CombineRules, which every
// other test in this package leaves untouched because they all rely on the
// zero-value default (0, "User wins"). matchedRules stops at the first
// matching rule when ContinueProcessing is unset, so whichever ruleset
// CombineRules places first in fh.Rules is the one that actually paints the
// file -- that ordering is exactly what this test checks.
func TestFileHighlighter_CombineRules_ThemeWinsPriority(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	oldCfg := config.App
	config.App.HighlightPriority = 1 // Theme wins
	defer func() { config.App = oldCfg }()

	highlighter := &FileHighlighter{}
	highlighter.LoadThemeRules(ini.Parse(strings.NewReader(`[Highlight_0]
Name = ThemeGreen
Mask = *
NormalColor = foreground:#00FF00
`)))
	highlighter.LoadUserRules(ini.Parse(strings.NewReader(`[Highlight_0]
Name = UserRed
Mask = *
NormalColor = foreground:#FF0000
`)))

	item := vfs.VFSItem{Name: "any.txt"}
	fg := vtui.GetRGBFore(highlighter.GetColor(&item, 0, false, false))
	if fg != 0x00FF00 {
		t.Fatalf("GetColor foreground = %06X, want the theme rule's 00FF00 (HighlightPriority=1 must place ThemeRules first)", fg)
	}
}

// TestFileHighlighter_MatchedRulesCached_BypassesCacheForVolatileRules
// covers matchedRulesCached's early return when fh.hasVolatileRules is
// true. TestFileHighlighter_RelativeDateFiltering already exercises a
// volatile (DateRelative) rule, but only through HighlightRule.Match
// directly -- it never calls GetMarker/GetColor, so it never reaches
// matchedRulesCached at all. This test drives the same kind of rule through
// GetMarker, which is the only caller that can actually take the bypass
// branch.
func TestFileHighlighter_MatchedRulesCached_BypassesCacheForVolatileRules(t *testing.T) {
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini.Parse(strings.NewReader(`[Highlight_0]
Name = RecentlyModified
Mask = *
Mark = V
DateRelative = 1
DateAfter = 1d
`)))

	if !highlighter.hasVolatileRules {
		t.Fatal("hasVolatileRules = false, want true for a DateRelative rule with DateAfterDur set")
	}

	item := vfs.VFSItem{Name: "fresh.txt", MTime: time.Now()}
	if marker := highlighter.GetMarker(&item); marker != "V" {
		t.Fatalf("GetMarker = %q, want %q (volatile rule must still match through the uncached path)", marker, "V")
	}
}

// TestFileHighlighter_MatchCache_EvictsAtCapacity covers matchedRulesCached
// dropping the whole cache once it reaches highlightCacheMaxEntries. Feeding
// it one more distinct key than the cap holds must reset the map to just
// that one new entry, rather than growing past the cap or panicking.
func TestFileHighlighter_MatchCache_EvictsAtCapacity(t *testing.T) {
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini.Parse(strings.NewReader(`[Highlight_0]
Name = All
Mask = *
Mark = *
`)))

	for i := 0; i < highlightCacheMaxEntries+1; i++ {
		item := vfs.VFSItem{Name: fmt.Sprintf("file%d.txt", i)}
		if marker := highlighter.GetMarker(&item); marker != "*" {
			t.Fatalf("GetMarker(file%d) = %q, want %q", i, marker, "*")
		}
	}

	if got := len(highlighter.matchCache); got != 1 {
		t.Fatalf("matchCache has %d entries after exceeding the cap by one, want 1 (cap hit must reset the map before inserting the new key)", got)
	}
}

// TestHighlightRule_ParseRuleSections_MaskDotStarNormalized covers
// ParseRuleSections rewriting the literal mask "*.*" to "*". Unlike "*",
// filepath.Match requires a literal dot for "*.*", so a file with no
// extension at all only matches once the rewrite has happened -- this test
// fails if ParseRuleSections ever stops normalizing it.
func TestHighlightRule_ParseRuleSections_MaskDotStarNormalized(t *testing.T) {
	rules := ParseHighlightRules(ini.Parse(strings.NewReader(`[Highlight_0]
Mask = *.*
`)))
	if len(rules) != 1 {
		t.Fatalf("ParseHighlightRules returned %d rules, want 1", len(rules))
	}
	if got := rules[0].Masks; len(got) != 1 || got[0] != "*" {
		t.Fatalf("Masks = %v, want [\"*\"] (\"*.*\" must be normalized to \"*\")", got)
	}

	item := vfs.VFSItem{Name: "README"} // no dot at all
	if !rules[0].Match(&item) {
		t.Error("rule with Mask=*.* did not match an extensionless file; the mask was not normalized to \"*\"")
	}
}

// TestHighlightRule_ParseRuleSections_DateTypeStrings covers the DateType
// ini string aliases ("create"/"created"/"c" and "access"/"accessed"/"a")
// in ParseRuleSections's switch. TestHighlightRule_DateTypes elsewhere in
// this package only ever sets HighlightRule.DateType directly on the Go
// struct, never through ini parsing, so that switch itself was never run.
func TestHighlightRule_ParseRuleSections_DateTypeStrings(t *testing.T) {
	tests := []struct {
		value string
		want  DateType
	}{
		{"create", DateCreated},
		{"created", DateCreated},
		{"c", DateCreated},
		{"access", DateAccessed},
		{"accessed", DateAccessed},
		{"a", DateAccessed},
	}
	for _, tt := range tests {
		iniData := fmt.Sprintf("[Highlight_0]\nDateType = %s\n", tt.value)
		rules := ParseHighlightRules(ini.Parse(strings.NewReader(iniData)))
		if len(rules) != 1 {
			t.Fatalf("DateType=%q: ParseHighlightRules returned %d rules, want 1", tt.value, len(rules))
		}
		if rules[0].DateType != tt.want {
			t.Errorf("DateType=%q: parsed DateType = %v, want %v", tt.value, rules[0].DateType, tt.want)
		}
	}
}

// TestHighlightRule_ParseRuleSections_DateBefore covers the DateBefore side
// of ParseRuleSections, both the DateRelative ("Nd") and absolute
// ("YYYY-MM-DD HH:MM:SS") forms. Every existing test in this package only
// ever sets DateAfter; DateBefore's parsing branches (relative and
// absolute) were never exercised at all.
func TestHighlightRule_ParseRuleSections_DateBefore(t *testing.T) {
	relRules := ParseHighlightRules(ini.Parse(strings.NewReader(`[Highlight_0]
DateRelative = 1
DateBefore = 3d
`)))
	if len(relRules) != 1 {
		t.Fatalf("relative DateBefore: got %d rules, want 1", len(relRules))
	}
	if want := 3 * 24 * time.Hour; relRules[0].DateBeforeDur != want {
		t.Errorf("relative DateBefore=3d: DateBeforeDur = %v, want %v", relRules[0].DateBeforeDur, want)
	}

	absRules := ParseHighlightRules(ini.Parse(strings.NewReader(`[Highlight_0]
DateBefore = 2020-01-01 00:00:00
`)))
	if len(absRules) != 1 {
		t.Fatalf("absolute DateBefore: got %d rules, want 1", len(absRules))
	}
	want, err := time.Parse("2006-01-02 15:04:05", "2020-01-01 00:00:00")
	if err != nil {
		t.Fatalf("test setup: %v", err)
	}
	if !absRules[0].DateBefore.Equal(want) {
		t.Errorf("absolute DateBefore: parsed = %v, want %v", absRules[0].DateBefore, want)
	}
}

// TestHighlightRule_ParseRuleSections_RelativeDurationEdgeCases covers the
// two branches inside ParseRuleSections's parseDuration closure that
// TestFileHighlighter_RelativeDateFiltering's "2d" example never reaches:
// an invalid day count (the strconv.Atoi error path, silently ignored like
// every other malformed ini value in this parser) and a relative duration
// with no "d" suffix at all (which falls straight through to
// time.ParseDuration).
func TestHighlightRule_ParseRuleSections_RelativeDurationEdgeCases(t *testing.T) {
	invalid := ParseHighlightRules(ini.Parse(strings.NewReader(`[Highlight_0]
DateRelative = 1
DateAfter = xxd
`)))
	if len(invalid) != 1 {
		t.Fatalf("invalid duration: got %d rules, want 1", len(invalid))
	}
	if invalid[0].DateAfterDur != 0 {
		t.Errorf("DateAfter=xxd: DateAfterDur = %v, want 0 (invalid day count must be ignored)", invalid[0].DateAfterDur)
	}

	noSuffix := ParseHighlightRules(ini.Parse(strings.NewReader(`[Highlight_0]
DateRelative = 1
DateAfter = 90m
`)))
	if len(noSuffix) != 1 {
		t.Fatalf("no-suffix duration: got %d rules, want 1", len(noSuffix))
	}
	if want := 90 * time.Minute; noSuffix[0].DateAfterDur != want {
		t.Errorf("DateAfter=90m: DateAfterDur = %v, want %v (must fall through to time.ParseDuration)", noSuffix[0].DateAfterDur, want)
	}
}

// TestParseAttrFlags_ReadOnlyAndArchiveAliases covers the "readonly"/"ro"
// and "archive"/"arc" cases of parseAttrFlags's switch, the same way
// TestHighlightRule_MatchSymlinkAttribute already covers the symlink
// aliases by calling parseAttrFlags directly.
func TestParseAttrFlags_ReadOnlyAndArchiveAliases(t *testing.T) {
	parsed := parseAttrFlags("readonly,ro,archive,arc")
	if parsed&AttrReadOnly == 0 {
		t.Error("parseAttrFlags failed to parse readonly/ro aliases")
	}
	if parsed&AttrArchive == 0 {
		t.Error("parseAttrFlags failed to parse archive/arc aliases")
	}
}

// TestHighlightRule_MatchArchiveAttribute covers matchAttr's AttrArchive
// case inside HighlightRule.Match. isArchive is only ever derived from
// WinAttrs (FILE_ATTRIBUTE_ARCHIVE), so on every non-Windows platform it is
// always false; the CI matrix's Windows leg covers the true case via
// WinAttrs, so this test -- like TestHighlightRule_PlatformAttributes right
// above it in highlight_files_test.go -- checks whatever runtime.GOOS
// actually makes true.
func TestHighlightRule_MatchArchiveAttribute(t *testing.T) {
	ruleArchive := HighlightRule{AttrSet: AttrArchive}
	ruleNotArchive := HighlightRule{AttrClear: AttrArchive}

	var item vfs.VFSItem
	wantArchive := runtime.GOOS == "windows"
	if wantArchive {
		item = vfs.VFSItem{Name: "backup.zip", WinAttrs: 32} // FILE_ATTRIBUTE_ARCHIVE
	} else {
		item = vfs.VFSItem{Name: "backup.zip"}
	}

	if got := ruleArchive.Match(&item); got != wantArchive {
		t.Errorf("AttrSet: AttrArchive match = %v, want %v on %s", got, wantArchive, runtime.GOOS)
	}
	if got := ruleNotArchive.Match(&item); got == wantArchive {
		t.Errorf("AttrClear: AttrArchive match = %v, want %v on %s", got, !wantArchive, runtime.GOOS)
	}
}

// TestFileHighlighter_DateFiltering_Before covers the DateBefore side of
// HighlightRule.Match's date filter, both DateBeforeDur (relative) and
// DateBefore (absolute). TestFileHighlighter_DateFiltering and
// TestFileHighlighter_RelativeDateFiltering elsewhere in this package only
// ever exercise the DateAfter side.
func TestFileHighlighter_DateFiltering_Before(t *testing.T) {
	now := time.Now()

	ruleRelative := HighlightRule{
		DateRelative:  true,
		DateBeforeDur: 24 * time.Hour, // only files last touched more than a day ago
	}
	tests := []struct {
		item vfs.VFSItem
		want bool
	}{
		{vfs.VFSItem{Name: "just-now.txt", MTime: now}, false},
		{vfs.VFSItem{Name: "old.txt", MTime: now.Add(-72 * time.Hour)}, true},
	}
	for _, tt := range tests {
		if got := ruleRelative.Match(&tt.item); got != tt.want {
			t.Errorf("relative DateBefore filter failed for %s: got %v, want %v", tt.item.Name, got, tt.want)
		}
	}

	ruleAbsolute := HighlightRule{
		DateBefore: now.Add(-24 * time.Hour),
	}
	absTests := []struct {
		item vfs.VFSItem
		want bool
	}{
		{vfs.VFSItem{Name: "new.txt", MTime: now}, false},
		{vfs.VFSItem{Name: "old.txt", MTime: now.Add(-48 * time.Hour)}, true},
	}
	for _, tt := range absTests {
		if got := ruleAbsolute.Match(&tt.item); got != tt.want {
			t.Errorf("absolute DateBefore filter failed for %s: got %v, want %v", tt.item.Name, got, tt.want)
		}
	}
}

// TestFileHighlighter_GetColor_CursorColorApplied covers GetColor's
// isCursor&&!isSelected branch actually applying rule.CursorStr.
// TestFileHighlighter_CursorSemantics only checks the opposite: that the
// cursor color stays untouched when CursorColor is unset. This is the
// positive case, where it must be applied.
func TestFileHighlighter_GetColor_CursorColorApplied(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	// Contrast correction may nudge the foreground away from the exact
	// value CursorColor requested depending on the cursor background this
	// palette happens to use; disable it so this test stays about whether
	// CursorStr is applied at all, not about CorrectContrast's own math
	// (covered separately elsewhere in this package).
	oldCfg := config.App
	config.App.EnforceColorCorrection = false
	defer func() { config.App = oldCfg }()

	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini.Parse(strings.NewReader(`[Highlight_0]
Name = Executables
Mask = *.exe
CursorColor = foreground:#123456
`)))

	item := vfs.VFSItem{Name: "app.exe"}
	got := highlighter.GetColor(&item, vtui.Palette[ColPanelCursor], false, true)
	if fg := vtui.GetRGBFore(got); fg != 0x123456 {
		t.Fatalf("GetColor(isCursor=true, isSelected=false) foreground = %06X, want CursorColor 123456", fg)
	}
}
