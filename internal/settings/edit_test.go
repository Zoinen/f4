package settings

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"github.com/unxed/winkeys"
)

func TestSettingsReadOnlyInputSelectionCopyAndMutation(t *testing.T) {
	e := &settingsEdit{Edit: vtui.NewEdit(0, 0, 30, "example path")}
	e.SetDisabled(true)
	e.SetFocus(true)
	if !e.CanFocus() || e.IsDisabled() {
		t.Fatal("read-only input must accept focus")
	}
	changes := 0
	e.OnTextChange = func(string) { changes++ }
	key := func(vk uint16, mods winkeys.ControlKeyState) {
		e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: mods})
	}
	key(vtinput.VK_A, vtinput.LeftCtrlPressed)
	key(vtinput.VK_C, vtinput.LeftCtrlPressed)
	if vtui.GetClipboard() != "example path" {
		t.Fatal("Ctrl+A/C did not copy the read-only value")
	}
	key(vtinput.VK_HOME, 0)
	key(vtinput.VK_RIGHT, vtinput.ShiftPressed)
	key(vtinput.VK_INSERT, vtinput.LeftCtrlPressed)
	if vtui.GetClipboard() != "e" {
		t.Fatal("cursor selection and Ctrl+Insert did not copy")
	}
	for _, event := range []vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, Char: 'z'},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_X, ControlKeyState: vtinput.LeftCtrlPressed},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_V, ControlKeyState: vtinput.LeftCtrlPressed},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT, ControlKeyState: vtinput.ShiftPressed},
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE, ControlKeyState: vtinput.ShiftPressed},
		{Type: vtinput.PasteEventType, PasteStart: true},
		{Type: vtinput.KeyEventType, KeyDown: true, Char: 'p'},
		{Type: vtinput.PasteEventType},
	} {
		e.ProcessKey(&event)
	}
	if e.GetText() != "example path" || changes != 0 {
		t.Fatal("read-only edit changed its value or published a draft edit")
	}
	e.ProcessMouse(&vtinput.InputEvent{KeyDown: true, MouseX: 0, MouseY: 0, ButtonState: vtinput.FromLeft1stButtonPressed})
	e.ProcessMouse(&vtinput.InputEvent{MouseX: 7, MouseY: 0, ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: vtinput.MouseMoved})
	e.ProcessMouse(&vtinput.InputEvent{})
	key(vtinput.VK_C, vtinput.LeftCtrlPressed)
	if vtui.GetClipboard() != "example" {
		t.Fatal("mouse drag selection did not copy")
	}
	e.SetDisabled(false)
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'z'})
	if changes != 1 {
		t.Fatal("input did not become editable when its dependency was enabled")
	}
}

func TestSettingsReadOnlyInputSelectionPalette(t *testing.T) {
	palette := append([]uint64(nil), vtui.Palette...)
	defer copy(vtui.Palette, palette)
	e := &settingsEdit{Edit: vtui.NewEdit(0, 0, 20, "path")}
	e.SetDisabled(true)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(30, 2)
	for i := 0; i < 2; i++ {
		vtui.Palette[vtui.ColDialogEdit] = vtui.SetRGBBoth(0, uint32(0xd0c0b0+i*0x101010), 0x202020)
		vtui.Palette[vtui.ColDialogEditSelected] = vtui.SetRGBBoth(0, 0xffffff, uint32(0x104090+i*0x101010))
		vtui.Palette[vtui.ColDialogEditUnchanged] = vtui.SetRGBBoth(0, 0, 0)
		e.SelectAll()
		e.SetFocus(true)
		e.Show(scr)
		if scr.GetCell(0, 0).Attributes != vtui.Palette[vtui.ColDialogEditSelected] {
			t.Fatal("focused selection does not follow palette")
		}
		e.SetFocus(false)
		e.Show(scr)
		if scr.GetCell(0, 0).Attributes != vtui.DimColor(vtui.Palette[vtui.ColDialogEdit]) {
			t.Fatal("inactive read-only text is not dimmed normal input text")
		}
	}
}
