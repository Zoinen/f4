package panel

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// The production Settings hook must not intercept an in-place editing workflow.
func TestContextEditorsRemainLocalWithSettingsAvailable(t *testing.T) {
	old, oldMenu := OpenSettingsAt, OpenUserMenuSettings
	OpenSettingsAt = func(string, string, string, bool) bool { t.Error("context editor opened Settings"); return true }
	OpenUserMenuSettings = func(MenuSettingsSource, *vtui.VMenu, int, bool, bool) bool {
		t.Error("user-menu editor opened Settings")
		return true
	}
	t.Cleanup(func() { OpenSettingsAt, OpenUserMenuSettings = old, oldMenu })
	for _, kind := range []string{"drive-new", "drive-edit", "bookmark", "user-menu", "submenu"} {
		t.Run(kind, func(t *testing.T) {
			t.Cleanup(testutil.SwapFrameManager(t))
			scr := vtui.NewSilentScreenBuf()
			scr.AllocBuf(100, 40)
			vtui.FrameManager.Init(scr)
			switch kind {
			case "drive-new", "drive-edit":
				idx := -1
				if kind == "drive-edit" {
					idx = 0
				}
				pf := &PanelsFrame{}
				pf.openDriveBookmarkEditor(-1, nil, []DriveBookmark{{Name: "Example", Path: "/example", Hotkey: "Q"}}, idx, func() {})
				select {
				case task := <-vtui.FrameManager.TaskChan:
					task()
				case <-time.After(time.Second):
					t.Fatal("drive editor was not scheduled")
				}
				d, ok := vtui.FrameManager.GetTopFrame().(*driveBookmarkEditDialog)
				if !ok {
					t.Fatalf("expected drive link dialog, got %T", vtui.FrameManager.GetTopFrame())
				}
				if idx == 0 && (d.nameEdit.GetText() != "Example" || d.pathEdit.GetText() != "/example" || d.HotkeyEdit.GetText() != "Q") {
					t.Fatal("existing link fields lost")
				}
			case "bookmark":
				d := &BookmarksDialog{Set: BookmarkSet{1: {Path: "/example"}}}
				d.editPath(1)
			default:
				showEditItemDialog(&userMenuState{}, nil, nil, 0, true, kind == "submenu")
			}
			if _, ok := vtui.FrameManager.GetTopFrame().(vtui.Container); !ok {
				t.Fatalf("expected local editor, got %T", vtui.FrameManager.GetTopFrame())
			}
		})
	}
}
