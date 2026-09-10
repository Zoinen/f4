package cmdline

import (
	"github.com/unxed/vtui"
)

func CloseActiveAutocompleteMenus() {
	if vtui.FrameManager == nil {
		return
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		if _, ok := frame.(*vtui.AutoCompleteMenu); ok {
			frame.Close()
		}
	}
}
