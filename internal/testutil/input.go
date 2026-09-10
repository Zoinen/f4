package testutil

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// PressKey dispatches a key through the production input path: the caller's
// macro/hotkey filter first, because that is where action hotkeys are
// dispatched, then the frame's own ProcessKey for widget-level keys.
//
// The filter is a parameter rather than a global because it belongs to the
// application, not to the harness. A caller that has no macro manager passes
// nil and gets the frame's own handling.
func PressKey(f vtui.Frame, e *vtinput.InputEvent, filter func(*vtinput.InputEvent) bool) bool {
	if vtui.FrameManager == nil || len(vtui.FrameManager.Screens) == 0 {
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	}
	inStack := false
	for _, s := range vtui.FrameManager.Screens {
		for _, fr := range s.Frames {
			if fr == f {
				inStack = true
				break
			}
		}
	}
	if !inStack {
		vtui.FrameManager.Push(f)
	}
	if filter != nil && filter(e) {
		return true
	}
	return f.ProcessKey(e)
}
