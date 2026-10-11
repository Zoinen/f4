package dialog

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// NewFileInputBox shares the compact, horizontally resizable input layout
// used by select/deselect group. File creation and history stay with the caller.
func NewFileInputBox(onOK func(string)) *MaskDialog {
	screenWidth := 80
	if vtui.FrameManager != nil {
		screenWidth = vtui.FrameManager.GetScreenSize()
	}
	return compactInputBox(compactInputWidth(screenWidth), i18n.Msg("Edit.NewFileTitle"), i18n.Msg("Edit.NewFilePrompt"), "", onOK)
}
