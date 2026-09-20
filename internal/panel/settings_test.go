package panel

import "testing"

func TestPanelsFrameOpenSettingsForwarding(t *testing.T) {
	oldOpen := OpenSettingsAt
	oldCategory := OpenSettingsCategoryOnly
	oldDefault := SetSettingsRecordDefault
	t.Cleanup(func() {
		OpenSettingsAt = oldOpen
		OpenSettingsCategoryOnly = oldCategory
		SetSettingsRecordDefault = oldDefault
	})

	var pf PanelsFrame
	OpenSettingsAt = nil
	if pf.OpenSettings("General", "", "", false) {
		t.Fatal("nil settings opener returned success")
	}
	called := false
	OpenSettingsAt = func(category, collection, record string, create bool) bool {
		called = category == "General" && collection == "UI" && record == "theme" && create
		return called
	}
	if !pf.OpenSettings("General", "UI", "theme", true) || !called {
		t.Fatal("settings opener did not receive its arguments")
	}
}
