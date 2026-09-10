package app

import (
	"github.com/unxed/f4/internal/panel"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestColors_HelpBoxOverrideReachesHelpViewFrame(t *testing.T) {
	oldPalette := append([]uint64(nil), vtui.Palette...)
	oldTheme := vtui.ThemePalette
	oldCfg := config.App
	t.Cleanup(func() {
		vtui.Palette = oldPalette
		vtui.ThemePalette = oldTheme
		config.App = oldCfg
	})

	config.App.EnforceColorCorrection = false
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	theme.InitColors(ini.Parse(strings.NewReader(`[farcolors]
Help.Box = foreground:#102030 | background:#405060
`)))

	Engine := vtui.NewHelpEngine(dialog.NewMemoryHelpVFS(map[string]string{}))
	Engine.AddTopic(&vtui.HelpTopic{Name: "Test", Lines: []string{"text"}})
	view := vtui.NewHelpView(Engine, "Test")
	view.SetPosition(0, 0, 30, 5)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(32, 7)
	view.Show(scr)

	if got, want := scr.GetCell(0, 0).Attributes, vtui.Palette[vtui.ColHelpBox]; got != want {
		t.Fatalf("Help.Box frame attribute = %#x, want %#x", got, want)
	}
}

// Help draws its scrollbar inside its own window, so it must not follow the
// shared Scrollbar key that is tuned for lists sitting on the dialog
// background (issue #261).
func TestColors_HelpScrollbarOverrideReachesHelpViewScrollbar(t *testing.T) {
	oldPalette := append([]uint64(nil), vtui.Palette...)
	oldTheme := vtui.ThemePalette
	oldCfg := config.App
	t.Cleanup(func() {
		vtui.Palette = oldPalette
		vtui.ThemePalette = oldTheme
		config.App = oldCfg
	})

	config.App.EnforceColorCorrection = false
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	theme.InitColors(ini.Parse(strings.NewReader(`[farcolors]
Scrollbar = foreground:#C0C0C0 | background:#0000A0
Help.Scrollbar = foreground:#102030 | background:#405060
`)))

	Engine := vtui.NewHelpEngine(dialog.NewMemoryHelpVFS(map[string]string{}))
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "help line"
	}
	Engine.AddTopic(&vtui.HelpTopic{Name: "Long", Lines: lines})
	view := vtui.NewHelpView(Engine, "Long")
	view.SetPosition(0, 0, 30, 8)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(32, 10)
	view.Show(scr)

	// The scrollbar runs down the right padding column of the help window.
	cell := scr.GetCell(28, 1)
	if cell.Char != vtui.ScrollUpArrow {
		t.Fatalf("no scrollbar drawn at the right padding column: got %#x", cell.Char)
	}
	if got, want := cell.Attributes, vtui.Palette[vtui.ColHelpScrollbar]; got != want {
		t.Fatalf("help scrollbar attribute = %#x, want %#x", got, want)
	}
	if cell.Attributes == vtui.Palette[vtui.ColScrollBar] {
		t.Fatal("help scrollbar still follows the shared Scrollbar color")
	}
}

func TestFileEntry_HighlightIntegration(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

	oldConfig := config.App
	defer func() { config.App = oldConfig }()
	config.App.ShowHighlightMarks = true

	// Загружаем тестовые правила в глобальный объект подсветки
	iniData := `[Highlight_0]
Name = TestGo
Mask = *.go
Mark = •
NormalColor = foreground:#00FF00
`
	ini := ini.Parse(strings.NewReader(iniData))
	theme.GlobalFileHighlighter.LoadFromIni(ini)

	// Создаем тестовую структуру файла панели
	entry := &panel.FileEntry{
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
