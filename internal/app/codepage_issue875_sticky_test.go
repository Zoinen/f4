package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// #875, second round. A codepage picked in the viewer's Shift+F8 menu used
// to switch the *global* auto-detect off and make that codepage the global
// default, so the next file opened in whatever the previous one had been
// switched to; "Auto-detect" in the menu was a toggle of the same global
// switch. Now the menu is about the file on screen only: the choice is
// remembered for that file, Auto-detect forgets it and detects again, and
// the settings are left alone -- so the next file is still detected.
func TestViewer_Issue875_MenuChoiceDoesNotStickToNextFile(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	oldState, oldAuto, oldDefault := fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage
	defer func() {
		fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage = oldState, oldAuto, oldDefault
	}()
	fileops.GlobalFileState = nil // no per-file memory in this test: the point is the globals
	config.App.ViewerAutodetectCodePage = true
	config.App.ViewerDefaultCodePage = 65001

	dir := t.TempDir()
	v := vfs.NewOSVFS(dir)
	samples := issue875WriteSamples(t, dir)
	var cp1251, cp866 issue875Sample
	for _, s := range samples {
		switch s.codepage {
		case 1251:
			cp1251 = s
		case 866:
			cp866 = s
		}
	}
	if cp1251.name == "" || cp866.name == "" {
		t.Fatal("samples for 1251 and 866 are required")
	}

	// Open the CP1251 file and switch it, through the menu, to CP866.
	first, err := viewer.NewViewerView(context.Background(), v, filepath.Join(dir, cp1251.name))
	if err != nil {
		t.Fatal(err)
	}
	first.ShowCodepageDialog()
	menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
	if !ok {
		t.Fatalf("top frame is %T, want the codepage menu", vtui.FrameManager.GetTopFrame())
	}
	picked := -1
	for i, it := range menu.Items {
		if id, ok := it.UserData.(int); ok && id == 866 {
			picked = i
		}
	}
	if picked < 0 {
		t.Fatal("866 not in the menu")
	}
	menu.OnAction(picked)
	if first.Codepage != 866 {
		t.Fatalf("after picking 866 the viewer is in %d", first.Codepage)
	}
	first.Close()

	if !config.App.ViewerAutodetectCodePage {
		t.Error("picking a codepage for one file switched global auto-detect off")
	}
	if config.App.ViewerDefaultCodePage != 65001 {
		t.Errorf("picking a codepage for one file rewrote the global default to %d", config.App.ViewerDefaultCodePage)
	}

	// The next file must still be detected, not opened in 866.
	second, err := viewer.NewViewerView(context.Background(), v, filepath.Join(dir, cp866.name))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Codepage != 866 {
		t.Errorf("second file detected as %d, want 866", second.Codepage)
	}
	third, err := viewer.NewViewerView(context.Background(), v, filepath.Join(dir, cp1251.name))
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	if third.Codepage != 1251 {
		t.Errorf("reopened CP1251 file came up as %d: the earlier choice leaked into the defaults", third.Codepage)
	}
}

// Auto-detect in the menu detects the file even when the global switch is
// off: the user asked for this file to be detected.
func TestViewer_Issue875_MenuAutoDetectDetectsRegardlessOfGlobalSwitch(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	oldState, oldAuto, oldDefault := fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage
	defer func() {
		fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage = oldState, oldAuto, oldDefault
	}()
	fileops.GlobalFileState = nil
	config.App.ViewerAutodetectCodePage = false
	config.App.ViewerDefaultCodePage = 1252

	dir := t.TempDir()
	v := vfs.NewOSVFS(dir)
	var cp866 issue875Sample
	for _, s := range issue875WriteSamples(t, dir) {
		if s.codepage == 866 {
			cp866 = s
		}
	}
	vv, err := viewer.NewViewerView(context.Background(), v, filepath.Join(dir, cp866.name))
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()
	if vv.Codepage != 1252 {
		t.Fatalf("with auto-detect off the file should open in the default 1252, got %d", vv.Codepage)
	}
	vv.ReloadWithAutoDetect()
	if vv.Codepage != 866 {
		t.Errorf("Auto-detect from the menu gave %d, want 866", vv.Codepage)
	}
	if config.App.ViewerAutodetectCodePage {
		t.Error("Auto-detect from the menu flipped the global switch")
	}
}
