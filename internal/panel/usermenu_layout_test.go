package panel

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

func TestUserMenuItemEditorLayout(t *testing.T) {
	for _, size := range [][2]int{{80, 25}, {120, 40}} {
		for _, submenu := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/submenu=%t", size[0], size[1], submenu), func(t *testing.T) {
				t.Cleanup(testutil.SwapFrameManager(t))
				screen := vtui.NewSilentScreenBuf()
				screen.AllocBuf(size[0], size[1])
				vtui.FrameManager.Init(screen)
				item := UserMenuItem{HotKey: "F12", Label: "WinMerge", Commands: []string{"first command", "second command"}}
				showEditItemDialog(&userMenuState{}, nil, []UserMenuItem{item}, 0, false, submenu)
				dlg, ok := vtui.FrameManager.GetTopFrame().(*userMenuItemDialog)
				if !ok {
					t.Fatal("menu-item editor was not opened")
				}
				if !dlg.ShowClose {
					t.Fatal("menu-item editor must retain its close button")
				}
				rules := vtui.DefaultLayoutRules
				rules.FrameClearanceY = 0
				vtui.AssertLayoutWithRules(t, dlg, rules)

				var edits []*vtui.Edit
				var commands *vtui.MultiLineEdit
				var separators []*vtui.Separator
				var buttons []*vtui.Button
				for _, child := range dlg.GetChildren() {
					switch child := child.(type) {
					case *vtui.Edit:
						edits = append(edits, child)
					case *vtui.MultiLineEdit:
						commands = child
					case *vtui.Separator:
						separators = append(separators, child)
					case *vtui.Button:
						buttons = append(buttons, child)
					}
				}
				if len(edits) != 2 || len(buttons) != 2 {
					t.Fatal("hotkey, label or buttons are missing")
				}
				x, y, x2, _ := edits[0].GetPosition()
				if x != dlg.X1+2 || y != dlg.Y1+2 || x2-x+1 != 3 || edits[0].GetText() != "F12" {
					t.Fatal("hotkey must remain a short field below its label")
				}
				x, y, x2, _ = edits[1].GetPosition()
				if x != dlg.X1+2 || y != dlg.Y1+4 || x2 != dlg.X2-2 || edits[1].GetText() != item.Label {
					t.Fatal("item label must fill its own row below its caption")
				}
				wantSeparators := 2
				if submenu {
					wantSeparators = 1
					if commands != nil {
						t.Fatal("submenu editor must not contain a command field")
					}
				} else {
					if commands == nil {
						t.Fatal("command field is missing")
					}
					cx, cy, cx2, cy2 := commands.GetPosition()
					if cx != dlg.X1+2 || cx2 != dlg.X2-2 || cy != dlg.Y1+7 || cy2-cy+1 != 6 {
						t.Fatal("command field must span six full-width rows")
					}
					lines := commands.GetLines()
					if len(lines) < 2 || lines[0] != item.Commands[0] || lines[1] != item.Commands[1] {
						t.Fatal("existing command lines were lost")
					}
				}
				if len(separators) != wantSeparators {
					t.Fatalf("got %d separators, want %d", len(separators), wantSeparators)
				}
				last := separators[len(separators)-1]
				sx, sy, sx2, _ := last.GetPosition()
				_, by, _, _ := buttons[0].GetPosition()
				if sx != dlg.X1 || sx2 != dlg.X2 || by != sy+1 || by != dlg.Y2-1 {
					t.Fatal("button separator must join the frame immediately above the buttons")
				}
				for _, resized := range [][2]int{size, {55, 25}, {54, 25}, {120, 40}, {60, 12}, size} {
					screen.AllocBuf(resized[0], resized[1])
					dlg.ResizeConsole(resized[0], resized[1])
					dlg.Show(screen)
					left, _, _, _ := buttons[0].GetPosition()
					_, _, right, _ := buttons[1].GetPosition()
					leftPad, rightPad := left-dlg.X1, dlg.X2-right
					if leftPad != rightPad && leftPad != rightPad-1 {
						t.Fatalf("buttons lost centering at %dx%d: left=%d right=%d", resized[0], resized[1], leftPad, rightPad)
					}
				}
				screen.AllocBuf(160, 60)
				rules.MaxWidth = 158
				dlg.ResizeConsole(160, 60)
				for _, bounds := range [][2]int{{90, 24}, {100, 28}, {40, 10}, {30, 6}, {70, 16}} {
					dlg.ChangeSize(bounds[0], bounds[1])
					dlg.Show(screen)
					lx, ly, lx2, _ := edits[1].GetPosition()
					if lx != dlg.X1+2 || lx2 != dlg.X2-2 || ly != dlg.Y1+4 {
						t.Fatal("label must follow the resized dialog width")
					}
					for i, sep := range separators {
						sx, sy, sx2, _ := sep.GetPosition()
						if sx != dlg.X1 || sx2 != dlg.X2 || (i == len(separators)-1 && sy != dlg.Y2-2) {
							t.Fatal("separators must span the resized frame and follow the button row")
						}
					}
					left, by, _, _ := buttons[0].GetPosition()
					_, cy, right, _ := buttons[1].GetPosition()
					if by != dlg.Y2-1 || cy != by || (left-dlg.X1 != dlg.X2-right && left-dlg.X1 != dlg.X2-right-1) {
						t.Fatal("buttons must stay centered at the bottom after dialog resizing")
					}
					if commands != nil {
						cx, cy, cx2, cy2 := commands.GetPosition()
						wantTop := dlg.Y1 + 7
						if dlg.Y2-dlg.Y1+1 == 11 {
							if cy2-cy+1 != 1 {
								t.Fatal("minimum dialog height must leave one command row")
							}
						}
						if cx != dlg.X1+2 || cx2 != dlg.X2-2 || cy != wantTop || cy2 != dlg.Y2-3 {
							t.Fatal("commands must fill the resized width and height above the buttons")
						}
						if commands.GetLines()[0] != item.Commands[0] {
							t.Fatal("resizing lost command text")
						}
					}
					if dlg.X2-dlg.X1+1 < 40 || dlg.Y2-dlg.Y1+1 < dlg.MinH {
						t.Fatal("dialog shrank below its minimum size")
					}
					vtui.AssertLayoutWithRules(t, dlg, rules)
				}
			})
		}
	}
}
