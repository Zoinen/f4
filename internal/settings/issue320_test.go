package settings

import (
	"context"
	"testing"
)

// TestSettingsCenterMarksTheEnterButtonAsDefault is the regression test for
// issue #320 (round two): Enter in the Settings Center presses Apply, but the
// button looked like its neighbours because it was never flagged as the
// dialog's default, so vtui had no reason to highlight it.
func TestSettingsCenterMarksTheEnterButtonAsDefault(t *testing.T) {
	d, _ := (coreSettingsProvider{}).Begin(context.Background())
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (coreSettingsProvider{}).Catalog(), draft: d}})

	if !c.apply.IsDefault {
		t.Error("Apply answers Enter but is not the default button, so it is not highlighted")
	}
	if c.ok.IsDefault || c.cancel.IsDefault {
		t.Error("only the button that answers Enter may be the default one")
	}
}
