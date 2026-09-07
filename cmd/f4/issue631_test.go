package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestIssue631TrashSettingIsInPanelSettings(t *testing.T) {
	oldConfig := AppConfig
	defer func() { AppConfig = oldConfig }()

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	AppConfig.UseTrash = true
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	actionPanelSettings(pf)
	panelFrame := vtui.FrameManager.GetTopFrame()
	panelDialog := panelFrame.(vtui.Container)
	wantText := getCleanText(vtui.NewCheckbox(0, 0, Msg("PanelSettings.UseTrash"), false))

	var trashCheckbox *vtui.Checkbox
	for _, child := range panelDialog.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok && getCleanText(checkbox) == wantText {
			trashCheckbox = checkbox
			break
		}
	}
	if trashCheckbox == nil {
		t.Fatalf("trash setting %q is missing from Panel Settings", wantText)
	}
	if trashCheckbox.State != 1 {
		t.Fatalf("trash setting state = %d, want enabled state from AppConfig", trashCheckbox.State)
	}

	panelFrame.SetExitCode(-1)
	vtui.FrameManager.Pop()
	actionConfirmationsSettings(pf)
	confirmationsFrame := vtui.FrameManager.GetTopFrame()
	confirmationsDialog := confirmationsFrame.(vtui.Container)
	for _, child := range confirmationsDialog.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok && getCleanText(checkbox) == wantText {
			t.Fatalf("trash setting %q is duplicated in Confirmations Settings", wantText)
		}
	}
	confirmationsFrame.SetExitCode(-1)
	vtui.FrameManager.Pop()
}
