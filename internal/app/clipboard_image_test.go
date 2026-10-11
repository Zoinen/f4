package app

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/goclip"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type imagePasteTestDriver struct{ text string }

func (*imagePasteTestDriver) Name() string                { return "test-image" }
func (*imagePasteTestDriver) Available() bool             { return true }
func (d *imagePasteTestDriver) ReadText() (string, error) { return d.text, nil }
func (*imagePasteTestDriver) WriteText(string) error      { return nil }
func (*imagePasteTestDriver) Clear() error                { return nil }
func (*imagePasteTestDriver) ReadImage(context.Context) (image.Image, error) {
	return image.NewNRGBA(image.Rect(0, 0, 2, 2)), nil
}

func TestPanelPasteHotkeysAndActionDispatch(t *testing.T) {
	oldDriver, oldConfig := goclip.ActiveDriver(), config.App
	defer func() { terminal.WaitForAsyncClipboard(); goclip.SetActiveDriver(oldDriver); config.App = oldConfig }()
	config.App = config.DefaultConfig()
	oldHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	keymap.GlobalHotkeysMgr.Bind("Shell", "CtrlShiftV", "Panel.Paste")
	defer func() { keymap.GlobalHotkeysMgr = oldHotkeys }()
	goclip.SetActiveDriver(&imagePasteTestDriver{})
	for _, key := range []struct {
		vk   uint16
		mods vtinput.ControlKeyState
	}{{vtinput.VK_V, vtinput.LeftCtrlPressed}, {vtinput.VK_INSERT, vtinput.ShiftPressed}, {vtinput.VK_V, vtinput.LeftCtrlPressed | vtinput.ShiftPressed}, {0, 0}} {
		pf := seedPanelForRestore(t, []string{"keep.txt"})
		dir := t.TempDir()
		pf.GetActivePanel().Vfs = vfs.NewOSVFS(dir)
		if err := os.WriteFile(filepath.Join(dir, "keep.txt"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		if key.vk == 0 {
			if !RunAction("Panel.Paste") {
				t.Fatal("action dispatch failed")
			}
		} else if !pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key.vk, ControlKeyState: key.mods}) {
			t.Fatal("paste shortcut not consumed")
		}
		terminal.WaitForAsyncClipboard()
		deadline := time.After(10 * time.Second)
		for pf.GetActivePanel().GetRawSelectedName() != "screenshot001.png" {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			case <-deadline:
				t.Fatal("clipboard image was not saved and focused")
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "screenshot001.png")); err != nil {
			t.Fatal(err)
		}
		pf.Close()
	}
}

func TestPanelPasteMixedClipboardOffersChoice(t *testing.T) {
	oldDriver := goclip.ActiveDriver()
	defer func() { terminal.WaitForAsyncClipboard(); goclip.SetActiveDriver(oldDriver) }()
	goclip.SetActiveDriver(&imagePasteTestDriver{text: "actual clipboard text"})
	pf := seedPanelForRestore(t, []string{"keep.txt"})
	pf.GetActivePanel().Vfs = vfs.NewOSVFS(t.TempDir())
	if !RunAction("Panel.Paste") {
		t.Fatal("action dispatch failed")
	}
	terminal.WaitForAsyncClipboard()
	testutil.DrainUITasks()
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatal("mixed-content choice not shown")
	}
	for _, child := range dlg.GetChildren() {
		if button, ok := child.(*vtui.Button); ok && button.GetCaption() == "Paste text" {
			button.OnClick()
		}
	}
	if got := pf.CmdLine.Edit.GetText(); got != "actual clipboard text" {
		t.Fatal(got)
	}
	if dlg.GetTitle() != i18n.Msg("ClipboardImage.Title") {
		t.Fatal("wrong dialog")
	}
}
