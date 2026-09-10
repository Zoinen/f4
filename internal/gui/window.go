package gui

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// restoreGuiWindowPosition applies a position saved by Shift+F9. GUI hosts
// call this from their setup callback while the native window is still being
// initialized, so the first visible frame appears at the saved location.
func restoreGuiWindowPosition() {
	if config.App.GuiPositionSaved {
		vtui.SetWindowPosition(config.App.GuiPosX, config.App.GuiPosY)
	}
}
