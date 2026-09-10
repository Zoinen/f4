package app

import (
	"github.com/unxed/vtui"
	"testing"
)

// The two helpers the editor tests that stayed here need. They read only
// vtui and the piece table, so each package keeps its own rather than one
// exporting scaffolding to the other.

func unsavedChangesConfirms(screen *vtui.AppScreen) []vtui.Frame {
	var confirms []vtui.Frame
	for _, frame := range screen.Frames {
		if frame.GetType() == vtui.TypeDialog && frame.GetTitle() == " Confirm " {
			confirms = append(confirms, frame)
		}
	}
	return confirms
}

func requireUnsavedChangesConfirm(t *testing.T) vtui.Frame {
	t.Helper()
	if vtui.FrameManager.ActiveIdx < 0 || vtui.FrameManager.ActiveIdx >= len(vtui.FrameManager.Screens) {
		t.Fatalf("active workspace index = %d, want an existing workspace", vtui.FrameManager.ActiveIdx)
	}
	confirms := unsavedChangesConfirms(vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx])
	if len(confirms) != 1 {
		t.Fatalf("active workspace has %d unsaved-changes confirmations, want exactly one", len(confirms))
	}
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		t.Fatal("top frame is nil, want unsaved-changes confirmation")
	}
	if top != confirms[0] {
		t.Fatalf("top frame = %T %q, want unsaved-changes confirmation", top, top.GetTitle())
	}
	return top
}
