package app

import (
	"github.com/unxed/f4/internal/panel"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestIssue631TrashSettingIsInPanelSettings(t *testing.T) {
	oldConfig := config.App
	defer func() { config.App = oldConfig }()

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	config.App.UseTrash = true
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	ActionPanelSettings(pf)
	panelFrame := vtui.FrameManager.GetTopFrame()
	panelDialog := panelFrame.(vtui.Container)
	wantText := testutil.GetCleanText(vtui.NewCheckbox(0, 0, i18n.Msg("PanelSettings.UseTrash"), false))

	var trashCheckbox *vtui.Checkbox
	for _, child := range panelDialog.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok && testutil.GetCleanText(checkbox) == wantText {
			trashCheckbox = checkbox
			break
		}
	}
	if trashCheckbox == nil {
		t.Fatalf("trash setting %q is missing from Panel Settings", wantText)
	}
	if trashCheckbox.State != 1 {
		t.Fatalf("trash setting state = %d, want enabled state from config.App", trashCheckbox.State)
	}

	panelFrame.SetExitCode(-1)
	vtui.FrameManager.Pop()
	ActionConfirmationsSettings(pf)
	confirmationsFrame := vtui.FrameManager.GetTopFrame()
	confirmationsDialog := confirmationsFrame.(vtui.Container)
	for _, child := range confirmationsDialog.GetChildren() {
		if checkbox, ok := child.(*vtui.Checkbox); ok && testutil.GetCleanText(checkbox) == wantText {
			t.Fatalf("trash setting %q is duplicated in Confirmations Settings", wantText)
		}
	}
	confirmationsFrame.SetExitCode(-1)
	vtui.FrameManager.Pop()
}
