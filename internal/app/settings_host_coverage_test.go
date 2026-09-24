package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestSettingsHostApplyRuntimeWithoutFrames(t *testing.T) {
	savedConfig := config.App
	oldFrameManager := vtui.FrameManager
	t.Cleanup(func() {
		config.App = savedConfig
		vtui.FrameManager = oldFrameManager
	})

	// The no-frame path is used while settings are applied before the UI has
	// been created, and keeps this test independent of a screen fixture.
	vtui.FrameManager = nil
	before := config.App
	config.App.ColorStyle = "missing-style"
	config.App.EditorColorerCatalog = "changed"
	(settingsHost{}).ApplyRuntime(before, []string{"ColorStyle", "EditorColorerScheme", "unrelated"})
}
