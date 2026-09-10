package app

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestHandlePanelsAppCommandDispatchesFileAndSettingsActions(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(120, 60)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()

	// A zero-value frame has no active filesystem panel. File actions therefore
	// take their documented no-op path, while the settings actions only build
	// their dialogs. This exercises the application-side dispatch without
	// touching user files or starting an editor.
	pf := &panel.PanelsFrame{}
	commands := []int{
		appcmd.CmNew,
		appcmd.CmView,
		appcmd.CmEdit,
		appcmd.CmCopy,
		appcmd.CmMove,
		appcmd.CmRename,
		appcmd.CmMkDir,
		appcmd.CmDelete,
		appcmd.CmFindFile,
		appcmd.CmPanelSettings,
		appcmd.CmEditorSettings,
		appcmd.CmColorerSettings,
		appcmd.CmAppearanceSettings,
		appcmd.CmConfirmationsSettings,
		appcmd.CmHotkeyConfig,
		appcmd.CmLanguage,
		appcmd.CmHelpLanguage,
		appcmd.CmUpdateSettings,
		appcmd.CmPlugins,
		appcmd.CmPlugRing,
	}

	for _, cmd := range commands {
		cmd := cmd
		t.Run(fmt.Sprintf("command-%d", cmd), func(t *testing.T) {
			before := vtui.FrameManager.GetTopFrame()
			if !handlePanelsAppCommand(pf, cmd, nil) {
				t.Fatalf("handlePanelsAppCommand(%d) was not handled", cmd)
			}
			for vtui.FrameManager.GetTopFrame() != before {
				vtui.FrameManager.Pop()
			}
		})
	}
}

func TestHandlePanelsAppCommandDispatchesWorkspaceActionWithoutManager(t *testing.T) {
	oldManager := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = oldManager })

	pf := &panel.PanelsFrame{}
	if got := handlePanelsAppCommand(pf, appcmd.CmWorkspaceNew, nil); got {
		t.Errorf("handlePanelsAppCommand(CmWorkspaceNew) = true with nil FrameManager, want false")
	}
}

func TestHandlePanelsAppCommandRejectsUnknownCommand(t *testing.T) {
	if got := handlePanelsAppCommand(&panel.PanelsFrame{}, -1, nil); got {
		t.Fatal("unknown command was handled")
	}
}
