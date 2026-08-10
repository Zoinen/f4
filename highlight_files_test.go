package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestSemanticHighlightStyleAndIconURLs(t *testing.T) {
	baseDir := t.TempDir()
	iconPath := filepath.Join(baseDir, "custom.svg")
	if err := os.WriteFile(iconPath, []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	ini := ParseIni(strings.NewReader(`
[Highlight_0]
Name = Source
Mask = *.go
Icon = file:custom.svg
NormalColor = foreground:#112233
SelectedColor = background:#445566
ContinueProcessing = 1
[Highlight_1]
Name = Generic
Mask = *
NormalColor = background:#778899
`))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIniAt(ini, baseDir)
	id, style := highlighter.SemanticStyle(&vfs.VFSItem{Name: "main.go"})
	if id == "" || highlighter.Revision == 0 {
		t.Fatal("semantic style and revision must be stable and non-empty")
	}
	if style.Icon != "file://"+iconPath {
		t.Fatalf("resolved icon = %q, want file://%s", style.Icon, iconPath)
	}
	if style.Normal.Foreground != "#112233" || style.Normal.Background != "#778899" {
		t.Fatalf("unexpected cascaded normal patch: %#v", style.Normal)
	}
	if style.Selected.Foreground != "" || style.Selected.Background != "#778899" {
		t.Fatalf("unexpected selected fallback/cascade: %#v", style.Selected)
	}
	if len(style.Groups) != 2 || style.Groups[0].Name != "Source" {
		t.Fatalf("unexpected groups: %#v", style.Groups)
	}
}

func TestSemanticHighlightParentIconAndRejectedSchemes(t *testing.T) {
	ini := ParseIni(strings.NewReader(`
[Highlight_0]
Name = Parent
Mask = ..
IncludeAttributes = Directory
Icon = qrc:/F4QtHost/icons/lucide/folder-up.svg
NormalColor = foreground:#ffffff
Mark = X
[Highlight_1]
Name = Network
Mask = *.net
Icon = https://example.invalid/file.svg
`))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)
	_, parent := highlighter.SemanticStyle(&vfs.VFSItem{Name: "..", IsDir: true})
	if parent.Icon != "qrc:/F4QtHost/icons/lucide/folder-up.svg" {
		t.Fatalf("parent icon = %q", parent.Icon)
	}
	if parent.Marker != "" || parent.Normal.Foreground != "" {
		t.Fatalf("parent must keep console color/marker special case: %#v", parent)
	}
	_, network := highlighter.SemanticStyle(&vfs.VFSItem{Name: "file.net"})
	if network.Icon != "" {
		t.Fatalf("network icon scheme was accepted: %q", network.Icon)
	}
}

func TestHighlightRule_Match(t *testing.T) {
	rule := HighlightRule{
		Masks:      []string{"*.go", "*.sh"},
		IgnoreCase: true,
	}

	tests := []struct {
		item vfs.VFSItem
		want bool
	}{
		{vfs.VFSItem{Name: "main.go"}, true},
		{vfs.VFSItem{Name: "SCRIPT.SH"}, true},
		{vfs.VFSItem{Name: "readme.txt"}, false},
	}

	for _, tt := range tests {
		if got := rule.Match(&tt.item); got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.item.Name, got, tt.want)
		}
	}
}

func TestHighlightRule_MatchAttributes(t *testing.T) {
	ruleDir := HighlightRule{
		AttrSet: AttrDirectory,
	}
	ruleExec := HighlightRule{
		AttrSet: AttrExecutable,
	}

	tests := []struct {
		item vfs.VFSItem
		rule HighlightRule
		want bool
	}{
		{vfs.VFSItem{Name: "dir", IsDir: true}, ruleDir, true},
		{vfs.VFSItem{Name: "file", IsDir: false}, ruleDir, false},
		{vfs.VFSItem{Name: "run.sh", IsExecutable: true}, ruleExec, true},
	}

	for _, tt := range tests {
		if got := tt.rule.Match(&tt.item); got != tt.want {
			t.Errorf("Rule Match on attributes failed for %q: got %v, want %v", tt.item.Name, got, tt.want)
		}
	}
}

