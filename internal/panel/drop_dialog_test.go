package panel

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"testing"
)

type dialogLayoutRig struct {
	manager    *vtui.FrameManagerType
	screen     *vtui.ScreenBuf
	baseScreen *vtui.AppScreen
	panels     *PanelsFrame
	localVFS   vfs.VFS
}

func newDialogLayoutRig(t *testing.T, dir string) *dialogLayoutRig {
	t.Helper()
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 60)
	manager := vtui.FrameManager
	manager.Init(screen)

	localVFS := vfs.NewOSVFS(dir)
	if err := localVFS.SetPath(dir); err != nil {
		t.Fatal(err)
	}
	panels := NewPanelsFrame()
	left := NewFileSystemPanel(0, 0, 40, 20, localVFS)
	right := NewFileSystemPanel(40, 0, 40, 20, localVFS.Clone())
	waitForLoad(t, left)
	waitForLoad(t, right)
	panels.Panels[0] = left
	panels.Panels[1] = right
	panels.ResizeConsole(120, 60)
	waitForLoad(t, panels.Panels[0].(*FileSystemPanel))
	waitForLoad(t, panels.Panels[1].(*FileSystemPanel))
	manager.Push(panels)

	return &dialogLayoutRig{
		manager:    manager,
		screen:     screen,
		baseScreen: manager.Screens[manager.ActiveIdx],
		panels:     panels,
		localVFS:   localVFS,
	}
}
