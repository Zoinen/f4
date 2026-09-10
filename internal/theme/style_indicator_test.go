package theme

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
)

func TestIndicatorBackgroundThemeCompatibility(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	defer func() { vtui.Palette = saved }()
	oldConfig := config.App
	defer func() { config.App = oldConfig }()
	oldPath := UserColorOverridesPath
	UserColorOverridesPath = func() string { return filepath.Join(t.TempDir(), "none.ini") }
	defer func() { UserColorOverridesPath = oldPath }()
	for _, name := range []string{"Modern", "Classic", "Default Dark", "Radiola", "Modern"} {
		if err := ApplyColorStyle(name); err != nil {
			t.Fatal(err)
		}
		attr := vtui.Palette[vtui.ColDialogIndicatorBackground]
		if name == "Modern" {
			_, want := GetColorRGBBoth(vtui.Palette[vtui.ColDialogEdit])
			_, got := GetColorRGBBoth(attr)
			if got != want {
				t.Fatal("Modern indicator must match input background")
			}
		} else if attr != 0 {
			t.Fatalf("%s unexpectedly overrides indicator background", name)
		}
	}
	for _, name := range []string{"Modern", "Classic"} {
		if err := ApplyColorStyle(name); err != nil {
			t.Fatal(err)
		}
		want := vtui.Palette[vtui.ColDialogIndicatorBackground]
		path := filepath.Join(t.TempDir(), "colors.ini")
		if err := ExportColors(path); err != nil {
			t.Fatal(err)
		}
		ini := ini.Load(path)
		vtui.SetDefaultPalette()
		SetDefaultF4Palette()
		InitColors(ini)
		got := vtui.Palette[vtui.ColDialogIndicatorBackground]
		if want == 0 && got != 0 || want != 0 && vtui.GetRGBBack(want) != vtui.GetRGBBack(got) {
			t.Fatal("indicator export did not round-trip")
		}
	}
}

// Single-component palette slots must survive the contrast pass unchanged,
// while ordinary text pairs still receive correction.
func TestContrastPreservesSettingsAndCursorSlots(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	oldConfig := config.App
	t.Cleanup(func() { vtui.Palette = saved; config.App = oldConfig })
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	config.App.EnforceColorCorrection = true
	attr := vtui.SetRGBBoth(0, 0x808080, 0x808080)
	slots := []int{ColDialogSettingsBackground, vtui.ColDialogIndicatorBackground, ColTerminalCursor}
	for _, slot := range slots {
		vtui.Palette[slot] = attr
	}
	vtui.Palette[vtui.ColDialogText] = attr
	AdjustContrastLevels()
	for _, slot := range slots {
		if vtui.Palette[slot] != attr {
			t.Errorf("special slot %d was contrast-corrected", slot)
		}
	}
	if vtui.Palette[vtui.ColDialogText] == attr {
		t.Error("ordinary dialog text did not receive contrast correction")
	}
}
