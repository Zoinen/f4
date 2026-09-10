package app

import (
	"github.com/unxed/f4/internal/panel"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/gui"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// This one stayed behind when the font catalogue left: it drives the appearance
// dialog through a real panels frame, which is what it is actually about.

func TestAppearanceSettingsFontComboRemainsEditable(t *testing.T) {
	previous := gui.DiscoverInstalledGuiFonts
	gui.DiscoverInstalledGuiFonts = func(string) []string { return []string{"/fonts/NotoSansCJK.ttc", "/fonts/Mono.ttf"} }
	t.Cleanup(func() { gui.DiscoverInstalledGuiFonts = previous })

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	oldConfig := config.App
	oldPath := config.GetUserConfigIniPath
	config.App.GuiFont = "/custom/font.otf"
	config.App.Language = "zh"
	config.GetUserConfigIniPath = func() string { return t.TempDir() + "/settings.ini" }
	t.Cleanup(func() {
		config.App = oldConfig
		config.GetUserConfigIniPath = oldPath
	})

	pf := panel.NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	ActionAppearanceSettings(pf)
	top := vtui.FrameManager.GetTopFrame().(vtui.Container)

	var fontCombo *vtui.ComboBox
	for _, child := range top.GetChildren() {
		combo, ok := child.(*vtui.ComboBox)
		if !ok || len(combo.Menu.Items) == 0 {
			continue
		}
		if combo.Menu.Items[0].Text == "/custom/font.otf" {
			fontCombo = combo
			break
		}
	}
	if fontCombo == nil {
		t.Fatal("font catalog combobox not found")
	}
	if fontCombo.DropdownOnly {
		t.Fatal("font catalog combobox must preserve manual entry")
	}
	fontCombo.Edit.SetText("/manually/entered/font.ttf")
	testutil.ClickDialogButton(t, top, "Ok")
	if config.App.GuiFont != "/manually/entered/font.ttf" {
		t.Fatalf("manual font path = %q", config.App.GuiFont)
	}
}
