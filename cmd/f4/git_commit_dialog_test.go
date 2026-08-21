package main

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newCommitDialogTestScreen(t *testing.T, width, height int) *vtui.ScreenBuf {
	t.Helper()
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(width, height)
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() {
		reset := vtui.NewSilentScreenBuf()
		reset.AllocBuf(80, 25)
		vtui.FrameManager.Init(reset)
	})
	return screen
}

func TestCommitMessageDialogCollectsMultilineMessageAndSign(t *testing.T) {
	newCommitDialogTestScreen(t, 90, 30)

	var accepted []vfs.CommitDialogResult
	dialog := newCommitMessageDialog(vfs.CommitDialogRequest{
		InitialMessage: "subject\n\nbody",
		InitialSign:    true,
	}, func(result vfs.CommitDialogResult) {
		accepted = append(accepted, result)
	})

	if got, want := dialog.message.GetText(), "subject\n\nbody"; got != want {
		t.Fatalf("initial multiline message = %q, want %q", got, want)
	}
	if dialog.sign.State != 1 {
		t.Fatal("initial signing choice was not reflected in the checkbox")
	}
	if !dialog.message.IsFocused() {
		t.Fatal("commit message editor was not initially focused")
	}

	// Enter remains an ordinary newline in the multiline field, while
	// Ctrl+Enter uses the dialog's default Commit button.
	if !dialog.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
	}) {
		t.Fatal("plain Enter was not consumed by the multiline message editor")
	}
	if got, want := dialog.message.GetText(), "\nsubject\n\nbody"; got != want {
		t.Fatalf("plain Enter message = %q, want %q", got, want)
	}
	if !dialog.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}) {
		t.Fatal("Ctrl+Enter was not routed to the default Commit action")
	}

	want := []vfs.CommitDialogResult{{Message: "\nsubject\n\nbody", Sign: true}}
	if !reflect.DeepEqual(accepted, want) {
		t.Fatalf("accepted result = %#v, want %#v", accepted, want)
	}
	if !dialog.IsDone() {
		t.Fatal("commit dialog did not close after accepting")
	}

	// A stale duplicate activation must not invoke the callback twice.
	dialog.commit.OnClick()
	if !reflect.DeepEqual(accepted, want) {
		t.Fatalf("callback ran after dialog close: %#v", accepted)
	}
}

func TestCommitMessageDialogTracksLiveDialogPalette(t *testing.T) {
	indices := []int{
		vtui.ColDialogText,
		vtui.ColDialogHighlightText,
		vtui.ColDialogBox,
		vtui.ColDialogBoxTitle,
		vtui.ColDialogEdit,
		vtui.ColDialogButton,
		vtui.ColDialogSelectedButton,
		vtui.ColDialogHighlightButton,
		vtui.ColDialogHighlightSelectedButton,
	}
	original := make(map[int]uint64, len(indices))
	for _, index := range indices {
		original[index] = vtui.Palette[index]
	}
	t.Cleanup(func() {
		for index, attribute := range original {
			vtui.Palette[index] = attribute
		}
	})

	screen := newCommitDialogTestScreen(t, 90, 30)
	first := commitDialogTestPalette(0x10)
	for index, attribute := range first {
		vtui.Palette[index] = attribute
	}
	dialog := newCommitMessageDialog(vfs.CommitDialogRequest{InitialMessage: "subject"}, nil)

	// Normal controls and the focused message field use only semantic dialog
	// palette indices. MultiLineEdit has no visual scrollbar in vtui; the
	// border and both focused interactive control variants are asserted below.
	dialog.Show(screen)
	assertCommitDialogTheme(t, dialog, screen, first, false, false)
	x, y, visible, _ := screen.GetCursorStateForTesting()
	if !visible || x != dialog.message.X1 || y != dialog.message.Y1 {
		t.Fatalf("focused message cursor = (%d,%d,%t), want (%d,%d,true)", x, y, visible, dialog.message.X1, dialog.message.Y1)
	}

	dialog.SetFocusedItem(dialog.sign)
	dialog.Show(screen)
	assertCommitDialogTheme(t, dialog, screen, first, true, false)

	dialog.SetFocusedItem(dialog.commit)
	dialog.Show(screen)
	assertCommitDialogTheme(t, dialog, screen, first, false, true)

	// Change the palette after the dialog and every child control already
	// exist. Rendering must use the new semantic values rather than cached
	// attributes captured during construction.
	second := commitDialogTestPalette(0x50)
	for index, attribute := range second {
		vtui.Palette[index] = attribute
	}
	dialog.Show(screen)
	assertCommitDialogTheme(t, dialog, screen, second, false, true)
	vtui.AssertLayout(t, dialog)
}

