package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

// useTempSettings points the configuration at a settings.ini holding content
// and restores the paths and App afterwards.
func useTempSettings(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.ini")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	origUserPathFunc := GetUserConfigIniPath
	origPathsFunc := GetConfigIniPaths
	oldCfg := App
	t.Cleanup(func() {
		GetUserConfigIniPath = origUserPathFunc
		GetConfigIniPaths = origPathsFunc
		App = oldCfg
	})
	GetUserConfigIniPath = func() string { return path }
	GetConfigIniPaths = func() []string { return []string{path} }
	return path
}

// f4 #1154: the caret shapes and blinking survive a save and a load.
func TestConfig_CursorStyleSaveAndLoad(t *testing.T) {
	useTempSettings(t, "")

	App.CursorInsertShape = "bar"
	App.CursorOvertypeShape = "underline"
	App.CursorBlink = false
	SaveConfig()

	App.CursorInsertShape = "underline"
	App.CursorOvertypeShape = "block"
	App.CursorBlink = true
	LoadConfig()

	if App.CursorInsertShape != "bar" || App.CursorOvertypeShape != "underline" || App.CursorBlink {
		t.Fatalf("restored %q/%q/blink=%v, want bar/underline/false",
			App.CursorInsertShape, App.CursorOvertypeShape, App.CursorBlink)
	}
}

// Without the keys f4 keeps the caret it always had.
func TestConfig_CursorStyleDefaultsWhenKeysAreAbsent(t *testing.T) {
	useTempSettings(t, "[Panel]\nShowHiddenFiles = 1\n")
	App.CursorInsertShape, App.CursorOvertypeShape, App.CursorBlink = "bar", "bar", false
	LoadConfig()
	if App.CursorInsertShape != "underline" || App.CursorOvertypeShape != "block" || !App.CursorBlink {
		t.Fatalf("defaults %q/%q/blink=%v, want underline/block/true",
			App.CursorInsertShape, App.CursorOvertypeShape, App.CursorBlink)
	}
}

// A hand-edited value is folded to a known name, so the Settings Center never
// meets a choice it would reject.
func TestConfig_CursorStyleNormalizesHandEditedValues(t *testing.T) {
	useTempSettings(t, "[Panel]\nCursorInsertShape = BAR\nCursorOvertypeShape = hollow\n")
	LoadConfig()
	if App.CursorInsertShape != "bar" {
		t.Errorf("CursorInsertShape = %q, want bar", App.CursorInsertShape)
	}
	if App.CursorOvertypeShape != "block" {
		t.Errorf("unknown CursorOvertypeShape loaded as %q, want the block default", App.CursorOvertypeShape)
	}
}

func TestApplyCursorSettingsPushesIntoVtui(t *testing.T) {
	oldCfg := App
	oldManage := vtui.ManageCursorStyle
	insert, overtype, blink := vtui.InsertCursorShape(), vtui.OvertypeCursorShape(), vtui.CursorBlinks()
	t.Cleanup(func() {
		App = oldCfg
		vtui.ManageCursorStyle = oldManage
		vtui.SetCursorStyle(insert, overtype, blink)
	})

	App.KeepTerminalCursor = true
	App.CursorInsertShape = "bar"
	App.CursorOvertypeShape = "underline"
	App.CursorBlink = false
	ApplyCursorSettings()

	if vtui.ManageCursorStyle {
		t.Error("KeepTerminalCursor did not turn cursor-style management off")
	}
	if vtui.InsertCursorShape() != vtui.CursorShapeBar || vtui.OvertypeCursorShape() != vtui.CursorShapeUnderline || vtui.CursorBlinks() {
		t.Errorf("vtui got %v/%v/blink=%v, want bar/underline/false",
			vtui.InsertCursorShape(), vtui.OvertypeCursorShape(), vtui.CursorBlinks())
	}
}