func TestFileHighlighter_GetColor(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = false
	defer func() { AppConfig = oldCfg }()

	iniData := `[Highlight_0]
Name = Executables
Mask = *.exe, *.sh
NormalColor = foreground:#00FF00
SelectedColor = foreground:#00FF00 | background:#0000FF

[Highlight_1]
Name = Archives
Mask = *.zip
NormalColor = foreground:#FF00FF
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	tests := []struct {
		item       vfs.VFSItem
		defaultVal uint64
		selected   bool
		cursor     bool
		wantRGBFg  uint32
		wantRGBBg  uint32
	}{
		{
			item:       vfs.VFSItem{Name: "app.exe"},
			defaultVal: 0,
			selected:   false,
			cursor:     false,
			wantRGBFg:  0x00FF00,
			wantRGBBg:  0x000000,
		},
		{
			item:       vfs.VFSItem{Name: "app.exe"},
			defaultVal: 0,
			selected:   true,
			cursor:     false,
			wantRGBFg:  0x00FF00,
			wantRGBBg:  0x0000FF,
		},
		{
			item:       vfs.VFSItem{Name: "archive.zip"},
			defaultVal: 0,
			selected:   false,
			cursor:     false,
			wantRGBFg:  0xFF00FF,
			wantRGBBg:  0x000000,
		},
	}

	for _, tt := range tests {
		got := highlighter.GetColor(&tt.item, tt.defaultVal, tt.selected, tt.cursor)
		fg := vtui.GetRGBFore(got)
		bg := vtui.GetRGBBack(got)

		if fg != tt.wantRGBFg {
			t.Errorf("[%s] Fg mismatch: got %06X, want %06X", tt.item.Name, fg, tt.wantRGBFg)
		}
		if bg != tt.wantRGBBg {
			t.Errorf("[%s] Bg mismatch: got %06X, want %06X", tt.item.Name, bg, tt.wantRGBBg)
		}
	}
}

func TestFileHighlighter_RulesSortingAndPrecedence(t *testing.T) {
	// Проверяем, что правила сортируются по индексу независимо от порядка их объявления в INI.
	// Highlight_1 совпадает со всеми файлами, но Highlight_0 имеет приоритет выше.
	iniData := `[Highlight_1]
Name = Low Priority
Mask = *
NormalColor = foreground:#FF0000

[Highlight_0]
Name = High Priority
Mask = *.go
NormalColor = foreground:#00FF00
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	if len(highlighter.Rules) != 2 {
		t.Fatalf("Expected 2 rules loaded, got %d", len(highlighter.Rules))
	}

	item := vfs.VFSItem{Name: "main.go"}
	color := highlighter.GetColor(&item, 0, false, false)
	fg := vtui.GetRGBFore(color)

	if fg != 0x00FF00 {
		t.Errorf("Precedence error: expected higher priority Highlight_0 (0x00FF00), got %06X", fg)
	}
}

func TestFileHighlighter_AttributeExclusion(t *testing.T) {
	rule := HighlightRule{
		AttrSet:   AttrExecutable,
		AttrClear: AttrDirectory,
		Masks:     []string{"*"},
	}

	tests := []struct {
		item vfs.VFSItem
		want bool
	}{
		{vfs.VFSItem{Name: "run.sh", IsExecutable: true, IsDir: false}, true},
		{vfs.VFSItem{Name: "run.sh", IsExecutable: true, IsDir: true}, false}, // Исключено, так как это папка
	}

	for _, tt := range tests {
		if got := rule.Match(&tt.item); got != tt.want {
			t.Errorf("Match(%q) with exclusion failed: got %v, want %v", tt.item.Name, got, tt.want)
		}
	}
}

