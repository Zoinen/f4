package dialog

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestNewFileInputBoxDefaultWidth(t *testing.T) {
	for _, size := range []struct{ screenWidth, dialogWidth int }{{40, 30}, {60, 30}, {80, 40}, {120, 60}, {200, 100}} {
		t.Run(fmt.Sprint(size.screenWidth), func(t *testing.T) {
			t.Cleanup(testutil.SwapFrameManager(t))
			screen := vtui.NewSilentScreenBuf()
			screen.AllocBuf(size.screenWidth, 25)
			vtui.FrameManager.Init(screen)
			theme.SetDefaultF4Palette()
			dlg := NewFileInputBox(nil)
			defer vtui.FrameManager.Pop()
			if width := dlg.X2 - dlg.X1 + 1; width != size.dialogWidth {
				t.Fatalf("default width=%d, want %d", width, size.dialogWidth)
			}
			if dlg.X1 != (size.screenWidth-size.dialogWidth)/2 {
				t.Fatal("default dialog must be centered")
			}
			dlg.ResizeConsole(size.screenWidth, 25)
			if width := dlg.X2 - dlg.X1 + 1; width != size.dialogWidth {
				t.Fatal("initial and resized widths must match")
			}
			rules := vtui.DefaultLayoutRules
			rules.FrameClearanceY = 0
			rules.MaxWidth = 120
			vtui.AssertLayoutWithRules(t, dlg, rules)
		})
	}
}

func TestNewFileInputBoxMinimumWidthAllLanguages(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	theme.SetDefaultF4Palette()
	base := vtui.SnapshotStrings()
	t.Cleanup(func() { vtui.ReplaceStrings(base) })
	packs := i18n.LoadAllLanguagePacks()
	if len(packs) == 0 {
		t.Fatal("no language packs found")
	}
	for _, pack := range packs {
		t.Run(pack.Name, func(t *testing.T) {
			vtui.ReplaceStrings(base)
			vtui.AddStrings(pack.Strings)
			dlg := NewFileInputBox(nil)
			defer vtui.FrameManager.Pop()
			dlg.ChangeSize(30, 6)
			dlg.Show(screen)
			assertDialogLayout(t, dlg)
			if dlg.X2-dlg.X1+1 != 30 || dlg.Y2-dlg.Y1+1 != 6 {
				t.Fatal("expected a 30x6 dialog")
			}
			if width := vtui.StringWidth(dlg.label.GetText()); width > 26 {
				t.Errorf("prompt needs %d cells, only 26 available", width)
			}
		})
	}
}

func TestNewFileInputBoxKeyboardAndCancel(t *testing.T) {
	for _, method := range []string{"enter", "escape", "cancel"} {
		t.Run(method, func(t *testing.T) {
			t.Cleanup(testutil.SwapFrameManager(t))
			screen := vtui.NewSilentScreenBuf()
			screen.AllocBuf(80, 25)
			vtui.FrameManager.Init(screen)
			theme.SetDefaultF4Palette()
			calls, got := 0, ""
			dlg := NewFileInputBox(func(name string) { calls++; got = name })
			defer vtui.FrameManager.Pop()
			if dlg.edit.GetText() != "" {
				t.Fatal("new file input must start empty")
			}
			dlg.edit.SetText("notes.txt")
			if method == "cancel" {
				for _, child := range dlg.GetChildren() {
					if button, ok := child.(*vtui.Button); ok && !button.IsDefault {
						button.OnClick()
					}
				}
			} else {
				var key uint16 = vtinput.VK_RETURN
				if method == "escape" {
					key = vtinput.VK_ESCAPE
				}
				dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key})
			}
			if !dlg.IsDone() {
				t.Fatal("dialog did not close")
			}
			if method == "enter" {
				if calls != 1 || got != "notes.txt" {
					t.Fatalf("confirmation: calls=%d name=%q", calls, got)
				}
			} else if calls != 0 {
				t.Fatal("cancellation called confirmation handler")
			}
		})
	}
}