func commitDialogTestPalette(seed uint32) map[int]uint64 {
	return map[int]uint64{
		vtui.ColDialogText:                    vtui.SetRGBBoth(0, seed+0x01, seed+0x11),
		vtui.ColDialogHighlightText:           vtui.SetRGBBoth(0, seed+0x02, seed+0x12),
		vtui.ColDialogBox:                     vtui.SetRGBBoth(0, seed+0x03, seed+0x13),
		vtui.ColDialogBoxTitle:                vtui.SetRGBBoth(0, seed+0x04, seed+0x14),
		vtui.ColDialogEdit:                    vtui.SetRGBBoth(0, seed+0x05, seed+0x15),
		vtui.ColDialogButton:                  vtui.SetRGBBoth(0, seed+0x06, seed+0x16),
		vtui.ColDialogSelectedButton:          vtui.SetRGBBoth(0, seed+0x07, seed+0x17),
		vtui.ColDialogHighlightButton:         vtui.SetRGBBoth(0, seed+0x08, seed+0x18),
		vtui.ColDialogHighlightSelectedButton: vtui.SetRGBBoth(0, seed+0x09, seed+0x19),
	}
}

func assertCommitDialogTheme(
	t *testing.T,
	dialog *commitMessageDialog,
	screen *vtui.ScreenBuf,
	want map[int]uint64,
	signFocused bool,
	commitFocused bool,
) {
	t.Helper()
	checks := []struct {
		name       string
		x, y       int
		paletteIdx int
	}{
		{name: "message label", x: dialog.messageLabel.X1, y: dialog.messageLabel.Y1, paletteIdx: vtui.ColDialogText},
		{name: "message edit", x: dialog.message.X1, y: dialog.message.Y1, paletteIdx: vtui.ColDialogEdit},
		{name: "dialog border", x: dialog.X1, y: dialog.Y1 + 1, paletteIdx: vtui.ColDialogBox},
		{name: "dialog title", x: (dialog.X1 + dialog.X2) / 2, y: dialog.Y1, paletteIdx: vtui.ColDialogBoxTitle},
	}
	if signFocused {
		checks = append(checks,
			struct {
				name       string
				x, y       int
				paletteIdx int
			}{name: "focused sign checkbox", x: dialog.sign.X1, y: dialog.sign.Y1, paletteIdx: vtui.ColDialogSelectedButton},
			struct {
				name       string
				x, y       int
				paletteIdx int
			}{name: "focused sign hotkey", x: dialog.sign.X1 + 4, y: dialog.sign.Y1, paletteIdx: vtui.ColDialogHighlightSelectedButton},
		)
	} else {
		checks = append(checks, struct {
			name       string
			x, y       int
			paletteIdx int
		}{name: "sign checkbox", x: dialog.sign.X1, y: dialog.sign.Y1, paletteIdx: vtui.ColDialogText})
	}
	if commitFocused {
		checks = append(checks,
			struct {
				name       string
				x, y       int
				paletteIdx int
			}{name: "focused commit button", x: dialog.commit.X1, y: dialog.commit.Y1, paletteIdx: vtui.ColDialogSelectedButton},
			struct {
				name       string
				x, y       int
				paletteIdx int
			}{name: "focused commit hotkey", x: dialog.commit.X1 + 2, y: dialog.commit.Y1, paletteIdx: vtui.ColDialogHighlightSelectedButton},
		)
	} else {
		checks = append(checks, struct {
			name       string
			x, y       int
			paletteIdx int
		}{name: "commit button", x: dialog.commit.X1, y: dialog.commit.Y1, paletteIdx: vtui.ColDialogButton})
	}

	for _, check := range checks {
		if got := screen.GetCell(check.x, check.y).Attributes; got != want[check.paletteIdx] {
			t.Errorf("%s attr = %#x, want palette[%d] %#x", check.name, got, check.paletteIdx, want[check.paletteIdx])
		}
	}
}