func TestFileHighlighter_CascadingColors(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	// Настраиваем правило, которое переопределяет только цвет текста (Foreground)
	iniData := `[Highlight_0]
Name = GreenText
Mask = *.txt
NormalColor = foreground:#00FF00
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	// В качестве базового цвета передаем синий фон (0x0000FF)
	defaultAttr := vtui.SetRGBBack(0, 0x0000FF)
	item := vfs.VFSItem{Name: "readme.txt"}

	got := highlighter.GetColor(&item, defaultAttr, false, false)
	fg := vtui.GetRGBFore(got)
	bg := vtui.GetRGBBack(got)

	if fg != 0x00FF00 {
		t.Errorf("Expected foreground to be updated to 0x00FF00, got %06X", fg)
	}
	if bg != 0x0000FF {
		t.Errorf("Expected background to preserve original color 0x0000FF, got %06X", bg)
	}
}

func TestFileHighlighter_SizeFiltering(t *testing.T) {
	ruleBig := HighlightRule{
		SizeAbove: 10 * 1024 * 1024, // > 10MB
	}
	ruleSmall := HighlightRule{
		SizeBelow: 1024, // < 1KB
	}

	tests := []struct {
		item vfs.VFSItem
		rule HighlightRule
		want bool
	}{
		{vfs.VFSItem{Name: "big.zip", Size: 20 * 1024 * 1024}, ruleBig, true},
		{vfs.VFSItem{Name: "mid.zip", Size: 5 * 1024 * 1024}, ruleBig, false},
		{vfs.VFSItem{Name: "tiny.txt", Size: 512}, ruleSmall, true},
		{vfs.VFSItem{Name: "normal.txt", Size: 2048}, ruleSmall, false},
	}

	for _, tt := range tests {
		if got := tt.rule.Match(&tt.item); got != tt.want {
			t.Errorf("Size filter failed for %s: got %v, want %v", tt.item.Name, got, tt.want)
		}
	}
}

func TestFileHighlighter_DateFiltering(t *testing.T) {
	now := time.Now()
	ruleNew := HighlightRule{
		DateAfter: now.Add(-24 * time.Hour), // Изменен в последние 24 часа
	}

	tests := []struct {
		item vfs.VFSItem
		want bool
	}{
		{vfs.VFSItem{Name: "new.txt", MTime: now.Add(-1 * time.Hour)}, true},
		{vfs.VFSItem{Name: "old.txt", MTime: now.Add(-48 * time.Hour)}, false},
	}

	for _, tt := range tests {
		if got := ruleNew.Match(&tt.item); got != tt.want {
			t.Errorf("Date filter failed for %s: got %v, want %v", tt.item.Name, got, tt.want)
		}
	}
}

func TestFileHighlighter_RelativeDateFiltering(t *testing.T) {
	// Имитируем загрузку относительной даты с поддержкой дней (например, "2d" назад)
	iniData := `[Highlight_0]
Name = RelativeNew
DateRelative = 1
DateAfter = 2d
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	now := time.Now()
	tests := []struct {
		item vfs.VFSItem
		want bool
	}{
		{vfs.VFSItem{Name: "fresh.txt", MTime: now.Add(-12 * time.Hour)}, true},
		{vfs.VFSItem{Name: "stale.txt", MTime: now.Add(-72 * time.Hour)}, false},
	}

	for _, tt := range tests {
		if got := highlighter.Rules[0].Match(&tt.item); got != tt.want {
			t.Errorf("Relative Date check failed for %s: got %v, want %v", tt.item.Name, got, tt.want)
		}
	}
}

