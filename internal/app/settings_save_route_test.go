package app

import (
	"testing"
)

func TestSettingsSaveActionIsCenterDeepLink(t *testing.T) {
	action, ok := GetAction("App.SaveSettings")
	if !ok || !action.HideFromMenu || settingsDeepLinks["app.savesettings"] != "workspaces" {
		t.Fatal("Save Settings must be a hidden Settings Center deep link")
	}
	if len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != "ShiftF9:NoAltScreenApp" {
		t.Fatal("legacy shortcut lost")
	}
}

func TestSettingsPathHintsRoute(t *testing.T) {
	if settingsDeepLinks["settings.pathhints"] != "terminal" {
		t.Fatal("old shortcut points to removed category")
	}
}