func TestFileHighlighter_GetMarker(t *testing.T) {
	iniData := `[Highlight_0]
Name = Executables
Mask = *.sh
Mark = *

[Highlight_1]
Name = Dirs
IncludeAttributes = Directory
Mark = /
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	tests := []struct {
		item vfs.VFSItem
		want string
	}{
		{vfs.VFSItem{Name: "run.sh"}, "*"},
		{vfs.VFSItem{Name: "folder", IsDir: true}, "/"},
		{vfs.VFSItem{Name: "readme.txt"}, ""},
	}

	for _, tt := range tests {
		if got := highlighter.GetMarker(&tt.item); got != tt.want {
			t.Errorf("[%s] Expected marker %q, got %q", tt.item.Name, tt.want, got)
		}
	}
}

func TestFileHighlighter_ContinueProcessing(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	// Настройка двух правил:
	// 1. Изменяет цвет текста (foreground) в зеленый и разрешает каскад
	// 2. Изменяет цвет фона (background) в синий
	iniData := `[Highlight_0]
Name = GreenText
Mask = *.txt
NormalColor = foreground:#00FF00
ContinueProcessing = 1

[Highlight_1]
Name = BlueBackground
Mask = *
NormalColor = background:#0000FF
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	item := vfs.VFSItem{Name: "readme.txt"}
	got := highlighter.GetColor(&item, 0, false, false)

	fg := vtui.GetRGBFore(got)
	bg := vtui.GetRGBBack(got)

	if fg != 0x00FF00 {
		t.Errorf("Expected foreground blended from Highlight_0: %06X", fg)
	}
	if bg != 0x0000FF {
		t.Errorf("Expected background blended from Highlight_1: %06X", bg)
	}
}

func TestHighlightRule_PlatformAttributes(t *testing.T) {
	rule := HighlightRule{
		AttrSet: AttrReadOnly,
	}

	var item vfs.VFSItem
	if runtime.GOOS == "windows" {
		item = vfs.VFSItem{Name: "readonly.txt", WinAttrs: 1} // FILE_ATTRIBUTE_READONLY
	} else {
		item = vfs.VFSItem{Name: "readonly.txt", UnixMode: 0444} // Нет прав на запись (mask 0222)
	}

	if !rule.Match(&item) {
		t.Errorf("Platform-specific ReadOnly attribute matching failed on %s", runtime.GOOS)
	}
}

func TestFileEntry_HighlightIntegration(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	// Загружаем тестовые правила в глобальный объект подсветки
	iniData := `[Highlight_0]
Name = TestGo
Mask = *.go
Mark = •
NormalColor = foreground:#00FF00
`
	ini := ParseIni(strings.NewReader(iniData))
	GlobalFileHighlighter.LoadFromIni(ini)

	// Создаем тестовую структуру файла панели
	entry := &fileEntry{
		VFSItem: vfs.VFSItem{Name: "main.go", IsDir: false},
	}

	// 1. Проверяем интеграцию вывода имени файла с маркером
	text := entry.GetCellText(0)
	expectedText := "• main.go"
	if text != expectedText {
		t.Errorf("Marker integration in GetCellText failed: got %q, want %q", text, expectedText)
	}

	// 2. Проверяем интеграцию получения цвета
	attr := entry.GetCellAttr(0, 0)
	fg := vtui.GetRGBFore(attr)
	if fg != 0x00FF00 {
		t.Errorf("Color integration in GetCellAttr failed: got %06X, want 0x00FF00", fg)
	}
}

func TestFileHighlighter_ParentDirSkipped(t *testing.T) {
	iniData := `[Highlight_0]
Mask = *
Mark = *
NormalColor = foreground:#00FF00
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	item := vfs.VFSItem{Name: "..", IsDir: true}
	color := highlighter.GetColor(&item, 0, false, false)
	marker := highlighter.GetMarker(&item)

	if color != 0 {
		t.Error("Parent directory '..' should never be colored by highlighter")
	}
	if marker != "" {
		t.Error("Parent directory '..' should never receive a marker")
	}
}

func TestHighlightRule_DateTypes(t *testing.T) {
	now := time.Now()
	mtime := now.Add(-1 * time.Hour)
	ctime := now.Add(-5 * time.Hour)
	atime := now.Add(-10 * time.Hour)

	item := vfs.VFSItem{
		Name:  "dates.txt",
		MTime: mtime,
		CTime: ctime,
		ATime: atime,
	}

	// Должно отвалиться, т.к. доступ был 10 часов назад (лимит - 6 часов)
	ruleAccess := HighlightRule{
		DateType:  DateAccessed,
		DateAfter: now.Add(-6 * time.Hour),
	}

	// Должно пройти, т.к. создание было 5 часов назад (лимит - 6 часов)
	ruleCreate := HighlightRule{
		DateType:  DateCreated,
		DateAfter: now.Add(-6 * time.Hour),
	}

	if ruleAccess.Match(&item) {
		t.Error("DateAccessed matching failed (should have failed)")
	}
	if !ruleCreate.Match(&item) {
		t.Error("DateCreated matching failed (should have passed)")
	}
}

func TestFileHighlighter_GetColor_ContrastCorrection(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	oldCfg := AppConfig
	AppConfig.EnforceColorCorrection = true
	defer func() { AppConfig = oldCfg }()

	// Low contrast rule: Dark Gray on Black background
	iniData := `[Highlight_0]
Name = DarkOnBlack
Mask = *.txt
NormalColor = foreground:#111111 | background:#000000
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	item := vfs.VFSItem{Name: "doc.txt"}
	attr := highlighter.GetColor(&item, 0, false, false)

	fg := vtui.GetRGBFore(attr)
	bg := vtui.GetRGBBack(attr)

	dE := deltaE2000(rgbToLAB(toRGBF(fg)), rgbToLAB(toRGBF(bg)))
	if dE < 29.0 {
		t.Errorf("highlighter left deltaE2000 at %.2f, want the ~30 far2l aims for", dE)
	}
}
func TestFileHighlighter_CursorSemantics(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	iniData := `[Highlight_0]
Name = Executables
Mask = *.exe
NormalColor = foreground:#00FF00
`
	ini := ParseIni(strings.NewReader(iniData))
	highlighter := &FileHighlighter{}
	highlighter.LoadFromIni(ini)

	// Item under cursor (default cursor attribute is ColPanelCursor)
	cursorAttr := vtui.Palette[ColPanelCursor]
	item := vfs.VFSItem{Name: "app.exe"}

	got := highlighter.GetColor(&item, cursorAttr, false, true)

	// Since the rule has no CursorColor defined, it must remain the default cursorAttr!
	if got != cursorAttr {
		t.Errorf("Expected cursor color to be untouched, got %X, want %X", got, cursorAttr)
	}
}

func TestFileHighlighterCachesMatchesByEntryMetadata(t *testing.T) {
	highlighter := &FileHighlighter{
		Revision: 1,
		Rules: []HighlightRule{{
			Masks:      []string{"*.go"},
			IgnoreCase: true,
			NormalStr:  "foreground:#00FF00",
		}},
	}
	item := vfs.VFSItem{Name: "main.go", Size: 10}
	if _, style := highlighter.SemanticStyle(&item); highlightStyleEmpty(style) {
		t.Fatal("matching item did not produce a style")
	}
	if got := len(highlighter.matchCache); got != 1 {
		t.Fatalf("match cache entries = %d, want 1", got)
	}
	highlighter.GetColor(&item, 0, false, false)
	highlighter.GetMarker(&item)
	if got := len(highlighter.matchCache); got != 1 {
		t.Fatalf("shared consumers recomputed the same entry: cache entries = %d", got)
	}

	item.Name = "README.txt"
	if _, style := highlighter.SemanticStyle(&item); !highlightStyleEmpty(style) {
		t.Fatal("changed item metadata reused a stale cached match")
	}
	if got := len(highlighter.matchCache); got != 2 {
		t.Fatalf("changed metadata did not create a distinct cache entry: %d", got)
	}
}
